# 依赖注入容器使用指南

## 依赖注入核心概念

ABE 框架采用 `github.com/samber/do/v2` 作为依赖注入(DI)容器，提供了全局和请求级两种作用域的依赖注入能力。

## 全局依赖注入

### 获取全局容器
```go
engine := abe.NewEngine()
injector := engine.Injector()

// 从容器中获取服务
config := do.MustInvoke[*viper.Viper](injector)
logger := do.MustInvoke[*slog.Logger](injector)
db := do.MustInvoke[*gorm.DB](injector)
```

### 全局容器中预注册的服务
在应用启动时，以下核心服务会自动注册到全局容器中：

- `*viper.Viper` - 配置管理器
- `*slog.Logger` - 日志记录器
- `*gorm.DB` - 数据库引擎
- `EventBus` - 事件总线
- `*ants.Pool` - 协程池管理器
- `*casbin.Enforcer` - 权限策略管理器

### 注册全局服务
```go
// 注册单例服务
do.Provide(injector, func(i *do.RootScope) (*MyService, error) {
    return &MyService{}, nil
})

// 注册工厂服务（每次获取都是新实例）
do.ProvideValue(injector, &MySingletonService{})

// 条件注册
do.Provide(injector, func(i *do.RootScope) (*DatabaseService, error) {
    config := do.MustInvoke[*viper.Viper](i)
    if config.GetBool("database.enabled") {
        return NewDatabaseService(config)
    }
    return &MockDatabaseService{}, nil
})
```

## 请求级依赖注入

### 请求级容器特性
- 每个请求拥有独立的容器实例
- 请求结束后自动释放容器资源
- 支持注入请求特定的元信息

### 使用请求级容器
```go
// 在中间件或处理器中获取当前请求的容器
func myHandler(ctx *gin.Context) {
    injector := abe.Injector(ctx)
    
    // 从请求级容器获取服务
    db := do.MustInvoke[*gorm.DB](injector)
    requestMeta := do.MustInvoke[RequestMeta](injector)
    
    // 业务逻辑...
}
```

### 请求元信息
```go
type RequestMeta struct {
    RequestID   string    // 当前请求的唯一标识
    RequestTime time.Time // 请求开始时间
}

// 在处理器中使用
func (uc *UserController) getUser(ctx *gin.Context) {
    injector := abe.Injector(ctx)
    meta := do.MustInvoke[RequestMeta](injector)
    
    logger := do.MustInvoke[*slog.Logger](injector)
    logger.Info("处理用户请求",
        "request_id", meta.RequestID,
        "user_id", ctx.Param("id"),
    )
}
```

## UseCase 模式

### UseCase 接口定义
```go
// UseCase 定义了业务用例的通用接口
type UseCase[T any] interface {
    Handle(ctx *gin.Context) (T, error)
}
```

### UseCase 实现示例
```go
// 用户相关 UseCase
type GetUserUseCase struct {
    db *gorm.DB
}

func NewGetUserUseCase(db *gorm.DB) *GetUserUseCase {
    return &GetUserUseCase{db: db}
}

func (uc *GetUserUseCase) Handle(ctx *gin.Context) (*User, error) {
    userID := ctx.Param("id")
    var user User
    err := uc.db.First(&user, userID).Error
    return &user, err
}

// 创建用户 UseCase
type CreateUserUseCase struct {
    db    *gorm.DB
    event EventBus
}

func NewCreateUserUseCase(db *gorm.DB, event EventBus) *CreateUserUseCase {
    return &CreateUserUseCase{db: db, event: event}
}

func (uc *CreateUserUseCase) Handle(ctx *gin.Context) (*User, error) {
    var req CreateUserRequest
    if err := ctx.ShouldBindJSON(&req); err != nil {
        return nil, err
    }
    
    user := &User{
        Name:  req.Name,
        Email: req.Email,
    }
    
    if err := uc.db.Create(user).Error; err != nil {
        return nil, err
    }
    
    // 发布用户创建事件
    uc.event.Publish("user.created", UserCreatedEvent{UserID: user.ID})
    
    return user, nil
}
```

### 在控制器中使用 UseCase
```go
func (uc *UserController) getUser(ctx *gin.Context) {
    // 方式1：手动获取 UseCase
    injector := abe.Injector(ctx)
    getUserUC := do.MustInvoke[*GetUserUseCase](injector)
    user, err := getUserUC.Handle(ctx)
    
    // 方式2：使用 Invoke 辅助函数
    user, err := abe.Invoke[*GetUserUseCase, *User](ctx)
    
    if err != nil {
        ctx.JSON(404, gin.H{"error": "用户不存在"})
        return
    }
    
    ctx.JSON(200, user)
}
```

## 依赖注入最佳实践

### 1. 构造函数注入
```go
// ✅ 推荐：通过构造函数明确声明依赖
type UserService struct {
    db     *gorm.DB
    logger *slog.Logger
    cache  CacheService
}

func NewUserService(db *gorm.DB, logger *slog.Logger, cache CacheService) *UserService {
    return &UserService{
        db:     db,
        logger: logger,
        cache:  cache,
    }
}

// ❌ 避免：隐藏依赖或过多依赖
type BadService struct {
    engine *abe.Engine  // 过于宽泛的依赖
}

func NewBadService(engine *abe.Engine) *BadService {
    return &BadService{engine: engine}
}
```

