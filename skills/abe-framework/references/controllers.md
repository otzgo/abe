# 控制器开发指南

## 控制器核心概念

### Controller 接口规范
所有控制器必须实现标准的 `Controller` 接口：

```go
type Controller interface {
    RegisterRoutes(router gin.IRouter, mg *MiddlewareManager, engine *Engine)
}
```

### ControllerProvider 模式
使用函数式提供者模式实现延迟实例化：

```go
// ControllerProvider 类型定义
type ControllerProvider func() Controller

// Provider 工具函数
func Provider(controller Controller) ControllerProvider {
    return func() Controller {
        return controller
    }
}
```

## 控制器实现示例

### 基础控制器结构
```go
package controllers

import (
    "github.com/gin-gonic/gin"
    "github.com/otzgo/abe"
    "gorm.io/gorm"
    "log/slog"
)

// UserController 用户控制器示例
type UserController struct {
    db     *gorm.DB
    logger *slog.Logger
    engine *abe.Engine
}

// NewUserController 构造函数
func NewUserController(db *gorm.DB, logger *slog.Logger, engine *abe.Engine) *UserController {
    return &UserController{
        db:     db,
        logger: logger,
        engine: engine,
    }
}

// RegisterRoutes 实现 Controller 接口
func (uc *UserController) RegisterRoutes(router gin.IRouter, mg *MiddlewareManager, engine *Engine) {
    // 创建路由组
    userGroup := router.Group("/users")
    {
        // 公共路由
        userGroup.GET("/", uc.listUsers)        // 获取用户列表
        userGroup.GET("/:id", uc.getUserByID)   // 获取单个用户
        
        // 需要认证的路由
        authorized := userGroup.Group("/", mg.MustShared("auth"))
        {
            authorized.POST("/", uc.createUser)     // 创建用户
            authorized.PUT("/:id", uc.updateUser)   // 更新用户
            authorized.DELETE("/:id", uc.deleteUser) // 删除用户
        }
    }
}
```

### RESTful 路由设计
```go
// 标准 RESTful 操作
func (uc *UserController) RegisterRoutes(router gin.IRouter, mg *MiddlewareManager, engine *Engine) {
    group := router.Group("/users")
    {
        // Collection 资源操作
        group.GET("/", uc.listUsers)      // GET /users - 列表
        group.POST("/", uc.createUser)    // POST /users - 创建
        
        // Individual 资源操作  
        group.GET("/:id", uc.getUserByID)       // GET /users/:id - 详情
        group.PUT("/:id", uc.updateUser)        // PUT /users/:id - 更新
        group.PATCH("/:id", uc.patchUser)       // PATCH /users/:id - 部分更新
        group.DELETE("/:id", uc.deleteUser)     // DELETE /users/:id - 删除
        
        // 子资源操作
        group.GET("/:id/orders", uc.getUserOrders)  // GET /users/:id/orders
        group.POST("/:id/avatar", uc.uploadAvatar)  // POST /users/:id/avatar
    }
}
```

## 请求处理方法实现

### 参数绑定和验证
```go
// DTO 定义
type CreateUserRequest struct {
    Name     string `json:"name" binding:"required,min=2,max=50"`
    Email    string `json:"email" binding:"required,email"`
    Password string `json:"password" binding:"required,min=8"`
}

type UserResponse struct {
    ID    uint   `json:"id"`
    Name  string `json:"name"`
    Email string `json:"email"`
}

// 创建用户
func (uc *UserController) createUser(ctx *gin.Context) {
    var req CreateUserRequest
    
    // 绑定请求参数
    if err := ctx.ShouldBindJSON(&req); err != nil {
        ctx.JSON(400, gin.H{"error": err.Error()})
        return
    }
    
    // 业务逻辑处理
    user := &User{
        Name:  req.Name,
        Email: req.Email,
    }
    
    if err := uc.db.Create(user).Error; err != nil {
        uc.logger.Error("创建用户失败", "error", err)
        ctx.JSON(500, gin.H{"error": "创建用户失败"})
        return
    }
    
    // 返回响应
    ctx.JSON(201, UserResponse{
        ID:    user.ID,
        Name:  user.Name,
        Email: user.Email,
    })
}
```

