# 日志系统使用指南

## 日志系统概述

ABE 框架集成了结构化日志系统，基于 `log/slog` 包实现，支持多种输出格式和灵活的配置选项。日志系统提供统一的日志记录接口，便于应用程序的调试和监控。

## 配置选项

### 日志配置参数

在 `config.yaml` 中配置日志系统：

```yaml
logger:
  level: "info"           # 日志级别: debug, info, warn, error
  format: "json"          # 输出格式: text, json
  type: "console"         # 输出类型: console, file
  file:                   # 文件日志配置（当 type 为 file 时有效）
    path: "./logs/app.log" # 日志文件路径
    max_size: 100         # 单个日志文件最大尺寸（MB）
    max_backups: 10       # 保留的旧日志文件最大数量
    max_age: 30           # 保留旧日志文件的最大天数
    compress: true        # 是否压缩旧日志文件
```

### 日志级别说明

- **DEBUG**: 调试信息，最详细的日志级别
- **INFO**: 一般信息，记录应用程序正常运行状态
- **WARN**: 警告信息，潜在问题但不影响正常运行
- **ERROR**: 错误信息，发生了错误但应用程序仍可继续运行
- **CRITICAL**: 严重错误，可能导致应用程序无法正常工作

## 基本使用方法

### 获取日志记录器

```go
func main() {
    engine := abe.NewEngine()
    
    // 获取全局日志记录器
    logger := engine.Logger()
    
    // 在应用程序中使用
    logger.Info("应用启动", "version", "1.0.0", "port", 8080)
    
    engine.Run()
}
```

### 在控制器中使用日志

```go
type UserController struct {
    logger *slog.Logger
    db     *gorm.DB
}

func NewUserController(logger *slog.Logger, db *gorm.DB) *UserController {
    return &UserController{
        logger: logger,
        db:     db,
    }
}

func (uc *UserController) createUser(ctx *gin.Context) {
    var req CreateUserRequest
    if err := ctx.ShouldBindJSON(&req); err != nil {
        uc.logger.Error("参数绑定失败", 
            "error", err.Error(),
            "request_id", getRequestID(ctx),
        )
        ctx.JSON(400, gin.H{"error": "参数错误"})
        return
    }
    
    uc.logger.Info("开始创建用户",
        "username", req.Username,
        "email", req.Email,
        "request_id", getRequestID(ctx),
    )
    
    // 业务逻辑...
    
    uc.logger.Info("用户创建成功",
        "user_id", user.ID,
        "username", user.Username,
        "request_id", getRequestID(ctx),
    )
}
```

## 结构化日志记录

### 基本日志记录

```go
logger := engine.Logger()

// 基本信息日志
logger.Info("用户登录", 
    "user_id", 123,
    "ip", "192.168.1.100",
    "user_agent", ctx.Request.UserAgent(),
)

// 错误日志
logger.Error("数据库连接失败",
    "error", err.Error(),
    "host", dbHost,
    "port", dbPort,
    "attempt", retryCount,
)

// 警告日志
logger.Warn("内存使用率过高",
    "usage_percent", 85.5,
    "threshold", 80.0,
    "available_memory", "2GB",
)
```

### 复杂数据结构日志

```go
// 记录复杂对象
user := User{
    ID:       123,
    Username: "john_doe",
    Email:    "john@example.com",
    Profile: Profile{
        FirstName: "John",
        LastName:  "Doe",
        Age:       30,
    },
}

logger.Info("用户信息",
    "user", user,
    "operation", "user_created",
    "timestamp", time.Now().Unix(),
)

// 记录数组和切片
permissions := []string{"read", "write", "delete"}
logger.Info("用户权限",
    "user_id", user.ID,
    "permissions", permissions,
    "count", len(permissions),
)
```

## 日志上下文管理

### 请求ID跟踪

