package abe

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	ut "github.com/go-playground/universal-translator"
	"github.com/go-playground/validator/v10"
	"gorm.io/gorm"
)

// ErrorCode 业务错误码类型（与 HTTP 状态码解耦）
// 编码规则：
//   - 1xxx: 输入验证类错误
//   - 2xxx: 认证授权类错误
//   - 3xxx: 资源操作类错误
//   - 4xxx: 系统限制类错误
//   - 5xxx: 服务端错误
//   - 6xxx: 外部依赖错误
//   - 9xxx: 业务逻辑错误（预留）
type ErrorCode int

const (
	CodeValidationFailed ErrorCode = 1001 // 参数验证失败
	CodeInvalidJSON      ErrorCode = 1002 // JSON 格式错误
	CodeInvalidFormat    ErrorCode = 1003 // 数据格式错误

	CodeUnauthorized     ErrorCode = 2001 // 未认证
	CodeTokenExpired     ErrorCode = 2002 // 令牌过期
	CodeTokenInvalid     ErrorCode = 2003 // 令牌无效
	CodeForbidden        ErrorCode = 2101 // 无权限
	CodeInsufficientPerm ErrorCode = 2102 // 权限不足

	CodeNotFound         ErrorCode = 3001 // 资源不存在
	CodeAlreadyExists    ErrorCode = 3002 // 资源已存在
	CodeConflict         ErrorCode = 3003 // 资源冲突
	CodeResourceGone     ErrorCode = 3004 // 资源已删除
	CodePreconditionFail ErrorCode = 3005 // 前置条件失败

	CodeRateLimited      ErrorCode = 4001 // 请求限流
	CodeQuotaExceeded    ErrorCode = 4002 // 配额超限
	CodeRequestTooLarge  ErrorCode = 4003 // 请求体过大
	CodeMethodNotAllowed ErrorCode = 4004 // 方法不允许

	CodeInternalError ErrorCode = 5001 // 内部错误
	CodeDatabaseError ErrorCode = 5002 // 数据库错误
	CodeCacheError    ErrorCode = 5003 // 缓存错误

	CodeServiceUnavail   ErrorCode = 6001 // 服务不可用
	CodeGatewayTimeout   ErrorCode = 6002 // 网关超时
	CodeExternalAPIError ErrorCode = 6003 // 外部 API 错误
)

// ErrorResponse API 错误响应结构
// 由 ErrorHandlerMiddleware 统一构造并输出
type ErrorResponse struct {
	Code    ErrorCode     `json:"code"`              // 业务错误码
	Message string        `json:"message"`           // 错误信息
	Details []ErrorDetail `json:"details,omitempty"` // 错误详情列表
}

// ErrorDetail 错误详情
// 用于携带字段级别的错误信息（主要用于参数验证）
type ErrorDetail struct {
	Field  string `json:"field,omitempty"` // 字段名（验证错误场景）
	Reason string `json:"reason"`          // 错误原因描述
}

// HTTPStatus 定义可返回 HTTP 状态码的错误接口
type HTTPStatus interface {
	HTTPStatusCode() int
}

// ============ 哨兵错误（简单场景） ============

var (
	ErrUnauthorized        = errors.New("unauthorized")          // 未认证
	ErrForbidden           = errors.New("forbidden")             // 无权限
	ErrInternalServerError = errors.New("internal server error") // 内部错误
)

// ============ 具体错误类型（复杂场景） ============

// ValidationError 参数验证错误
type ValidationError struct {
	Message string
	Details []ErrorDetail
}

func (e *ValidationError) Error() string       { return e.Message }
func (e *ValidationError) HTTPStatusCode() int { return http.StatusBadRequest }

// NotFoundError 资源不存在错误
type NotFoundError struct {
	Resource string // 资源名称，如"管理员"、"订单"
	ID       string // 资源标识（可选）
}

func (e *NotFoundError) Error() string {
	if e.ID != "" {
		return fmt.Sprintf("%s[%s]不存在", e.Resource, e.ID)
	}
	return e.Resource + "不存在"
}
func (e *NotFoundError) HTTPStatusCode() int { return http.StatusNotFound }

