package abe

import (
	"errors"
	"fmt"
	"reflect"
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

// UserTokenClaims 用户令牌声明接口
// 定义用户登录成功后获得的访问令牌必须实现的标准契约
//
// 此接口继承 jwt.Claims，确保与 JWT 标准兼容，同时扩展了用户身份和角色相关的方法。
// 任何实现此接口的类型都可以作为用户认证令牌的声明数据在框架中使用。
//
// 使用场景：
//   - 在认证中间件中验证和提取用户信息
//   - 在授权中间件中获取用户角色进行权限检查
//   - 在业务逻辑中获取当前登录用户的身份信息
//
// 示例实现：
//
//	type MyClaims struct {
//	    UID         string   `json:"uid"`
//	    PrimaryRole string   `json:"primary_role"`
//	    AllRoles    []string `json:"roles"`
//	    jwt.RegisteredClaims
//	}
//
//	func (c *MyClaims) UserID() string {
//	    return c.UID
//	}
//
//	func (c *MyClaims) Role() string {
//	    return c.PrimaryRole
//	}
//
//	func (c *MyClaims) Roles() []string {
//	    return c.AllRoles
//	}
type UserTokenClaims interface {
	jwt.Claims

	// UserID 返回当前访问令牌所属的用户唯一标识
	// 此ID应在系统中唯一标识一个用户，通常为用户的主键或UUID
	UserID() string

	// Role 返回当前访问令牌所属用户的主角色
	// 主角色通常表示用户的主要身份或职能，在单角色场景中使用
	// 返回空字符串表示用户没有主角色
	Role() string

	// Roles 返回当前访问令牌所属用户的所有角色列表
	// 支持多角色场景，返回用户拥有的全部角色标识
	// 返回空切片或nil表示用户没有任何角色
	Roles() []string
}

// AuthenticationMiddleware 全局身份认证中间件（泛型版本）
// 这是一个通用的 JWT 认证中间件，支持任何实现了 UserTokenClaims 接口的自定义声明类型
//
// 类型参数：
//   - T: 必须实现 UserTokenClaims 接口的具体类型（UserTokenClaims 已包含 jwt.Claims）
//
// 功能：
//   - 从 HTTP 请求头中提取 Authorization: Bearer {token}
//   - 使用配置的 JWT 密钥验证和解析令牌
//   - 将解析出的用户声明（UserTokenClaims 接口）存储到 Gin 上下文
//   - 处理各种认证错误并通过统一错误处理机制响应
//
// 参数：
//   - engine: *Engine 实例，用于访问配置和其他依赖
//
// 返回：
//   - gin.HandlerFunc: Gin 中间件函数
//
// 使用示例：
//
//	// 1. 在应用层定义自定义 Claims
//	type MyAppClaims struct {
//	    UID         string   `json:"uid"`
//	    Username    string   `json:"username"`
//	    MainRole    string   `json:"main_role"`
//	    AllRoles    []string `json:"all_roles"`
//	    jwt.RegisteredClaims
//	}
//
//	// 实现 UserTokenClaims 接口
//	func (c *MyAppClaims) UserID() string { return c.UID }
//	func (c *MyAppClaims) Role() string { return c.MainRole }
//	func (c *MyAppClaims) Roles() []string { return c.AllRoles }
//
//	// 2. 使用自定义 Claims 类型创建中间件
//	engine.Router().Use(abe.AuthenticationMiddleware[*MyAppClaims](engine))
//
//	// 3. 在处理器中获取声明
//	func myHandler(ctx *gin.Context) {
//	    claims, ok := abe.GetUserTokenClaims(ctx)
//	    if !ok {
//	        return
//	    }
//	    userID := claims.UserID()
//	    // 使用用户信息...
//	}
//
// 错误处理：
//   - 未提供认证信息 -> ErrUnauthorized
//   - 认证头格式错误 -> ErrUnauthorized
//   - 令牌已过期 -> ErrTokenExpired
//   - 令牌签名无效 -> ErrUnauthorized
//   - 令牌格式错误 -> ErrUnauthorized
//   - 其他解析错误 -> ErrInternalServer
//
// 注意：
//   - T 必须是指针类型（如 *MyAppClaims），因为 JWT 解析需要指针来填充数据
//   - 解析后的声明可通过 GetUserTokenClaims(ctx) 以接口形式获取
//   - 所有错误都会通过 ctx.Error() 传递给 errorHandlerMiddleware 统一处理
func AuthenticationMiddleware[T UserTokenClaims](engine *Engine) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		// 1. 提取 Authorization 头
		authHeader := ctx.GetHeader("Authorization")
		if authHeader == "" {
			_ = ctx.Error(fmt.Errorf("未提供认证信息: %w", ErrUnauthorized))
			ctx.Abort()
			return
		}

		// 2. 验证 Bearer 格式
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			_ = ctx.Error(fmt.Errorf("认证头格式错误，应为 'Bearer {token}': %w", ErrUnauthorized))
			ctx.Abort()
			return
		}

		tokenString := parts[1]

		// 3. 获取 JWT 配置
		secret := engine.Config().GetString("auth.jwt_secret")
		if secret == "" {
			_ = ctx.Error(fmt.Errorf("JWT 密钥未配置: %w", ErrInternalServer))
			ctx.Abort()
			return
		}

		// 4. 解析令牌 - 使用泛型类型 T
		claims, err := ParseToken[T](tokenString, secret)
		if err != nil {
			// 5. 错误分类处理
			switch {
			case errors.Is(err, jwt.ErrTokenExpired):
				_ = ctx.Error(fmt.Errorf("令牌已过期: %w", ErrTokenExpired))
			case errors.Is(err, ErrInvalidToken), errors.Is(err, ErrInvalidSigningKey):
				_ = ctx.Error(fmt.Errorf("无效令牌: %w", ErrUnauthorized))
			default:
				_ = ctx.Error(fmt.Errorf("认证处理失败: %w", ErrInternalServer))
			}
			ctx.Abort()
			return
		}

		// 6. 将声明存储到上下文
		// 存储为 UserTokenClaims 接口类型，方便后续使用
		ctx.Set(contextKeyUserClaims, UserTokenClaims(claims))

		// 7. 继续处理请求
		ctx.Next()
	}
}

