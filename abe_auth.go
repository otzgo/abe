package abe

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/casbin/casbin/v2"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/spf13/viper"
)

// AuthManager 认证授权管理器
// 封装 JWT 令牌生成/解析和 Casbin 权限检查功能
type AuthManager struct {
	config   *viper.Viper
	enforcer *casbin.Enforcer
}

// newAuthManager 创建认证授权管理器
func newAuthManager(config *viper.Viper, enforcer *casbin.Enforcer) *AuthManager {
	return &AuthManager{
		config:   config,
		enforcer: enforcer,
	}
}

// GetAuthConfig 从配置中解析认证配置
// 若未设置 TokenExpiry，则返回默认值 24 小时
func (am *AuthManager) GetAuthConfig() (AuthConfig, error) {
	var cfg AuthConfig
	if err := am.config.UnmarshalKey("auth", &cfg); err != nil {
		return cfg, err
	}
	if cfg.TokenExpiry == 0 {
		cfg.TokenExpiry = 24
	}
	return cfg, nil
}

// GenerateToken 根据用户声明生成 JWT 字符串
// 配置项：
// - auth.jwt_secret: HMAC 密钥（必须）
// - auth.token_expiry: 过期时间（小时，默认 24）
func (am *AuthManager) GenerateToken(claims *UserClaims) (string, error) {
	cfg, err := am.GetAuthConfig()
	if err != nil {
		return "", fmt.Errorf("解析认证配置失败: %w", err)
	}
	secret := cfg.JWTSecret
	if secret == "" {
		return "", fmt.Errorf("JWT 密钥未配置: %w", ErrInvalidSigningKey)
	}

	expHours := cfg.TokenExpiry
	if expHours == 0 {
		expHours = 24
	}

	now := time.Now()
	if claims.IssuedAt == nil {
		claims.IssuedAt = jwt.NewNumericDate(now)
	}
	if claims.ExpiresAt == nil {
		claims.ExpiresAt = jwt.NewNumericDate(now.Add(time.Duration(expHours) * time.Hour))
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// ParseToken 解析 JWT 字符串并返回用户声明
func (am *AuthManager) ParseToken(tokenString string) (*UserClaims, error) {
	cfg, cfgErr := am.GetAuthConfig()
	if cfgErr != nil {
		return nil, fmt.Errorf("解析认证配置失败: %w", cfgErr)
	}
	secret := cfg.JWTSecret
	if secret == "" {
		return nil, fmt.Errorf("JWT 密钥未配置: %w", ErrInvalidSigningKey)
	}

	token, err := jwt.ParseWithClaims(tokenString, &UserClaims{}, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidSigningKey
		}
		return []byte(secret), nil
	})
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		return nil, err
	}

	claims, ok := token.Claims.(*UserClaims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}
	return claims, nil
}

// AuthenticationMiddleware 身份认证中间件：
// - 提取 Authorization: Bearer {token}
// - 校验 JWT
// - 将 UserClaims 写入上下文
// - 出错时通过 ErrorHandlerMiddleware 统一响应
func (am *AuthManager) AuthenticationMiddleware() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		authHeader := ctx.GetHeader("Authorization")
		if authHeader == "" {
			ctx.Error(fmt.Errorf("未提供认证信息: %w", ErrUnauthorized))
			ctx.Abort()
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			ctx.Error(fmt.Errorf("认证头格式错误，应为 'Bearer {token}': %w", ErrUnauthorized))
			ctx.Abort()
			return
		}

		claims, err := am.ParseToken(parts[1])
		if err != nil {
			switch {
			case errors.Is(err, ErrTokenExpired):
				ctx.Error(fmt.Errorf("令牌已过期: %w", ErrTokenExpired))
			case errors.Is(err, ErrInvalidToken), errors.Is(err, ErrInvalidSigningKey):
				ctx.Error(fmt.Errorf("无效令牌: %w", ErrUnauthorized))
			default:
				ctx.Error(fmt.Errorf("认证处理失败: %w", ErrInternalServerError))
			}
			ctx.Abort()
			return
		}

		ctx.Set(contextKeyUserClaims, claims)
		ctx.Next()
	}
}

