package abe

import (
	"github.com/gin-gonic/gin"
)

// errorHandlerMiddleware 统一错误处理中间件
// 处理 4xx 客户端错误，5xx 错误由 ginRecovery 处理
func errorHandlerMiddleware(e *Engine) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		ctx.Next()

		if len(ctx.Errors) == 0 {
			return
		}

		err := ctx.Errors.Last().Err

		if len(e.errorHandlers) == 0 {
			return
		}

		for _, handler := range e.errorHandlers {
			resp, status := handler(err)
			if resp != nil {
				ctx.AbortWithStatusJSON(status, *resp)
				return
			}
		}

		panic(err)
	}
}