```go
// 从上下文中获取请求ID
func getRequestID(ctx *gin.Context) string {
    if requestID, exists := ctx.Get("request_id"); exists {
        if id, ok := requestID.(string); ok {
            return id
        }
    }
    return ""
}

// 在中间件中设置请求ID
func requestIDMiddleware() gin.HandlerFunc {
    return func(ctx *gin.Context) {
        requestID := uuid.New().String()
        ctx.Set("request_id", requestID)
        ctx.Header("X-Request-ID", requestID)
        ctx.Next()
    }
}

// 在日志中使用请求ID
func logWithContext(ctx *gin.Context, logger *slog.Logger) *slog.Logger {
    requestID := getRequestID(ctx)
    if requestID != "" {
        return logger.With("request_id", requestID)
    }
    return logger
}
```

### 用户上下文日志

```go
func userContextLogger(ctx *gin.Context, logger *slog.Logger) *slog.Logger {
    // 获取用户信息
    userID := getUserIDFromContext(ctx)
    username := getUsernameFromContext(ctx)
    
    return logger.With(
        "user_id", userID,
        "username", username,
        "request_id", getRequestID(ctx),
    )
}

// 使用示例
func (uc *UserController) updateUser(ctx *gin.Context) {
    userLogger := userContextLogger(ctx, uc.logger)
    
    userLogger.Info("开始更新用户信息")
    
    // 业务逻辑...
    
    userLogger.Info("用户信息更新完成", 
        "updated_fields", []string{"email", "phone"},
    )
}
```

## 不同场景的日志使用

### 1. 数据库操作日志

```go
type UserService struct {
    db     *gorm.DB
    logger *slog.Logger
}

func (s *UserService) FindUserByID(id uint) (*User, error) {
    logger := s.logger.With("operation", "find_user", "user_id", id)
    
    logger.Info("开始查询用户")
    
    var user User
    start := time.Now()
    
    err := s.db.First(&user, id).Error
    duration := time.Since(start)
    
    if err != nil {
        if errors.Is(err, gorm.ErrRecordNotFound) {
            logger.Warn("用户不存在", "duration_ms", duration.Milliseconds())
            return nil, err
        }
        logger.Error("查询用户失败", 
            "error", err.Error(),
            "duration_ms", duration.Milliseconds(),
        )
        return nil, err
    }
    
    logger.Info("用户查询成功", 
        "duration_ms", duration.Milliseconds(),
        "user_name", user.Username,
    )
    
    return &user, nil
}
```

### 2. API 请求日志

```go
func apiRequestLogger() gin.HandlerFunc {
    return func(ctx *gin.Context) {
        logger := abe.MustGetLogger(ctx)
        
        // 请求开始信息
        logger.Info("API请求开始",
            "method", ctx.Request.Method,
            "path", ctx.Request.URL.Path,
            "client_ip", ctx.ClientIP(),
            "user_agent", ctx.Request.UserAgent(),
            "request_id", getRequestID(ctx),
        )
        
        start := time.Now()
        ctx.Next()
        duration := time.Since(start)
        
        // 请求结束信息
        logger.Info("API请求完成",
            "status", ctx.Writer.Status(),
            "duration_ms", duration.Milliseconds(),
            "response_size", ctx.Writer.Size(),
            "request_id", getRequestID(ctx),
        )
    }
}
```

### 3. 业务逻辑日志

```go
func (s *OrderService) ProcessOrder(order *Order) error {
    logger := s.logger.With(
        "order_id", order.ID,
        "customer_id", order.CustomerID,
        "amount", order.Amount,
    )
    
    logger.Info("开始处理订单")
    
    // 验证订单
    if err := s.validateOrder(order); err != nil {
        logger.Error("订单验证失败", "error", err.Error())
        return fmt.Errorf("订单验证失败: %w", err)
    }
    
    logger.Info("订单验证通过")
    
    // 处理支付
    paymentResult, err := s.processPayment(order)
    if err != nil {
        logger.Error("支付处理失败", 
            "error", err.Error(),
            "payment_method", order.PaymentMethod,
        )
        return fmt.Errorf("支付处理失败: %w", err)
    }
    
    logger.Info("支付处理成功", 
        "transaction_id", paymentResult.TransactionID,
        "payment_status", paymentResult.Status,
    )
    
    // 更新订单状态
    order.Status = "completed"
    if err := s.db.Save(order).Error; err != nil {
        logger.Error("更新订单状态失败", "error", err.Error())
        return fmt.Errorf("更新订单状态失败: %w", err)
    }
    
    logger.Info("订单处理完成")
    return nil
}
```

