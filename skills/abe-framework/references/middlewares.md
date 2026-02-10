# 中间件系统使用指南

## 中间件核心概念

### 中间件类型
ABE 框架支持两种中间件：
- **全局中间件**：应用于所有路由
- **路由级中间件**：应用于特定路由或路由组

### 中间件管理器
通过 `MiddlewareManager` 管理中间件的注册和获取：

```go
// 获取中间件管理器
mg := engine.MiddlewareManager()

// 注册全局中间件
mg.RegisterGlobal(corsMiddleware())

// 注册命名中间件
mg.Register("auth", authMiddleware())
mg.Register("rate_limit", rateLimitMiddleware())

// 获取中间件
authMiddleware := mg.MustShared("auth")
```

## 内置中间件

### CORS 中间件
```go
import "github.com/gin-contrib/cors"

// 基本 CORS 配置
corsConfig := cors.DefaultConfig()
corsConfig.AllowOrigins = []string{"http://localhost:3000"}
corsConfig.AllowMethods = []string{"GET", "POST", "PUT", "DELETE"}
corsConfig.AllowHeaders = []string{"Origin", "Content-Type", "Authorization"}

corsMiddleware := cors.New(corsConfig)

// 注册到引擎
engine.MiddlewareManager().RegisterGlobal(corsMiddleware)
```

### 日志中间件
```go
// 请求日志中间件
func requestLogger() gin.HandlerFunc {
    return func(ctx *gin.Context) {
        start := time.Now()
        
        // 处理请求
        ctx.Next()
        
        // 记录日志
        latency := time.Since(start)
        logger := abe.MustGetLogger(ctx)
        logger.Info("HTTP Request",
            "method", ctx.Request.Method,
            "path", ctx.Request.URL.Path,
            "status", ctx.Writer.Status(),
            "latency", latency,
            "client_ip", ctx.ClientIP(),
        )
    }
}

// 在主程序中注册
engine.MiddlewareManager().RegisterGlobal(requestLogger())
```

### 认证中间件
```go
// JWT 认证中间件示例
func authMiddleware() gin.HandlerFunc {
    return func(ctx *gin.Context) {
        // 从 Header 获取 Token
        authHeader := ctx.GetHeader("Authorization")
        if authHeader == "" {
            ctx.JSON(401, gin.H{"error": "缺少认证信息"})
            ctx.Abort()
            return
        }
        
        // 解析 Token
        tokenString := strings.TrimPrefix(authHeader, "Bearer ")
        claims, err := parseJWTToken(tokenString)
        if err != nil {
            ctx.JSON(401, gin.H{"error": "无效的认证令牌"})
            ctx.Abort()
            return
        }
        
        // 将用户信息存储到上下文
        ctx.Set("user_id", claims.UserID)
        ctx.Set("user_claims", claims)
        
        ctx.Next()
    }
}

// 注册认证中间件
engine.MiddlewareManager().Register("auth", authMiddleware())
```

## 自定义中间件开发

### 基础中间件模板
```go
func customMiddleware() gin.HandlerFunc {
    return func(ctx *gin.Context) {
        // 请求前处理
        // 1. 预处理逻辑
        // 2. 权限检查
        // 3. 参数验证
        
        // 调用下一个中间件或处理器
        ctx.Next()
        
        // 响应后处理
        // 1. 响应修改
        // 2. 日志记录
        // 3. 资源清理
    }
}
```

