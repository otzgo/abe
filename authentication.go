package abe

import (
	"errors"
	"net/http"
	"strings"

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

// AuthenticationMiddleware 身份认证中间件：
// - 提取 Authorization: Bearer {token}
// - 校验 JWT
// - 将 UserClaims 写入上下文
// - 出错时通过 ErrorHandlerMiddleware 统一响应
func AuthenticationMiddleware(engine *Engine) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		authHeader := ctx.GetHeader("Authorization")
		if authHeader == "" {
			ctx.Error(&HTTPError{Status: http.StatusUnauthorized, Code: CodeUnauthorized, Message: "未提供认证信息", Details: []ErrorDetail{AuthDetail("missing Authorization header")}})
			ctx.Abort()
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			ctx.Error(&HTTPError{Status: http.StatusUnauthorized, Code: CodeUnauthorized, Message: "认证头格式错误，应为 'Bearer {token}'", Details: []ErrorDetail{AuthDetail("invalid auth header format")}})
			ctx.Abort()
			return
		}

		claims, err := engine.ParseToken(parts[1])
		if err != nil {
			switch {
			case errors.Is(err, ErrTokenExpired):
				ctx.Error(&HTTPError{Status: http.StatusUnauthorized, Code: CodeUnauthorized, Message: "令牌已过期", Details: []ErrorDetail{AuthDetail("token expired")}})
			case errors.Is(err, ErrInvalidToken), errors.Is(err, ErrInvalidSigningKey):
				ctx.Error(&HTTPError{Status: http.StatusUnauthorized, Code: CodeUnauthorized, Message: "无效令牌", Details: []ErrorDetail{AuthDetail("invalid token")}})
			default:
				ctx.Error(&HTTPError{Status: http.StatusInternalServerError, Code: CodeInternalServerError, Message: "认证处理失败", Details: []ErrorDetail{AuthDetail(err.Error())}})
			}
			ctx.Abort()
			return
		}

		ctx.Set(contextKeyUserClaims, claims)
		ctx.Next()
	}
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