// ConflictError 资源冲突错误
type ConflictError struct {
	Message string
	Field   string // 冲突字段
}

func (e *ConflictError) Error() string       { return e.Message }
func (e *ConflictError) HTTPStatusCode() int { return http.StatusConflict }

// RateLimitError 限流错误
type RateLimitError struct {
	Scope      string  // 限流作用域："ip", "global", "path", "user" 等
	Rule       string  // 规则标识（如路径、规则ID）
	Rate       float64 // 每秒令牌速率
	Burst      int     // 突发容量
	RetryAfter int64   // 建议重试时间（秒）
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("请求过于频繁，请 %d 秒后重试", e.RetryAfter)
}
func (e *RateLimitError) HTTPStatusCode() int { return http.StatusTooManyRequests }

// InvalidJSONError JSON 解析错误
type InvalidJSONError struct {
	Reason string
}

func (e *InvalidJSONError) Error() string {
	return "JSON 格式错误: " + e.Reason
}
func (e *InvalidJSONError) HTTPStatusCode() int { return http.StatusBadRequest }

// classifyError 将错误转换为 ErrorResponse
func classifyError(ctx *gin.Context, err error) *ErrorResponse {
	// 1. 处理 validator 验证错误
	var verrs validator.ValidationErrors
	if errors.As(err, &verrs) {
		return buildValidationResponse(ctx, verrs)
	}

	// 2. 处理具体错误类型
	var validErr *ValidationError
	if errors.As(err, &validErr) {
		return &ErrorResponse{
			Code:    CodeValidationFailed,
			Message: validErr.Message,
			Details: validErr.Details,
		}
	}

	var notFoundErr *NotFoundError
	if errors.As(err, &notFoundErr) {
		msg := notFoundErr.Resource + "不存在"
		if notFoundErr.ID != "" {
			msg = fmt.Sprintf("%s[%s]不存在", notFoundErr.Resource, notFoundErr.ID)
		}
		return &ErrorResponse{
			Code:    CodeNotFound,
			Message: msg,
		}
	}

	var conflictErr *ConflictError
	if errors.As(err, &conflictErr) {
		var details []ErrorDetail
		if conflictErr.Field != "" {
			details = append(details, ErrorDetail{
				Field:  conflictErr.Field,
				Reason: "字段值冲突",
			})
		}
		return &ErrorResponse{
			Code:    CodeConflict,
			Message: conflictErr.Message,
			Details: details,
		}
	}

	var rateLimitErr *RateLimitError
	if errors.As(err, &rateLimitErr) {
		reason := fmt.Sprintf(
			"%s级限流触发: 速率%.1freq/s, 容量%d, %d秒后重试",
			rateLimitErr.Scope,
			rateLimitErr.Rate,
			rateLimitErr.Burst,
			rateLimitErr.RetryAfter,
		)
		return &ErrorResponse{
			Code:    CodeRateLimited,
			Message: fmt.Sprintf("请求过于频繁，请 %d 秒后重试", rateLimitErr.RetryAfter),
			Details: []ErrorDetail{{Field: "rate_limit", Reason: reason}},
		}
	}

	var jsonErr *InvalidJSONError
	if errors.As(err, &jsonErr) {
		return &ErrorResponse{
			Code:    CodeInvalidJSON,
			Message: "请求数据格式错误",
			Details: []ErrorDetail{{Reason: jsonErr.Reason}},
		}
	}

	// 3. 处理哨兵错误
	if errors.Is(err, ErrUnauthorized) {
		return &ErrorResponse{
			Code:    CodeUnauthorized,
			Message: "未授权",
		}
	}

	if errors.Is(err, ErrForbidden) {
		return &ErrorResponse{
			Code:    CodeForbidden,
			Message: "无权限访问",
		}
	}

	// 4. 识别 GORM 错误
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &ErrorResponse{
			Code:    CodeNotFound,
			Message: "记录不存在",
		}
	}

	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return &ErrorResponse{
			Code:    CodeAlreadyExists,
			Message: "记录已存在",
		}
	}

	// 5. 识别 Context 超时错误
	if errors.Is(err, context.DeadlineExceeded) {
		return &ErrorResponse{
			Code:    CodeGatewayTimeout,
			Message: "请求处理超时",
		}
	}

	if errors.Is(err, context.Canceled) {
		return &ErrorResponse{
			Code:    CodeInternalError,
			Message: "请求已取消",
		}
	}

	// 6. 默认返回 400
	return &ErrorResponse{
		Code:    CodeValidationFailed,
		Message: err.Error(),
	}
}