### 权限控制中间件
```go
// 基于角色的权限控制
func rbacMiddleware(requiredRole string) gin.HandlerFunc {
    return func(ctx *gin.Context) {
        // 获取用户角色
        userClaims, exists := ctx.Get("user_claims")
        if !exists {
            ctx.JSON(401, gin.H{"error": "未认证"})
            ctx.Abort()
            return
        }
        
        claims := userClaims.(*JWTClaims)
        
        // 检查角色权限
        if !hasRole(claims.Roles, requiredRole) {
            ctx.JSON(403, gin.H{"error": "权限不足"})
            ctx.Abort()
            return
        }
        
        ctx.Next()
    }
}

// 在控制器中使用
func (uc *UserController) RegisterRoutes(router gin.IRouter, mg *MiddlewareManager, engine *Engine) {
    adminGroup := router.Group("/admin", rbacMiddleware("admin"))
    {
        adminGroup.POST("/users", uc.createUser)
        adminGroup.DELETE("/users/:id", uc.deleteUser)
    }
}
```

### 速率限制中间件
```go
import "golang.org/x/time/rate"

// 令牌桶速率限制
func rateLimitMiddleware(rps int) gin.HandlerFunc {
    limiter := rate.NewLimiter(rate.Limit(rps), rps)
    
    return func(ctx *gin.Context) {
        if !limiter.Allow() {
            ctx.JSON(429, gin.H{"error": "请求过于频繁"})
            ctx.Abort()
            return
        }
        ctx.Next()
    }
}

// 使用示例
func (uc *UserController) RegisterRoutes(router gin.IRouter, mg *MiddlewareManager, engine *Engine) {
    // 对登录接口进行速率限制
    router.POST("/login", rateLimitMiddleware(5), uc.login) // 每秒最多5次
}
```

## 中间件链和执行顺序

### 执行顺序
```go
// 中间件执行顺序示例
func main() {
    engine := abe.NewEngine()
    
    // 全局中间件注册顺序
    engine.MiddlewareManager().RegisterGlobal(requestIDMiddleware())    // 1
    engine.MiddlewareManager().RegisterGlobal(loggingMiddleware())       // 2
    engine.MiddlewareManager().RegisterGlobal(authenticationMiddleware()) // 3
    
    // 路由级中间件
    engine.AddController(abe.Provider(&UserController{}))
    
    engine.Run()
}

// 在控制器中
func (uc *UserController) RegisterRoutes(router gin.IRouter, mg *MiddlewareManager, engine *Engine) {
    // 路由组中间件
    userGroup := router.Group("/users", validationMiddleware()) // 4
    {
        userGroup.GET("/", authorizationMiddleware(), uc.listUsers) // 5
    }
}

// 实际执行顺序：
// 1. requestIDMiddleware
// 2. loggingMiddleware  
// 3. authenticationMiddleware
// 4. validationMiddleware
// 5. authorizationMiddleware
// 6. 实际的处理器函数
```

## 中间件最佳实践

### 1. 中间件职责单一
```go
// ✅ 好的做法：每个中间件只做一件事
func authMiddleware() gin.HandlerFunc {
    return func(ctx *gin.Context) {
        // 只处理认证逻辑
        if !isValidToken(ctx.GetHeader("Authorization")) {
            ctx.JSON(401, gin.H{"error": "未认证"})
            ctx.Abort()
            return
        }
        ctx.Next()
    }
}

func permissionMiddleware() gin.HandlerFunc {
    return func(ctx *gin.Context) {
        // 只处理权限检查
        if !hasPermission(ctx, "read_users") {
            ctx.JSON(403, gin.H{"error": "权限不足"})
            ctx.Abort()
            return
        }
        ctx.Next()
    }
}

// 在路由中组合使用
userGroup := router.Group("/users", authMiddleware(), permissionMiddleware())
```

### 2. 上下文数据传递
```go
// 认证中间件设置用户信息
func authMiddleware() gin.HandlerFunc {
    return func(ctx *gin.Context) {
        user, err := authenticateUser(ctx.GetHeader("Authorization"))
        if err != nil {
            ctx.JSON(401, gin.H{"error": "认证失败"})
            ctx.Abort()
            return
        }
        
        // 将用户信息存储到上下文
        ctx.Set("user", user)
        ctx.Set("user_id", user.ID)
        ctx.Next()
    }
}

// 在处理器中获取用户信息
func (uc *UserController) getCurrentUser(ctx *gin.Context) {
    user, exists := ctx.Get("user")
    if !exists {
        ctx.JSON(401, gin.H{"error": "用户信息不存在"})
        return
    }
    
    currentUser := user.(*User)
    ctx.JSON(200, currentUser)
}
```

