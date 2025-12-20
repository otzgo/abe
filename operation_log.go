package abe

import (
	"bytes"
	"io"
	"time"

	"github.com/gin-gonic/gin"
)

// OperationLogEntry 操作日志条目（框架层定义，业务无关）
type OperationLogEntry struct {
	// 操作主体
	AdminID     uint
	Username    string
	RealName    string
	DisplayName string // 别名：与 RealName 相同

	// 操作行为
	Module      string
	Action      string
	Resource    *string
	Description string

	// 请求信息
	RequestID string
	Method    string
	Path      string
	IPAddress string
	UserAgent *string

	// 数据信息
	RequestBody     *string
	ResponseStatus  int
	ResponseMessage *string
	IsSuccess       bool

	// 元信息
	DurationMs int64
	RiskLevel  string

	// 原始上下文（供业务扩展使用）
	Context *gin.Context
}

// OperationInfoParser 操作信息解析器接口
// 从请求上下文中提取操作相关信息
type OperationInfoParser interface {
	Parse(ctx *gin.Context) (module, action string, resource *string, description string)
}

// RiskLevelDeterminer 风险等级判定器接口
// 根据模块和操作类型判定风险等级
type RiskLevelDeterminer interface {
	Determine(module, action string) string
}

// OperationLogWriter 操作日志写入器接口
// 负责将日志条目持久化存储
type OperationLogWriter interface {
	Write(entry *OperationLogEntry) error
}

// OperationLogBehavior 操作日志行为接口（组合接口）
type OperationLogBehavior interface {
	OperationInfoParser
	RiskLevelDeterminer
	OperationLogWriter
}

// OperationLogConfig 操作日志中间件配置
type OperationLogConfig struct {
	// Behavior 操作日志行为实现（必填）
	Behavior OperationLogBehavior

	// MaxBodySize 请求体最大字节数（可选，默认 10KB）
	MaxBodySize int64

	// SensitiveFields 需要脱敏的字段列表（可选，默认包含常见敏感字段）
	SensitiveFields []string

	// LogHighRisk 是否对高风险操作额外记录警告日志（可选，默认 true）
	LogHighRisk bool
}

// OperationLogger 操作日志记录器（面向对象封装）
type OperationLogger struct {
	engine *Engine
	config *OperationLogConfig
}

// NewOperationLogger 创建操作日志记录器
func NewOperationLogger(engine *Engine, config *OperationLogConfig) *OperationLogger {
	if config.Behavior == nil {
		panic("OperationLogConfig.Behavior cannot be nil")
	}

	// 设置默认值
	if config.MaxBodySize <= 0 {
		config.MaxBodySize = 10 * 1024 // 10KB
	}

	if config.SensitiveFields == nil {
		config.SensitiveFields = []string{
			"password", "oldPassword", "newPassword", "confirmPassword",
			"passwordHash", "token", "accessToken", "refreshToken",
		}
	}

	return &OperationLogger{
		engine: engine,
		config: config,
	}
}

// Middleware 返回 Gin 中间件函数
//
// 【重要】中间件执行时机说明:
// - 本中间件必须注册为全局中间件
// - 认证中间件(如JWT)应注册为共享中间件或路由组中间件
// - 本中间件通过 ctx.Next() 等待请求完成后再记录日志
// - 当 ctx.Next() 返回时,路由组的认证中间件已经将用户信息注入上下文
// - 因此可以正确获取 UserClaims 进行日志记录
//
// 特性:
//   - 仅记录已认证用户的操作
//   - 仅记录写操作(POST/PUT/DELETE/PATCH)
//   - 异步写入,不阻塞主请求
//   - 自动脱敏敏感字段
//   - 可配置的信息解析、风险判定和存储逻辑
func (l *OperationLogger) Middleware() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		// 1. 仅记录写操作(POST/PUT/DELETE/PATCH)
		if !l.shouldLogOperation(ctx) {
			ctx.Next()
			return
		}

		// 2. 记录请求开始时间
		startTime := GetRequestTime(ctx)
		if startTime.IsZero() {
			startTime = time.Now()
		}

		// 3. 捕获请求体(带缓冲,允许后续读取)
		var requestBodyBytes []byte
		if l.shouldCaptureRequestBody(ctx) {
			requestBodyBytes = l.captureAndBufferRequestBody(ctx)
		}

		// 4. 继续处理请求(等待路由组中间件执行,包括认证中间件)
		ctx.Next()

		// 5. 请求处理完成,此时认证中间件已注入用户信息
		// 检查是否为已认证用户,如果不是则不记录日志
		claims, hasClaims := GetUserClaims(ctx)
		if !hasClaims {
			// 未认证用户(如登录接口)不记录操作日志
			return
		}

		// 6. 异步记录操作日志
		go l.recordOperationLog(ctx, claims, requestBodyBytes, startTime)
	}
}

