package abe

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/panjf2000/ants/v2"
	"github.com/spf13/viper"
)

// PoolConfig 协程池配置
type PoolConfig struct {
	Size             int           `mapstructure:"size"`               // 协程池大小
	ExpiryDuration   time.Duration `mapstructure:"expiry_duration"`    // 协程过期时间
	PreAlloc         bool          `mapstructure:"pre_alloc"`          // 是否预分配内存
	MaxBlockingTasks int           `mapstructure:"max_blocking_tasks"` // 最大阻塞任务数
	Nonblocking      bool          `mapstructure:"nonblocking"`        // 是否为非阻塞模式
}

// DefaultPoolConfig 返回默认协程池配置
func DefaultPoolConfig() PoolConfig {
	return PoolConfig{
		Size:             50000,            // 默认池大小
		ExpiryDuration:   10 * time.Second, // 默认过期时间
		PreAlloc:         true,             // 默认预分配内存
		MaxBlockingTasks: 10000,            // 默认最大阻塞任务数
		Nonblocking:      false,            // 默认阻塞模式
	}
}

// getPoolConfigFromViper 从 viper 配置中获取协程池配置
func getPoolConfigFromViper(v *viper.Viper) PoolConfig {
	// 使用默认配置作为基础
	poolConfig := DefaultPoolConfig()

	// 如果配置中有协程池设置，则使用配置中的设置
	if v != nil {
		if size := v.GetInt("goroutine_pool.size"); size > 0 {
			poolConfig.Size = size
		}
		if expiryDuration := v.GetInt("goroutine_pool.expiry_duration"); expiryDuration > 0 {
			poolConfig.ExpiryDuration = time.Duration(expiryDuration) * time.Second
		}
		if v.IsSet("goroutine_pool.pre_alloc") {
			poolConfig.PreAlloc = v.GetBool("goroutine_pool.pre_alloc")
		}
		if maxBlockingTasks := v.GetInt("goroutine_pool.max_blocking_tasks"); maxBlockingTasks > 0 {
			poolConfig.MaxBlockingTasks = maxBlockingTasks
		}
		if v.IsSet("goroutine_pool.nonblocking") {
			poolConfig.Nonblocking = v.GetBool("goroutine_pool.nonblocking")
		}
	}

	return poolConfig
}

// poolSlogAdapter 是 slog.Logger 到 ants.Logger 的适配器
type poolSlogAdapter struct {
	Logger *slog.Logger
}

// Printf 实现 ants.Logger 接口
func (a *poolSlogAdapter) Printf(format string, args ...any) {
	a.Logger.Info(fmt.Sprintf(format, args...))
}

// newPool 创建协程池实例
// 根据配置创建协程池实例
// 参数:
//   - config: viper 配置实例
//   - logger: 日志记录器
//
// 返回:
//   - 协程池实例
func newPool(config *viper.Viper, logger *slog.Logger) *ants.Pool {
	// 从配置中获取协程池设置
	poolConfig := getPoolConfigFromViper(config)

	// 初始化协程池
	pool, err := initializePool(poolConfig, logger)
	if err != nil {
		panic("初始化协程池失败: " + err.Error())
	}

	return pool
}

// initializePool 初始化协程池
// 根据配置创建协程池实例
// 参数:
//   - config: 协程池配置
//   - logger: 日志记录器
//
// 返回:
//   - 协程池实例和可能的错误
func initializePool(config PoolConfig, logger *slog.Logger) (*ants.Pool, error) {
	// 创建日志适配器
	logAdapter := &poolSlogAdapter{Logger: logger}

	// 创建协程池选项
	options := []ants.Option{
		ants.WithExpiryDuration(config.ExpiryDuration),
		ants.WithPreAlloc(config.PreAlloc),
		ants.WithMaxBlockingTasks(config.MaxBlockingTasks),
		ants.WithNonblocking(config.Nonblocking),
		ants.WithLogger(logAdapter),
		ants.WithPanicHandler(func(i any) {
			logger.Error("协程池任务发生panic", "error", i)
		}),
	}

	// 创建协程池
	pool, err := ants.NewPool(config.Size, options...)
	if err != nil {
		return nil, err
	}

	return pool, nil
}

// newPoolWithFunc 创建函数任务协程池
// 参数:
//   - fn: 函数任务处理函数
//   - size: 协程池大小，如果为0则使用默认大小
//   - logger: 日志记录器
//
// 返回:
//   - 函数任务协程池实例
func newPoolWithFunc(fn func(any), size int, logger *slog.Logger) (*ants.PoolWithFunc, error) {
	// 使用默认配置
	cfg := DefaultPoolConfig()
	if size > 0 {
		cfg.Size = size
	}

	// 创建日志适配器
	logAdapter := &poolSlogAdapter{Logger: logger}

	// 创建协程池选项
	options := []ants.Option{
		ants.WithExpiryDuration(cfg.ExpiryDuration),
		ants.WithPreAlloc(cfg.PreAlloc),
		ants.WithMaxBlockingTasks(cfg.MaxBlockingTasks),
		ants.WithNonblocking(cfg.Nonblocking),
		ants.WithLogger(logAdapter),
		ants.WithPanicHandler(func(i any) {
			logger.Error("函数协程池任务发生panic", "error", i)
		}),
	}

	// 创建函数任务协程池
	pool, err := ants.NewPoolWithFunc(cfg.Size, fn, options...)
	if err != nil {
		return nil, err
	}

	return pool, nil
}