### 3. 错误处理
```go
// 统一错误处理中间件
func errorHandler() gin.HandlerFunc {
    return func(ctx *gin.Context) {
        defer func() {
            if err := recover(); err != nil {
                // 记录错误日志
                logger := abe.MustGetLogger(ctx)
                logger.Error("中间件恐慌", "error", err, "stack", string(debug.Stack()))
                
                // 返回统一错误响应
                ctx.JSON(500, gin.H{
                    "code":    500,
                    "message": "服务器内部错误",
                })
                ctx.Abort()
            }
        }()
        
        ctx.Next()
    }
}
```

## 常用中间件示例

### 请求ID中间件
```go
import (
    "crypto/rand"
    "encoding/hex"
)

func requestIDMiddleware() gin.HandlerFunc {
    return func(ctx *gin.Context) {
        // 生成或获取请求ID
        requestID := ctx.GetHeader("X-Request-ID")
        if requestID == "" {
            // 生成新的请求ID
            bytes := make([]byte, 16)
            rand.Read(bytes)
            requestID = hex.EncodeToString(bytes)
        }
        
        // 设置请求ID到响应头
        ctx.Header("X-Request-ID", requestID)
        
        // 存储到上下文
        ctx.Set("request_id", requestID)
        
        ctx.Next()
    }
}
```

### 响应时间中间件
```go
func responseTimeMiddleware() gin.HandlerFunc {
    return func(ctx *gin.Context) {
        start := time.Now()
        ctx.Next()
        
        // 计算处理时间
        duration := time.Since(start)
        
        // 添加到响应头
        ctx.Header("X-Response-Time", duration.String())
        
        // 记录慢请求
        if duration > time.Second {
            logger := abe.MustGetLogger(ctx)
            logger.Warn("慢请求警告",
                "path", ctx.Request.URL.Path,
                "duration", duration,
            )
        }
    }
}
```

### 跨站请求伪造保护
```go
func csrfMiddleware() gin.HandlerFunc {
    return func(ctx *gin.Context) {
        // 对于安全敏感的操作进行CSRF检查
        if ctx.Request.Method == "POST" || ctx.Request.Method == "PUT" || ctx.Request.Method == "DELETE" {
            csrfToken := ctx.GetHeader("X-CSRF-Token")
            if !validateCSRFToken(csrfToken) {
                ctx.JSON(403, gin.H{"error": "CSRF token 无效"})
                ctx.Abort()
                return
            }
        }
        ctx.Next()
    }
}
```

## 测试中间件

### 中间件单元测试
```go
func TestAuthMiddleware(t *testing.T) {
    // 创建测试路由器
    router := gin.New()
    router.Use(authMiddleware())
    
    // 添加测试路由
    router.GET("/protected", func(ctx *gin.Context) {
        userID, exists := ctx.Get("user_id")
        if !exists {
            ctx.JSON(401, gin.H{"error": "未认证"})
            return
        }
        ctx.JSON(200, gin.H{"user_id": userID})
    })
    
    // 测试有效token
    req := httptest.NewRequest("GET", "/protected", nil)
    req.Header.Set("Authorization", "Bearer valid-token")
    
    w := httptest.NewRecorder()
    router.ServeHTTP(w, req)
    
    assert.Equal(t, 200, w.Code)
    // 验证响应内容...
    
    // 测试无效token
    req = httptest.NewRequest("GET", "/protected", nil)
    req.Header.Set("Authorization", "Bearer invalid-token")
    
    w = httptest.NewRecorder()
    router.ServeHTTP(w, req)
    
    assert.Equal(t, 401, w.Code)
}
```