# CORS 中间件配置与使用

## CORS 概述

跨域资源共享（CORS）是现代 Web 应用中必不可少的安全机制。ABE 框架基于 `gin-contrib/cors` 提供了完整的 CORS 中间件支持，可以灵活配置跨域访问策略。

## 基础配置

### 配置文件设置

在 `config.yaml` 中配置 CORS 参数：

```yaml
cors:
  allow_origins:
    - "http://localhost:3000"
    - "https://myapp.com"
    - "*"  # 允许所有源（生产环境慎用）
  allow_methods:
    - "GET"
    - "POST"
    - "PUT"
    - "DELETE"
    - "PATCH"
    - "OPTIONS"
  allow_headers:
    - "Origin"
    - "Content-Type"
    - "Accept"
    - "Authorization"
    - "X-Requested-With"
    - "X-Access-Token"
  expose_headers:
    - "Content-Length"
    - "Access-Control-Allow-Origin"
  allow_credentials: true
  max_age_seconds: 86400  # 24小时
```

### 代码中配置 CORS

```go
import (
    "github.com/gin-contrib/cors"
    "github.com/otzgo/abe"
)

func setupCORS(engine *abe.Engine) {
    // 从配置文件读取 CORS 设置
    config := cors.DefaultConfig()
    
    // 允许的源
    origins := engine.Config().GetStringSlice("cors.allow_origins")
    if len(origins) > 0 {
        config.AllowOrigins = origins
    } else {
        config.AllowAllOrigins = true // 默认允许所有源
    }
    
    // 允许的方法
    methods := engine.Config().GetStringSlice("cors.allow_methods")
    if len(methods) > 0 {
        config.AllowMethods = methods
    }
    
    // 允许的头部
    headers := engine.Config().GetStringSlice("cors.allow_headers")
    if len(headers) > 0 {
        config.AllowHeaders = headers
    }
    
    // 暴露的头部
    exposeHeaders := engine.Config().GetStringSlice("cors.expose_headers")
    if len(exposeHeaders) > 0 {
        config.ExposeHeaders = exposeHeaders
    }
    
    // 凭据设置
    config.AllowCredentials = engine.Config().GetBool("cors.allow_credentials")
    
    // 预检请求缓存时间
    maxAge := engine.Config().GetInt("cors.max_age_seconds")
    if maxAge > 0 {
        config.MaxAge = time.Duration(maxAge) * time.Second
    }
    
    // 注册 CORS 中间件
    corsMiddleware := cors.New(config)
    engine.MiddlewareManager().RegisterGlobal(corsMiddleware)
}
```

## 常见 CORS 配置场景

### 1. 开发环境配置

```go
func setupDevelopmentCORS(engine *abe.Engine) {
    config := cors.DefaultConfig()
    config.AllowAllOrigins = true  // 允许所有源
    config.AllowMethods = []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}
    config.AllowHeaders = []string{
        "Origin", "Content-Type", "Accept", "Authorization",
        "X-Requested-With", "X-Access-Token",
    }
    config.AllowCredentials = true
    config.MaxAge = 12 * time.Hour
    
    engine.MiddlewareManager().RegisterGlobal(cors.New(config))
}
```

### 2. 生产环境配置

```go
func setupProductionCORS(engine *abe.Engine) {
    config := cors.DefaultConfig()
    config.AllowOrigins = []string{
        "https://myapp.com",
        "https://www.myapp.com",
        "https://admin.myapp.com",
    }
    config.AllowMethods = []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}
    config.AllowHeaders = []string{
        "Origin", "Content-Type", "Accept", "Authorization",
        "X-Requested-With", "X-API-Key",
    }
    config.ExposeHeaders = []string{"Content-Length"}
    config.AllowCredentials = true
    config.MaxAge = 24 * time.Hour
    
    engine.MiddlewareManager().RegisterGlobal(cors.New(config))
}
```

### 3. 动态源配置