// buildValidationResponse 将 validator 验证错误转换为 ErrorResponse
func buildValidationResponse(ctx *gin.Context, validationErrors validator.ValidationErrors) *ErrorResponse {
	trans := GetTranslator(ctx)
	details := make([]ErrorDetail, 0, len(validationErrors))

	for _, fe := range validationErrors {
		// 翻译消息
		reason := translateFieldError(fe, trans)
		// 字段名统一小写首字母（使用结构体原始字段名）
		field := lowerFirst(fe.StructField())
		// 组装详情
		details = append(details, ErrorDetail{
			Field:  field,
			Reason: reason,
		})
	}

	return &ErrorResponse{
		Code:    CodeValidationFailed,
		Message: "输入验证失败",
		Details: details,
	}
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

// determineHTTPStatus 确定 HTTP 状态码
// 优先级：
// 1. 实现 HTTPStatus 接口
// 2. 哨兵错误映射
// 3. 默认 400
func determineHTTPStatus(err error) int {
	// 1. 检查是否实现 HTTPStatus 接口
	if hs, ok := err.(HTTPStatus); ok {
		return hs.HTTPStatusCode()
	}

	// 2. 哨兵错误映射
	switch {
	case errors.Is(err, ErrUnauthorized):
		return http.StatusUnauthorized
	case errors.Is(err, ErrForbidden):
		return http.StatusForbidden
	case errors.Is(err, gorm.ErrRecordNotFound):
		return http.StatusNotFound
	case errors.Is(err, gorm.ErrDuplicatedKey):
		return http.StatusConflict
	case errors.Is(err, context.DeadlineExceeded):
		return http.StatusGatewayTimeout
	case errors.Is(err, ErrInternalServerError):
		return http.StatusInternalServerError
	}

	// 3. 默认 400
	return http.StatusBadRequest
}

// ErrorHandlerMiddleware 统一错误处理中间件（A 模式：HTTP 语义正确 + 响应体携带业务码）
// 处理 4xx 客户端错误，5xx 错误由 ginRecovery 处理
// 用法示例：
//
//	engine.MiddlewareManager().RegisterGlobal(ErrorHandlerMiddleware(engine))
func ErrorHandlerMiddleware(e *Engine) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		// 继续执行后续中间件/处理器
		ctx.Next()

		if len(ctx.Errors) == 0 {
			return
		}

		// 取最后一个错误作为主错误（更具体）
		err := ctx.Errors.Last().Err

		// 转换为 ErrorResponse
		resp := classifyError(ctx, err)

		// 确定 HTTP 状态码
		status := determineHTTPStatus(err)

		// 记录错误日志（按状态区分级别）
		attrs := []any{
			slog.String("path", ctx.Request.URL.Path),
			slog.String("method", ctx.Request.Method),
			slog.String("client_ip", ctx.ClientIP()),
			slog.Int("status", status),
			slog.Int("code", int(resp.Code)),
			slog.String("message", resp.Message),
		}
		if len(resp.Details) > 0 {
			attrs = append(attrs, slog.Any("details", resp.Details))
		}
		if rid := ctx.GetHeader("X-Request-ID"); rid != "" {
			attrs = append(attrs, slog.String("request_id", rid))
		}

		// 4xx 使用 Warn 级别，便于区分客户端错误与服务端错误
		e.logger.Warn("请求错误", attrs...)

		// 统一输出响应
		ctx.AbortWithStatusJSON(status, resp)
	}
}