// GetUserTokenClaims 从 Gin 上下文中获取用户令牌声明
// 这是一个辅助函数，用于从上下文中提取实现了 UserTokenClaims 接口的声明
//
// 参数：
//   - ctx: Gin 上下文
//
// 返回：
//   - UserTokenClaims: 用户令牌声明接口，如果不存在或类型不匹配则返回 nil
//   - bool: 是否成功获取
//
// 使用示例：
//
//	func myHandler(ctx *gin.Context) {
//	    claims, ok := abe.GetUserTokenClaims(ctx)
//	    if !ok {
//	        // 处理未认证的情况
//	        return
//	    }
//
//	    userID := claims.UserID()
//	    role := claims.Role()
//	    roles := claims.Roles()
//	    // 使用用户信息...
//	}
func GetUserTokenClaims(ctx *gin.Context) (UserTokenClaims, bool) {
	v, ok := ctx.Get(contextKeyUserClaims)
	if !ok {
		return nil, false
	}

	// 尝试转换为 UserTokenClaims 接口
	if claims, ok := v.(UserTokenClaims); ok {
		return claims, true
	}

	return nil, false
}

// NewToken 生成 JWT 令牌的泛型函数
// 使用 HS256 (HMAC SHA256) 签名算法对传入的声明进行签名
//
// 类型参数：
//   - T: 必须实现 jwt.Claims 接口的声明类型
//
// 参数：
//   - claims: JWT 声明数据
//   - secret: HMAC 签名密钥
//
// 返回值：
//   - string: 签名后的 JWT 字符串
//   - error: 签名过程中的错误
//
// 示例：
//
//	type MyClaims struct {
//	    UserID string `json:"user_id"`
//	    jwt.RegisteredClaims
//	}
//
//	claims := MyClaims{
//	    UserID: "123",
//	    RegisteredClaims: jwt.RegisteredClaims{
//	        ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
//	        IssuedAt:  jwt.NewNumericDate(time.Now()),
//	    },
//	}
//
//	token, err := abe.NewToken(claims, "my-secret-key")
//	if err != nil {
//	    // 处理错误
//	}
func NewToken[T jwt.Claims](claims T, secret string) (string, error) {
	if secret == "" {
		return "", fmt.Errorf("密钥不能为空: %w", ErrInvalidSigningKey)
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signedToken, err := token.SignedString([]byte(secret))
	if err != nil {
		return "", fmt.Errorf("签名令牌失败: %w", err)
	}

	return signedToken, nil
}

// ParseToken 解析并验证 JWT 令牌的泛型函数
// 使用 HS256 (HMAC SHA256) 算法验证令牌签名并解析声明
//
// 类型参数：
//   - T: 必须实现 jwt.Claims 接口的声明类型
//
// 参数：
//   - tokenString: 待解析的 JWT 字符串
//   - secret: HMAC 验证密钥（必须与生成时使用的密钥一致）
//
// 返回值：
//   - T: 解析后的声明数据
//   - error: 解析或验证过程中的错误
//
// 错误处理：
//   - 令牌过期：返回包装了 jwt.ErrTokenExpired 的错误
//   - 签名无效：返回包装了 ErrInvalidSigningKey 的错误
//   - 令牌格式错误：返回包装了 ErrInvalidToken 的错误
//   - 其他错误：返回原始错误
//
// 示例：
//
//	type MyClaims struct {
//	    UserID string `json:"user_id"`
//	    jwt.RegisteredClaims
//	}
//
//	claims, err := abe.ParseToken[MyClaims](tokenString, "my-secret-key")
//	if err != nil {
//	    if errors.Is(err, jwt.ErrTokenExpired) {
//	        // 处理令牌过期
//	    }
//	    // 处理其他错误
//	}
func ParseToken[T jwt.Claims](tokenString string, secret string) (T, error) {
	var zero T

	if secret == "" {
		return zero, fmt.Errorf("密钥不能为空: %w", ErrInvalidSigningKey)
	}

	if tokenString == "" {
		return zero, fmt.Errorf("令牌字符串不能为空: %w", ErrInvalidToken)
	}

	// 创建 claims 实例
	// 使用反射来处理指针和值类型
	var claims jwt.Claims
	claimsType := reflect.TypeOf(zero)
	if claimsType.Kind() == reflect.Ptr {
		// 如果 T 是指针类型，创建其元素类型的新实例
		claims = reflect.New(claimsType.Elem()).Interface().(jwt.Claims)
	} else {
		// 如果 T 是值类型，创建指向新实例的指针
		claims = reflect.New(claimsType).Interface().(jwt.Claims)
	}

	// 解析令牌
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (any, error) {
		// 验证签名方法
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v: %w", token.Header["alg"], ErrInvalidSigningKey)
		}
		return []byte(secret), nil
	})

	if err != nil {
		// 处理 JWT 相关错误
		if errors.Is(err, jwt.ErrTokenExpired) {
			return zero, fmt.Errorf("令牌已过期: %w", jwt.ErrTokenExpired)
		}
		if errors.Is(err, jwt.ErrTokenMalformed) {
			return zero, fmt.Errorf("令牌格式错误: %w", ErrInvalidToken)
		}
		if errors.Is(err, jwt.ErrTokenSignatureInvalid) {
			return zero, fmt.Errorf("令牌签名无效: %w", ErrInvalidSigningKey)
		}
		if errors.Is(err, jwt.ErrTokenNotValidYet) {
			return zero, fmt.Errorf("令牌尚未生效: %w", ErrInvalidToken)
		}
		// 其他错误
		return zero, fmt.Errorf("解析令牌失败: %w", err)
	}

	// 验证令牌有效性
	if !token.Valid {
		return zero, fmt.Errorf("令牌无效: %w", ErrInvalidToken)
	}

	// 提取声明
	if parsedClaims, ok := token.Claims.(T); ok {
		return parsedClaims, nil
	}

	return zero, fmt.Errorf("令牌声明类型不匹配: %w", ErrInvalidToken)
}

