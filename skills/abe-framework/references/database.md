# 数据库集成与 GORM 使用

## 数据库集成概述

ABE 框架深度集成了 GORM ORM 库，提供了完整的数据库操作能力。框架自动配置数据库连接，支持多种数据库类型，并提供了便利的迁移、事务管理和查询构建功能。

## 配置选项

### 数据库配置参数

在 `config.yaml` 中配置数据库连接：

```yaml
database:
  type: "mysql"              # 数据库类型: mysql, postgres, sqlite, sqlserver
  host: "localhost"          # 数据库主机
  port: 3306                 # 数据库端口
  user: "root"               # 用户名
  password: "password"       # 密码
  dbname: "myapp"            # 数据库名
  charset: "utf8mb4"         # 字符集
  parse_time: "True"         # 解析时间
  loc: "Local"               # 时区
  ssl_mode: "disable"        # SSL 模式 (PostgreSQL)
  max_idle_conns: 10         # 最大空闲连接数
  max_open_conns: 100        # 最大打开连接数
  conn_max_lifetime: "1h"    # 连接最大生存时间
```

## 基本使用方法

### 获取数据库实例

```go
func main() {
    engine := abe.NewEngine()
    
    // 获取数据库实例
    db := engine.DB()
    
    // 测试数据库连接
    sqlDB, err := db.DB()
    if err != nil {
        log.Fatal("获取数据库连接失败:", err)
    }
    
    if err := sqlDB.Ping(); err != nil {
        log.Fatal("数据库连接测试失败:", err)
    }
    
    engine.Run()
}
```

### 模型定义

```go
type User struct {
    ID        uint      `gorm:"primaryKey" json:"id"`
    Username  string    `gorm:"size:50;uniqueIndex" json:"username"`
    Email     string    `gorm:"size:100;uniqueIndex" json:"email"`
    Password  string    `gorm:"size:255" json:"-"`
    Nickname  string    `gorm:"size:50" json:"nickname"`
    Avatar    string    `gorm:"size:255" json:"avatar"`
    Status    int       `gorm:"default:1" json:"status"`
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
    DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

type Profile struct {
    ID     uint   `gorm:"primaryKey" json:"id"`
    UserID uint   `gorm:"uniqueIndex" json:"user_id"`
    Bio    string `gorm:"type:text" json:"bio"`
    Phone  string `gorm:"size:20" json:"phone"`
    User   User   `gorm:"foreignKey:UserID" json:"user"`
}

type Order struct {
    ID         uint      `gorm:"primaryKey" json:"id"`
    UserID     uint      `json:"user_id"`
    Amount     float64   `json:"amount"`
    Status     string    `gorm:"size:20;default:'pending'" json:"status"`
    CreatedAt  time.Time `json:"created_at"`
    UpdatedAt  time.Time `json:"updated_at"`
    User       User      `gorm:"foreignKey:UserID" json:"user"`
}
```

## 数据库迁移

### 自动迁移

```go
// 在 main.go 中执行自动迁移
func main() {
    engine := abe.NewEngine()
    
    // 执行自动迁移
    migrateModels(engine.DB())
    
    engine.Run()
}

func migrateModels(db *gorm.DB) {
    // 自动迁移模型
    err := db.AutoMigrate(
        &User{},
        &Profile{},
        &Order{},
        // 添加更多模型...
    )
    
    if err != nil {
        panic(fmt.Sprintf("数据库迁移失败: %v", err))
    }
    
    // 创建索引
    createIndexes(db)
    
    // 初始化数据
    seedData(db)
}

func createIndexes(db *gorm.DB) {
    // 创建复合索引
    db.Exec("CREATE INDEX IF NOT EXISTS idx_users_status_created ON users(status, created_at)")
    db.Exec("CREATE INDEX IF NOT EXISTS idx_orders_user_status ON orders(user_id, status)")
}

func seedData(db *gorm.DB) {
    // 初始化默认数据
    var adminCount int64
    db.Model(&User{}).Where("username = ?", "admin").Count(&adminCount)
    
    if adminCount == 0 {
        admin := User{
            Username: "admin",
            Email:    "admin@example.com",
            Password: hashPassword("admin123"),
            Nickname: "管理员",
            Status:   1,
        }
        db.Create(&admin)
    }
}
```

