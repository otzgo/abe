//go:build wireinject
// +build wireinject

package abe

import (
	"github.com/google/wire"
)

// InitializeEngine 初始化并返回 abe 框架的应用引擎实例
//
// 此函数是 abe 框架的入口函数，负责创建和组装所有核心组件，包括：
//   - 配置系统（支持命令行参数、环境变量、配置文件）
//   - 日志系统（结构化日志，支持控制台和文件输出）
//   - HTTP 路由（基于 Gin 框架）
//   - 数据库连接（基于 GORM）
//   - 定时任务调度器（基于 Cron）
//
// 重要提示：
//   - 此函数应该在应用启动时仅调用一次（通常在 main 函数中）
//   - 返回的 Engine 实例包含所有共享服务，应在整个应用中复用
//   - 多次调用会创建多个独立的引擎实例，可能导致资源浪费和行为不一致
//
// 使用示例：
//
//	func main() {
//	    engine := abe.InitializeEngine()
//	    // 配置路由、定时任务等
//	    engine.Run()  // 启动 HTTP 服务器和定时任务调度器
//	}
func InitializeEngine() *Engine {
	wire.Build(
		wire.Struct(
			new(Engine),
			"config", "router", "db", "cron", "events", "pool",
			"logger", "enforcer", "validator", "middlewares", "i18nBundle",
		),
		newCron,
		newConfig,
		newLogger,
		newDB,
		newRouter,
		newGoChannelBus,
		newGoChannelConfig,
		newGoChannelLogger,
		newEnforcer,
		newPool,
		newValidator,
		newMiddlewareManager,
		newI18nBundle,
		wire.Bind(new(EventBus), new(*goChannelBus)),
	)
	return nil
}