// AuthorizationMiddleware 全局权限鉴权中间件
// 这是一个通用的 Casbin 权限检查中间件，支持任何实现了 UserTokenClaims 接口的自定义声明类型
//
// 功能：
//   - 从 Gin 上下文中获取用户声明（需要先经过 AuthenticationMiddleware）
//   - 使用 Casbin enforcer 检查用户是否有权限访问指定资源
//   - 支持用户特殊权限和角色权限两种权限类型
//   - 权限优先级：超级管理员 > 用户特殊权限 > 角色权限
//
// 参数：
//   - engine: *Engine 实例，用于访问 Casbin enforcer
//   - resource: 资源标识符（如 "/api/users" 或自定义资源名）
//   - action: 操作类型（如 "read", "write", "delete" 等）
//
// 返回：
//   - gin.HandlerFunc: Gin 中间件函数
//
// 权限检查逻辑：
//  1. 超级管理员检查：UserID == "1" 的用户拥有所有权限
//  2. 用户特殊权限：检查 "user:{userID}" 主体对资源的权限
//  3. 角色权限：检查用户的所有角色 "role:{roleName}" 对资源的权限
//  4. 如果 Roles() 为空，则使用 Role() 作为备选角色
//
// 使用示例：
//
//	// 1. 定义路由并应用中间件
//	router.GET("/api/users",
//	    abe.AuthenticationMiddleware[*MyAppClaims](engine),  // 先认证
//	    abe.AuthorizationMiddleware(engine, "/api/users", "read"), // 再鉴权
//	    handler,
//	)
//
//	// 2. 动态资源路径
//	router.DELETE("/api/users/:id",
//	    abe.AuthenticationMiddleware[*MyAppClaims](engine),
//	    func(ctx *gin.Context) {
//	        // 可以在处理器中动态构建资源路径
//	        resource := "/api/users/" + ctx.Param("id")
//	        abe.AuthorizationMiddleware(engine, resource, "delete")(ctx)
//	    },
//	    handler,
//	)
//
// 错误处理：
//   - 未认证（无法获取用户声明）-> ErrUnauthorized
//   - 权限不足 -> ErrForbidden
//
// 注意：
//   - 此中间件必须在 AuthenticationMiddleware 之后使用
//   - 需要预先在 Casbin 中配置好权限策略
//   - 角色名称直接使用 "role:" + roleName 格式，不进行类型转换
func AuthorizationMiddleware(engine *Engine, resource string, action string) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		// 1. 从上下文获取用户声明
		claims, ok := GetUserTokenClaims(ctx)
		if !ok {
			_ = ctx.Error(fmt.Errorf("未认证的用户: %w", ErrUnauthorized))
			ctx.Abort()
			return
		}

		// 2. 检查权限
		if !checkPermission(engine, claims, resource, action) {
			_ = ctx.Error(fmt.Errorf("权限不足，无法访问此资源: %w", ErrForbidden))
			ctx.Abort()
			return
		}

		ctx.Next()
	}
}

// checkPermission 检查用户权限（支持超级管理员 + 用户特殊权限 + 角色权限）
func checkPermission(engine *Engine, claims UserTokenClaims, resource, action string) bool {
	// 0. 检查是否为超级管理员（UserID = "1"）
	// 超级管理员拥有所有权限，无需查询 Casbin
	if claims.UserID() == "1" {
		return true
	}

	enforcer := engine.Enforcer()

	// 1. 检查用户特殊权限
	// 格式："user:{userID}"
	userSub := "user:" + claims.UserID()
	if allowed, _ := enforcer.Enforce(userSub, resource, action); allowed {
		return true
	}

	// 2. 检查角色权限
	// 获取角色列表
	roles := claims.Roles()

	// 如果 Roles() 为空，使用 Role() 作为备选
	if len(roles) == 0 {
		if role := claims.Role(); role != "" {
			roles = []string{role}
		}
	}

	// 检查每个角色的权限
	// 格式："role:{roleName}"
	for _, roleName := range roles {
		if roleName == "" {
			continue
		}
		roleSub := "role:" + roleName
		if allowed, _ := enforcer.Enforce(roleSub, resource, action); allowed {
			return true
		}
	}

	return false
}
