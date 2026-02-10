# 协程池管理

## 协程池概述

在高并发的应用场景中，ABE 框架集成了基于 `ants` 库的协程池功能，用于高效地管理 goroutine，避免频繁创建和销毁带来的性能开销。

## 配置选项

### 协程池配置参数

在 `config.yaml` 中配置协程池：

```yaml
pool:
  size: 50000                         # 协程池大小
  expiry_duration: "10s"              # 协程过期时间
  pre_alloc: true                     # 是否预分配内存
  max_blocking_tasks: 10000           # 最大阻塞任务数
  nonblocking: false                  # 是否为非阻塞模式
```

## 基本使用方法

### 获取协程池实例

```go
func main() {
    engine := abe.NewEngine()
    
    // 获取默认协程池
    pool := engine.Pool()
    
    // 创建函数任务协程池
    taskPool, err := engine.NewPoolWithFunc(processTask, 1000)
    if err != nil {
        log.Fatal("创建协程池失败:", err)
    }
    
    // 使用协程池执行任务
    executeTasks(pool, taskPool)
    
    engine.Run()
}

func processTask(data interface{}) {
    // 处理任务的逻辑
    fmt.Printf("处理任务: %v\n", data)
}

func executeTasks(pool *ants.Pool, taskPool *ants.PoolWithFunc) {
    // 提交普通任务
    for i := 0; i < 10; i++ {
        index := i
        pool.Submit(func() {
            fmt.Printf("执行任务 %d\n", index)
            time.Sleep(100 * time.Millisecond)
        })
    }
    
    // 提交函数任务
    for i := 0; i < 5; i++ {
        taskPool.Invoke(fmt.Sprintf("数据%d", i))
    }
}
```

## 协程池类型

### 1. 普通协程池

```go
type TaskExecutor struct {
    pool *ants.Pool
}

func NewTaskExecutor(pool *ants.Pool) *TaskExecutor {
    return &TaskExecutor{pool: pool}
}

func (te *TaskExecutor) ExecuteAsync(task func()) error {
    return te.pool.Submit(task)
}

func (te *TaskExecutor) ExecuteWithTimeout(task func(), timeout time.Duration) error {
    done := make(chan struct{})
    
    err := te.pool.Submit(func() {
        defer close(done)
        task()
    })
    
    if err != nil {
        return err
    }
    
    select {
    case <-done:
        return nil
    case <-time.After(timeout):
        return fmt.Errorf("任务执行超时")
    }
}

// 使用示例
func (te *TaskExecutor) ProcessBatchData(data []interface{}) {
    var wg sync.WaitGroup
    
    for _, item := range data {
        wg.Add(1)
        item := item // 捕获循环变量
        
        te.pool.Submit(func() {
            defer wg.Done()
            processItem(item)
        })
    }
    
    wg.Wait() // 等待所有任务完成
}
```

### 2. 函数任务协程池

```go
type DataProcessor struct {
    taskPool *ants.PoolWithFunc
    logger   *slog.Logger
}

func NewDataProcessor(taskPool *ants.PoolWithFunc, logger *slog.Logger) *DataProcessor {
    return &DataProcessor{
        taskPool: taskPool,
        logger:   logger,
    }
}

func (dp *DataProcessor) ProcessItems(items []interface{}) error {
    var errors []error
    var mu sync.Mutex
    var wg sync.WaitGroup
    
    for _, item := range items {
        wg.Add(1)
        item := item
        
        err := dp.taskPool.Invoke(func() {
            defer wg.Done()
            
            if err := dp.processSingleItem(item); err != nil {
                mu.Lock()
                errors = append(errors, err)
                mu.Unlock()
            }
        })
        
        if err != nil {
            return fmt.Errorf("提交任务失败: %w", err)
        }
    }
    
    wg.Wait()
    
    if len(errors) > 0 {
        return fmt.Errorf("处理过程中出现 %d 个错误: %v", len(errors), errors)
    }
    
    return nil
}

func (dp *DataProcessor) processSingleItem(item interface{}) error {
    // 具体的处理逻辑
    dp.logger.Info("处理数据项", "item", item)
    time.Sleep(50 * time.Millisecond)
    return nil
}
```