## CRUD 操作

### 创建操作

```go
type UserService struct {
    db *gorm.DB
}

func (s *UserService) CreateUser(user *User) error {
    // 基本创建
    result := s.db.Create(user)
    if result.Error != nil {
        return result.Error
    }
    
    return nil
}

func (s *UserService) BatchCreateUsers(users []*User) error {
    // 批量创建
    result := s.db.CreateInBatches(users, 100) // 每批100条
    return result.Error
}

// 使用示例
func (uc *UserController) createUser(ctx *gin.Context) {
    var req CreateUserRequest
    if err := ctx.ShouldBindJSON(&req); err != nil {
        ctx.JSON(400, gin.H{"error": "参数错误"})
        return
    }
    
    user := &User{
        Username: req.Username,
        Email:    req.Email,
        Password: hashPassword(req.Password),
        Nickname: req.Nickname,
    }
    
    if err := uc.userService.CreateUser(user); err != nil {
        ctx.JSON(500, gin.H{"error": "创建用户失败"})
        return
    }
    
    ctx.JSON(201, user)
}
```

### 查询操作

```go
func (s *UserService) FindUserByID(id uint) (*User, error) {
    var user User
    result := s.db.First(&user, id)
    if result.Error != nil {
        if errors.Is(result.Error, gorm.ErrRecordNotFound) {
            return nil, fmt.Errorf("用户不存在")
        }
        return nil, result.Error
    }
    return &user, nil
}

func (s *UserService) FindUsersByCondition(condition map[string]interface{}) ([]*User, error) {
    var users []*User
    
    query := s.db.Model(&User{})
    
    // 动态条件查询
    if username, ok := condition["username"]; ok {
        query = query.Where("username LIKE ?", "%"+username.(string)+"%")
    }
    
    if status, ok := condition["status"]; ok {
        query = query.Where("status = ?", status)
    }
    
    if err := query.Find(&users).Error; err != nil {
        return nil, err
    }
    
    return users, nil
}

func (s *UserService) FindUsersWithPagination(page, pageSize int, filters map[string]interface{}) (*PaginationResult, error) {
    var users []*User
    var total int64
    
    // 构建查询
    query := s.db.Model(&User{})
    
    // 应用过滤条件
    if keyword, ok := filters["keyword"]; ok && keyword != "" {
        keywordStr := keyword.(string)
        query = query.Where("username LIKE ? OR email LIKE ? OR nickname LIKE ?", 
            "%"+keywordStr+"%", "%"+keywordStr+"%", "%"+keywordStr+"%")
    }
    
    if status, ok := filters["status"]; ok {
        query = query.Where("status = ?", status)
    }
    
    // 获取总数
    query.Count(&total)
    
    // 分页查询
    offset := (page - 1) * pageSize
    if err := query.Offset(offset).Limit(pageSize).Find(&users).Error; err != nil {
        return nil, err
    }
    
    return &PaginationResult{
        Data:     users,
        Total:    total,
        Page:     page,
        PageSize: pageSize,
    }, nil
}
```

### 更新操作

```go
func (s *UserService) UpdateUser(id uint, updates map[string]interface{}) error {
    // 过滤不允许更新的字段
    allowedFields := []string{"nickname", "avatar", "status"}
    filteredUpdates := make(map[string]interface{})
    
    for _, field := range allowedFields {
        if value, exists := updates[field]; exists {
            filteredUpdates[field] = value
        }
    }
    
    // 添加更新时间
    filteredUpdates["updated_at"] = time.Now()
    
    result := s.db.Model(&User{}).Where("id = ?", id).Updates(filteredUpdates)
    return result.Error
}

func (s *UserService) UpdateUserSelective(id uint, user *User) error {
    // 选择性更新（只更新非零值字段）
    result := s.db.Model(&User{}).Where("id = ?", id).Select("*").Omit("id", "created_at").Updates(user)
    return result.Error
}

// 使用示例
func (uc *UserController) updateUser(ctx *gin.Context) {
    userID, _ := strconv.Atoi(ctx.Param("id"))
    
    var req UpdateUserRequest
    if err := ctx.ShouldBindJSON(&req); err != nil {
        ctx.JSON(400, gin.H{"error": "参数错误"})
        return
    }
    
    updates := make(map[string]interface{})
    if req.Nickname != nil {
        updates["nickname"] = *req.Nickname
    }
    if req.Avatar != nil {
        updates["avatar"] = *req.Avatar
    }
    
    if err := uc.userService.UpdateUser(uint(userID), updates); err != nil {
        ctx.JSON(500, gin.H{"error": "更新用户失败"})
        return
    }
    
    ctx.JSON(200, gin.H{"message": "更新成功"})
}
```