```go
func setupDynamicCORS(engine *abe.Engine) {
    config := cors.DefaultConfig()
    
    // 动态允许源
    config.AllowOriginFunc = func(origin string) bool {
        // 允许本地开发域名
        if strings.HasPrefix(origin, "http://localhost:") {
            return true
        }
        
        // 允许特定域名模式
        allowedPatterns := []string{
            `^https://[\w-]+\.myapp\.com$`,
            `^https://admin\.myapp\.com$`,
        }
        
        for _, pattern := range allowedPatterns {
            matched, _ := regexp.MatchString(pattern, origin)
            if matched {
                return true
            }
        }
        
        return false
    }
    
    config.AllowMethods = []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}
    config.AllowHeaders = []string{
        "Origin", "Content-Type", "Accept", "Authorization",
    }
    config.AllowCredentials = true
    
    engine.MiddlewareManager().RegisterGlobal(cors.New(config))
}
```

## 高级使用技巧

### 1. 路由级 CORS 配置

```go
func (uc *UserController) RegisterRoutes(router gin.IRouter, mg *MiddlewareManager, engine *Engine) {
    // 为特定路由组设置不同的 CORS 策略
    publicAPI := router.Group("/api/public")
    {
        // 公共 API 允许更多源
        publicAPI.Use(createPublicCORS())
        publicAPI.GET("/data", uc.getPublicData)
        publicAPI.GET("/stats", uc.getStats)
    }
    
    privateAPI := router.Group("/api/private")
    {
        // 私有 API 限制源
        privateAPI.Use(createPrivateCORS())
        privateAPI.GET("/profile", uc.getProfile)
        privateAPI.PUT("/settings", uc.updateSettings)
    }
}

func createPublicCORS() gin.HandlerFunc {
    config := cors.DefaultConfig()
    config.AllowOrigins = []string{"*"}  // 允许所有源
    config.AllowMethods = []string{"GET", "OPTIONS"}
    return cors.New(config)
}

func createPrivateCORS() gin.HandlerFunc {
    config := cors.DefaultConfig()
    config.AllowOrigins = []string{
        "https://myapp.com",
        "https://admin.myapp.com",
    }
    config.AllowMethods = []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}
    config.AllowCredentials = true
    return cors.New(config)
}
```

### 2. 条件 CORS 配置

```go
func conditionalCORS() gin.HandlerFunc {
    return func(ctx *gin.Context) {
        origin := ctx.GetHeader("Origin")
        
        // 根据请求特征应用不同 CORS 策略
        if strings.Contains(origin, "admin") {
            // 管理员站点的宽松策略
            ctx.Header("Access-Control-Allow-Origin", origin)
            ctx.Header("Access-Control-Allow-Credentials", "true")
            ctx.Header("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
            ctx.Header("Access-Control-Allow-Headers", "Authorization,Content-Type")
        } else {
            // 普通用户的严格策略
            ctx.Header("Access-Control-Allow-Origin", "https://myapp.com")
            ctx.Header("Access-Control-Allow-Methods", "GET,OPTIONS")
        }
        
        if ctx.Request.Method == "OPTIONS" {
            ctx.AbortWithStatus(204)
            return
        }
        
        ctx.Next()
    }
}
```

## 常见问题解决

### 1. 预检请求处理

```go
// 确保正确处理 OPTIONS 预检请求
func handlePreflight() gin.HandlerFunc {
    return func(ctx *gin.Context) {
        if ctx.Request.Method == "OPTIONS" {
            ctx.Header("Access-Control-Allow-Origin", "*")
            ctx.Header("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,PATCH,OPTIONS")
            ctx.Header("Access-Control-Allow-Headers", "Content-Type,Authorization,X-Requested-With")
            ctx.Header("Access-Control-Max-Age", "86400")
            ctx.AbortWithStatus(204)
            return
        }
        ctx.Next()
    }
}

// 在主程序中使用
func main() {
    engine := abe.NewEngine()
    
    // 先注册预检处理中间件
    engine.MiddlewareManager().RegisterGlobal(handlePreflight())
    
    // 再注册 CORS 中间件
    setupCORS(engine)
    
    engine.Run()
}
```

### 2. 凭据和通配符问题

```go
// ❌ 错误：AllowCredentials=true 时不能使用通配符
/*
config := cors.DefaultConfig()
config.AllowAllOrigins = true
config.AllowCredentials = true  // 这会导致错误
*/

// ✅ 正确：明确指定允许的源
func correctCORSWithCredentials() gin.HandlerFunc {
    config := cors.DefaultConfig()
    config.AllowOrigins = []string{
        "https://myapp.com",
        "https://admin.myapp.com",
    }
    config.AllowCredentials = true
    config.AllowMethods = []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}
    config.AllowHeaders = []string{"Authorization", "Content-Type"}
    
    return cors.New(config)
}
```

### 3. 调试 CORS 问题

```go
func debugCORS() gin.HandlerFunc {
    return func(ctx *gin.Context) {
        // 记录 CORS 相关信息
        logger := abe.MustGetLogger(ctx)
        logger.Info("CORS Debug",
            "origin", ctx.GetHeader("Origin"),
            "method", ctx.Request.Method,
            "headers", ctx.GetHeader("Access-Control-Request-Headers"),
        )
        
        // 检查是否为预检请求
        if ctx.Request.Method == "OPTIONS" {
            logger.Info("Handling preflight request")
        }
        
        ctx.Next()
        
        // 记录响应头
        logger.Info("Response CORS Headers",
            "allow_origin", ctx.Writer.Header().Get("Access-Control-Allow-Origin"),
            "allow_credentials", ctx.Writer.Header().Get("Access-Control-Allow-Credentials"),
        )
    }
}
```

## 最佳实践

### 1. 安全配置原则

```go
// ✅ 推荐的安全配置
func secureCORSConfig(engine *abe.Engine) {
    config := cors.DefaultConfig()
    
    // 明确指定允许的源，避免使用通配符
    config.AllowOrigins = []string{
        "https://myapp.com",
        "https://www.myapp.com",
    }
    
    // 限制允许的方法
    config.AllowMethods = []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}
    
    // 严格控制允许的头部
    config.AllowHeaders = []string{
        "Authorization",
        "Content-Type",
        "X-Requested-With",
        "X-API-Key",
    }
    
    // 只在必要时允许凭据
    config.AllowCredentials = true
    
    // 合理设置预检请求缓存时间
    config.MaxAge = 2 * time.Hour
    
    engine.MiddlewareManager().RegisterGlobal(cors.New(config))
}
```

### 2. 环境差异化配置

```go
func setupEnvironmentCORS(engine *abe.Engine) {
    env := engine.Config().GetString("app.env")
    
    switch env {
    case "production":
        setupProductionCORS(engine)
    case "staging":
        setupStagingCORS(engine)
    default: // development
        setupDevelopmentCORS(engine)
    }
}

