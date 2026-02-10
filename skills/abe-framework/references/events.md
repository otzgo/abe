# 事件驱动系统

## 事件系统概述

ABE 框架提供了内置的事件总线机制，基于 Watermill 库实现，支持进程内的消息发布和订阅功能，用于组件间的松耦合异步通信。

## 配置选项

### 事件系统配置

在 `config.yaml` 中配置事件系统：

```yaml
event:
  output_buffer: 128  # 输出缓冲区大小，默认为64
```

## 基本使用方法

### 获取事件总线

```go
func main() {
    engine := abe.NewEngine()
    
    // 获取事件总线实例
    eventBus := engine.EventBus()
    
    // 启动事件监听器
    startEventListeners(context.Background(), eventBus)
    
    engine.Run()
}

func startEventListeners(ctx context.Context, eventBus abe.EventBus) {
    // 启动各种事件监听器
    go listenUserEvents(ctx, eventBus)
    go listenOrderEvents(ctx, eventBus)
    go listenSystemEvents(ctx, eventBus)
}
```

### 创建和发布事件

```go
// 定义事件数据结构
type UserRegisteredEvent struct {
    UserID   uint      `json:"user_id"`
    Username string    `json:"username"`
    Email    string    `json:"email"`
    RegisterTime time.Time `json:"register_time"`
}

type OrderCreatedEvent struct {
    OrderID    uint      `json:"order_id"`
    UserID     uint      `json:"user_id"`
    Amount     float64   `json:"amount"`
    Items      []OrderItem `json:"items"`
    CreateTime time.Time `json:"create_time"`
}

// 发布事件
func publishUserRegistered(eventBus abe.EventBus, user *User) error {
    event := UserRegisteredEvent{
        UserID:       user.ID,
        Username:     user.Username,
        Email:        user.Email,
        RegisterTime: time.Now(),
    }
    
    payload, err := json.Marshal(event)
    if err != nil {
        return fmt.Errorf("序列化事件失败: %w", err)
    }
    
    message := abe.NewMessage(payload)
    return eventBus.Publish("user.registered", message)
}

func publishOrderCreated(eventBus abe.EventBus, order *Order) error {
    event := OrderCreatedEvent{
        OrderID:    order.ID,
        UserID:     order.UserID,
        Amount:     order.Amount,
        Items:      order.Items,
        CreateTime: time.Now(),
    }
    
    payload, err := json.Marshal(event)
    if err != nil {
        return fmt.Errorf("序列化事件失败: %w", err)
    }
    
    message := abe.NewMessage(payload)
    return eventBus.Publish("order.created", message)
}
```

## 事件订阅和处理

### 基本订阅模式

```go
func listenUserEvents(ctx context.Context, eventBus abe.EventBus) {
    // 订阅用户相关事件
    eventChan, err := eventBus.Subscribe(ctx, "user.registered")
    if err != nil {
        log.Printf("订阅用户注册事件失败: %v", err)
        return
    }
    
    for {
        select {
        case event := <-eventChan:
            handleUserRegistered(event)
        case <-ctx.Done():
            return
        }
    }
}

func handleUserRegistered(event abe.EventMessage) {
    defer event.Ack() // 确认消息处理完成
    
    var userEvent UserRegisteredEvent
    if err := json.Unmarshal(event.Payload(), &userEvent); err != nil {
        log.Printf("解析用户注册事件失败: %v", err)
        event.Nack() // 处理失败，拒绝消息
        return
    }
    
    // 处理业务逻辑
    processNewUserRegistration(&userEvent)
}

func processNewUserRegistration(event *UserRegisteredEvent) {
    log.Printf("处理新用户注册: 用户ID=%d, 用户名=%s", 
        event.UserID, event.Username)
    
    // 发送欢迎邮件
    sendWelcomeEmail(event.Email, event.Username)
    
    // 创建用户档案
    createUserProfile(event.UserID)
    
    // 记录注册统计
    recordRegistrationStats(event.RegisterTime)
}
```

### 多事件订阅