## 高级协程池管理

### 动态协程池调整

```go
type DynamicPoolManager struct {
    pools     map[string]*ants.Pool
    config    PoolConfig
    mutex     sync.RWMutex
    logger    *slog.Logger
}

type PoolConfig struct {
    BaseSize        int           `json:"base_size"`
    MaxSize         int           `json:"max_size"`
    ScaleThreshold  float64       `json:"scale_threshold"`  // 0.0-1.0
    ScaleInterval   time.Duration `json:"scale_interval"`
    ExpiryDuration  time.Duration `json:"expiry_duration"`
}

func NewDynamicPoolManager(config PoolConfig, logger *slog.Logger) *DynamicPoolManager {
    manager := &DynamicPoolManager{
        pools:  make(map[string]*ants.Pool),
        config: config,
        logger: logger,
    }
    
    // 启动自动扩缩容监控
    go manager.monitorAndScale()
    
    return manager
}

func (dpm *DynamicPoolManager) GetOrCreatePool(name string, initialSize int) (*ants.Pool, error) {
    dpm.mutex.RLock()
    if pool, exists := dpm.pools[name]; exists {
        dpm.mutex.RUnlock()
        return pool, nil
    }
    dpm.mutex.RUnlock()
    
    dpm.mutex.Lock()
    defer dpm.mutex.Unlock()
    
    // 双重检查
    if pool, exists := dpm.pools[name]; exists {
        return pool, nil
    }
    
    // 创建新协程池
    pool, err := ants.NewPool(initialSize, ants.WithExpiryDuration(dpm.config.ExpiryDuration))
    if err != nil {
        return nil, fmt.Errorf("创建协程池失败: %w", err)
    }
    
    dpm.pools[name] = pool
    dpm.logger.Info("创建协程池", "name", name, "size", initialSize)
    
    return pool, nil
}

func (dpm *DynamicPoolManager) monitorAndScale() {
    ticker := time.NewTicker(dpm.config.ScaleInterval)
    defer ticker.Stop()
    
    for range ticker.C {
        dpm.scalePoolsIfNeeded()
    }
}

func (dpm *DynamicPoolManager) scalePoolsIfNeeded() {
    dpm.mutex.RLock()
    defer dpm.mutex.RUnlock()
    
    for name, pool := range dpm.pools {
        // 计算使用率
        running := pool.Running()
        capacity := pool.Cap()
        usage := float64(running) / float64(capacity)
        
        dpm.logger.Debug("协程池状态",
            "name", name,
            "running", running,
            "capacity", capacity,
            "usage", usage)
        
        // 根据使用率调整大小
        if usage > dpm.config.ScaleThreshold && capacity < dpm.config.MaxSize {
            newCapacity := min(capacity*2, dpm.config.MaxSize)
            if err := pool.Tune(newCapacity); err != nil {
                dpm.logger.Error("调整协程池大小失败", "name", name, "error", err)
            } else {
                dpm.logger.Info("扩容协程池", "name", name, "from", capacity, "to", newCapacity)
            }
        } else if usage < 0.3 && capacity > dpm.config.BaseSize {
            newCapacity := max(capacity/2, dpm.config.BaseSize)
            if err := pool.Tune(newCapacity); err != nil {
                dpm.logger.Error("收缩协程池大小失败", "name", name, "error", err)
            } else {
                dpm.logger.Info("收缩协程池", "name", name, "from", capacity, "to", newCapacity)
            }
        }
    }
}
```

### 任务优先级管理