### 删除操作

```go
func (s *UserService) DeleteUser(id uint) error {
    result := s.db.Delete(&User{}, id)
    return result.Error
}

func (s *UserService) SoftDeleteUser(id uint) error {
    // 软删除（使用 DeletedAt 字段）
    result := s.db.Where("id = ?", id).Delete(&User{})
    return result.Error
}

func (s *UserService) BatchDeleteUsers(ids []uint) error {
    result := s.db.Where("id IN ?", ids).Delete(&User{})
    return result.Error
}

// 使用示例
func (uc *UserController) deleteUser(ctx *gin.Context) {
    userID, _ := strconv.Atoi(ctx.Param("id"))
    
    if err := uc.userService.SoftDeleteUser(uint(userID)); err != nil {
        ctx.JSON(500, gin.H{"error": "删除用户失败"})
        return
    }
    
    ctx.JSON(200, gin.H{"message": "删除成功"})
}
```

## 关联查询

### 预加载关联数据

```go
func (s *UserService) GetUserWithProfile(id uint) (*User, error) {
    var user User
    result := s.db.Preload("Profile").First(&user, id)
    return &user, result.Error
}

func (s *OrderService) GetOrderWithUser(id uint) (*Order, error) {
    var order Order
    result := s.db.Preload("User").First(&order, id)
    return &order, result.Error
}

func (s *OrderService) GetUserOrdersWithDetails(userID uint) ([]*Order, error) {
    var orders []*Order
    result := s.db.
        Preload("User").
        Preload("User.Profile").
        Where("user_id = ?", userID).
        Find(&orders)
    return orders, result.Error
}
```

### 条件预加载

```go
func (s *OrderService) GetUserActiveOrders(userID uint) ([]*Order, error) {
    var orders []*Order
    result := s.db.
        Preload("User", func(db *gorm.DB) *gorm.DB {
            return db.Select("id, username, email, nickname")
        }).
        Where("user_id = ? AND status IN ?", userID, []string{"pending", "processing"}).
        Find(&orders)
    return orders, result.Error
}
```

## 事务处理

### 基本事务

```go
func (s *UserService) TransferPoints(fromUserID, toUserID uint, points int) error {
    return s.db.Transaction(func(tx *gorm.DB) error {
        // 扣除发送方积分
        if err := tx.Model(&User{}).
            Where("id = ? AND points >= ?", fromUserID, points).
            Update("points", gorm.Expr("points - ?", points)).Error; err != nil {
            return err
        }
        
        // 增加接收方积分
        if err := tx.Model(&User{}).
            Where("id = ?", toUserID).
            Update("points", gorm.Expr("points + ?", points)).Error; err != nil {
            return err
        }
        
        // 记录转账日志
        transferLog := TransferLog{
            FromUserID: fromUserID,
            ToUserID:   toUserID,
            Points:     points,
            CreatedAt:  time.Now(),
        }
        
        if err := tx.Create(&transferLog).Error; err != nil {
            return err
        }
        
        return nil
    })
}
```

### 嵌套事务