// UserAuthorizationMiddleware 基于用户主体的权限控制
func (am *AuthManager) UserAuthorizationMiddleware(opts ...AuthorizationOption) gin.HandlerFunc {
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

		allowed, err := am.enforcer.Enforce(sub, obj, act)
		if err != nil {
			ctx.Error(fmt.Errorf("权限检查失败: %w", ErrForbidden))
			ctx.Abort()
			return
		}
		if !allowed {
			ctx.Error(fmt.Errorf("权限不足，无法访问此资源: %w", ErrForbidden))
			ctx.Abort()
			return
		}
		ctx.Next()
	}
}

// RoleAuthorizationMiddleware 基于主角色的权限控制
func (am *AuthManager) RoleAuthorizationMiddleware(opts ...AuthorizationOption) gin.HandlerFunc {
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
			ctx.Error(fmt.Errorf("缺少主角色: %w", ErrForbidden))
			ctx.Abort()
			return
		}

		sub := EncodeRoleSub(claims.PrimaryRole)
		obj := buildObject(cfg.objectPrefix, ctx)
		act := strings.ToLower(ctx.Request.Method)

		allowed, err := am.enforcer.Enforce(sub, obj, act)
		if err != nil {
			ctx.Error(fmt.Errorf("权限检查失败: %w", ErrForbidden))
			ctx.Abort()
			return
		}
		if !allowed {
			ctx.Error(fmt.Errorf("权限不足，无法访问此资源: %w", ErrForbidden))
			ctx.Abort()
			return
		}
		ctx.Next()
	}
}

// MultiRoleAuthorizationMiddleware 遍历用户所有角色，任一角色通过则放行
func (am *AuthManager) MultiRoleAuthorizationMiddleware(opts ...AuthorizationOption) gin.HandlerFunc {
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
			ctx.Error(fmt.Errorf("缺少角色信息: %w", ErrForbidden))
			ctx.Abort()
			return
		}

		obj := buildObject(cfg.objectPrefix, ctx)
		act := strings.ToLower(ctx.Request.Method)

		for _, role := range roles {
			sub := EncodeRoleSub(role)
			allowed, err := am.enforcer.Enforce(sub, obj, act)
			if err == nil && allowed {
				ctx.Next()
				return
			}
			// 清除上一次的错误，继续尝试下一个角色
			ctx.Errors = ctx.Errors[:0]
		}

		ctx.Error(fmt.Errorf("权限不足，无法访问此资源: %w", ErrForbidden))
		ctx.Abort()
	}
}

// EndpointAuthorization 单端点的用户主体权限检查
func (am *AuthManager) EndpointAuthorization(obj string, act string) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		claims, ok := getUserClaimsOrAbort(ctx)
		if !ok {
			return
		}

		sub := EncodeUserSub(claims.UserID)
		allowed, err := am.enforcer.Enforce(sub, obj, act)
		if err != nil {
			ctx.Error(fmt.Errorf("权限检查失败: %w", ErrForbidden))
			ctx.Abort()
			return
		}
		if !allowed {
			ctx.Error(fmt.Errorf("权限不足，无法访问此资源: %w", ErrForbidden))
			ctx.Abort()
			return
		}
		ctx.Next()
	}
}

// RoleEndpointAuthorization 单端点的角色主体权限检查
func (am *AuthManager) RoleEndpointAuthorization(obj string, act string) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		claims, ok := getUserClaimsOrAbort(ctx)
		if !ok {
			return
		}

		role := claims.PrimaryRole
		if role == "" {
			ctx.Error(fmt.Errorf("缺少主角色: %w", ErrForbidden))
			ctx.Abort()
			return
		}

		allowed, err := am.enforcer.Enforce(EncodeRoleSub(role), obj, act)
		if err != nil {
			ctx.Error(fmt.Errorf("权限检查失败: %w", ErrForbidden))
			ctx.Abort()
			return
		}
		if !allowed {
			ctx.Error(fmt.Errorf("权限不足，无法访问此资源: %w", ErrForbidden))
			ctx.Abort()
			return
		}
		ctx.Next()
	}
}