```go
type PriorityTask struct {
    Priority int
    Task     func()
    ID       string
}

type PriorityQueuePool struct {
    pool       *ants.Pool
    taskQueue  chan PriorityTask
    workers    int
    logger     *slog.Logger
}

func NewPriorityQueuePool(poolSize, queueSize, workerCount int, logger *slog.Logger) *PriorityQueuePool {
    pqp := &PriorityQueuePool{
        taskQueue: make(chan PriorityTask, queueSize),
        workers:   workerCount,
        logger:    logger,
    }
    
    var err error
    pqp.pool, err = ants.NewPool(poolSize)
    if err != nil {
        panic(fmt.Sprintf("创建协程池失败: %v", err))
    }
    
    // 启动优先级工作者
    pqp.startPriorityWorkers()
    
    return pqp
}

func (pqp *PriorityQueuePool) Submit(priority int, task func(), taskID string) error {
    select {
    case pqp.taskQueue <- PriorityTask{Priority: priority, Task: task, ID: taskID}:
        return nil
    default:
        return fmt.Errorf("任务队列已满")
    }
}

func (pqp *PriorityQueuePool) startPriorityWorkers() {
    for i := 0; i < pqp.workers; i++ {
        workerID := i
        pqp.pool.Submit(func() {
            pqp.priorityWorker(workerID)
        })
    }
}

func (pqp *PriorityQueuePool) priorityWorker(workerID int) {
    // 使用优先级队列
    pq := make(PriorityQueue, 0)
    heap.Init(&pq)
    
    for {
        select {
        case task := <-pqp.taskQueue:
            heap.Push(&pq, &task)
            
            // 处理队列中的所有任务
            for pq.Len() > 0 {
                highestPriorityTask := heap.Pop(&pq).(*PriorityTask)
                pqp.executeTask(highestPriorityTask)
            }
        }
    }
}

func (pqp *PriorityQueuePool) executeTask(task *PriorityTask) {
    defer func() {
        if r := recover(); r != nil {
            pqp.logger.Error("任务执行恐慌",
                "task_id", task.ID,
                "priority", task.Priority,
                "panic", r)
        }
    }()
    
    start := time.Now()
    task.Task()
    duration := time.Since(start)
    
    pqp.logger.Debug("任务执行完成",
        "task_id", task.ID,
        "priority", task.Priority,
        "duration_ms", duration.Milliseconds())
}

// 优先级队列实现
type PriorityQueue []*PriorityTask

func (pq PriorityQueue) Len() int { return len(pq) }
func (pq PriorityQueue) Less(i, j int) bool {
    return pq[i].Priority > pq[j].Priority // 数值越大优先级越高
}
func (pq PriorityQueue) Swap(i, j int) { pq[i], pq[j] = pq[j], pq[i] }

func (pq *PriorityQueue) Push(x interface{}) {
    *pq = append(*pq, x.(*PriorityTask))
}

func (pq *PriorityQueue) Pop() interface{} {
    old := *pq
    n := len(old)
    item := old[n-1]
    *pq = old[0 : n-1]
    return item
}
```

## 协程池监控和统计

### 性能监控