```go
func (s *OrderService) ProcessOrderPayment(orderID uint, paymentInfo PaymentInfo) error {
    return s.db.Transaction(func(tx *gorm.DB) error {
        // 获取订单
        var order Order
        if err := tx.First(&order, orderID).Error; err != nil {
            return err
        }
        
        // 处理支付（嵌套事务）
        if err := s.processPayment(tx, order, paymentInfo); err != nil {
            return err
        }
        
        // 更新订单状态
        if err := tx.Model(&order).Update("status", "paid").Error; err != nil {
            return err
        }
        
        // 发送通知（另一个嵌套事务）
        if err := s.sendOrderNotification(tx, order); err != nil {
            return err
        }
        
        return nil
    })
}

func (s *OrderService) processPayment(tx *gorm.DB, order Order, paymentInfo PaymentInfo) error {
    return tx.Transaction(func(subTx *gorm.DB) error {
        // 处理支付逻辑
        paymentRecord := PaymentRecord{
            OrderID:      order.ID,
            Amount:       order.Amount,
            PaymentType:  paymentInfo.Type,
            PaymentID:    paymentInfo.PaymentID,
            Status:       "completed",
            CompletedAt:  time.Now(),
        }
        
        if err := subTx.Create(&paymentRecord).Error; err != nil {
            return err
        }
        
        return nil
    })
}
```

## 高级查询技巧

### 复杂查询构建

```go
func (s *OrderService) GetOrderStatistics(filters map[string]interface{}) (*OrderStatistics, error) {
    var stats OrderStatistics
    
    query := s.db.Model(&Order{})
    
    // 应用日期范围过滤
    if startDate, ok := filters["start_date"]; ok {
        query = query.Where("created_at >= ?", startDate)
    }
    
    if endDate, ok := filters["end_date"]; ok {
        query = query.Where("created_at <= ?", endDate)
    }
    
    // 按状态分组统计
    var statusStats []struct {
        Status string
        Count  int64
        Total  float64
    }
    
    query.Select("status, COUNT(*) as count, SUM(amount) as total").
        Group("status").
        Scan(&statusStats)
    
    stats.StatusDistribution = statusStats
    
    // 总体统计
    query.Select("COUNT(*) as total_orders, SUM(amount) as total_amount").
        Scan(&stats)
    
    return &stats, nil
}

type OrderStatistics struct {
    TotalOrders      int64
    TotalAmount      float64
    StatusDistribution []struct {
        Status string
        Count  int64
        Total  float64
    }
}
```

### 原生 SQL 查询

```go
func (s *UserService) GetUserRanking(limit int) ([]UserRanking, error) {
    var rankings []UserRanking
    
    sql := `
        SELECT u.id, u.username, u.nickname, 
               COUNT(o.id) as order_count,
               SUM(o.amount) as total_amount,
               RANK() OVER (ORDER BY SUM(o.amount) DESC) as ranking
        FROM users u
        LEFT JOIN orders o ON u.id = o.user_id 
        WHERE u.status = 1 
        GROUP BY u.id, u.username, u.nickname
        ORDER BY total_amount DESC
        LIMIT ?
    `
    
    err := s.db.Raw(sql, limit).Scan(&rankings).Error
    return rankings, err
}

type UserRanking struct {
    ID          uint    `json:"id"`
    Username    string  `json:"username"`
    Nickname    string  `json:"nickname"`
    OrderCount  int64   `json:"order_count"`
    TotalAmount float64 `json:"total_amount"`
    Ranking     int64   `json:"ranking"`
}
```

## 性能优化

### 连接池配置

```go
func setupConnectionPool(db *gorm.DB) {
    sqlDB, err := db.DB()
    if err != nil {
        panic(fmt.Sprintf("获取数据库连接失败: %v", err))
    }
    
    // 设置连接池参数
    sqlDB.SetMaxIdleConns(10)           // 最大空闲连接数
    sqlDB.SetMaxOpenConns(100)          // 最大打开连接数
    sqlDB.SetConnMaxLifetime(time.Hour) // 连接最大生存时间
}

// 在应用启动时调用
func main() {
    engine := abe.NewEngine()
    
    // 配置连接池
    setupConnectionPool(engine.DB())
    
    engine.Run()
}
```

### 查询优化

```go
func (s *UserService) OptimizedUserQuery() ([]*User, error) {
    var users []*User
    
    // 优化查询：只选择需要的字段
    err := s.db.
        Select("id, username, email, nickname, created_at").
        Where("status = ?", 1).
        Order("created_at DESC").
        Limit(100).
        Find(&users).Error
    
    return users, err
}

// 使用索引提示
func (s *OrderService) GetRecentOrders(userID uint) ([]*Order, error) {
    var orders []*Order
    
    err := s.db.
        Select("id, user_id, amount, status, created_at").
        Where("user_id = ?", userID).
        Order("created_at DESC").
        Limit(50).
        Find(&orders).Error
    
    return orders, err
}
```

