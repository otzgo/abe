# 插件机制详解

## 插件核心概念

ABE 框架提供了灵活的插件机制，允许开发者通过插件扩展框架功能，实现功能的热插拔和模块化管理。

## 插件接口定义

### 基础插件接口
```go
type Plugin interface {
    // Name 返回插件名称
    Name() string
    
    // Version 返回插件版本
    Version() string
    
    // Init 在插件注册时调用，用于注入全局 Engine 并完成基础初始化
    // 插件可在此阶段注册控制器、中间件、事件订阅、定时任务等
    Init(engine *Engine) error
}
```

### 生命周期钩子接口
```go
// 前置挂载钩子 - 在挂载控制器与全局中间件前触发
type BeforeMountHook interface {
    OnBeforeMount(engine *Engine) error
}

// 后置挂载钩子 - 在挂载控制器完成后触发
type AfterMountHook interface {
    OnAfterMount(engine *Engine) error
}

// 服务启动前钩子 - 在 HTTP Server 初始化完成、启动前触发
type BeforeServerStartHook interface {
    OnBeforeServerStart(engine *Engine) error
}

// 关闭钩子 - 在应用优雅退出开始阶段触发
type ShutdownHook interface {
    OnShutdown(engine *Engine) error
}

// 引擎版本要求（可选）
type EngineVersionRequirement interface {
    MinEngineVersion() string
}
```

## 插件开发示例

### 完整插件实现
```go
package myplugin

import (
    "fmt"
    "github.com/gin-gonic/gin"
    "log/slog"
    "github.com/otzgo/abe"
)

// MyPlugin 示例插件
type MyPlugin struct {
    name    string
    version string
    engine  *abe.Engine
}

// NewMyPlugin 创建新的插件实例
func NewMyPlugin() *MyPlugin {
    return &MyPlugin{
        name:    "my-plugin",
        version: "1.0.0",
    }
}

// 实现 Plugin 接口
func (p *MyPlugin) Name() string {
    return p.name
}

func (p *MyPlugin) Version() string {
    return p.version
}

func (p *MyPlugin) Init(engine *abe.Engine) error {
    slog.Info("插件初始化", "name", p.name, "version", p.version)
    
    // 注入引擎引用
    p.engine = engine
    
    // 注册控制器
    engine.AddController(abe.Provider(&MyController{}))
    
    // 注册中间件
    engine.MiddlewareManager().Register("my-middleware", p.myMiddleware())
    
    // 注册定时任务
    engine.Cron().AddFunc("@every 1h", p.scheduledTask)
    
    // 订阅事件
    engine.EventBus().Subscribe("user.created", p.handleUserCreated)
    
    return nil
}

// 实现 BeforeMountHook 钩子
func (p *MyPlugin) OnBeforeMount(engine *abe.Engine) error {
    slog.Info("插件前置挂载钩子执行", "name", p.name)
    
    // 可以在此处添加全局中间件
    engine.MiddlewareManager().AddGlobal(func(c *gin.Context) {
        slog.Info("插件全局中间件执行")
        c.Next()
    })
    
    return nil
}

// 实现 BeforeServerStartHook 钩子
func (p *MyPlugin) OnBeforeServerStart(engine *abe.Engine) error {
    slog.Info("插件服务启动前钩子执行", "name", p.name)
    
    // 可以在此处进行预加载操作
    p.preloadData()
    
    return nil
}

// 实现 ShutdownHook 钩子
func (p *MyPlugin) OnShutdown(engine *abe.Engine) error {
    slog.Info("插件关闭钩子执行", "name", p.name)
    
    // 清理资源
    p.cleanup()
    
    return nil
}

// 实现 EngineVersionRequirement 接口（可选）
func (p *MyPlugin) MinEngineVersion() string {
    return "1.0.0"
}

// 自定义中间件
func (p *MyPlugin) myMiddleware() gin.HandlerFunc {
    return func(ctx *gin.Context) {
        // 插件特定的中间件逻辑
        ctx.Header("X-My-Plugin-Version", p.version)
        ctx.Next()
    }
}

// 定时任务
func (p *MyPlugin) scheduledTask() {
    slog.Info("执行插件定时任务")
    // 定时任务逻辑
}

// 事件处理器
func (p *MyPlugin) handleUserCreated(event interface{}) error {
    slog.Info("处理用户创建事件", "event", event)
    // 事件处理逻辑
    return nil
}

// 预加载数据
func (p *MyPlugin) preloadData() {
    slog.Info("预加载插件数据")
    // 预加载逻辑
}

// 清理资源
func (p *MyPlugin) cleanup() {
    slog.Info("清理插件资源")
    // 清理逻辑
}

// MyController 示例控制器
type MyController struct{}

func (c *MyController) RegisterRoutes(router gin.IRouter, mg *abe.MiddlewareManager, engine *abe.Engine) {
    router.GET("/api/my-plugin/hello", func(ctx *gin.Context) {
        ctx.JSON(200, gin.H{"message": "Hello from my plugin!"})
    })
}
```