## 日志格式化和输出

### 自定义日志格式

```go
// JSON 格式日志处理器
func newJSONHandler(w io.Writer, opts *slog.HandlerOptions) slog.Handler {
    return slog.NewJSONHandler(w, opts)
}

// 文本格式日志处理器
func newTextHandler(w io.Writer, opts *slog.HandlerOptions) slog.Handler {
    return slog.NewTextHandler(w, opts)
}

// 自定义格式化
func customLogFormatter() slog.Handler {
    return slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
        AddSource: true, // 添加源代码位置
        Level:     slog.LevelInfo,
        ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
            // 自定义属性格式化
            if a.Key == slog.TimeKey {
                // 自定义时间格式
                return slog.String(a.Key, a.Value.Time().Format("2006-01-02 15:04:05"))
            }
            if a.Key == slog.LevelKey {
                // 自定义级别显示
                level := a.Value.Any().(slog.Level)
                return slog.String(a.Key, level.String())
            }
            return a
        },
    })
}
```

### 多输出日志

```go
func setupMultiOutputLogger() *slog.Logger {
    // 控制台输出
    consoleHandler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
        Level: slog.LevelInfo,
    })
    
    // 文件输出
    file, err := os.OpenFile("./logs/app.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
    if err != nil {
        panic(fmt.Sprintf("无法创建日志文件: %v", err))
    }
    
    fileHandler := slog.NewJSONHandler(file, &slog.HandlerOptions{
        Level: slog.LevelDebug,
    })
    
    // 多处理器
    multiHandler := slog.NewTextHandler(io.MultiWriter(os.Stdout, file), nil)
    
    return slog.New(multiHandler)
}
```

## 日志性能优化

### 1. 条件日志记录

```go
// 避免不必要的字符串格式化
func efficientLogging(logger *slog.Logger, user *User) {
    // 只在需要时进行复杂计算
    if logger.Enabled(context.Background(), slog.LevelDebug) {
        expensiveData := calculateExpensiveData(user)
        logger.Debug("详细用户信息", "data", expensiveData)
    }
    
    // 基本信息总是记录
    logger.Info("用户操作", "user_id", user.ID)
}
```

### 2. 日志采样

```go
func sampledLogger() *slog.Logger {
    // 对高频日志进行采样
    return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
        Level: slog.LevelInfo,
        ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
            // 实现采样逻辑
            if a.Key == "sample_rate" {
                // 根据一定概率决定是否记录
                if rand.Float32() > 0.1 { // 10% 采样率
                    return slog.Attr{} // 跳过此条日志
                }
            }
            return a
        },
    }))
}
```

## 错误处理和日志

### 统一错误日志格式

```go
func logError(logger *slog.Logger, err error, operation string, attrs ...any) {
    // 提取错误信息
    errorMsg := err.Error()
    var stackTrace string
    
    // 如果是包装错误，提取堆栈信息
    if stackErr, ok := err.(interface{ StackTrace() errors.StackTrace }); ok {
        stackTrace = fmt.Sprintf("%+v", stackErr.StackTrace())
    }
    
    // 构建日志属性
    logAttrs := []any{
        "operation", operation,
        "error", errorMsg,
    }
    
    if stackTrace != "" {
        logAttrs = append(logAttrs, "stack_trace", stackTrace)
    }
    
    logAttrs = append(logAttrs, attrs...)
    
    logger.Error("操作失败", logAttrs...)
}

// 使用示例
func (s *Service) doSomething() error {
    if err := someOperation(); err != nil {
        logError(s.logger, err, "some_operation", 
            "param1", "value1",
            "param2", "value2",
        )
        return err
    }
    return nil
}
```