## 最佳实践

### 1. 模型设计规范

```go
// ✅ 推荐的模型设计
type BaseModel struct {
    ID        uint           `gorm:"primaryKey" json:"id"`
    CreatedAt time.Time      `json:"created_at"`
    UpdatedAt time.Time      `json:"updated_at"`
    DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

type User struct {
    BaseModel
    Username string `gorm:"size:50;uniqueIndex" json:"username"`
    Email    string `gorm:"size:100;uniqueIndex" json:"email"`
    // 其他字段...
}

// ❌ 避免的设计问题
type BadUser struct {
    Id       int       `json:"id"`        // 应使用 uint 和 gorm 标签
    UserName string    `json:"userName"`  // 命名不一致
    Created  time.Time `json:"created"`   // 应使用 CreatedAt
}
```

### 2. 查询构建器模式

```go
type UserQueryBuilder struct {
    db      *gorm.DB
    filters map[string]interface{}
}

func NewUserQueryBuilder(db *gorm.DB) *UserQueryBuilder {
    return &UserQueryBuilder{
        db:      db.Model(&User{}),
        filters: make(map[string]interface{}),
    }
}

func (b *UserQueryBuilder) WithKeyword(keyword string) *UserQueryBuilder {
    if keyword != "" {
        b.filters["keyword"] = keyword
    }
    return b
}

func (b *UserQueryBuilder) WithStatus(status int) *UserQueryBuilder {
    b.filters["status"] = status
    return b
}

func (b *UserQueryBuilder) Build() *gorm.DB {
    query := b.db
    
    if keyword, ok := b.filters["keyword"]; ok {
        k := keyword.(string)
        query = query.Where("username LIKE ? OR email LIKE ?", "%"+k+"%", "%"+k+"%")
    }
    
    if status, ok := b.filters["status"]; ok {
        query = query.Where("status = ?", status)
    }
    
    return query
}

// 使用示例
func (s *UserService) SearchUsers(keyword string, status int) ([]*User, error) {
    var users []*User
    
    query := NewUserQueryBuilder(s.db).
        WithKeyword(keyword).
        WithStatus(status).
        Build()
    
    err := query.Find(&users).Error
    return users, err
}
```

### 3. 错误处理和日志

```go
func (s *UserService) SafeFindUser(id uint) (*User, error) {
    var user User
    
    start := time.Now()
    result := s.db.First(&user, id)
    duration := time.Since(start)
    
    // 记录查询性能
    logger := abe.MustGetLogger(context.Background())
    logger.Info("数据库查询",
        "table", "users",
        "operation", "find_by_id",
        "user_id", id,
        "duration_ms", duration.Milliseconds(),
    )
    
    if result.Error != nil {
        if errors.Is(result.Error, gorm.ErrRecordNotFound) {
            logger.Warn("用户不存在", "user_id", id)
            return nil, fmt.Errorf("用户不存在")
        }
        
        logger.Error("数据库查询失败",
            "error", result.Error.Error(),
            "user_id", id,
            "duration_ms", duration.Milliseconds(),
        )
        return nil, fmt.Errorf("数据库查询失败: %w", result.Error)
    }
    
    return &user, nil
}
```

## 故障排除

### 常见数据库问题

1. **连接池耗尽**
   ```go
   // 增加连接池大小
   sqlDB.SetMaxOpenConns(200)
   sqlDB.SetMaxIdleConns(20)
   ```

2. **慢查询问题**
   ```go
   // 启用慢查询日志
   db = db.Debug() // 开发环境使用
   ```

3. **死锁问题**
   ```go
   // 使用事务超时
   ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
   defer cancel()
   
   tx := db.WithContext(ctx).Begin()
   defer func() {
       if r := recover(); r != nil {
           tx.Rollback()
       }
   }()
   ```

4. **索引缺失**
   ```go
   // 检查查询计划
   db.Debug().Model(&User{}).Where("username = ?", "test").Find(&users)
   ```