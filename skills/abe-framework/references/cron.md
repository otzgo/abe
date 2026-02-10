# 定时任务 (Cron) 使用指南

## 定时任务概述

ABE 框架集成了 `robfig/cron/v3` 库，提供了强大的定时任务调度功能。支持标准的 cron 表达式，可以轻松实现各种周期性任务的自动化执行。

## Cron 表达式语法

Cron 表达式由 5 个或 6 个字段组成（框架使用 5 字段格式）：

```
分钟 小时 日 月 星期
```

### 字段说明

| 字段 | 允许值 | 特殊字符 |
|------|--------|----------|
| 分钟 | 0-59 | * , - / |
| 小时 | 0-23 | * , - / |
| 日 | 1-31 | * , - / ? L W |
| 月 | 1-12 | * , - / |
| 星期 | 0-6 (0=周日) | * , - / ? L # |

### 常用表达式示例

```bash
# 每分钟执行
* * * * *

# 每小时执行（每小时的第0分钟）
0 * * * *

# 每天凌晨2点执行
0 2 * * *

# 每周一上午9点执行
0 9 * * 1

# 每月1号凌晨执行
0 0 1 * *

# 每5分钟执行一次
*/5 * * * *

# 工作日上午10点执行
0 10 * * 1-5

# 每天的9点到17点每小时执行
0 9-17 * * *

# 每月最后一天执行
0 0 L * *
```

## 基本使用方法

### 获取 Cron 实例

```go
func main() {
    engine := abe.NewEngine()
    
    // 获取 Cron 调度器
    cron := engine.Cron()
    
    // 添加定时任务
    setupCronJobs(cron)
    
    engine.Run()
}

func setupCronJobs(cron *cron.Cron) {
    // 添加各种定时任务
    addDataCleanupJob(cron)
    addReportGenerationJob(cron)
    addHealthCheckJob(cron)
}
```

## 常规定时任务示例

### 1. 数据清理任务

```go
func addDataCleanupJob(cron *cron.Cron) {
    // 每天凌晨3点清理过期数据
    _, err := cron.AddFunc("0 3 * * *", func() {
        logger := getLogger()
        logger.Info("开始执行数据清理任务")
        
        // 清理过期的临时文件
        cleanupTemporaryFiles()
        
        // 清理过期的日志记录
        cleanupExpiredLogs()
        
        // 清理过期的缓存数据
        cleanupExpiredCache()
        
        logger.Info("数据清理任务执行完成")
    })
    
    if err != nil {
        panic(fmt.Sprintf("添加数据清理任务失败: %v", err))
    }
}

func cleanupTemporaryFiles() {
    // 清理7天前的临时文件
    cutoffTime := time.Now().AddDate(0, 0, -7)
    
    // 实际的文件清理逻辑
    // ...
}

func cleanupExpiredLogs() {
    // 清理30天前的日志记录
    // ...
}

func cleanupExpiredCache() {
    // 清理过期缓存
    // ...
}
```

### 2. 报表生成任务

```go
func addReportGenerationJob(cron *cron.Cron) {
    // 每周一凌晨4点生成上周报表
    _, err := cron.AddFunc("0 4 * * 1", func() {
        logger := getLogger()
        logger.Info("开始生成周报")
        
        // 生成用户活跃度报告
        generateUserActivityReport()
        
        // 生成销售报告
        generateSalesReport()
        
        // 生成系统性能报告
        generatePerformanceReport()
        
        logger.Info("周报生成完成")
    })
    
    if err != nil {
        panic(fmt.Sprintf("添加报表生成任务失败: %v", err))
    }
}

func generateUserActivityReport() {
    // 生成用户活跃度报告的逻辑
    startDate := time.Now().AddDate(0, 0, -7) // 上周
    endDate := time.Now().AddDate(0, 0, -1)   // 昨天
    
    // 查询数据并生成报告
    // ...
}

func generateSalesReport() {
    // 生成销售报告的逻辑
    // ...
}
```

### 3. 健康检查任务

```go
func addHealthCheckJob(cron *cron.Cron) {
    // 每5分钟执行一次健康检查
    _, err := cron.AddFunc("*/5 * * * *", func() {
        logger := getLogger()
        logger.Debug("执行健康检查")
        
        // 检查数据库连接
        if err := checkDatabaseConnection(); err != nil {
            logger.Error("数据库连接异常", "error", err.Error())
            alertAdmin("数据库连接异常: " + err.Error())
        }
        
        // 检查外部服务
        if err := checkExternalServices(); err != nil {
            logger.Warn("外部服务异常", "error", err.Error())
        }
        
        // 检查磁盘空间
        if usage := checkDiskUsage(); usage > 90 {
            logger.Warn("磁盘空间不足", "usage_percent", usage)
            alertAdmin(fmt.Sprintf("磁盘使用率过高: %.2f%%", usage))
        }
    })
    
    if err != nil {
        panic(fmt.Sprintf("添加健康检查任务失败: %v", err))
    }
}

func checkDatabaseConnection() error {
    // 数据库连接检查逻辑
    // ...
    return nil
}

func checkExternalServices() error {
    // 外部服务检查逻辑
    // ...
    return nil
}

func checkDiskUsage() float64 {
    // 磁盘使用率检查
    // ...
    return 85.5 // 返回使用百分比
}
```

