package abe

import (
	"context"
	"log/slog"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/ThreeDotsLabs/watermill/pubsub/gochannel"
	"github.com/spf13/viper"
)

// EventMessage 消息结构体
type EventMessage struct {
	msg *message.Message
}

// NewMessage 创建一个新的 EventMessage 实例。
func NewMessage(payload []byte) *EventMessage {
	return &EventMessage{msg: message.NewMessage(watermill.NewUUID(), payload)}
}

// Payload 返回消息的有效载荷（Payload）。
func (m *EventMessage) Payload() []byte {
	return m.msg.Payload
}

// Ack 确认消息，用于成功处理消息。
func (m *EventMessage) Ack() bool {
	return m.msg.Ack()
}

// Acked 确认通道，当消息被成功处理时关闭。
func (m *EventMessage) Acked() <-chan struct{} {
	return m.msg.Acked()
}

// Nack 负确认消息，用于拒绝处理消息。
func (m *EventMessage) Nack() bool {
	return m.msg.Nack()
}

// Nacked 负确认通道，当消息被拒绝时关闭。
func (m *EventMessage) Nacked() <-chan struct{} {
	return m.msg.Nacked()
}

func (m *EventMessage) UUID() string {
	return m.msg.UUID
}

// EventBus 为事件总线的抽象接口，按消息层面暴露能力。
// 通过泛型辅助函数提供类型安全的 publish/subscribe。
type EventBus interface {
	Publish(topic string, messages ...*EventMessage) error
	Subscribe(ctx context.Context, topic string) (<-chan *EventMessage, error)
	close() error
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

func newGoChannelConfig(config *viper.Viper) *gochannel.Config {
	// 从配置读取缓冲大小，优先级：环境变量/CLI Flag/配置文件 > 默认值
	var buf int64 = 0
	if config != nil {
		buf = int64(config.GetInt("event.output_buffer"))
	}
	if buf <= 0 {
		buf = 64
	}
	return &gochannel.Config{
		OutputChannelBuffer: buf,
	}
}

func newGoChannelLogger(logger *slog.Logger) watermill.LoggerAdapter {
	return newSlogLoggerAdapter(logger)
}

// Publish 发布原始消息。
func (b *goChannelBus) Publish(topic string, msg ...*EventMessage) error {
	// GoChannel Publisher 不使用 ctx，这里直接发布。
	// 将 EventMessage 转换为 message.Message
	msgWatermill := make([]*message.Message, len(msg))
	for i, m := range msg {
		msgWatermill[i] = m.msg
	}
	return b.ps.Publish(topic, msgWatermill...)
}

// Subscribe 订阅原始消息（单协程处理）。
func (b *goChannelBus) Subscribe(ctx context.Context, topic string) (<-chan *EventMessage, error) {
	ch, err := b.ps.Subscribe(ctx, topic)
	if err != nil {
		return nil, err
	}
	// 将 message.Message 转换为 EventMessage
	msgCh := make(chan *EventMessage)
	go func() {
		defer close(msgCh)
		for msg := range ch {
			msgCh <- &EventMessage{msg: msg}
		}
	}()
	return msgCh, nil
}

// close 关闭底层 Pub/Sub。
func (b *goChannelBus) close() error {
	return b.ps.Close()
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