```go
func listenOrderEvents(ctx context.Context, eventBus abe.EventBus) {
    topics := []string{"order.created", "order.updated", "order.cancelled"}
    
    for _, topic := range topics {
        go func(topic string) {
            eventChan, err := eventBus.Subscribe(ctx, topic)
            if err != nil {
                log.Printf("订阅订单事件 %s 失败: %v", topic, err)
                return
            }
            
            for event := range eventChan {
                handleOrderEvent(topic, event)
            }
        }(topic)
    }
}

func handleOrderEvent(topic string, event abe.EventMessage) {
    defer func() {
        if r := recover(); r != nil {
            log.Printf("处理订单事件恐慌: %v", r)
            event.Nack()
        } else {
            event.Ack()
        }
    }()
    
    switch topic {
    case "order.created":
        handleOrderCreated(event)
    case "order.updated":
        handleOrderUpdated(event)
    case "order.cancelled":
        handleOrderCancelled(event)
    }
}
```

## 高级事件处理

### 批量事件处理

```go
type BatchEventHandler struct {
    batchSize int
    buffer    []abe.EventMessage
    flushChan chan struct{}
    processor EventProcessor
}

func NewBatchEventHandler(batchSize int, processor EventProcessor) *BatchEventHandler {
    handler := &BatchEventHandler{
        batchSize: batchSize,
        buffer:    make([]abe.EventMessage, 0, batchSize),
        flushChan: make(chan struct{}, 1),
        processor: processor,
    }
    
    // 启动批量处理协程
    go handler.batchProcessor()
    
    return handler
}

func (beh *BatchEventHandler) HandleEvent(event abe.EventMessage) {
    beh.buffer = append(beh.buffer, event)
    
    // 达到批次大小时触发处理
    if len(beh.buffer) >= beh.batchSize {
        select {
        case beh.flushChan <- struct{}{}:
        default:
        }
    }
}

func (beh *BatchEventHandler) batchProcessor() {
    ticker := time.NewTicker(5 * time.Second) // 最大等待时间
    defer ticker.Stop()
    
    for {
        select {
        case <-beh.flushChan:
            beh.processBatch()
        case <-ticker.C:
            if len(beh.buffer) > 0 {
                beh.processBatch()
            }
        }
    }
}

func (beh *BatchEventHandler) processBatch() {
    if len(beh.buffer) == 0 {
        return
    }
    
    // 处理批量事件
    events := make([]interface{}, len(beh.buffer))
    for i, event := range beh.buffer {
        var orderEvent OrderCreatedEvent
        json.Unmarshal(event.Payload(), &orderEvent)
        events[i] = orderEvent
    }
    
    // 批量处理业务逻辑
    if err := beh.processor.ProcessBatch(events); err != nil {
        log.Printf("批量处理失败: %v", err)
        // nack 所有消息
        for _, event := range beh.buffer {
            event.Nack()
        }
    } else {
        // ack 所有消息
        for _, event := range beh.buffer {
            event.Ack()
        }
    }
    
    // 清空缓冲区
    beh.buffer = beh.buffer[:0]
}
```

### 事件过滤和路由

```go
type EventRouter struct {
    routes map[string]EventHandler
    filters map[string][]EventFilter
}

type EventHandler func(abe.EventMessage) error
type EventFilter func(abe.EventMessage) bool

func NewEventRouter() *EventRouter {
    return &EventRouter{
        routes:  make(map[string]EventHandler),
        filters: make(map[string][]EventFilter),
    }
}

func (er *EventRouter) AddRoute(topic string, handler EventHandler) {
    er.routes[topic] = handler
}

func (er *EventRouter) AddFilter(topic string, filter EventFilter) {
    er.filters[topic] = append(er.filters[topic], filter)
}

func (er *EventRouter) HandleEvent(topic string, event abe.EventMessage) {
    // 应用过滤器
    if filters, exists := er.filters[topic]; exists {
        for _, filter := range filters {
            if !filter(event) {
                event.Ack() // 过滤掉的消息直接确认
                return
            }
        }
    }
    
    // 处理事件
    if handler, exists := er.routes[topic]; exists {
        if err := handler(event); err != nil {
            log.Printf("处理事件失败 %s: %v", topic, err)
            event.Nack()
        } else {
            event.Ack()
        }
    } else {
        log.Printf("未找到事件处理器: %s", topic)
        event.Nack()
    }
}

// 使用示例
func setupEventRouting(eventBus abe.EventBus) {
    router := NewEventRouter()
    
    // 添加处理器
    router.AddRoute("user.registered", handleUserRegisteredEvent)
    router.AddRoute("order.created", handleOrderCreatedEvent)
    
    // 添加过滤器
    router.AddFilter("order.created", func(event abe.EventMessage) bool {
        var orderEvent OrderCreatedEvent
        json.Unmarshal(event.Payload(), &orderEvent)
        // 只处理金额大于100的订单
        return orderEvent.Amount > 100
    })
    
    // 启动监听
    go func() {
        ctx := context.Background()
        eventChan, _ := eventBus.Subscribe(ctx, "user.registered")
        for event := range eventChan {
            router.HandleEvent("user.registered", event)
        }
    }()
}
```