## 插件注册和管理

### 注册插件
```go
func main() {
    app := abe.NewEngine()
    
    // 注册插件
    myPlugin := NewMyPlugin()
    if err := app.Plugins().Register(myPlugin); err != nil {
        slog.Error("插件注册失败", "error", err)
        panic(err)
    }
    
    // 启动应用
    app.Run()
}
```

### 插件管理器操作
```go
// 获取插件管理器
pluginManager := engine.Plugins()

// 列出所有已注册插件
plugins := pluginManager.List()
for _, plugin := range plugins {
    fmt.Printf("插件: %s, 版本: %s\n", plugin.Name(), plugin.Version())
}

// 按名称或别名查找插件
if plugin, exists := pluginManager.LookupByAliasOrName("my-plugin"); exists {
    fmt.Printf("找到插件: %s\n", plugin.Name())
}

// 检查插件是否存在
if pluginManager.Exists("my-plugin") {
    fmt.Println("插件已注册")
}

// 获取插件信息
info := pluginManager.Info("my-plugin")
fmt.Printf("插件信息: %+v\n", info)
```

## 插件配置管理

### 配置文件示例
```yaml
# config.yaml
plugins:
  enabled: true                    # 是否启用插件系统，默认为 true
  conflict_mode: alias            # 冲突处理模式：alias（生成别名）或 error（拒绝注册）
  hook_failure_mode: warn         # 钩子执行失败处理模式：warn（警告）或 error（终止）
  compat:
    strict: false                 # 版本兼容性检查是否严格
  enable:
    "github.com/example/myplugin.MyPlugin": true  # 按唯一键启用特定插件
  aliases:
    "github.com/example/myplugin.MyPlugin": "my-plugin-alias"  # 为插件指定别名
```

### 运行时配置
```go
// 动态启用/禁用插件
pluginManager.Enable("my-plugin", true)   // 启用
pluginManager.Enable("other-plugin", false) // 禁用

// 设置配置
pluginManager.SetConfig(abe.PluginConfig{
    Enabled:         true,
    ConflictMode:    abe.AliasConflictMode,
    HookFailureMode: abe.WarnHookFailureMode,
})
```

## 高级插件开发

### 插件间通信
```go
// 插件A发布事件
type PluginA struct {
    eventBus abe.EventBus
}

func (p *PluginA) Init(engine *abe.Engine) error {
    p.eventBus = engine.EventBus()
    return nil
}

func (p *PluginA) DoSomething() {
    // 发布事件供其他插件消费
    p.eventBus.Publish("plugin-a.event", map[string]interface{}{
        "data": "some data",
        "timestamp": time.Now(),
    })
}

// 插件B订阅事件
type PluginB struct{}

func (p *PluginB) Init(engine *abe.Engine) error {
    // 订阅插件A发布的事件
    engine.EventBus().Subscribe("plugin-a.event", p.handlePluginAEvent)
    return nil
}

func (p *PluginB) handlePluginAEvent(event interface{}) error {
    eventData := event.(map[string]interface{})
    slog.Info("插件B收到事件", "data", eventData["data"])
    return nil
}
```

### 插件资源共享
```go
// 插件提供共享服务
type SharedServicePlugin struct {
    sharedService *SharedService
}

func (p *SharedServicePlugin) Init(engine *abe.Engine) error {
    p.sharedService = NewSharedService()
    
    // 将服务注册到全局容器供其他插件使用
    do.Provide(engine.Injector(), func(i *do.RootScope) (*SharedService, error) {
        return p.sharedService, nil
    })
    
    return nil
}

// 其他插件使用共享服务
type ConsumerPlugin struct{}

func (p *ConsumerPlugin) Init(engine *abe.Engine) error {
    // 从容器获取共享服务
    sharedService := do.MustInvoke[*SharedService](engine.Injector())
    // 使用共享服务...
    return nil
}
```

### 条件插件加载
```go
type ConditionalPlugin struct {
    name string
}

func NewConditionalPlugin() *ConditionalPlugin {
    return &ConditionalPlugin{name: "conditional-plugin"}
}

func (p *ConditionalPlugin) Name() string { return p.name }
func (p *ConditionalPlugin) Version() string { return "1.0.0" }

func (p *ConditionalPlugin) Init(engine *abe.Engine) error {
    config := engine.Config()
    
    // 根据配置决定是否激活功能
    if config.GetBool("plugins.conditional.enabled") {
        // 激活插件功能
        engine.AddController(abe.Provider(&ConditionalController{}))
        slog.Info("条件插件已激活")
    } else {
        slog.Info("条件插件未激活")
        // 可以选择不注册任何组件
    }
    
    return nil
}
```

## 插件测试

