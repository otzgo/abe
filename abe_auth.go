package abe

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/casbin/casbin/v3"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/spf13/viper"
	"gorm.io/gorm"
)

// AuthManager 认证授权管理器
// 封装 JWT 令牌生成/解析和 Casbin 权限检查功能
//
// 权限中间件使用指南：
// AuthorizationMiddleware(resource, action)
//   - 在代码中直接指定权限
//   - 适用于：需要明确权限控制的场景
//   - 示例：engine.Auth().AuthorizationMiddleware("member", "read")
//
// 所有权限中间件均支持：
// - 角色权限：通过 role:{role_name} 主体定义
// - 用户特殊权限：通过 user:{admin_id} 主体定义
// - 权限优先级：用户特殊权限 > 角色权限
type AuthManager struct {
	config   *viper.Viper
	enforcer *casbin.Enforcer
	db       *gorm.DB
}

// newAuthManager 创建认证授权管理器
func newAuthManager(config *viper.Viper, enforcer *casbin.Enforcer, db *gorm.DB) *AuthManager {
	return &AuthManager{
		config:   config,
		enforcer: enforcer,
		db:       db,
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
// - 出错时通过 errorHandlerMiddleware 统一响应
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
				ctx.Error(fmt.Errorf("认证处理失败: %w", ErrInternalServer))
			}
			ctx.Abort()
			return
		}

		ctx.Set(contextKeyUserClaims, claims)
		ctx.Next()
	}
}

// AuthorizationMiddleware 基于抽象资源权限码的权限控制
// resource: 资源名称（如 "member"）
// action: 操作名称（如 "read", "write"）
//
// 使用场景：在代码中直接指定权限，适用于需要明确权限控制的场景
// 支持角色权限和用户特殊权限
func (am *AuthManager) AuthorizationMiddleware(resource string, action string) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		claims, ok := getUserClaimsOrAbort(ctx)
		if !ok {
			return
		}

		// 使用统一的权限检查逻辑（支持用户特殊权限 + 角色权限）
		if !am.checkPermission(claims, resource, action) {
			ctx.Error(fmt.Errorf("权限不足，无法访问此资源: %w", ErrForbidden))
			ctx.Abort()
			return
		}

		ctx.Next()
	}
}

// checkPermission 检查用户权限（支持角色权限 + 用户特殊权限）
func (am *AuthManager) checkPermission(claims *UserClaims, resource, action string) bool {
	// 0. 检查是否为超级管理员（username = "admin"）
	// 超级管理员拥有所有权限，无需查询数据库或 Casbin
	if claims.Username == "admin" {
		return true
	}

	// 1. 检查用户特殊权限
	userSub := EncodeUserSub(claims.UserID)
	if allowed, _ := am.enforcer.Enforce(userSub, resource, action); allowed {
		return true
	}

	// 2. 检查角色权限（roles 中存储的是角色ID的字符串形式）
	roleIDs := append([]string(nil), claims.Roles...)
	if len(roleIDs) == 0 && claims.PrimaryRole != "" {
		roleIDs = append(roleIDs, claims.PrimaryRole)
	}

	for _, roleIDStr := range roleIDs {
		// 角色ID已经是字符串格式，直接转换为uint
		roleID, err := strconv.ParseUint(roleIDStr, 10, 32)
		if err != nil {
			continue // 跳过无效的角色ID
		}
		roleSub := EncodeRoleSub(uint(roleID))
		if allowed, _ := am.enforcer.Enforce(roleSub, resource, action); allowed {
			return true
		}
	}

	return false
}
