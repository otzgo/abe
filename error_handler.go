package abe

import (
	"errors"
	"log/slog"
	"maps"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	ut "github.com/go-playground/universal-translator"
	"github.com/go-playground/validator/v10"
)

// ErrorCode 业务错误码类型
// 建议业务侧使用自定义区间，并与 HTTP 状态语义配合
// 例如：
//   - 400xx 参数/校验错误
//   - 401xx 认证错误
//   - 403xx 授权错误
//   - 429xx 限流错误
//   - 500xx 服务端错误
type ErrorCode int

const (
	CodeBadRequest          ErrorCode = 40001
	CodeUnauthorized        ErrorCode = 40101
	CodeForbidden           ErrorCode = 40301
	CodeTooManyRequests     ErrorCode = 42901
	CodeInternalServerError ErrorCode = 50001
)

// HTTPError 统一错误模型，承载 HTTP 状态与业务码
// 其它中间件或业务代码应通过 ctx.Error(NewHTTPError(...)) 上报
// 由 ErrorHandlerMiddleware 统一输出响应
type HTTPError struct {
	Status  int            `json:"-"`                 // HTTP 状态码（语义正确：401/403/429/500 等）
	Code    ErrorCode      `json:"code"`              // 业务错误码
	Message string         `json:"message"`           // 错误信息
	Details []ErrorDetail  `json:"details,omitempty"` // 强类型错误细节
	Meta    map[string]any `json:"meta,omitempty"`    // 扩展信息
}

// ErrorDetailType 表示错误细节类型
// 使用枚举样式字符串，便于客户端/日志分类
type ErrorDetailType string

const (
	DetailValidation ErrorDetailType = "validation"
	DetailRateLimit  ErrorDetailType = "rate_limit"
	DetailAuth       ErrorDetailType = "auth"
	DetailGeneric    ErrorDetailType = "generic"
)

// ErrorDetail 强类型错误细节
// 不同类型场景复用同一结构，通过 Type 区分场景
// 未覆盖的场景可通过 HTTPError.Meta 扩展
type ErrorDetail struct {
	Type       ErrorDetailType `json:"type"`
	Field      string          `json:"field,omitempty"` // validation 场景
	Tag        string          `json:"tag,omitempty"`   // validation 规则标签
	Message    string          `json:"message,omitempty"`
	Scope      string          `json:"scope,omitempty"`       // rate_limit：global/ip/path 等
	Rule       string          `json:"rule,omitempty"`        // rate_limit：规则ID或路径
	Rate       float64         `json:"rate,omitempty"`        // rate_limit：每秒令牌速率
	Burst      int             `json:"burst,omitempty"`       // rate_limit：突发容量
	RetryAfter int64           `json:"retry_after,omitempty"` // rate_limit：建议重试时间（秒）
	Reason     string          `json:"reason,omitempty"`      // auth：失败原因
}

func (e *HTTPError) Error() string { return e.Message }

// NewHTTPError 构造通用 HTTPError
func NewHTTPError(code ErrorCode, status int, message string, details ...ErrorDetail) *HTTPError {
	return &HTTPError{
		Status:  status,
		Code:    code,
		Message: message,
		Details: details,
	}
}

// BadRequest 错误请求
func BadRequest(message string, details ...ErrorDetail) *HTTPError {
	return NewHTTPError(CodeBadRequest, http.StatusBadRequest, message, details...)
}

// Unauthorized 未授权
func Unauthorized(message string, details ...ErrorDetail) *HTTPError {
	return NewHTTPError(CodeUnauthorized, http.StatusUnauthorized, message, details...)
}

// Forbidden 禁止访问
func Forbidden(message string, details ...ErrorDetail) *HTTPError {
	return NewHTTPError(CodeForbidden, http.StatusForbidden, message, details...)
}

// TooManyRequests 太多请求
func TooManyRequests(message string, details ...ErrorDetail) *HTTPError {
	return NewHTTPError(CodeTooManyRequests, http.StatusTooManyRequests, message, details...)
}

// InternalServerError 内部服务器错误
func InternalServerError(message string, details ...ErrorDetail) *HTTPError {
	return NewHTTPError(CodeInternalServerError, http.StatusInternalServerError, message, details...)
}

// ValidationDetail 细节构造器
func ValidationDetail(field, tag, message string) ErrorDetail {
	return ErrorDetail{Type: DetailValidation, Field: field, Tag: tag, Message: message}
}