## 事件持久化和可靠性

### 事件存储

```go
type EventStore struct {
    db *gorm.DB
}

type StoredEvent struct {
    ID        uint      `gorm:"primaryKey" json:"id"`
    Topic     string    `gorm:"index" json:"topic"`
    Payload   string    `json:"payload"`
    Status    string    `gorm:"size:20;default:'pending'" json:"status"`
    RetryCount int      `gorm:"default:0" json:"retry_count"`
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
}

func (es *EventStore) StoreEvent(topic string, payload []byte) error {
    event := &StoredEvent{
        Topic:   topic,
        Payload: string(payload),
        Status:  "pending",
    }
    
    return es.db.Create(event).Error
}

func (es *EventStore) MarkEventProcessed(eventID uint) error {
    return es.db.Model(&StoredEvent{}).
        Where("id = ?", eventID).
        Update("status", "processed").Error
}

func (es *EventStore) MarkEventFailed(eventID uint, errMsg string) error {
    return es.db.Model(&StoredEvent{}).
        Where("id = ?", eventID).
        Updates(map[string]interface{}{
            "status":      "failed",
            "retry_count": gorm.Expr("retry_count + 1"),
            "error_msg":   errMsg,
        }).Error
}
```

### 可靠事件发布

```go
type ReliableEventPublisher struct {
    eventBus   abe.EventBus
    eventStore *EventStore
    logger     *slog.Logger
}

func (rep *ReliableEventPublisher) Publish(topic string, event interface{}) error {
    payload, err := json.Marshal(event)
    if err != nil {
        return fmt.Errorf("序列化事件失败: %w", err)
    }
    
    // 先存储事件
    if err := rep.eventStore.StoreEvent(topic, payload); err != nil {
        return fmt.Errorf("存储事件失败: %w", err)
    }
    
    // 发布事件
    message := abe.NewMessage(payload)
    if err := rep.eventBus.Publish(topic, message); err != nil {
        rep.logger.Error("发布事件失败", "topic", topic, "error", err)
        return fmt.Errorf("发布事件失败: %w", err)
    }
    
    return nil
}

func (rep *ReliableEventPublisher) PublishWithRetry(topic string, event interface{}, maxRetries int) error {
    for i := 0; i <= maxRetries; i++ {
        err := rep.Publish(topic, event)
        if err == nil {
            return nil
        }
        
        if i < maxRetries {
            waitTime := time.Duration(i+1) * time.Second
            time.Sleep(waitTime)
            rep.logger.Info("重试发布事件", "topic", topic, "attempt", i+1)
        }
    }
    
    return fmt.Errorf("发布事件失败，已重试 %d 次", maxRetries)
}
```

## 事件监控和管理

### 事件统计和监控

