package abe

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
)

// CORSMiddleware 基于配置的跨域中间件
// 设计要点：
// - 支持域名白名单（含通配 *.example.com）与 "*"；当允许凭证时，自动避免 "*"，改为回显匹配的 Origin
// - 预检请求（OPTIONS）直接 204 返回并携带 CORS 头，避免触达业务处理器
// - 方法/头/暴露头/凭证/缓存时间均可配置；未配置时使用合理默认值
// - 与 abe 的中间件管理配合：通过 Engine.MiddlewareManager().RegisterGlobal(CORSMiddleware(engine.Config())) 注册到 "/api" 分组
func CORSMiddleware(cfg *viper.Viper) gin.HandlerFunc {
	allowedOrigins := getStringSlice(cfg, "server.cors.allow_origins", []string{"*"})
	allowedMethods := getStringSlice(cfg, "server.cors.allow_methods", []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"})
	allowedHeaders := getStringSlice(cfg, "server.cors.allow_headers", []string{"Content-Type", "Content-Length", "Accept", "Accept-Encoding", "Authorization", "Origin", "Cache-Control", "X-Requested-With"})
	exposeHeaders := getStringSlice(cfg, "server.cors.expose_headers", nil)
	allowCredentials := cfg.GetBool("server.cors.allow_credentials")
	maxAgeSeconds := cfg.GetInt("server.cors.max_age_seconds")
	if maxAgeSeconds <= 0 {
		maxAgeSeconds = int((24 * time.Hour).Seconds())
	}

	methods := strings.Join(allowedMethods, ", ")
	headers := strings.Join(allowedHeaders, ", ")
	expose := strings.Join(exposeHeaders, ", ")

	return func(ctx *gin.Context) {
		origin := ctx.GetHeader("Origin")

		// 非 CORS 请求直接透传
		if origin == "" {
			ctx.Next()
			return
		}

		// 计算允许的 Origin 值
		var allowOrigin string
		if contains(allowedOrigins, "*") && !allowCredentials {
			allowOrigin = "*"
		} else if originAllowed(origin, allowedOrigins) {
			// 当允许凭证或未使用 "*"，严格回显匹配到的 origin
			allowOrigin = origin
		}

		// 设置通用 CORS 响应头（仅在命中策略时）
		if allowOrigin != "" {
			ctx.Header("Access-Control-Allow-Origin", allowOrigin)
			if allowCredentials {
				ctx.Header("Access-Control-Allow-Credentials", "true")
			}
			ctx.Header("Access-Control-Allow-Methods", methods)

			// 允许头：优先使用配置；若未显式配置且客户端声明了请求头，则按需回显
			reqHeaders := ctx.GetHeader("Access-Control-Request-Headers")
			if reqHeaders != "" && len(allowedHeaders) == 0 {
				ctx.Header("Access-Control-Allow-Headers", reqHeaders)
			} else {
				ctx.Header("Access-Control-Allow-Headers", headers)
			}

			if expose != "" {
				ctx.Header("Access-Control-Expose-Headers", expose)
			}
			if maxAgeSeconds > 0 {
				ctx.Header("Access-Control-Max-Age", fmt.Sprintf("%d", maxAgeSeconds))
			}
		}

		// 预检请求直接返回 204，避免进入后续链条
		if ctx.Request.Method == http.MethodOptions {
			ctx.AbortWithStatus(http.StatusNoContent)
			return
		}

		// 继续执行后续中间件/处理器
		ctx.Next()
	}
}

// getStringSlice 读取字符串切片配置，支持逗号分隔的字符串与原生切片
func getStringSlice(cfg *viper.Viper, key string, defaults []string) []string {
	v := cfg.Get(key)
	switch vv := v.(type) {
	case []string:
		if len(vv) == 0 && defaults != nil {
			return defaults
		}
		return vv
	case []any:
		out := make([]string, 0, len(vv))
		for _, x := range vv {
			out = append(out, strings.TrimSpace(fmt.Sprint(x)))
		}
		if len(out) == 0 && defaults != nil {
			return defaults
		}
		return out
	case string:
		s := strings.TrimSpace(vv)
		if s == "" {
			if defaults != nil {
				return defaults
			}
			return nil
		}
		parts := strings.Split(s, ",")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				out = append(out, p)
			}
		}
		return out
	default:
		if defaults != nil {
			return defaults
		}
		return nil
	}
}

// contains 判断切片是否包含指定字符串（大小写敏感）
func contains(xs []string, s string) bool {
	return slices.Contains(xs, s)
}

// originAllowed 判断请求 Origin 是否被允许
// 支持：
// - 精确匹配（大小写不敏感）
// - 通配符前缀 "*.example.com"（大小写不敏感，按后缀匹配）
// - 全匹配 "*"（已在上游处理）
func originAllowed(origin string, allowed []string) bool {
	lo := strings.ToLower(origin)
	for _, a := range allowed {
		la := strings.ToLower(strings.TrimSpace(a))
		if la == "*" {
			return true
		}
		if lo == la {
			return true
		}
		if after, ok := strings.CutPrefix(la, "*."); ok {
			suf := after
			if strings.HasSuffix(lo, suf) {
				return true
			}
		}
	}
	return false
}