### 查询参数处理
```go
// 分页查询参数
type ListUsersQuery struct {
    Page     int    `form:"page,default=1" binding:"min=1"`
    PageSize int    `form:"page_size,default=10" binding:"min=1,max=100"`
    Sort     string `form:"sort,default=id"`
    Order    string `form:"order,default=desc" binding:"oneof=asc desc"`
    Keyword  string `form:"keyword"`
}

func (uc *UserController) listUsers(ctx *gin.Context) {
    var query ListUsersQuery
    
    // 绑定查询参数
    if err := ctx.ShouldBindQuery(&query); err != nil {
        ctx.JSON(400, gin.H{"error": err.Error()})
        return
    }
    
    // 构建查询
    var users []User
    db := uc.db.Model(&User{})
    
    // 搜索条件
    if query.Keyword != "" {
        db = db.Where("name LIKE ? OR email LIKE ?", 
            "%"+query.Keyword+"%", "%"+query.Keyword+"%")
    }
    
    // 排序
    orderBy := query.Sort + " " + query.Order
    db = db.Order(orderBy)
    
    // 分页
    offset := (query.Page - 1) * query.PageSize
    db = db.Offset(offset).Limit(query.PageSize)
    
    // 执行查询
    if err := db.Find(&users).Error; err != nil {
        ctx.JSON(500, gin.H{"error": "查询失败"})
        return
    }
    
    // 获取总数
    var total int64
    uc.db.Model(&User{}).Count(&total)
    
    ctx.JSON(200, gin.H{
        "data": users,
        "pagination": gin.H{
            "page":      query.Page,
            "page_size": query.PageSize,
            "total":     total,
        },
    })
}
```

## 中间件使用

### 全局中间件
```go
// 在主程序中注册全局中间件
func main() {
    engine := abe.NewEngine()
    
    // 注册全局中间件
    engine.MiddlewareManager().RegisterGlobal(cors.Default())
    engine.MiddlewareManager().RegisterGlobal(requestLogger())
    
    // 添加控制器
    engine.AddController(abe.Provider(NewUserController(engine.DB(), engine.Logger(), engine)))
    
    engine.Run()
}
```

### 路由级中间件
```go
func (uc *UserController) RegisterRoutes(router gin.IRouter, mg *MiddlewareManager, engine *Engine) {
    // 获取中间件实例
    authMiddleware := mg.MustShared("auth")
    rateLimitMiddleware := mg.MustShared("rate_limit")
    
    userGroup := router.Group("/users")
    {
        // 公共路由
        userGroup.GET("/", uc.listUsers)
        userGroup.GET("/:id", uc.getUserByID)
        
        // 需要认证的路由组
        authorized := userGroup.Group("/", authMiddleware)
        {
            authorized.POST("/", uc.createUser)
            authorized.PUT("/:id", uc.updateUser)
            authorized.DELETE("/:id", uc.deleteUser)
        }
        
        // 需要速率限制的路由
        userGroup.POST("/login", rateLimitMiddleware, uc.login)
    }
}
```

### 自定义中间件示例
```go
// 权限检查中间件
func permissionCheck(permission string) gin.HandlerFunc {
    return func(ctx *gin.Context) {
        // 从上下文获取用户信息
        userID, exists := ctx.Get("user_id")
        if !exists {
            ctx.JSON(401, gin.H{"error": "未认证"})
            ctx.Abort()
            return
        }
        
        // 检查权限（使用 Casbin）
        enforcer := abe.MustGetEnforcer(ctx)
        allowed, err := enforcer.Enforce(userID, "users", permission)
        if err != nil || !allowed {
            ctx.JSON(403, gin.H{"error": "权限不足"})
            ctx.Abort()
            return
        }
        
        ctx.Next()
    }
}

// 在控制器中使用
func (uc *UserController) RegisterRoutes(router gin.IRouter, mg *MiddlewareManager, engine *Engine) {
    adminGroup := router.Group("/admin/users", permissionCheck("admin"))
    {
        adminGroup.DELETE("/:id", uc.adminDeleteUser)
        adminGroup.POST("/batch-delete", uc.batchDeleteUsers)
    }
}
```

## 错误处理和响应