// RateLimitDetail 限流细节
func RateLimitDetail(scope, rule string, rate float64, burst int, retryAfter int64) ErrorDetail {
	return ErrorDetail{Type: DetailRateLimit, Scope: scope, Rule: rule, Rate: rate, Burst: burst, RetryAfter: retryAfter}
}

func AuthDetail(reason string) ErrorDetail {
	return ErrorDetail{Type: DetailAuth, Reason: reason}
}

func GenericDetail(message string) ErrorDetail {
	return ErrorDetail{Type: DetailGeneric, Message: message}
}

// SetMeta Meta 辅助：设置或增加扩展信息
func (e *HTTPError) SetMeta(meta map[string]any) *HTTPError {
	if e.Meta == nil {
		e.Meta = make(map[string]any)
	}
	maps.Copy(e.Meta, meta)
	return e
}

func (e *HTTPError) WithMeta(key string, value any) *HTTPError {
	if e.Meta == nil {
		e.Meta = make(map[string]any)
	}
	e.Meta[key] = value
	return e
}

// classifyError 归类为 HTTPError；未知错误统一按 500 返回
func classifyError(err error) *HTTPError {
	var he *HTTPError
	if errors.As(err, &he) {
		return he
	}
	return InternalServerError("内部服务器错误")
}

// classifyErrorWithContext 增强错误归类，支持 validator 验证错误
func classifyErrorWithContext(ctx *gin.Context, err error) *HTTPError {
	var verrs validator.ValidationErrors
	if errorsAsValidationErrors(err, &verrs) {
		return newValidationHTTPError(ctx, verrs)
	}
	return classifyError(err)
}

// newValidationHTTPError 将验证错误转换为 HTTPError（400 BadRequest）
func newValidationHTTPError(ctx *gin.Context, validationErrors validator.ValidationErrors) *HTTPError {
	trans := GetTranslator(ctx)
	details := make([]ErrorDetail, 0, len(validationErrors))

	for _, fe := range validationErrors {
		// 翻译消息
		msg := translateFieldError(fe, trans)
		// 字段名统一小写首字母（使用结构体原始字段名）
		field := lowerFirst(fe.StructField())
		// 组装详情
		details = append(details, ValidationDetail(field, fe.Tag(), msg))
	}

	return BadRequest("输入验证失败", details...)
}

// translateFieldError 翻译单个字段错误
func translateFieldError(fe validator.FieldError, trans ut.Translator) string {
	if trans != nil {
		if s := fe.Translate(trans); s != "" {
			return s
		}
	}
	// 回退：简单组合
	return fe.Field() + " 字段验证失败: " + fe.Tag()
}

// lowerFirst 将首字母转为小写
func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}

// errorsAsValidationErrors 封装 errors.As,避免额外导入
func errorsAsValidationErrors(err error, target *validator.ValidationErrors) bool {
	var validatorErr validator.ValidationErrors
	if errors.As(err, &validatorErr) {
		*target = validatorErr
		return true
	}
	return false
}

// ErrorHandlerMiddleware 统一错误处理中间件（A 模式：HTTP 语义正确 + 响应体携带业务码）
// 用法示例：
//
//	engine.Middlewares().RegisterGlobal(ErrorHandlerMiddleware(engine.Logger()))
func ErrorHandlerMiddleware(e *Engine) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		// 继续执行后续中间件/处理器
		ctx.Next()

		if len(ctx.Errors) == 0 {
			return
		}

		// 取最后一个错误作为主错误（更具体）
		err := ctx.Errors.Last().Err
		he := classifyErrorWithContext(ctx, err)

		// 记录错误日志（按状态区分级别）
		attrs := []any{
			slog.String("path", ctx.Request.URL.Path),
			slog.String("method", ctx.Request.Method),
			slog.String("client_ip", ctx.ClientIP()),
			slog.Int("status", he.Status),
			slog.Int("code", int(he.Code)),
			slog.String("message", he.Message),
		}
		if len(he.Details) > 0 {
			attrs = append(attrs, slog.Any("details", he.Details))
		}
		if he.Meta != nil {
			attrs = append(attrs, slog.Any("meta", he.Meta))
		}
		if rid := ctx.GetHeader("X-Request-ID"); rid != "" {
			attrs = append(attrs, slog.String("request_id", rid))
		}

		if he.Status >= 500 {
			e.logger.Error("请求错误", attrs...)
		} else {
			e.logger.Warn("请求错误", attrs...)
		}

		// 统一输出响应
		ctx.AbortWithStatusJSON(he.Status, gin.H{
			"code":    he.Code,
			"message": he.Message,
			"details": he.Details,
			"meta":    he.Meta,
		})
	}
}