### 插件单元测试
```go
func TestMyPlugin_Init(t *testing.T) {
    // 创建模拟引擎
    mockEngine := &MockEngine{
        controllers: []abe.ControllerProvider{},
        middlewares: make(map[string]gin.HandlerFunc),
    }
    
    // 创建插件实例
    plugin := NewMyPlugin()
    
    // 测试初始化
    err := plugin.Init(mockEngine)
    assert.NoError(t, err)
    
    // 验证控制器是否正确注册
    assert.Len(t, mockEngine.controllers, 1)
    
    // 验证中间件是否正确注册
    assert.Contains(t, mockEngine.middlewares, "my-middleware")
}

func TestMyPlugin_Hooks(t *testing.T) {
    plugin := NewMyPlugin()
    mockEngine := &MockEngine{}
    
    // 测试各个钩子
    err := plugin.OnBeforeMount(mockEngine)
    assert.NoError(t, err)
    
    err = plugin.OnBeforeServerStart(mockEngine)
    assert.NoError(t, err)
    
    err = plugin.OnShutdown(mockEngine)
    assert.NoError(t, err)
}
```

### 集成测试
```go
func TestPluginIntegration(t *testing.T) {
    // 创建真实引擎进行集成测试
    engine := abe.NewEngine()
    
    // 注册插件
    plugin := NewMyPlugin()
    err := engine.Plugins().Register(plugin)
    assert.NoError(t, err)
    
    // 启动应用（在测试环境中可能需要特殊处理）
    // engine.Run() // 注意：这会阻塞，需要在 goroutine 中运行
    
    // 测试插件功能
    // ...
}
```

## 最佳实践

### 1. 插件命名规范
```go
// ✅ 好的命名
type UserManagementPlugin struct{}    // 清晰描述功能
type LoggingEnhancementPlugin struct{} // 明确表示增强功能

// ❌ 避免的命名
type Plugin1 struct{}                 // 太模糊
type Utils struct{}                   // 不符合插件语义
```

### 2. 错误处理
```go
func (p *MyPlugin) Init(engine *abe.Engine) error {
    // 详细的错误信息
    if err := p.setupDatabase(); err != nil {
        return fmt.Errorf("插件初始化失败 - 数据库设置错误: %w", err)
    }
    
    if err := p.loadConfiguration(); err != nil {
        return fmt.Errorf("插件初始化失败 - 配置加载错误: %w", err)
    }
    
    return nil
}
```

### 3. 资源管理
```go
type ResourcePlugin struct {
    db    *sql.DB
    cache Cache
    file  *os.File
}

func (p *ResourcePlugin) OnShutdown(engine *abe.Engine) error {
    var errs []error
    
    // 按相反顺序清理资源
    if p.file != nil {
        if err := p.file.Close(); err != nil {
            errs = append(errs, fmt.Errorf("关闭文件失败: %w", err))
        }
    }
    
    if p.cache != nil {
        if err := p.cache.Close(); err != nil {
            errs = append(errs, fmt.Errorf("关闭缓存失败: %w", err))
        }
    }
    
    if p.db != nil {
        if err := p.db.Close(); err != nil {
            errs = append(errs, fmt.Errorf("关闭数据库失败: %w", err))
        }
    }
    
    if len(errs) > 0 {
        return fmt.Errorf("资源清理失败: %v", errs)
    }
    
    return nil
}
```

### 4. 配置管理
```go
type ConfigurablePlugin struct {
    config PluginConfig
}

type PluginConfig struct {
    Enabled     bool   `mapstructure:"enabled"`
    APIKey      string `mapstructure:"api_key"`
    Timeout     int    `mapstructure:"timeout"`
    RetryCount  int    `mapstructure:"retry_count"`
}

func (p *ConfigurablePlugin) Init(engine *abe.Engine) error {
    // 从主配置中读取插件配置
    if err := engine.Config().UnmarshalKey("plugins.myplugin", &p.config); err != nil {
        return fmt.Errorf("读取插件配置失败: %w", err)
    }
    
    // 验证必要配置
    if p.config.Enabled && p.config.APIKey == "" {
        return fmt.Errorf("启用插件时必须提供 API 密钥")
    }
    
    return nil
}
```

## 故障排除

### 常见问题

1. **插件注册失败**
   - 检查插件是否实现了必需的接口
   - 确认插件名称的唯一性
   - 验证版本兼容性要求

2. **钩子执行异常**
   - 查看日志了解具体错误信息
   - 检查钩子实现是否正确返回错误
   - 确认引擎实例在钩子执行时仍然有效

3. **资源泄露**
   - 确保在 `OnShutdown` 钩子中正确释放所有资源
   - 避免在插件中创建无法清理的全局状态
   - 使用 defer 语句确保资源及时释放

4. **性能问题**
   - 避免在钩子中执行耗时操作
   - 合理使用协程处理后台任务
   - 考虑缓存频繁访问的数据