## 高级定时任务

### 带参数的任务

```go
type TaskScheduler struct {
    db     *gorm.DB
    logger *slog.Logger
    cron   *cron.Cron
}

func NewTaskScheduler(db *gorm.DB, logger *slog.Logger) *TaskScheduler {
    return &TaskScheduler{
        db:     db,
        logger: logger,
        cron:   cron.New(),
    }
}

func (ts *TaskScheduler) AddBackupJob(schedule string, backupType string, retentionDays int) error {
    jobFunc := func() {
        ts.logger.Info("开始备份任务",
            "type", backupType,
            "retention_days", retentionDays,
        )
        
        switch backupType {
        case "database":
            ts.backupDatabase(retentionDays)
        case "files":
            ts.backupFiles(retentionDays)
        case "logs":
            ts.backupLogs(retentionDays)
        }
    }
    
    _, err := ts.cron.AddFunc(schedule, jobFunc)
    return err
}

func (ts *TaskScheduler) backupDatabase(retentionDays int) {
    // 数据库备份逻辑
    backupFile := fmt.Sprintf("backup_%s.sql", time.Now().Format("20060102_150405"))
    
    // 执行备份命令
    // ...
    
    // 清理过期备份
    ts.cleanupOldBackups("*.sql", retentionDays)
}

func (ts *TaskScheduler) backupFiles(retentionDays int) {
    // 文件备份逻辑
    // ...
}

func (ts *TaskScheduler) backupLogs(retentionDays int) {
    // 日志备份逻辑
    // ...
}

func (ts *TaskScheduler) cleanupOldBackups(pattern string, days int) {
    cutoffTime := time.Now().AddDate(0, 0, -days)
    // 清理逻辑
    // ...
}
```

### 任务依赖管理

```go
type DependentTask struct {
    Name     string
    Schedule string
    Depends  []string // 依赖的任务名称
    Execute  func() error
}

func (ts *TaskScheduler) AddDependentJob(task DependentTask) error {
    wrappedFunc := func() {
        ts.logger.Info("开始执行任务", "task", task.Name)
        
        // 检查依赖任务是否完成
        if !ts.checkDependencies(task.Depends) {
            ts.logger.Warn("依赖任务未完成，跳过执行", "task", task.Name)
            return
        }
        
        start := time.Now()
        err := task.Execute()
        duration := time.Since(start)
        
        if err != nil {
            ts.logger.Error("任务执行失败",
                "task", task.Name,
                "error", err.Error(),
                "duration_ms", duration.Milliseconds(),
            )
            alertAdmin(fmt.Sprintf("任务 %s 执行失败: %v", task.Name, err))
        } else {
            ts.logger.Info("任务执行成功",
                "task", task.Name,
                "duration_ms", duration.Milliseconds(),
            )
        }
    }
    
    _, err := ts.cron.AddFunc(task.Schedule, wrappedFunc)
    return err
}

func (ts *TaskScheduler) checkDependencies(depends []string) bool {
    // 检查依赖任务的执行状态
    // 这里可以查询任务执行记录表
    for _, dep := range depends {
        if !ts.isTaskCompleted(dep) {
            return false
        }
    }
    return true
}

func (ts *TaskScheduler) isTaskCompleted(taskName string) bool {
    // 查询任务执行状态
    var lastExecution TaskExecution
    err := ts.db.Where("task_name = ? AND status = ?", taskName, "completed").
        Order("executed_at DESC").
        First(&lastExecution).Error
    
    if err != nil {
        return false
    }
    
    // 检查是否是今天的执行记录
    return lastExecution.ExecutedAt.Day() == time.Now().Day()
}
```

## 任务监控和管理

### 任务执行记录

