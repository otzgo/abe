package abe

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// requestIDKey 与 requestStartKey 为上下文键名约定
const (
	requestIDKey    = "abe.request_id"
	requestStartKey = "abe.request_start"
)

type RequestMeta struct {
	RequestID   string
	RequestTime time.Time
}

// RequestIDMiddleware 生成/透传请求 ID，并写入上下文与响应头
// 约定：
// - 优先使用客户端请求头 X-Request-ID；若缺失则生成一个 UUIDv4
// - 将请求 ID 写入 gin.Context（键：abe.request_id）与响应头 X-Request-ID
// - 不阻断链条，调用 ctx.Next()
func RequestIDMiddleware() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		requestID := ctx.GetHeader("X-Request-ID")
		if requestID == "" {
			requestID = uuid.New().String()
		}
		ctx.Set(requestIDKey, requestID)
		ctx.Writer.Header().Set("X-Request-ID", requestID)
		ctx.Next()
	}
}

// RequestTimeMiddleware 记录请求开始时间到上下文
// 约定：
// - 在进入后续中间件/处理器前写入 time.Now()
// - 键名为 abe.request_start，供日志/耗时统计等使用
// - 不阻断链条，调用 ctx.Next()
func RequestTimeMiddleware() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		start := time.Now()
		ctx.Set(requestStartKey, start)
		ctx.Next()
	}
}

// GetRequestID 从上下文中获取请求 ID；若不存在则返回空字符串
func GetRequestID(ctx *gin.Context) string {
	if v, ok := ctx.Get(requestIDKey); ok {
		if s, ok2 := v.(string); ok2 {
			return s
		}
	}
	return ""
}

// GetRequestTime 从上下文中获取请求开始时间；若不存在则返回零值 time.Time{}
func GetRequestTime(ctx *gin.Context) time.Time {
	if v, ok := ctx.Get(requestStartKey); ok {
		if t, ok2 := v.(time.Time); ok2 {
			return t
		}
	}
	return time.Time{}
}

func GetRequestMeta(ctx *gin.Context) RequestMeta {
	return RequestMeta{
		RequestID:   GetRequestID(ctx),
		RequestTime: GetRequestTime(ctx),
	}
}
