package abe

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/ThreeDotsLabs/watermill/pubsub/gochannel"
)

// PublishEvent 发布事件。
func PublishEvent[T any](bus EventBus, topic string, event T) error {
	// 这里使用 context.Background() 作为默认上下文。
	ctx := context.Background()
	return publish(ctx, bus, topic, event, JSONCodec[T]{})
}

// SubscribeEvent 订阅事件。
func SubscribeEvent[T any](ctx context.Context, bus EventBus, topic string, handler func(context.Context, T) error, opts ...SubscribeOption) (*Subscription, error) {
	return subscribe(ctx, bus, topic, JSONCodec[T]{}, handler, opts...)
}

// Codec 为事件编解码抽象，通过泛型保证类型安全。
// 用户可自定义实现（如 JSON / MsgPack / Protobuf）。
type Codec[T any] interface {
	Encode(T) ([]byte, error)
	Decode([]byte) (T, error)
}

// JSONCodec 默认的 JSON 编解码器。
type JSONCodec[T any] struct{}

func (c JSONCodec[T]) Encode(v T) ([]byte, error) {
	return json.Marshal(v)
}

func (c JSONCodec[T]) Decode(b []byte) (T, error) {
	var v T
	err := json.Unmarshal(b, &v)
	return v, err
}

// Subscription 表示一次订阅，可用于取消订阅与等待处理结束。
type Subscription struct {
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// Unsubscribe 取消订阅并等待内部处理退出。
func (s *Subscription) Unsubscribe() {
	if s == nil {
		return
	}
	s.cancel()
	s.wg.Wait()
}

// SubscribeOption 订阅配置项。
type SubscribeOption func(*subscribeConfig)

type subscribeConfig struct {
	concurrency int // 处理并发度（消费协程数）
}

// WithConcurrency 设置订阅处理并发度，默认为 1。
func WithConcurrency(n int) SubscribeOption {
	return func(c *subscribeConfig) {
		if n <= 0 {
			n = 1
		}
		c.concurrency = n
	}
}

// EventBus 为事件总线的抽象接口，按消息层面暴露能力。
// 通过泛型辅助函数提供类型安全的 publish/subscribe。
type EventBus interface {
	Publish(ctx context.Context, topic string, msg *message.Message) error
	Subscribe(ctx context.Context, topic string, handler func(context.Context, *message.Message) error, opts ...SubscribeOption) (*Subscription, error)
	Close() error
}

// goChannelBus 基于 Watermill GoChannel 的进程内事件总线实现。
type goChannelBus struct {
	ps     *gochannel.GoChannel
	logger watermill.LoggerAdapter
}

// newGoChannelBus 创建一个基于 GoChannel 的事件总线。
// 可传入自定义 gochannel.Config 与 logger；传 nil 则使用默认。
func newGoChannelBus(cfg *gochannel.Config, logger watermill.LoggerAdapter) *goChannelBus {
	if logger == nil {
		logger = watermill.NewStdLogger(false, false)
	}
	var c gochannel.Config
	if cfg != nil {
		c = *cfg
	}
	ps := gochannel.NewGoChannel(c, logger)
	return &goChannelBus{ps: ps, logger: logger}
}

func newGoChannelConfig() *gochannel.Config {
	return &gochannel.Config{
		OutputChannelBuffer: 10,
	}
}

func newGoChannelLogger(logger *slog.Logger) watermill.LoggerAdapter {
	return newSlogLoggerAdapter(logger)
}

// Publish 发布原始消息。
func (b *goChannelBus) Publish(_ context.Context, topic string, msg *message.Message) error {
	// GoChannel Publisher 不使用 ctx，这里直接发布。
	return b.ps.Publish(topic, msg)
}

// Subscribe 订阅原始消息并按并发度处理。
func (b *goChannelBus) Subscribe(ctx context.Context, topic string, handler func(context.Context, *message.Message) error, opts ...SubscribeOption) (*Subscription, error) {
	cfg := &subscribeConfig{concurrency: 1}
	for _, o := range opts {
		o(cfg)
	}

	// 每个订阅都有独立可取消的上下文，便于 Unsubscribe。
	ctxSub, cancel := context.WithCancel(ctx)
	ch, err := b.ps.Subscribe(ctxSub, topic)
	if err != nil {
		cancel()
		return nil, err
	}

	s := &Subscription{cancel: cancel}
	if cfg.concurrency <= 0 {
		cfg.concurrency = 1
	}

	// 启动并发处理协程。
	for i := 0; i < cfg.concurrency; i++ {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			for {
				select {
				case <-ctxSub.Done():
					return
				case msg, ok := <-ch:
					if !ok {
						return
					}
					if err := handler(ctxSub, msg); err != nil {
						// 处理失败，尝试 Nack（GoChannel 的 Nack 语义为简单重投或忽略，视实现而定）
						msg.Nack()
					} else {
						msg.Ack()
					}
				}
			}
		}()
	}

	return s, nil
}