```go
type PoolMonitor struct {
    pool           *ants.Pool
    metrics        *PoolMetrics
    collectInterval time.Duration
    stopChan       chan struct{}
    logger         *slog.Logger
}

type PoolMetrics struct {
    RunningTasks   int64         `json:"running_tasks"`
    TotalTasks     int64         `json:"total_tasks"`
    SuccessTasks   int64         `json:"success_tasks"`
    FailedTasks    int64         `json:"failed_tasks"`
    AverageLatency time.Duration `json:"average_latency"`
    Capacity       int           `json:"capacity"`
    LastUpdated    time.Time     `json:"last_updated"`
}

func NewPoolMonitor(pool *ants.Pool, interval time.Duration, logger *slog.Logger) *PoolMonitor {
    monitor := &PoolMonitor{
        pool:            pool,
        metrics:         &PoolMetrics{},
        collectInterval: interval,
        stopChan:        make(chan struct{}),
        logger:          logger,
    }
    
    go monitor.collectMetrics()
    return monitor
}

func (pm *PoolMonitor) collectMetrics() {
    ticker := time.NewTicker(pm.collectInterval)
    defer ticker.Stop()
    
    for {
        select {
        case <-ticker.C:
            pm.updateMetrics()
        case <-pm.stopChan:
            return
        }
    }
}

func (pm *PoolMonitor) updateMetrics() {
    pm.metrics.RunningTasks = int64(pm.pool.Running())
    pm.metrics.Capacity = pm.pool.Cap()
    pm.metrics.LastUpdated = time.Now()
    
    pm.logger.Debug("协程池统计",
        "running", pm.metrics.RunningTasks,
        "capacity", pm.metrics.Capacity,
        "usage_percent", float64(pm.metrics.RunningTasks)/float64(pm.metrics.Capacity)*100)
}

func (pm *PoolMonitor) GetMetrics() *PoolMetrics {
    return &PoolMetrics{
        RunningTasks:   atomic.LoadInt64(&pm.metrics.RunningTasks),
        TotalTasks:     atomic.LoadInt64(&pm.metrics.TotalTasks),
        SuccessTasks:   atomic.LoadInt64(&pm.metrics.SuccessTasks),
        FailedTasks:    atomic.LoadInt64(&pm.metrics.FailedTasks),
        AverageLatency: time.Duration(atomic.LoadInt64((*int64)(&pm.metrics.AverageLatency))),
        Capacity:       pm.metrics.Capacity,
        LastUpdated:    pm.metrics.LastUpdated,
    }
}

func (pm *PoolMonitor) Stop() {
    close(pm.stopChan)
}
```

### 任务执行跟踪

```go
type TrackedTask struct {
    ID        string        `json:"id"`
    StartTime time.Time     `json:"start_time"`
    EndTime   time.Time     `json:"end_time"`
    Duration  time.Duration `json:"duration"`
    Status    string        `json:"status"` // running, completed, failed
    Error     string        `json:"error,omitempty"`
}

type TaskTracker struct {
    tasks     map[string]*TrackedTask
    mutex     sync.RWMutex
    maxTasks  int
    logger    *slog.Logger
}

func NewTaskTracker(maxTasks int, logger *slog.Logger) *TaskTracker {
    return &TaskTracker{
        tasks:    make(map[string]*TrackedTask),
        maxTasks: maxTasks,
        logger:   logger,
    }
}

func (tt *TaskTracker) TrackTask(taskID string, task func() error) func() error {
    return func() error {
        trackedTask := &TrackedTask{
            ID:        taskID,
            StartTime: time.Now(),
            Status:    "running",
        }
        
        tt.addTask(trackedTask)
        defer tt.removeTask(taskID)
        
        err := task()
        
        trackedTask.EndTime = time.Now()
        trackedTask.Duration = trackedTask.EndTime.Sub(trackedTask.StartTime)
        
        if err != nil {
            trackedTask.Status = "failed"
            trackedTask.Error = err.Error()
            tt.logger.Error("任务执行失败",
                "task_id", taskID,
                "duration_ms", trackedTask.Duration.Milliseconds(),
                "error", err)
        } else {
            trackedTask.Status = "completed"
            tt.logger.Debug("任务执行成功",
                "task_id", taskID,
                "duration_ms", trackedTask.Duration.Milliseconds())
        }
        
        return err
    }
}

func (tt *TaskTracker) addTask(task *TrackedTask) {
    tt.mutex.Lock()
    defer tt.mutex.Unlock()
    
    // 清理旧任务以控制内存使用
    if len(tt.tasks) >= tt.maxTasks {
        tt.cleanupOldTasks()
    }
    
    tt.tasks[task.ID] = task
}

func (tt *TaskTracker) removeTask(taskID string) {
    tt.mutex.Lock()
    defer tt.mutex.Unlock()
    delete(tt.tasks, taskID)
}

func (tt *TaskTracker) cleanupOldTasks() {
    cutoff := time.Now().Add(-10 * time.Minute) // 保留10分钟内的任务
    for id, task := range tt.tasks {
        if task.StartTime.Before(cutoff) {
            delete(tt.tasks, id)
        }
    }
}

func (tt *TaskTracker) GetRunningTasks() []*TrackedTask {
    tt.mutex.RLock()
    defer tt.mutex.RUnlock()
    
    var running []*TrackedTask
    for _, task := range tt.tasks {
        if task.Status == "running" {
            running = append(running, task)
        }
    }
    
    return running
}

func (tt *TaskTracker) GetTaskHistory(limit int) []*TrackedTask {
    tt.mutex.RLock()
    defer tt.mutex.RUnlock()
    
    tasks := make([]*TrackedTask, 0, len(tt.tasks))
    for _, task := range tt.tasks {
        tasks = append(tasks, task)
    }
    
    // 按开始时间排序
    sort.Slice(tasks, func(i, j int) bool {
        return tasks[i].StartTime.After(tasks[j].StartTime)
    })
    
    if len(tasks) > limit {
        tasks = tasks[:limit]
    }
    
    return tasks
}
```