```go
type TaskExecution struct {
    ID          uint      `gorm:"primaryKey" json:"id"`
    TaskName    string    `gorm:"index" json:"task_name"`
    ScheduledAt time.Time `json:"scheduled_at"`
    ExecutedAt  time.Time `json:"executed_at"`
    Duration    int64     `json:"duration_ms"`
    Status      string    `gorm:"size:20" json:"status"` // pending, running, completed, failed
    ErrorMessage string   `json:"error_message,omitempty"`
    Output      string    `json:"output,omitempty"`
}

func (ts *TaskScheduler) setupMonitoredJob(taskName, schedule string, jobFunc func() error) error {
    monitoredFunc := func() {
        execution := &TaskExecution{
            TaskName:    taskName,
            ScheduledAt: time.Now(),
            Status:      "running",
        }
        
        // 记录任务开始
        ts.db.Create(execution)
        
        start := time.Now()
        err := jobFunc()
        duration := time.Since(start)
        
        // 更新执行结果
        execution.ExecutedAt = time.Now()
        execution.Duration = duration.Milliseconds()
        
        if err != nil {
            execution.Status = "failed"
            execution.ErrorMessage = err.Error()
            ts.logger.Error("任务执行失败",
                "task", taskName,
                "error", err.Error(),
            )
        } else {
            execution.Status = "completed"
            ts.logger.Info("任务执行成功", "task", taskName)
        }
        
        ts.db.Save(execution)
    }
    
    _, err := ts.cron.AddFunc(schedule, monitoredFunc)
    return err
}
```

### 任务状态查询

```go
type TaskService struct {
    db   *gorm.DB
    cron *cron.Cron
}

func (ts *TaskService) GetTaskExecutions(taskName string, page, pageSize int) ([]*TaskExecution, int64, error) {
    var executions []*TaskExecution
    var total int64
    
    query := ts.db.Model(&TaskExecution{}).Where("task_name = ?", taskName)
    
    // 获取总数
    query.Count(&total)
    
    // 分页查询
    offset := (page - 1) * pageSize
    err := query.Order("executed_at DESC").
        Offset(offset).
        Limit(pageSize).
        Find(&executions).Error
    
    return executions, total, err
}

func (ts *TaskService) GetTaskStatistics() (map[string]interface{}, error) {
    var stats struct {
        TotalTasks     int64
        RunningTasks   int64
        FailedTasks    int64
        CompletedTasks int64
    }
    
    // 统计各类任务数量
    ts.db.Model(&TaskExecution{}).Count(&stats.TotalTasks)
    ts.db.Model(&TaskExecution{}).Where("status = ?", "running").Count(&stats.RunningTasks)
    ts.db.Model(&TaskExecution{}).Where("status = ?", "failed").Count(&stats.FailedTasks)
    ts.db.Model(&TaskExecution{}).Where("status = ?", "completed").Count(&stats.CompletedTasks)
    
    return map[string]interface{}{
        "total":     stats.TotalTasks,
        "running":   stats.RunningTasks,
        "failed":    stats.FailedTasks,
        "completed": stats.CompletedTasks,
    }, nil
}
```

## 动态任务管理

### 运行时添加/删除任务

```go
type DynamicTaskManager struct {
    cron      *cron.Cron
    jobIDs    map[string]cron.EntryID
    mutex     sync.RWMutex
    db        *gorm.DB
}

func NewDynamicTaskManager(db *gorm.DB) *DynamicTaskManager {
    return &DynamicTaskManager{
        cron:   cron.New(),
        jobIDs: make(map[string]cron.EntryID),
        db:     db,
    }
}

func (dtm *DynamicTaskManager) AddTask(task *ScheduledTask) error {
    dtm.mutex.Lock()
    defer dtm.mutex.Unlock()
    
    // 检查任务是否已存在
    if _, exists := dtm.jobIDs[task.Name]; exists {
        return fmt.Errorf("任务 %s 已存在", task.Name)
    }
    
    // 创建任务函数
    jobFunc := dtm.createJobFunction(task)
    
    // 添加到 cron 调度器
    entryID, err := dtm.cron.AddFunc(task.Schedule, jobFunc)
    if err != nil {
        return fmt.Errorf("添加任务失败: %w", err)
    }
    
    // 记录任务ID
    dtm.jobIDs[task.Name] = entryID
    
    // 保存到数据库
    task.EntryID = int(entryID)
    return dtm.db.Create(task).Error
}

func (dtm *DynamicTaskManager) RemoveTask(taskName string) error {
    dtm.mutex.Lock()
    defer dtm.mutex.Unlock()
    
    entryID, exists := dtm.jobIDs[taskName]
    if !exists {
        return fmt.Errorf("任务 %s 不存在", taskName)
    }
    
    // 从 cron 调度器移除
    dtm.cron.Remove(entryID)
    
    // 从内存中移除
    delete(dtm.jobIDs, taskName)
    
    // 从数据库删除
    return dtm.db.Where("name = ?", taskName).Delete(&ScheduledTask{}).Error
}

func (dtm *DynamicTaskManager) ListTasks() ([]*ScheduledTask, error) {
    var tasks []*ScheduledTask
    err := dtm.db.Find(&tasks).Error
    return tasks, err
}

type ScheduledTask struct {
    ID       uint      `gorm:"primaryKey" json:"id"`
    Name     string    `gorm:"uniqueIndex" json:"name"`
    Schedule string    `json:"schedule"`
    EntryID  int       `json:"entry_id"`
    Enabled  bool      `gorm:"default:true" json:"enabled"`
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
}
```