func setupStagingCORS(engine *abe.Engine) {
    config := cors.DefaultConfig()
    config.AllowOrigins = []string{
        "https://staging.myapp.com",
        "https://admin-staging.myapp.com",
    }
    // 其他配置...
    engine.MiddlewareManager().RegisterGlobal(cors.New(config))
}
```

### 3. 监控和日志

```go
func corsWithMonitoring() gin.HandlerFunc {
    return func(ctx *gin.Context) {
        startTime := time.Now()
        origin := ctx.GetHeader("Origin")
        
        ctx.Next()
        
        // 记录 CORS 相关指标
        logger := abe.MustGetLogger(ctx)
        logger.Info("CORS Request Processed",
            "origin", origin,
            "method", ctx.Request.Method,
            "status", ctx.Writer.Status(),
            "duration", time.Since(startTime),
            "allowed", ctx.Writer.Header().Get("Access-Control-Allow-Origin") != "",
        )
    }
}
```

## 故障排除

### 常见 CORS 错误

1. **"No 'Access-Control-Allow-Origin' header"**
   - 确认 CORS 中间件已正确注册
   - 检查 AllowOrigins 配置是否包含请求源
   - 验证请求方法是否在 AllowMethods 中

2. **"The value of the 'Access-Control-Allow-Origin' header is invalid"**
   - 当 AllowCredentials=true 时，不能使用 "*" 通配符
   - 必须明确指定具体的源域名

3. **预检请求失败**
   - 确保正确处理 OPTIONS 方法
   - 检查 AllowHeaders 配置是否包含自定义头部
   - 验证 MaxAge 设置是否合理

4. **凭据发送失败**
   - 确认 AllowCredentials 设置为 true
   - 检查前端是否正确设置了 withCredentials
   - 验证响应中是否包含 Access-Control-Allow-Credentials 头部