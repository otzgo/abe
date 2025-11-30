package abe

import (
	"log/slog"
	"time"

	"github.com/robfig/cron/v3"
)

// newCron 创建并返回一个新的定时任务管理器实例
// 每次调用都会返回一个新的实例
func newCron(logger *slog.Logger) *cron.Cron {
	slogLogger := slogCronLogger{logger: logger}
	corn := cron.New(
		cron.WithSeconds(),
		cron.WithLocation(time.Local),
		cron.WithLogger(slogLogger),
		cron.WithChain(
			cron.Recover(slogLogger),
			cron.SkipIfStillRunning(slogLogger),
		),
	)
	corn.Start()
	return corn
}

// slogCronLogger 适配器：将 slog.Logger 适配为 cron.Logger
type slogCronLogger struct {
	logger *slog.Logger
}

func (l slogCronLogger) Info(msg string, keysAndValues ...interface{}) {
	l.logger.Info(msg, keysAndValues...)
}

func (l slogCronLogger) Error(err error, msg string, keysAndValues ...interface{}) {
	kv := append([]interface{}{"error", err}, keysAndValues...)
	l.logger.Error(msg, kv...)
}