// Close 关闭底层 Pub/Sub。
func (b *goChannelBus) Close() error {
	return b.ps.Close()
}

// publish 泛型辅助：类型安全的事件发布。
func publish[T any](ctx context.Context, bus EventBus, topic string, event T, codec Codec[T]) error {
	payload, err := codec.Encode(event)
	if err != nil {
		return err
	}
	msg := message.NewMessage(watermill.NewUUID(), payload)
	return bus.Publish(ctx, topic, msg)
}

// subscribe 泛型辅助：类型安全的事件订阅。
func subscribe[T any](ctx context.Context, bus EventBus, topic string, codec Codec[T], handler func(context.Context, T) error, opts ...SubscribeOption) (*Subscription, error) {
	wrap := func(ctx context.Context, msg *message.Message) error {
		v, err := codec.Decode(msg.Payload)
		if err != nil {
			return err
		}
		return handler(ctx, v)
	}
	return bus.Subscribe(ctx, topic, wrap, opts...)
}

// slogAdapter 将 slog.Logger 适配为 Watermill 的 LoggerAdapter。
type slogAdapter struct {
	base   *slog.Logger
	fields watermill.LogFields
}

func newSlogLoggerAdapter(base *slog.Logger) watermill.LoggerAdapter {
	if base == nil {
		base = slog.Default()
	}
	return &slogAdapter{base: base, fields: nil}
}

func (l *slogAdapter) With(fields watermill.LogFields) watermill.LoggerAdapter {
	merged := make(watermill.LogFields, len(l.fields)+len(fields))
	for k, v := range l.fields {
		merged[k] = v
	}
	for k, v := range fields {
		merged[k] = v
	}
	return &slogAdapter{base: l.base, fields: merged}
}

func (l *slogAdapter) Trace(msg string, fields watermill.LogFields) {
	// slog 没有 Trace 级别，映射为 Debug
	l.log(slog.LevelDebug, msg, nil, fields)
}

func (l *slogAdapter) Debug(msg string, fields watermill.LogFields) {
	l.log(slog.LevelDebug, msg, nil, fields)
}

func (l *slogAdapter) Info(msg string, fields watermill.LogFields) {
	l.log(slog.LevelInfo, msg, nil, fields)
}

func (l *slogAdapter) Error(msg string, err error, fields watermill.LogFields) {
	l.log(slog.LevelError, msg, err, fields)
}

func (l *slogAdapter) log(level slog.Level, msg string, err error, fields watermill.LogFields) {
	// 合并字段
	attrs := make([]slog.Attr, 0, len(l.fields)+len(fields)+1)
	for k, v := range l.fields {
		attrs = append(attrs, slog.Any(k, v))
	}
	for k, v := range fields {
		attrs = append(attrs, slog.Any(k, v))
	}
	if err != nil {
		attrs = append(attrs, slog.String("error", err.Error()))
	}

	logger := l.base
	if len(attrs) > 0 {
		// 将 []slog.Attr 转换为 []any
		args := make([]any, len(attrs))
		for i, attr := range attrs {
			args[i] = attr
		}
		logger = l.base.With(args...)
	}

	switch level {
	case slog.LevelDebug:
		logger.Debug(msg)
	case slog.LevelInfo:
		logger.Info(msg)
	case slog.LevelError:
		logger.Error(msg)
	default:
		logger.Log(context.Background(), level, msg)
	}
}