// shouldLogOperation 判断是否需要记录操作
func (l *OperationLogger) shouldLogOperation(ctx *gin.Context) bool {
	method := ctx.Request.Method
	return method == "POST" || method == "PUT" || method == "DELETE" || method == "PATCH"
}

// shouldCaptureRequestBody 判断是否需要捕获请求体
func (l *OperationLogger) shouldCaptureRequestBody(ctx *gin.Context) bool {
	method := ctx.Request.Method
	return method == "POST" || method == "PUT" || method == "PATCH"
}

// captureAndBufferRequestBody 捕获请求体并恢复
func (l *OperationLogger) captureAndBufferRequestBody(ctx *gin.Context) []byte {
	bodyBytes, err := io.ReadAll(io.LimitReader(ctx.Request.Body, l.config.MaxBodySize+1))
	if err != nil {
		return nil
	}

	// 恢复请求体供后续使用
	ctx.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	// 检查是否超过大小限制
	if int64(len(bodyBytes)) > l.config.MaxBodySize {
		return []byte("[请求体过大，已截断]")
	}

	return bodyBytes
}

// recordOperationLog 记录操作日志（异步执行）
func (l *OperationLogger) recordOperationLog(ctx *gin.Context, claims *UserClaims, requestBodyBytes []byte, startTime time.Time) {
	// 防止 panic 影响主流程
	defer func() {
		if r := recover(); r != nil {
			l.engine.Logger().Error("记录操作日志时发生 panic", "error", r)
		}
	}()

	// 使用行为接口解析操作信息
	module, action, resource, description := l.config.Behavior.Parse(ctx)
	// 如果 module 为空，则不处理
	if module == "" {
		return
	}

	// 使用行为接口获取风险等级
	riskLevel := l.config.Behavior.Determine(module, action)

	// 获取客户端信息
	ipAddress := ctx.ClientIP()
	userAgent := ctx.Request.UserAgent()
	var userAgentPtr *string
	if userAgent != "" {
		userAgentPtr = &userAgent
	}

	// 处理请求体
	var requestBodyPtr *string
	if len(requestBodyBytes) > 0 {
		// 脱敏处理
		sanitizedBody := l.sanitizeRequestBody(requestBodyBytes)
		requestBodyPtr = &sanitizedBody
	}

	// 获取响应状态
	responseStatus := ctx.Writer.Status()
	isSuccess := responseStatus >= 200 && responseStatus < 300

	// 获取响应消息（从错误中提取）
	var responseMessage *string
	if len(ctx.Errors) > 0 {
		if le := ctx.Errors.Last(); le != nil {
			msg := le.Error()
			responseMessage = &msg
		}
	}

	// 计算耗时
	durationMs := time.Since(startTime).Milliseconds()

	// 构建日志条目
	entry := &OperationLogEntry{
		AdminID:         parseUint(claims.UserID),
		Username:        claims.Username,
		RealName:        claims.DisplayName,
		DisplayName:     claims.DisplayName,
		Module:          module,
		Action:          action,
		Resource:        resource,
		Description:     description,
		RequestID:       GetRequestID(ctx),
		Method:          ctx.Request.Method,
		Path:            ctx.FullPath(),
		IPAddress:       ipAddress,
		UserAgent:       userAgentPtr,
		RequestBody:     requestBodyPtr,
		ResponseStatus:  responseStatus,
		ResponseMessage: responseMessage,
		IsSuccess:       isSuccess,
		DurationMs:      durationMs,
		RiskLevel:       riskLevel,
		Context:         ctx,
	}

	// 使用行为接口写入日志
	if err := l.config.Behavior.Write(entry); err != nil {
		l.engine.Logger().Error("写入操作日志失败",
			"error", err,
			"username", claims.Username,
			"module", module,
			"action", action)
	}

	// 对于高风险操作，记录警告日志（如果启用）
	if l.config.LogHighRisk && (riskLevel == "critical" || riskLevel == "high") {
		l.engine.Logger().Warn("高风险操作",
			"username", claims.Username,
			"module", module,
			"action", action,
			"description", description,
			"risk_level", riskLevel,
			"ip", ipAddress)
	}
}

// sanitizeRequestBody 脱敏请求体
func (l *OperationLogger) sanitizeRequestBody(bodyBytes []byte) string {
	bodyStr := string(bodyBytes)

	// 检查是否包含敏感字段
	hasSensitive := false
	for _, field := range l.config.SensitiveFields {
		if containsSubstring(bodyStr, field) {
			hasSensitive = true
			break
		}
	}

	if hasSensitive {
		bodyStr = bodyStr + " [sensitive fields sanitized]"
	}

	return bodyStr
}

// parseUint 解析字符串为 uint
func parseUint(s string) uint {
	var result uint64
	for i := 0; i < len(s); i++ {
		if s[i] >= '0' && s[i] <= '9' {
			result = result*10 + uint64(s[i]-'0')
		}
	}
	return uint(result)
}

// containsSubstring 检查字符串是否包含子串
func containsSubstring(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