## 监控和告警集成

### 日志指标收集

```go
func metricsLogger() gin.HandlerFunc {
    return func(ctx *gin.Context) {
        logger := abe.MustGetLogger(ctx)
        
        start := time.Now()
        ctx.Next()
        duration := time.Since(start)
        
        // 记录性能指标
        logger.Info("请求指标",
            "path", ctx.Request.URL.Path,
            "method", ctx.Request.Method,
            "status", ctx.Writer.Status(),
            "duration_ms", duration.Milliseconds(),
            "slow_request", duration > time.Second,
        )
        
        // 可以集成到 Prometheus 等监控系统
        recordMetrics(ctx.Request.URL.Path, ctx.Request.Method, duration, ctx.Writer.Status())
    }
}

func recordMetrics(path, method string, duration time.Duration, status int) {
    // 实现指标记录逻辑
    // 例如：prometheus.CounterVec, statsd 等
}
```

## 最佳实践

### 1. 日志级别使用规范

```go
// ✅ 正确的日志级别使用
func goodLoggingPractice(logger *slog.Logger) {
    // DEBUG: 详细的调试信息
    logger.Debug("进入函数", "function", "ProcessData")
    
    // INFO: 重要的业务事件
    logger.Info("用户登录成功", "user_id", userID)
    
    // WARN: 潜在问题
    logger.Warn("缓存未命中", "key", cacheKey)
    
    // ERROR: 错误情况
    logger.Error("数据库连接失败", "error", err.Error())
}

// ❌ 避免：错误的日志级别使用
func badLoggingPractice(logger *slog.Logger) {
    // 不要在 INFO 级别记录调试信息
    logger.Info("变量a的值是:", "a", someVariable)
    
    // 不要在 ERROR 级别记录正常业务流程
    logger.Error("用户查看了个人资料") // 这不是错误！
}
```

### 2. 敏感信息处理

```go
// ❌ 错误：记录敏感信息
logger.Info("用户登录", 
    "username", username,
    "password", password,  // 危险！
)

// ✅ 正确：过滤敏感信息
func safeLog(logger *slog.Logger, username, password string) {
    logger.Info("用户登录尝试",
        "username", username,
        "password_length", len(password),  // 只记录长度
        "masked_password", maskString(password),  // 掩码处理
    )
}

func maskString(s string) string {
    if len(s) <= 3 {
        return "***"
    }
    return s[:3] + "***"
}
```

### 3. 上下文丰富的日志

```go
// ✅ 推荐：丰富的上下文信息
func contextualLogging(ctx *gin.Context, logger *slog.Logger) {
    requestLogger := logger.With(
        "request_id", getRequestID(ctx),
        "user_id", getCurrentUserID(ctx),
        "session_id", getSessionID(ctx),
        "client_ip", ctx.ClientIP(),
        "user_agent", ctx.Request.UserAgent(),
        "timestamp", time.Now().Unix(),
    )
    
    requestLogger.Info("处理用户请求")
}
```

## 故障排除

### 常见日志问题

1. **日志不输出**
   - 检查日志级别配置
   - 确认输出目标是否正确
   - 验证日志处理器是否正确初始化

2. **日志格式混乱**
   - 统一使用相同的日志格式
   - 检查 Handler 配置
   - 验证属性键名的一致性

3. **性能问题**
   - 避免在高频路径中进行复杂日志处理
   - 使用条件日志记录
   - 考虑日志采样策略

4. **磁盘空间问题**
   - 配置合理的日志轮转策略
   - 设置适当的保留期限
   - 启用日志压缩功能