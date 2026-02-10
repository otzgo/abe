# 引擎核心功能详解

## Engine 实例管理

### 创建引擎实例
```go
// 基本创建
engine := abe.NewEngine()

// 带配置选项的创建
engine := abe.NewEngine()
engine.Run(
    abe.WithBasePath("/api/v1"),  // 设置基础路由路径
)
```

### 核心服务获取器
Engine 提供统一的服务访问接口：

```go
// 配置管理
config := engine.Config()        // *viper.Viper

// 路由管理  
router := engine.Router()        // *gin.Engine

// 数据库访问
db := engine.DB()                // *gorm.DB

// 定时任务
cron := engine.Cron()            // *cron.Cron

// 事件总线
eventBus := engine.EventBus()    // EventBus

// 协程池
pool := engine.Pool()            // *ants.Pool

// 权限控制
enforcer := engine.Enforcer()    // *casbin.Enforcer

// 日志系统
logger := engine.Logger()        // *slog.Logger

// 中间件管理
middlewareMgr := engine.MiddlewareManager()  // *MiddlewareManager

// 验证器
validator := engine.Validator()  // *Validator

// 依赖注入容器
injector := engine.Injector()    // *do.RootScope
```

## 应用生命周期管理

### 启动应用
```go
// 基本启动
engine.Run()

// 带配置启动
engine.Run(
    abe.WithBasePath("/api/v1"),           // API 基础路径
    // 其他选项...
)

// 启动流程：
// 1. 执行运行选项配置
// 2. 包装依赖注入容器
// 3. 触发插件前置挂载钩子
// 4. 挂载控制器路由
// 5. 触发插件后置挂载钩子
// 6. 初始化 HTTP 服务器
// 7. 触发插件服务启动前钩子
// 8. 启动 HTTP 服务器（goroutine）
// 9. 等待中断信号
// 10. 执行优雅关闭
```

### 优雅关闭
Engine 自动处理应用的优雅关闭：
- HTTP 服务器平滑关闭
- 定时任务停止
- 事件总线关闭
- 协程池资源释放
- 插件关闭钩子执行

## 控制器管理

### 注册控制器
```go
// 注册单个控制器
engine.AddController(abe.Provider(&UserController{}))

// 批量注册控制器
engine.AddController(
    abe.Provider(&UserController{}),
    abe.Provider(&OrderController{}),
    abe.Provider(&ProductController{}),
)

// 控制器必须实现 Controller 接口
type Controller interface {
    RegisterRoutes(router gin.IRouter, mg *MiddlewareManager, engine *Engine)
}
```

### 错误处理器
```go
// 添加自定义错误处理器
engine.AddErrorHandler(func(err error) (*abe.ErrorResponse, int) {
    // 自定义错误处理逻辑
    return &abe.ErrorResponse{
        Code: 5000,
        Msg:  "自定义错误消息",
        Data: gin.H{"error": err.Error()},
    }, 500
})
```

## 协程池管理

### 创建函数任务协程池
```go
// 创建协程池
pool, err := engine.NewPoolWithFunc(func(data interface{}) {
    // 处理任务逻辑
    fmt.Printf("处理数据: %v\n", data)
}, 10) // 池大小为10

if err != nil {
    log.Fatal(err)
}

// 提交任务
pool.Invoke("任务数据1")
pool.Invoke("任务数据2")
```

## 配置管理

### 常用配置项
```yaml
# server 配置
server:
  address: ":8080"              # 服务器监听地址
  shutdown_timeout: "5s"        # 优雅关闭超时时间

# swagger 配置  
swagger:
  enabled: true                 # 是否启用 Swagger
  url: ""                       # 自定义 Swagger URL
  instance: "swagger"           # Swagger 实例名

# 数据库配置
database:
  host: "localhost"
  port: 5432
  user: "postgres"
  password: "password"
  dbname: "myapp"

# 日志配置
log:
  level: "info"                 # 日志级别
  format: "json"                # 输出格式
```

## 最佳实践

### 1. 引擎初始化顺序
```go
func main() {
    // 1. 创建引擎
    engine := abe.NewEngine()
    
    // 2. 配置中间件（可选）
    engine.MiddlewareManager().RegisterGlobal(corsMiddleware())
    
    // 3. 注册控制器
    engine.AddController(
        abe.Provider(NewUserController(engine.DB(), engine)),
        abe.Provider(NewOrderController(engine.DB(), engine)),
    )
    
    // 4. 注册插件（可选）
    plugin := NewMyPlugin()
    if err := engine.Plugins().Register(plugin); err != nil {
        log.Fatal("插件注册失败:", err)
    }
    
    // 5. 启动应用
    engine.Run(abe.WithBasePath("/api/v1"))
}
```

### 2. 服务依赖注入
```go
// 推荐：在控制器构造函数中注入所需服务
func NewUserController(db *gorm.DB, logger *slog.Logger) *UserController {
    return &UserController{
        db:     db,
        logger: logger,
    }
}

// 在主函数中获取服务并注入
userController := NewUserController(engine.DB(), engine.Logger())
engine.AddController(abe.Provider(userController))
```

### 3. 配置读取
```go
// 读取配置值
port := engine.Config().GetString("server.port")
debug := engine.Config().GetBool("debug")
timeout := engine.Config().GetDuration("server.timeout")

// 设置默认值
addr := engine.Config().GetString("server.address")
if addr == "" {
    addr = ":8080"  // 默认端口
}
```

## 故障排除

### 常见问题

1. **控制器路由未注册**
   - 确保控制器实现了 `Controller` 接口
   - 检查是否调用了 `AddController` 方法
   - 查看日志确认注册过程

2. **服务获取为空**
   - 确认引擎已正确初始化
   - 检查相关服务是否已配置
   - 查看启动日志中的服务初始化状态

3. **应用无法启动**
   - 检查端口是否被占用
   - 确认配置文件格式正确
   - 查看错误日志定位具体问题