## 最佳实践

### 1. 协程池配置优化

```go
// ✅ 推荐的协程池配置
func optimalPoolConfig() *ants.Options {
    return &ants.Options{
        ExpiryDuration:   10 * time.Second,  // 合理的过期时间
        PreAlloc:         true,              // 预分配提升性能
        MaxBlockingTasks: 10000,             // 限制阻塞任务数
        Nonblocking:      false,             // 阻塞模式避免任务丢失
    }
}

// ❌ 避免的配置问题
func problematicPoolConfig() *ants.Options {
    return &ants.Options{
        ExpiryDuration:   0,        // 不设置过期时间会导致内存泄漏
        PreAlloc:         false,    // 不预分配影响性能
        MaxBlockingTasks: 0,        // 无限制可能导致内存溢出
        Nonblocking:      true,     // 非阻塞模式可能丢弃任务
    }
}
```

### 2. 资源管理和清理

```go
type ResourceManager struct {
    pools    map[string]*ants.Pool
    trackers map[string]*TaskTracker
    mutex    sync.RWMutex
    logger   *slog.Logger
}

func NewResourceManager(logger *slog.Logger) *ResourceManager {
    return &ResourceManager{
        pools:    make(map[string]*ants.Pool),
        trackers: make(map[string]*TaskTracker),
        logger:   logger,
    }
}

func (rm *ResourceManager) CreatePool(name string, size int) (*ants.Pool, error) {
    rm.mutex.Lock()
    defer rm.mutex.Unlock()
    
    if _, exists := rm.pools[name]; exists {
        return nil, fmt.Errorf("协程池 %s 已存在", name)
    }
    
    pool, err := ants.NewPool(size)
    if err != nil {
        return nil, fmt.Errorf("创建协程池失败: %w", err)
    }
    
    rm.pools[name] = pool
    rm.trackers[name] = NewTaskTracker(1000, rm.logger)
    
    rm.logger.Info("创建协程池", "name", name, "size", size)
    return pool, nil
}

func (rm *ResourceManager) DestroyPool(name string) error {
    rm.mutex.Lock()
    defer rm.mutex.Unlock()
    
    pool, exists := rm.pools[name]
    if !exists {
        return fmt.Errorf("协程池 %s 不存在", name)
    }
    
    // 优雅关闭协程池
    pool.Release()
    
    delete(rm.pools, name)
    delete(rm.trackers, name)
    
    rm.logger.Info("销毁协程池", "name", name)
    return nil
}

func (rm *ResourceManager) GracefulShutdown(timeout time.Duration) {
    rm.mutex.Lock()
    defer rm.mutex.Unlock()
    
    ctx, cancel := context.WithTimeout(context.Background(), timeout)
    defer cancel()
    
    var wg sync.WaitGroup
    
    for name, pool := range rm.pools {
        wg.Add(1)
        go func(name string, pool *ants.Pool) {
            defer wg.Done()
            
            rm.logger.Info("关闭协程池", "name", name)
            pool.Release()
        }(name, pool)
    }
    
    wg.Wait()
    
    // 清理所有资源
    rm.pools = make(map[string]*ants.Pool)
    rm.trackers = make(map[string]*TaskTracker)
    
    rm.logger.Info("所有协程池已关闭")
}
```

### 3. 错误处理和恢复

