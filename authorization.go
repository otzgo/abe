package abe

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// AuthorizationOption 权限中间件选项
type AuthorizationOption func(*authorizationConfig)

// authorizationConfig 权限中间件配置
type authorizationConfig struct {
	objectPrefix string
}

// WithObjectPrefix 设置对象前缀
// obj 允许使用前缀模式："prefix" 或 "*"，当非 * 时将拼接请求路径
func WithObjectPrefix(prefix string) AuthorizationOption {
	return func(cfg *authorizationConfig) {
		cfg.objectPrefix = prefix
	}
}

// UserAuthorizationMiddleware 基于用户主体的权限控制
// obj 允许使用前缀模式："prefix" 或 "*"，当非 * 时将拼接请求路径
func UserAuthorizationMiddleware(e *Engine, opts ...AuthorizationOption) gin.HandlerFunc {
	cfg := &authorizationConfig{
		objectPrefix: "",
	}
	for _, opt := range opts {
		opt(cfg)
	}

	return func(ctx *gin.Context) {
		claims, ok := getUserClaimsOrAbort(ctx)
		if !ok {
			return
		}

		sub := EncodeUserSub(claims.UserID)
		obj := buildObject(cfg.objectPrefix, ctx)
		act := strings.ToLower(ctx.Request.Method)

		if !e.checkAuthorization(ctx, sub, obj, act) {
			ctx.Abort()
			return
		}
		ctx.Next()
	}
}

// RoleAuthorizationMiddleware 基于主角色的权限控制
func RoleAuthorizationMiddleware(e *Engine, opts ...AuthorizationOption) gin.HandlerFunc {
	cfg := &authorizationConfig{
		objectPrefix: "",
	}
	for _, opt := range opts {
		opt(cfg)
	}

	return func(ctx *gin.Context) {
		claims, ok := getUserClaimsOrAbort(ctx)
		if !ok {
			return
		}
		if claims.PrimaryRole == "" {
			ctx.Error(&HTTPError{Status: http.StatusForbidden, Code: CodeForbidden, Message: "缺少主角色"})
			ctx.Abort()
			return
		}

		sub := EncodeRoleSub(claims.PrimaryRole)
		obj := buildObject(cfg.objectPrefix, ctx)
		act := strings.ToLower(ctx.Request.Method)

		if !e.checkAuthorization(ctx, sub, obj, act) {
			ctx.Abort()
			return
		}
		ctx.Next()
	}
}

// MultiRoleAuthorizationMiddleware 遍历用户所有角色，任一角色通过则放行
func MultiRoleAuthorizationMiddleware(e *Engine, opts ...AuthorizationOption) gin.HandlerFunc {
	cfg := &authorizationConfig{
		objectPrefix: "",
	}
	for _, opt := range opts {
		opt(cfg)
	}

	return func(ctx *gin.Context) {
		claims, ok := getUserClaimsOrAbort(ctx)
		if !ok {
			return
		}

		roles := append([]string(nil), claims.Roles...)
		if len(roles) == 0 && claims.PrimaryRole != "" {
			roles = append(roles, claims.PrimaryRole)
		}
		if len(roles) == 0 {
			ctx.Error(&HTTPError{Status: http.StatusForbidden, Code: CodeForbidden, Message: "缺少角色信息"})
			ctx.Abort()
			return
		}

		obj := buildObject(cfg.objectPrefix, ctx)
		act := strings.ToLower(ctx.Request.Method)

		for _, role := range roles {
			sub := EncodeRoleSub(role)
			if e.checkAuthorization(ctx, sub, obj, act) {
				ctx.Next()
				return
			}
			// 清除上一次的错误，继续尝试下一个角色
			ctx.Errors = ctx.Errors[:0]
		}

		ctx.Error(&HTTPError{Status: http.StatusForbidden, Code: CodeForbidden, Message: "权限不足，无法访问此资源"})
		ctx.Abort()
	}
}

// getUserClaimsOrAbort 获取用户声明，失败时中止请求
func getUserClaimsOrAbort(ctx *gin.Context) (*UserClaims, bool) {
	claims, ok := GetUserClaims(ctx)
	if !ok {
		ctx.Error(&HTTPError{Status: http.StatusUnauthorized, Code: CodeUnauthorized, Message: "未认证的用户", Details: []ErrorDetail{AuthDetail("no user claims")}})
		ctx.Abort()
		return nil, false
	}
	return claims, true
}

// buildObject 根据前缀和请求构建对象路径
func buildObject(objectPrefix string, ctx *gin.Context) string {
	if objectPrefix == "*" {
		return objectPrefix
	}
	return objectPrefix + ctx.Request.URL.Path
}
