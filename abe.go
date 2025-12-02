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
	"github.com/golang-jwt/jwt/v5"
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
	config      *viper.Viper
	router      *gin.Engine
	db          *gorm.DB
	cron        *cron.Cron
	events      EventBus
	pool        *ants.Pool
	logger      *slog.Logger
	enforcer    *casbin.Enforcer
	validator   *Validator
	middlewares *MiddlewareManager
	i18nBundle  *i18n.Bundle

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

// AddController 批量追加控制器提供者
func (e *Engine) AddController(providers ...ControllerProvider) {
	if len(providers) == 0 {
		return
	}
	e.controllersMu.Lock()
	defer e.controllersMu.Unlock()
	e.controllerRegistry = append(e.controllerRegistry, providers...)
}

// AddJob 按 Cron 表达式注册一个实现了 cron.Job 接口的任务
//
// spec: Cron 表达式，支持秒级 6 字段（秒 分 时 日 月 周）或 @every 语法
// job: 实现了 Run() 方法的 Job 对象
// 返回: 任务 ID 和可能的错误
//
// 示例：
//
//	type CleanupJob struct{}
//	func (CleanupJob) Run() {
//	    fmt.Println("执行清理任务")
//	}
//
//	// 每小时执行一次
//	id, err := engine.AddJob("0 0 * * * *", CleanupJob{})
func (e *Engine) AddJob(spec string, job cron.Job) (cron.EntryID, error) {
	return e.cron.AddJob(spec, job)
}

// SubmitTask 提交任务到协程池
//
// task: 要执行的任务函数
// 返回: 错误信息
//
// 示例：
//
//	// 提交一个简单任务
//	err := engine.SubmitTask(func() {
//	    fmt.Println("任务执行中")
//	})
//	if err != nil {
//	    log.Printf("提交任务失败: %v", err)
//	}
//
//	// 批量提交任务
//	for i := 0; i < 100; i++ {
//	    index := i
//	    engine.SubmitTask(func() {
//	        fmt.Printf("处理任务 %d\n", index)
//	    })
//	}
func (e *Engine) SubmitTask(task func()) error {
	return e.pool.Submit(task)
}

// NewPoolWithFunc 创建函数任务协程池
//
// fn: 函数任务处理函数
// size: 协程池大小，如果为0则使用默认大小
// 返回: 函数任务协程池实例
//
// 示例：
//
//	// 定义处理函数
//	processor := func(arg any) {
//	    num := arg.(int)
//	    fmt.Printf("处理数字: %d\n", num)
//	}
//
//	// 创建协程池
//	pool, err := engine.NewPoolWithFunc(processor, 200)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer pool.Release() // 使用完毕后释放资源
//
//	// 批量提交任务
//	for i := 0; i < 1000; i++ {
//	    pool.Invoke(i)
//	}
func (e *Engine) NewPoolWithFunc(fn func(any), size int) (*ants.PoolWithFunc, error) {
	return newPoolWithFunc(fn, size, e.logger)
}

// Enforce 检查权限
// 直接使用 Casbin 进行权限校验
func (e *Engine) Enforce(sub string, obj string, act string) (bool, error) {
	return e.enforcer.Enforce(sub, obj, act)
}

// GetAuthConfig 从 Engine 的 viper 配置中解析认证配置
// 若未设置 TokenExpiry，则返回默认值 24 小时
func (e *Engine) GetAuthConfig() (AuthConfig, error) {
	var cfg AuthConfig
	if err := e.Config().UnmarshalKey("auth", &cfg); err != nil {
		return cfg, err
	}
	if cfg.TokenExpiry == 0 {
		cfg.TokenExpiry = 24
	}
	return cfg, nil
}

// GenerateToken 根据用户声明生成 JWT 字符串
// 配置项：
// - auth.jwt_secret: HMAC 密钥（必须）
// - auth.token_expiry: 过期时间（小时，默认 24）
func (e *Engine) GenerateToken(claims *UserClaims) (string, error) {
	cfg, err := e.GetAuthConfig()
	if err != nil {
		return "", InternalServerError("解析认证配置失败").WithMeta("error", err.Error())
	}
	secret := cfg.JWTSecret
	if secret == "" {
		return "", InternalServerError("JWT 密钥未配置").WithMeta("key", "auth.jwt_secret")
	}

	expHours := cfg.TokenExpiry
	if expHours == 0 {
		expHours = 24
	}

	now := time.Now()
	if claims.IssuedAt == nil {
		claims.IssuedAt = jwt.NewNumericDate(now)
	}
	if claims.ExpiresAt == nil {
		claims.ExpiresAt = jwt.NewNumericDate(now.Add(time.Duration(expHours) * time.Hour))
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// ParseToken 解析 JWT 字符串并返回用户声明
func (e *Engine) ParseToken(tokenString string) (*UserClaims, error) {
	cfg, cfgErr := e.GetAuthConfig()
	if cfgErr != nil {
		return nil, InternalServerError("解析认证配置失败").WithMeta("error", cfgErr.Error())
	}
	secret := cfg.JWTSecret
	if secret == "" {
		return nil, InternalServerError("JWT 密钥未配置").WithMeta("key", "auth.jwt_secret")
	}

	token, err := jwt.ParseWithClaims(tokenString, &UserClaims{}, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidSigningKey
		}
		return []byte(secret), nil
	})
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		return nil, err
	}

	claims, ok := token.Claims.(*UserClaims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}
	return claims, nil
}

// AuthorizeEndpoint 单端点的用户主体权限检查
func (e *Engine) AuthorizeEndpoint(obj string, act string) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		claims, ok := getUserClaimsOrAbort(ctx)
		if !ok {
			return
		}

		sub := EncodeUserSub(claims.UserID)
		if !e.checkAuthorization(ctx, sub, obj, act) {
			ctx.Abort()
			return
		}
		ctx.Next()
	}
}

// RoleAuthorizeEndpoint 单端点的角色主体权限检查
func (e *Engine) RoleAuthorizeEndpoint(obj string, act string) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		claims, ok := getUserClaimsOrAbort(ctx)
		if !ok {
			return
		}

		role := claims.PrimaryRole
		if role == "" {
			ctx.Error(&HTTPError{Status: http.StatusForbidden, Code: CodeForbidden, Message: "缺少主角色"})
			ctx.Abort()
			return
		}

		if !e.checkAuthorization(ctx, EncodeRoleSub(role), obj, act) {
			ctx.Abort()
			return
		}
		ctx.Next()
	}
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

// checkAuthorization 统一的权限检查逻辑
func (e *Engine) checkAuthorization(ctx *gin.Context, sub, obj, act string) bool {
	allowed, err := e.Enforce(sub, obj, act)
	if err != nil {
		ctx.Error(&HTTPError{Status: http.StatusInternalServerError, Code: CodeInternalServerError, Message: "权限检查失败", Details: []ErrorDetail{GenericDetail(err.Error())}})
		return false
	}
	if !allowed {
		ctx.Error(&HTTPError{Status: http.StatusForbidden, Code: CodeForbidden, Message: "权限不足，无法访问此资源"})
		return false
	}
	return true
}