```go
type EventMetrics struct {
    TotalEvents     int64         `json:"total_events"`
    SuccessEvents   int64         `json:"success_events"`
    FailedEvents    int64         `json:"failed_events"`
    AverageLatency  time.Duration `json:"average_latency"`
    Topics          map[string]int64 `json:"topics"`
}

type EventMonitor struct {
    metrics    *EventMetrics
    mutex      sync.RWMutex
    startTime  time.Time
    eventStore *EventStore
}

func NewEventMonitor(eventStore *EventStore) *EventMonitor {
    return &EventMonitor{
        metrics:    &EventMetrics{Topics: make(map[string]int64)},
        startTime:  time.Now(),
        eventStore: eventStore,
    }
}

func (em *EventMonitor) RecordEvent(topic string, success bool, latency time.Duration) {
    em.mutex.Lock()
    defer em.mutex.Unlock()
    
    em.metrics.TotalEvents++
    em.metrics.Topics[topic]++
    
    if success {
        em.metrics.SuccessEvents++
    } else {
        em.metrics.FailedEvents++
    }
    
    // 更新平均延迟
    totalLatency := int64(em.metrics.AverageLatency)*em.metrics.TotalEvents + int64(latency)
    em.metrics.AverageLatency = time.Duration(totalLatency / (em.metrics.TotalEvents + 1))
}

func (em *EventMonitor) GetMetrics() *EventMetrics {
    em.mutex.RLock()
    defer em.mutex.RUnlock()
    
    // 返回副本
    metrics := *em.metrics
    topics := make(map[string]int64)
    for k, v := range em.metrics.Topics {
        topics[k] = v
    }
    metrics.Topics = topics
    
    return &metrics
}
```

### 事件死信队列

```go
type DeadLetterQueue struct {
    db     *gorm.DB
    logger *slog.Logger
}

type DeadLetterEvent struct {
    ID        uint      `gorm:"primaryKey"`
    Topic     string    `json:"topic"`
    Payload   string    `json:"payload"`
    ErrorMsg  string    `json:"error_msg"`
    Failures  int       `json:"failures"`
    CreatedAt time.Time `json:"created_at"`
}

func (dlq *DeadLetterQueue) AddToDLQ(topic string, payload []byte, errorMsg string, failures int) error {
    deadEvent := &DeadLetterEvent{
        Topic:    topic,
        Payload:  string(payload),
        ErrorMsg: errorMsg,
        Failures: failures,
        CreatedAt: time.Now(),
    }
    
    if err := dlq.db.Create(deadEvent).Error; err != nil {
        dlq.logger.Error("添加死信事件失败", "error", err)
        return err
    }
    
    dlq.logger.Warn("事件进入死信队列",
        "topic", topic,
        "failures", failures,
        "error", errorMsg)
    
    return nil
}

func (dlq *DeadLetterQueue) ProcessDLQ(maxRetries int) {
    var deadEvents []DeadLetterEvent
    dlq.db.Where("failures < ?", maxRetries).
        Order("created_at ASC").
        Limit(100).
        Find(&deadEvents)
    
    for _, deadEvent := range deadEvents {
        // 尝试重新处理
        if err := dlq.retryEvent(&deadEvent); err != nil {
            // 更新失败次数
            dlq.db.Model(&deadEvent).Update("failures", gorm.Expr("failures + 1"))
        } else {
            // 处理成功，从死信队列删除
            dlq.db.Delete(&deadEvent)
        }
    }
}

func (dlq *DeadLetterQueue) retryEvent(deadEvent *DeadLetterEvent) error {
    // 实现重试逻辑
    // 可以重新发布事件或者调用特定的处理函数
    return nil
}
```

## 最佳实践

### 1. 事件设计原则

```go
// ✅ 推荐的事件设计
type UserActivityCreated struct {
    UserID      uint      `json:"user_id"`
    ActivityType string   `json:"activity_type"`
    Details     map[string]interface{} `json:"details"`
    Timestamp   time.Time `json:"timestamp"`
    CorrelationID string  `json:"correlation_id"` // 用于追踪相关事件
}

// ❌ 避免的设计问题
type BadEvent struct {
    // 避免使用复杂嵌套结构
    UserData    User      `json:"user_data"`  // 应该扁平化
    // 避免包含敏感信息
    Password    string    `json:"password"`   // 危险！
    // 避免包含数据库实体
    DBEntity    *User     `json:"db_entity"`  // 不应该包含指针
}
```

### 2. 事件版本管理

