package abe

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
)

func newRouter(cfg *viper.Viper, logger *slog.Logger) *gin.Engine {

	if cfg.GetBool("app.debug") {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}
	router := gin.New()
	router.Use(ginRecovery(logger))
	router.Use(ginLogger(logger))
	return router
}

// ginLogger 是 Gin 框架的日志中间件
// 将 Gin 的日志输出重定向到 core.Logger
// 使用结构化日志记录 HTTP 请求信息
func ginLogger(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 开始时间
		start := time.Now()
		// 请求路径
		path := c.Request.URL.Path
		// 请求方法
		method := c.Request.Method
		// 客户端 IP
		clientIP := c.ClientIP()

		// 处理请求
		c.Next()

		// 结束时间
		end := time.Now()
		// 执行时间
		latency := end.Sub(start)
		// 状态码
		statusCode := c.Writer.Status()
		// 错误信息
		errorMessage := c.Errors.ByType(gin.ErrorTypePrivate).String()

		// 根据状态码确定日志级别
		logLevel := slog.LevelInfo
		if statusCode >= 400 && statusCode < 500 {
			logLevel = slog.LevelWarn
		} else if statusCode >= 500 {
			logLevel = slog.LevelError
		}

		// 使用结构化日志记录请求信息
		logger.LogAttrs(
			c.Request.Context(),
			logLevel,
			"HTTP 请求",
			slog.String("client_ip", clientIP),
			slog.String("method", method),
			slog.String("path", path),
			slog.Int("status_code", statusCode),
			slog.Duration("latency", latency),
			slog.String("error", errorMessage),
			slog.String("user_agent", c.Request.UserAgent()),
		)
	}
}

// ginRecovery 是 Gin 框架的恢复中间件
// 捕获 panic 并使用 core.Logger 记录错误信息
func ginRecovery(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				// 记录 panic 错误
				logger.LogAttrs(
					c.Request.Context(),
					slog.LevelError,
					"Panic 恢复",
					slog.Any("error", err),
					slog.String("method", c.Request.Method),
					slog.String("path", c.Request.URL.Path),
					slog.String("client_ip", c.ClientIP()),
				)

				// 返回 500 错误
				c.AbortWithStatus(500)
			}
		}()
		c.Next()
	}
}