## 最佳实践

### 1. 任务设计原则

```go
// ✅ 推荐的任务设计
func recommendedTaskDesign() {
    // 1. 任务应该具有幂等性
    func idempotentTask() error {
        // 即使多次执行也不会产生副作用
        return nil
    }
    
    // 2. 任务应该有适当的超时控制
    func timeoutControlledTask() error {
        ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
        defer cancel()
        
        // 在上下文中执行任务
        return doWorkWithContext(ctx)
    }
    
    // 3. 任务应该有良好的错误处理
    func robustTask() error {
        defer func() {
            if r := recover(); r != nil {
                logError(fmt.Errorf("任务恐慌: %v", r))
            }
        }()
        
        // 业务逻辑...
        return nil
    }
}

// ❌ 避免的设计问题
func problematicTaskDesign() {
    // 避免长时间阻塞的任务
    func blockingTask() {
        time.Sleep(2 * time.Hour) // 不好的做法
    }
    
    // 避免没有错误处理的任务
    func unsafeTask() {
        // 可能panic的代码没有保护
        riskyOperation()
    }
}
```

### 2. 资源管理

```go
func resourceManagedTask() {
    // 使用 defer 确保资源释放
    func databaseTask() error {
        db, err := getConnection()
        if err != nil {
            return err
        }
        defer db.Close()
        
        // 使用数据库连接
        return performDatabaseOperation(db)
    }
    
    // 限制并发执行
    var semaphore = make(chan struct{}, 3) // 最多3个并发
    
    func concurrentSafeTask() error {
        semaphore <- struct{}{} // 获取信号量
        defer func() { <-semaphore }() // 释放信号量
        
        return doHeavyWork()
    }
}
```

### 3. 任务监控

```go
func monitoredTask(logger *slog.Logger) func() {
    return func() {
        taskName := "data_processing"
        start := time.Now()
        
        logger.Info("任务开始", "task", taskName)
        
        // 执行任务
        err := processData()
        duration := time.Since(start)
        
        // 记录结果
        if err != nil {
            logger.Error("任务失败",
                "task", taskName,
                "duration_ms", duration.Milliseconds(),
                "error", err.Error(),
            )
            
            // 发送告警
            alertOnError(taskName, err)
        } else {
            logger.Info("任务成功",
                "task", taskName,
                "duration_ms", duration.Milliseconds(),
            )
        }
        
        // 记录指标
        recordTaskMetrics(taskName, duration, err)
    }
}

func alertOnError(taskName string, err error) {
    // 发送告警通知
    // 可以集成邮件、短信、Slack 等通知方式
}

func recordTaskMetrics(taskName string, duration time.Duration, err error) {
    // 记录到监控系统
    // prometheus、statsd 等
}
```

## 故障排除

### 常见问题解决

1. **任务没有按预期执行**
   ```go
   // 检查 cron 表达式是否正确
   func validateCronExpression(expr string) error {
       _, err := cron.ParseStandard(expr)
       return err
   }
   
   // 检查调度器是否启动
   func checkCronRunning(cron *cron.Cron) bool {
       return len(cron.Entries()) > 0
   }
   ```

2. **任务执行时间过长**
   ```go
   // 设置任务超时
   func timeoutWrapper(jobFunc func() error, timeout time.Duration) func() {
       return func() {
           ctx, cancel := context.WithTimeout(context.Background(), timeout)
           defer cancel()
           
           done := make(chan error, 1)
           go func() {
               done <- jobFunc()
           }()
           
           select {
           case err := <-done:
               if err != nil {
                   logError(err)
               }
           case <-ctx.Done():
               logError(fmt.Errorf("任务超时"))
           }
       }
   }
   ```

3. **并发执行问题**
   ```go
   // 使用互斥锁防止重复执行
   var taskMutex sync.Mutex
   
   func exclusiveTask() {
       if !taskMutex.TryLock() {
           logger.Warn("任务已在执行中，跳过本次执行")
           return
       }
       defer taskMutex.Unlock()
       
       // 执行任务逻辑
   }
   ```

4. **时区问题**
   ```go
   // 确保使用正确的时区
   func setupCronWithTimezone() *cron.Cron {
       location, _ := time.LoadLocation("Asia/Shanghai")
       return cron.New(cron.WithLocation(location))
   }
   ```