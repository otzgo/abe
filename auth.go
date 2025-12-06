package abe

import (
	"errors"
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// 认证相关错误
var (
	ErrInvalidToken      = errors.New("invalid token")
	ErrTokenExpired      = errors.New("token expired")
	ErrInvalidSigningKey = errors.New("invalid signing key")
)

// contextKeyUserClaims 上下文键约定：存放用户声明
const contextKeyUserClaims = "abe.user_claims"

// UserClaims 定义 RESTFul API 通用的用户声明
// 覆盖常见用户身份、角色、租户、会话与标准 JWT 字段
// - UserID/Username 为必备
// - PrimaryRole 与 Roles 支持单/多角色
// - TenantID/ClientID/Scopes/SessionID 视场景选用
// - RegisteredClaims 遵循标准字段 (exp/iat/nbf/iss/aud/jti)
type UserClaims struct {
	UserID      string   `json:"uid"`
	Username    string   `json:"uname"`
	DisplayName string   `json:"display_name,omitempty"`
	TenantID    string   `json:"tenant_id,omitempty"`
	PrimaryRole string   `json:"primary_role,omitempty"`
	Roles       []string `json:"roles,omitempty"`
	Scopes      []string `json:"scopes,omitempty"`
	ClientID    string   `json:"client_id,omitempty"`
	SessionID   string   `json:"sid,omitempty"`

	jwt.RegisteredClaims
}

// AuthConfig 认证配置结构体
// 使用 mapstructure 标签以便通过 viper.UnmarshalKey("auth", &cfg) 解析。
// 字段说明：
// - JWTSecret：JWT HMAC 签名密钥（必须配置，生产环境请使用强随机字符串）
// - TokenExpiry：访问令牌过期时间（单位：小时；默认 24）
// - Issuer：令牌签发方（可选，用于标准 JWT 字段校验/标识）
// - Audience：令牌受众（可选，用于区分调用方或客户端标识）
// - ClockSkewSeconds：允许的时钟偏差（单位：秒；用于处理服务间时钟差）
// - EnableRefresh：是否启用刷新令牌机制（可选；仅作配置占位）
// - RefreshExpiry：刷新令牌过期时间（单位：小时；可选）
type AuthConfig struct {
	JWTSecret        string   `mapstructure:"jwt_secret"`
	TokenExpiry      int      `mapstructure:"token_expiry"`
	Issuer           string   `mapstructure:"issuer"`
	Audience         []string `mapstructure:"audience"`
	ClockSkewSeconds int      `mapstructure:"clock_skew_seconds"`
	EnableRefresh    bool     `mapstructure:"enable_refresh"`
	RefreshExpiry    int      `mapstructure:"refresh_expiry"`
}

// GetUserClaims 从上下文中获取用户声明
func GetUserClaims(ctx *gin.Context) (*UserClaims, bool) {
	v, ok := ctx.Get(contextKeyUserClaims)
	if !ok {
		return nil, false
	}
	claims, ok := v.(*UserClaims)
	return claims, ok
}

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

// getUserClaimsOrAbort 获取用户声明，失败时中止请求
func getUserClaimsOrAbort(ctx *gin.Context) (*UserClaims, bool) {
	claims, ok := GetUserClaims(ctx)
	if !ok {
		ctx.Error(fmt.Errorf("未认证的用户: %w", ErrUnauthorized))
		ctx.Abort()
		return nil, false
	}
	return claims, true
}