```go
type ResilientTaskExecutor struct {
    pool        *ants.Pool
    maxRetries  int
    retryDelay  time.Duration
    errorLogger *slog.Logger
}

func NewResilientTaskExecutor(pool *ants.Pool, maxRetries int, retryDelay time.Duration, logger *slog.Logger) *ResilientTaskExecutor {
    return &ResilientTaskExecutor{
        pool:        pool,
        maxRetries:  maxRetries,
        retryDelay:  retryDelay,
        errorLogger: logger,
    }
}

func (rte *ResilientTaskExecutor) ExecuteWithRetry(task func() error, taskID string) error {
    var lastErr error
    
    for attempt := 0; attempt <= rte.maxRetries; attempt++ {
        err := rte.executeTask(task, taskID)
        if err == nil {
            return nil
        }
        
        lastErr = err
        rte.errorLogger.Warn("任务执行失败，准备重试",
            "task_id", taskID,
            "attempt", attempt+1,
            "max_retries", rte.maxRetries,
            "error", err)
        
        if attempt < rte.maxRetries {
            time.Sleep(rte.retryDelay * time.Duration(attempt+1)) // 指数退避
        }
    }
    
    rte.errorLogger.Error("任务执行最终失败",
        "task_id", taskID,
        "attempts", rte.maxRetries+1,
        "final_error", lastErr)
    
    return fmt.Errorf("任务 %s 执行失败，已重试 %d 次: %w", taskID, rte.maxRetries, lastErr)
}

func (rte *ResilientTaskExecutor) executeTask(task func() error, taskID string) error {
    done := make(chan error, 1)
    
    err := rte.pool.Submit(func() {
        defer func() {
            if r := recover(); r != nil {
                done <- fmt.Errorf("任务恐慌: %v", r)
            }
        }()
        
        done <- task()
    })
    
    if err != nil {
        return fmt.Errorf("提交任务到协程池失败: %w", err)
    }
    
    select {
    case err := <-done:
        return err
    case <-time.After(30 * time.Second): // 任务超时
        return fmt.Errorf("任务执行超时")
    }
}
```

## 故障排除

### 常见协程池问题

1. **协程池耗尽**
   ```go
   // 监控协程池状态
   func monitorPoolExhaustion(pool *ants.Pool, threshold float64) {
       ticker := time.NewTicker(5 * time.Second)
       defer ticker.Stop()
       
       for range ticker.C {
           running := pool.Running()
           capacity := pool.Cap()
           usage := float64(running) / float64(capacity)
           
           if usage > threshold {
               log.Printf("警告: 协程池使用率过高 %.2f%% (运行中: %d, 容量: %d)",
                   usage*100, running, capacity)
           }
       }
   }
   ```

2. **任务堆积问题**
   ```go
   // 实现任务队列监控
   func monitorTaskQueue(queue chan interface{}, maxSize int) {
       ticker := time.NewTicker(1 * time.Second)
       defer ticker.Stop()
       
       for range ticker.C {
           queueLen := len(queue)
           usage := float64(queueLen) / float64(maxSize)
           
           if usage > 0.8 {
               log.Printf("警告: 任务队列使用率 %.2f%% (%d/%d)",
                   usage*100, queueLen, maxSize)
           }
       }
   }
   ```

3. **内存泄漏检测**
   ```go
   // 定期内存使用监控
   func monitorMemoryUsage() {
       ticker := time.NewTicker(30 * time.Second)
       defer ticker.Stop()
       
       var m runtime.MemStats
       for range ticker.C {
           runtime.ReadMemStats(&m)
           
           // 记录内存使用情况
           log.Printf("内存使用: Alloc=%.1fMB Sys=%.1fMB Goroutines=%d",
               float64(m.Alloc)/1024/1024,
               float64(m.Sys)/1024/1024,
               runtime.NumGoroutine())
           
           // 检查异常增长
           if m.Alloc > 1024*1024*1024 { // 1GB
               log.Printf("警告: 内存使用超过1GB")
           }
       }
   }
   ```