### 统一响应格式
```go
// 标准响应结构
type APIResponse struct {
    Code    int         `json:"code"`
    Message string      `json:"message"`
    Data    interface{} `json:"data,omitempty"`
}

// 成功响应
func Success(ctx *gin.Context, data interface{}) {
    ctx.JSON(200, APIResponse{
        Code:    0,
        Message: "success",
        Data:    data,
    })
}

// 错误响应
func Error(ctx *gin.Context, code int, message string) {
    ctx.JSON(code, APIResponse{
        Code:    code,
        Message: message,
        Data:    nil,
    })
}

// 在控制器中使用
func (uc *UserController) createUser(ctx *gin.Context) {
    var req CreateUserRequest
    if err := ctx.ShouldBindJSON(&req); err != nil {
        Error(ctx, 400, "参数验证失败: "+err.Error())
        return
    }
    
    // 业务逻辑...
    if err != nil {
        uc.logger.Error("创建用户失败", "error", err)
        Error(ctx, 500, "服务器内部错误")
        return
    }
    
    Success(ctx, userResponse)
}
```

## 最佳实践

### 1. 控制器职责分离
```go
// ✅ 好的做法：控制器只负责路由注册和参数处理
type UserController struct {
    usecase *UserUseCase  // 业务逻辑委托给 UseCase
}

func (uc *UserController) createUser(ctx *gin.Context) {
    var req CreateUserRequest
    if err := ctx.ShouldBindJSON(&req); err != nil {
        ctx.JSON(400, gin.H{"error": err.Error()})
        return
    }
    
    // 委托给 UseCase 处理业务逻辑
    result, err := uc.usecase.CreateUser(ctx, req)
    if err != nil {
        ctx.JSON(500, gin.H{"error": err.Error()})
        return
    }
    
    ctx.JSON(201, result)
}

// ❌ 避免：在控制器中实现复杂业务逻辑
func (uc *UserController) createUser(ctx *gin.Context) {
    // 大量业务逻辑代码...
    // 数据库操作...
    // 复杂的验证逻辑...
    // 发送邮件通知...
}
```

### 2. 依赖注入
```go
// 推荐：通过构造函数注入依赖
func NewUserController(
    db *gorm.DB,
    logger *slog.Logger,
    validator *abe.Validator,
    eventBus abe.EventBus,
) *UserController {
    return &UserController{
        db:        db,
        logger:    logger,
        validator: validator,
        eventBus:  eventBus,
    }
}

// 在主程序中注入依赖
func main() {
    engine := abe.NewEngine()
    
    userController := NewUserController(
        engine.DB(),
        engine.Logger(),
        engine.Validator(),
        engine.EventBus(),
    )
    
    engine.AddController(abe.Provider(userController))
    engine.Run()
}
```

### 3. 参数验证
```go
// 使用结构体标签进行验证
type UpdateUserRequest struct {
    Name  string `json:"name" binding:"omitempty,min=2,max=50"`
    Email string `json:"email" binding:"omitempty,email"`
    Age   *int   `json:"age" binding:"omitempty,min=1,max=150"`
}

// 自定义验证器
func (uc *UserController) updateUser(ctx *gin.Context) {
    var req UpdateUserRequest
    if err := ctx.ShouldBindJSON(&req); err != nil {
        // 参数验证失败
        validationErrors := err.(validator.ValidationErrors)
        translatedErrors := translateValidationErrors(validationErrors)
        ctx.JSON(400, gin.H{"errors": translatedErrors})
        return
    }
    
    // 业务逻辑...
}
```

## 测试控制器

### 单元测试示例
```go
func TestUserController_CreateUser(t *testing.T) {
    // 准备测试数据
    mockDB := &MockDB{}
    mockLogger := &MockLogger{}
    
    controller := NewUserController(mockDB, mockLogger, nil)
    
    // 创建测试请求
    req := httptest.NewRequest("POST", "/users", 
        strings.NewReader(`{"name":"test","email":"test@example.com"}`))
    req.Header.Set("Content-Type", "application/json")
    
    w := httptest.NewRecorder()
    ctx, _ := gin.CreateTestContext(w)
    ctx.Request = req
    
    // 执行测试
    controller.createUser(ctx)
    
    // 验证结果
    assert.Equal(t, 201, w.Code)
    // 更多断言...
}
```