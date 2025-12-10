package abe

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/casbin/casbin/v2"
	"github.com/gin-gonic/gin"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/panjf2000/ants/v2"
	"github.com/robfig/cron/v3"
	"github.com/spf13/viper"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"gorm.io/gorm"
)

const Version string = "1.0.0"

const defaultShutdownTimeout = 5 * time.Second

// Engine 应用引擎
type Engine struct {
	config        *viper.Viper
	router        *gin.Engine
	db            *gorm.DB
	cron          *cron.Cron
	events        EventBus
	pool          *ants.Pool
	logger        *slog.Logger
	enforcer      *casbin.Enforcer
	validator     *Validator
	middlewares   *MiddlewareManager
	i18nBundle    *i18n.Bundle
	authManager   *AuthManager
	dynamicConfig *DynamicConfigManager // 动态配置管理器

	basePath string // 路由基础路径

	controllersMu      sync.RWMutex
	mountOnce          sync.Once
	controllerRegistry []ControllerProvider

	httpServer *http.Server
	plugins    *PluginManager
}

// Run 运行应用
func (e *Engine) Run(opts ...RunOption) {
	for _, opt := range opts {
		opt(e)
	}

	e.Plugins().OnBeforeMount()
	e.mountControllers(e.basePath)
	e.Plugins().OnAfterMount()
	e.initializeHTTPServer()
	e.Plugins().OnBeforeServerStart()

	go e.startHTTPServer()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()

	e.shutdown()
}

// Config 配置管理器
func (e *Engine) Config() *viper.Viper {
	return e.config
}

// Router 路由管理器
func (e *Engine) Router() *gin.Engine {
	return e.router
}

// DB 数据库引擎
func (e *Engine) DB() *gorm.DB {
	return e.db
}

// Cron 定时任务管理器
func (e *Engine) Cron() *cron.Cron {
	return e.cron
}

// EventBus 事件总线
func (e *Engine) EventBus() EventBus {
	return e.events
}

// Pool 协程池管理器
func (e *Engine) Pool() *ants.Pool {
	return e.pool
}

// Enforcer 权限策略管理器
func (e *Engine) Enforcer() *casbin.Enforcer {
	return e.enforcer
}

// Logger 日志记录器
func (e *Engine) Logger() *slog.Logger {
	return e.logger
}

// Middlewares 中间件管理器
func (e *Engine) Middlewares() *MiddlewareManager {
	return e.middlewares
}

// Validator 验证器管理器
func (e *Engine) Validator() *Validator {
	return e.validator
}

// Plugins 插件管理器（懒加载）
func (e *Engine) Plugins() *PluginManager {
	if e.plugins == nil {
		e.plugins = newPluginManager(e)
	}
	return e.plugins
}

// Auth 认证授权管理器
func (e *Engine) Auth() *AuthManager {
	return e.authManager
}

// DynamicConfig 动态配置管理器
func (e *Engine) DynamicConfig() *DynamicConfigManager {
	return e.dynamicConfig
}

// AddController 批量追加控制器提供者
func (e *Engine) AddController(providers ...ControllerProvider) {
	if len(providers) == 0 {
		return
	}
	e.controllersMu.Lock()
	defer e.controllersMu.Unlock()
	e.controllerRegistry = append(e.controllerRegistry, providers...)
}

// NewPoolWithFunc 创建函数任务协程池
//
// fn: 函数任务处理函数
// size: 协程池大小，如果为0则使用默认大小
// 返回: 函数任务协程池实例
func (e *Engine) NewPoolWithFunc(fn func(any), size int) (*ants.PoolWithFunc, error) {
	return newPoolWithFunc(fn, size, e.logger)
}

// startHTTPServer 启动 HTTP 服务器
func (e *Engine) startHTTPServer() {
	if err := e.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		panic(fmt.Errorf("致命错误服务器运行：%w", err))
	}
}

// mountControllers 将所有控制器挂载到指定前缀分组（仅启动阶段，幂等）
func (e *Engine) mountControllers(basePath string) {
	e.mountOnce.Do(func() {
		e.controllersMu.RLock()
		snapshot := make([]ControllerProvider, len(e.controllerRegistry))
		copy(snapshot, e.controllerRegistry)
		e.controllersMu.RUnlock()

		if e.logger != nil {
			e.logger.Info("开始注册控制器路由到分组", "basePath", basePath, "count", len(snapshot))
		}

		routerGroup := e.router.Group(basePath, e.middlewares.getGlobals()...)

		if e.config.GetBool("swagger.enabled") {
			var opts []func(*ginSwagger.Config)
			if url := e.config.GetString("swagger.url"); url != "" {
				opts = append(opts, ginSwagger.URL(url))
			}
			if name := e.config.GetString("swagger.instance"); name != "" {
				opts = append(opts, ginSwagger.InstanceName(name))
			}
			routerGroup.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler, opts...))
		}

		for _, provider := range snapshot {
			ctrl := provider()
			func() {
				defer func() {
					if r := recover(); r != nil {
						if e.logger != nil {
							e.logger.Error("注册控制器路由发生异常", "basePath", basePath, "panic", r)
						}
					}
				}()
				ctrl.RegisterRoutes(routerGroup, e.middlewares)
			}()
		}

		if e.logger != nil {
			e.logger.Info("控制器路由注册完成（分组）", "basePath", basePath)
		}
	})
}

// initializeHTTPServer 创建 HTTP 服务器实例
func (e *Engine) initializeHTTPServer() {
	e.httpServer = &http.Server{
		Addr:    e.config.GetString("server.address"),
		Handler: e.router,
	}
}

// shutdownCron 优雅停止 cron 任务
func (e *Engine) shutdownCron() {
	stopCtx := e.cron.Stop()
	<-stopCtx.Done()
}

// shutdownHTTPServer 优雅关闭 HTTP 服务器
func (e *Engine) shutdownHTTPServer() {
	timeout := e.config.GetDuration("server.shutdown_timeout")
	if timeout <= 0 {
		timeout = defaultShutdownTimeout
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if err := e.httpServer.Shutdown(ctx); err != nil {
		if e.logger != nil {
			e.logger.Error("HTTP服务器关闭失败", "error", err)
		}
		panic(fmt.Errorf("优雅退出服务器失败：%w", err))
	}
}

// closeEventBus 关闭事件总线
func (e *Engine) closeEventBus() {
	if e.events == nil {
		return
	}

	if err := e.events.Close(); err != nil {
		if e.logger != nil {
			e.logger.Error("事件总线关闭失败", "error", err)
		} else {
			fmt.Printf("关闭事件总线时出错：%v\n", err)
		}
	}
}

// releasePool 释放协程池资源
func (e *Engine) releasePool() {
	if e.pool != nil {
		e.pool.Release()
	}
}

func (e *Engine) shutdown() {
	e.Plugins().OnShutdown()
	e.shutdownCron()
	e.shutdownHTTPServer()
	e.closeEventBus()
	e.releasePool()
}