```go
type EventVersion string

const (
    Version1 EventVersion = "v1"
    Version2 EventVersion = "v2"
)

type VersionedEvent struct {
    Version EventVersion    `json:"version"`
    Type    string         `json:"type"`
    Data    interface{}    `json:"data"`
    Meta    EventMetadata  `json:"meta"`
}

type EventMetadata struct {
    Timestamp     time.Time `json:"timestamp"`
    CorrelationID string    `json:"correlation_id"`
    Source        string    `json:"source"`
}

// 事件演化示例
func evolveUserEvent(oldEvent map[string]interface{}) *VersionedEvent {
    // 检查版本并进行转换
    version, _ := oldEvent["version"].(string)
    
    switch EventVersion(version) {
    case Version1:
        return convertV1ToV2(oldEvent)
    default:
        return &VersionedEvent{
            Version: Version2,
            Type:    "user.activity",
            Data:    oldEvent,
            Meta: EventMetadata{
                Timestamp: time.Now(),
                Source:    "event_converter",
            },
        }
    }
}
```

### 3. 事件幂等性处理

```go
type IdempotentEventHandler struct {
    processedEvents map[string]bool
    mutex          sync.RWMutex
    storage        *gorm.DB
}

func (ieh *IdempotentEventHandler) HandleEvent(event abe.EventMessage) error {
    // 提取事件ID
    eventID := extractEventID(event)
    
    // 检查是否已处理
    if ieh.isProcessed(eventID) {
        event.Ack()
        return nil
    }
    
    // 处理事件
    if err := ieh.processEvent(event); err != nil {
        return err
    }
    
    // 标记为已处理
    ieh.markProcessed(eventID)
    event.Ack()
    
    return nil
}

func (ieh *IdempotentEventHandler) isProcessed(eventID string) bool {
    ieh.mutex.RLock()
    defer ieh.mutex.RUnlock()
    
    if processed, exists := ieh.processedEvents[eventID]; exists {
        return processed
    }
    
    // 检查持久化存储
    var count int64
    ieh.storage.Model(&ProcessedEvent{}).
        Where("event_id = ?", eventID).
        Count(&count)
    
    return count > 0
}

func (ieh *IdempotentEventHandler) markProcessed(eventID string) {
    ieh.mutex.Lock()
    defer ieh.mutex.Unlock()
    
    ieh.processedEvents[eventID] = true
    
    // 持久化到数据库
    processedEvent := &ProcessedEvent{
        EventID:   eventID,
        Processed: time.Now(),
    }
    
    ieh.storage.Create(processedEvent)
}
```

## 故障排除

### 常见事件问题

1. **事件丢失问题**
   ```go
   // 实现事件确认机制
   func reliableEventHandler(eventBus abe.EventBus) {
       ctx := context.Background()
       eventChan, err := eventBus.Subscribe(ctx, "important.topic")
       if err != nil {
           log.Fatal("订阅重要事件失败:", err)
       }
       
       for event := range eventChan {
           // 使用事务确保处理和确认的原子性
           if err := processWithTransaction(event); err != nil {
               log.Printf("处理事件失败: %v", err)
               event.Nack()
           } else {
               event.Ack()
           }
       }
   }
   ```

2. **事件处理积压**
   ```go
   // 实现背压控制
   func backpressureHandler(eventBus abe.EventBus, maxConcurrency int) {
       semaphore := make(chan struct{}, maxConcurrency)
       
       ctx := context.Background()
       eventChan, _ := eventBus.Subscribe(ctx, "busy.topic")
       
       for event := range eventChan {
           semaphore <- struct{}{} // 获取许可
           
           go func(e abe.EventMessage) {
               defer func() { <-semaphore }() // 释放许可
               
               // 处理事件
               processEvent(e)
           }(event)
       }
   }
   ```

3. **事件循环问题**
   ```go
   // 避免事件处理过程中再次发布相同类型的事件
   func safeEventProcessing(eventBus abe.EventBus) {
       processingEvents := make(map[string]bool)
       mutex := sync.Mutex{}
       
       handler := func(event abe.EventMessage) {
           eventID := extractUniqueID(event)
           
           mutex.Lock()
           if processingEvents[eventID] {
               mutex.Unlock()
               event.Ack() // 避免重复处理
               return
           }
           processingEvents[eventID] = true
           mutex.Unlock()
           
           defer func() {
               mutex.Lock()
               delete(processingEvents, eventID)
               mutex.Unlock()
           }()
           
           // 安全处理事件
           processEvent(event)
           event.Ack()
       }
   }
   ```