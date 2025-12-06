package abe

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/casbin/casbin/v2"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/spf13/viper"
	"gorm.io/gorm"
)

// AuthManager 认证授权管理器
// 封装 JWT 令牌生成/解析和 Casbin 权限检查功能
//
// 权限中间件使用指南：
// 1. ResourceAuthorizationMiddleware(resource, action)
//   - 在代码中直接指定权限
//   - 适用于：需要明确权限控制的特殊场景
//   - 示例：engine.Auth().ResourceAuthorizationMiddleware("member", "read")
//
// 2. PathAuthorizationMiddleware(mapper)
//   - 通过编程方式动态映射权限
//   - 适用于：需要复杂逻辑判断的场景
//   - 示例：engine.Auth().PathAuthorizationMiddleware(customMapper)
//
// 3. AutoAuthorizationMiddleware()
//   - 自动从数据库查询权限映射
//   - 适用于：标准 RESTful 接口，支持热更新
//   - 示例：engine.Auth().AutoAuthorizationMiddleware()
//   - 推荐：首选方案，权限配置数据化
//
// 所有权限中间件均支持：
// - 角色权限：通过 role:{role_name} 主体定义
// - 用户特殊权限：通过 user:{admin_id} 主体定义
// - 权限优先级：用户特殊权限 > 角色权限
type AuthManager struct {
	config   *viper.Viper
	enforcer *casbin.Enforcer
	db       *gorm.DB

	// 权限映射缓存
	mappingCache sync.Map // key: "METHOD:PATH", value: *APIPermissionMapping
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

// ResourceAuthorizationMiddleware 基于抽象资源权限码的权限控制
// resource: 资源名称（如 "member"）
// action: 操作名称（如 "read", "write"）
//
// 使用场景：在代码中直接指定权限，适用于需要明确权限控制的场景
// 支持角色权限和用户特殊权限
func (am *AuthManager) ResourceAuthorizationMiddleware(resource string, action string) gin.HandlerFunc {
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

// PathAuthorizationMiddleware 基于路由路径自动映射权限码的中间件
// mapper: 路径到权限码的映射函数，返回 (resource, action, found)
//
// 使用场景：通过编程方式动态映射权限，适用于需要复杂逻辑的场景
// 支持角色权限和用户特殊权限
func (am *AuthManager) PathAuthorizationMiddleware(mapper func(method, path string) (resource, action string, found bool)) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		claims, ok := getUserClaimsOrAbort(ctx)
		if !ok {
			return
		}

		method := ctx.Request.Method
		path := ctx.FullPath()

		resource, action, found := mapper(method, path)
		if !found {
			ctx.Error(fmt.Errorf("未找到路径权限映射: %s %s: %w", method, path, ErrForbidden))
			ctx.Abort()
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

// LoadPermissionMappings 从数据库加载所有权限映射到缓存
// 应在应用启动时调用
func (am *AuthManager) LoadPermissionMappings() error {
	if am.db == nil {
		return fmt.Errorf("数据库连接未初始化")
	}

	var mappings []APIPermissionMapping
	if err := am.db.Where("is_active = ?", true).Find(&mappings).Error; err != nil {
		return fmt.Errorf("加载权限映射失败: %w", err)
	}

	// 清空旧缓存
	am.mappingCache.Range(func(key, value interface{}) bool {
		am.mappingCache.Delete(key)
		return true
	})

	// 加载新映射
	for i := range mappings {
		key := am.makeMappingKey(mappings[i].Method, mappings[i].Path)
		am.mappingCache.Store(key, &mappings[i])
	}

	return nil
}

// ReloadPermissionMappings 热加载权限映射
// 可通过管理接口调用实现动态更新
func (am *AuthManager) ReloadPermissionMappings() error {
	return am.LoadPermissionMappings()
}

// makeMappingKey 生成映射缓存的键
func (am *AuthManager) makeMappingKey(method, path string) string {
	return method + ":" + path
}

// getAPIPermissionMapping 查询权限映射
func (am *AuthManager) getAPIPermissionMapping(method, path string) (*APIPermissionMapping, error) {
	key := am.makeMappingKey(method, path)
	value, ok := am.mappingCache.Load(key)
	if !ok {
		return nil, fmt.Errorf("未找到权限映射: %s %s", method, path)
	}
	mapping, ok := value.(*APIPermissionMapping)
	if !ok {
		return nil, fmt.Errorf("权限映射类型错误")
	}
	return mapping, nil
}

// checkPermission 检查用户权限（支持角色权限 + 用户特殊权限）
func (am *AuthManager) checkPermission(claims *UserClaims, resource, action string) bool {
	// 1. 检查用户特殊权限
	userSub := EncodeUserSub(claims.UserID)
	if allowed, _ := am.enforcer.Enforce(userSub, resource, action); allowed {
		return true
	}

	// 2. 检查角色权限
	roles := append([]string(nil), claims.Roles...)
	if len(roles) == 0 && claims.PrimaryRole != "" {
		roles = append(roles, claims.PrimaryRole)
	}

	for _, role := range roles {
		roleSub := EncodeRoleSub(role)
		if allowed, _ := am.enforcer.Enforce(roleSub, resource, action); allowed {
			return true
		}
	}

	return false
}

// AutoAuthorizationMiddleware 基于数据库映射的自动权限中间件
// 自动从 api_permission_mappings 表查询当前路径所需权限
//
// 使用场景：标准 RESTful 接口，权限配置存储在数据库中，支持热更新
// 支持角色权限和用户特殊权限
func (am *AuthManager) AutoAuthorizationMiddleware() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		claims, ok := getUserClaimsOrAbort(ctx)
		if !ok {
			return
		}

		method := ctx.Request.Method
		path := ctx.FullPath()

		// 查询映射
		mapping, err := am.getAPIPermissionMapping(method, path)
		if err != nil {
			ctx.Error(fmt.Errorf("未找到接口权限配置: %s %s: %w", method, path, ErrForbidden))
			ctx.Abort()
			return
		}

		// 执行权限检查
		if !am.checkPermission(claims, mapping.Resource, mapping.Action) {
			ctx.Error(fmt.Errorf("权限不足: 需要 %s 权限: %w", mapping.Code(), ErrForbidden))
			ctx.Abort()
			return
		}

		ctx.Next()
	}
}