### 2. 依赖生命周期管理
```go
// 单例服务（全局共享）
do.Provide(injector, func(i *do.RootScope) (*DatabaseService, error) {
    return NewDatabaseService()
})

// 请求级服务（每个请求独立实例）
do.Provide(abe.RequestScope, func(i do.Injector) (*RequestService, error) {
    return &RequestService{}, nil
})

// 临时服务（每次获取都创建新实例）
do.ProvideTransient(injector, func(i do.Injector) (*TempService, error) {
    return &TempService{}, nil
})
```

### 3. 接口抽象
```go
// 定义接口
type UserRepository interface {
    FindByID(id uint) (*User, error)
    Create(user *User) error
}

// 实现接口
type GormUserRepository struct {
    db *gorm.DB
}

func NewGormUserRepository(db *gorm.DB) UserRepository {
    return &GormUserRepository{db: db}
}

func (r *GormUserRepository) FindByID(id uint) (*User, error) {
    var user User
    err := r.db.First(&user, id).Error
    return &user, err
}

// 在 UseCase 中依赖接口而不是具体实现
type GetUserUseCase struct {
    repo UserRepository  // 依赖接口
}

func NewGetUserUseCase(repo UserRepository) *GetUserUseCase {
    return &GetUserUseCase{repo: repo}
}
```

### 4. 配置注入
```go
// 配置结构体
type DatabaseConfig struct {
    Host     string
    Port     int
    Username string
    Password string
    Database string
}

// 从配置创建服务
func NewDatabaseService(config *DatabaseConfig) (*gorm.DB, error) {
    dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s",
        config.Username, config.Password,
        config.Host, config.Port, config.Database)
    
    return gorm.Open(mysql.Open(dsn), &gorm.Config{})
}

// 注册配置和相关服务
func setupDependencies(injector *do.RootScope) {
    // 注册配置
    do.ProvideValue(injector, &DatabaseConfig{
        Host:     "localhost",
        Port:     3306,
        Username: "user",
        Password: "pass",
        Database: "myapp",
    })
    
    // 基于配置创建数据库服务
    do.Provide(injector, func(i *do.RootScope) (*gorm.DB, error) {
        config := do.MustInvoke[*DatabaseConfig](i)
        return NewDatabaseService(config)
    })
}
```

## 容器作用域管理

### 作用域层次结构
```
全局作用域 (Global Scope)
├── 请求作用域 1 (Request Scope 1)
├── 请求作用域 2 (Request Scope 2)
└── 请求作用域 N (Request Scope N)
```

### 服务可见性
```go
// 全局服务在所有作用域中可见
globalService := do.MustInvoke[*GlobalService](globalInjector)
requestService := do.MustInvoke[*GlobalService](requestInjector) // 可以获取

// 请求级服务只能在请求作用域中获取
requestOnlyService := do.MustInvoke[*RequestService](requestInjector) // 正常
// globalOnlyService := do.MustInvoke[*RequestService](globalInjector) // 错误！
```

### 资源清理
```go
// 实现清理接口的服务会在容器关闭时自动清理
type CleanupService struct {
    conn *sql.DB
}

func (s *CleanupService) Close() error {
    return s.conn.Close()
}

// 注册带清理功能的服务
do.Provide(injector, func(i *do.RootScope) (*CleanupService, error) {
    conn, err := sql.Open("mysql", "connection_string")
    if err != nil {
        return nil, err
    }
    return &CleanupService{conn: conn}, nil
})

// 容器关闭时会自动调用 Close() 方法
```

## 故障排除

### 常见问题解决

1. **依赖循环引用**
```go
// ❌ 错误：A 依赖 B，B 依赖 A
type ServiceA struct { b *ServiceB }
type ServiceB struct { a *ServiceA }

// ✅ 解决：使用接口打破循环
type ServiceBInterface interface { /* ... */ }
type ServiceA struct { b ServiceBInterface }
type ServiceB struct { /* 不直接依赖 A */ }
```

2. **服务未注册**
```go
// 检查服务是否已注册
if do.CanInvoke[*MyService](injector) {
    service := do.MustInvoke[*MyService](injector)
    // 使用服务
} else {
    // 服务未注册，需要先注册
    do.Provide(injector, func(i do.Injector) (*MyService, error) {
        return &MyService{}, nil
    })
}
```

3. **作用域混淆**
```go
// 确保在正确的上下文中获取容器
func handler(ctx *gin.Context) {
    // ✅ 正确：获取请求级容器
    injector := abe.Injector(ctx)
    
    // ❌ 错误：试图在请求外获取请求级服务
    // globalInjector := engine.Injector()
    // requestService := do.MustInvoke[*RequestService](globalInjector) // 失败
}
```

## 性能优化

### 1. 避免频繁的依赖解析
```go
// ✅ 好的做法：缓存解析结果
type Handler struct {
    userService UserService
    orderService OrderService
}

func NewHandler(injector do.Injector) *Handler {
    return &Handler{
        userService: do.MustInvoke[UserService](injector),
        orderService: do.MustInvoke[OrderService](injector),
    }
}

// ❌ 避免：每次都解析依赖
func badHandler(ctx *gin.Context) {
    injector := abe.Injector(ctx)
    userService := do.MustInvoke[UserService](injector) // 每次都解析
    // ...
}
```

### 2. 合理使用作用域
```go
// 对于昂贵的资源使用全局单例
do.Provide(injector, func(i *do.RootScope) (*ExpensiveService, error) {
    return NewExpensiveService() // 昂贵的初始化
})

// 对于轻量级、请求相关的服务使用请求作用域
do.Provide(abe.RequestScope, func(i do.Injector) (*RequestTracker, error) {
    return &RequestTracker{}, nil
})
```