package abe

import (
	"github.com/gin-gonic/gin"
	"github.com/samber/do/v2"
)

// doInjectorKey 为请求级 DI 容器在 gin.Context 中的键名
const doInjectorKey = "abe.do_injector"

// containerMiddleware 在每个请求开始时创建一个 do.Injector，并注册框架级依赖与请求级元信息。
// 生命周期：在请求结束时统一执行 injector.Shutdown()，确保资源优雅释放。
func containerMiddleware(engine *Engine) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		inj := do.New()

		// 框架级依赖注册（单例值）
		do.ProvideValue(inj, engine)            // *Engine
		do.ProvideValue(inj, engine.Config())   // *viper.Viper
		do.ProvideValue(inj, engine.Logger())   // *slog.Logger
		do.ProvideValue(inj, engine.DB())       // *gorm.DB
		do.ProvideValue(inj, engine.EventBus()) // EventBus
		do.ProvideValue(inj, engine.Pool())     // *ants.Pool
		do.ProvideValue(inj, engine.Enforcer()) // *casbin.Enforcer
		do.ProvideValue(inj, engine.Auth())     // *AuthManager

		do.ProvideValue(inj, GetRequestMeta(ctx))

		// 将注入器放入上下文，供后续中间件/控制器使用
		ctx.Set(doInjectorKey, inj)

		// 继续后续处理
		ctx.Next()

		// 请求结束，统一关闭注入器，触发已注册服务的 Shutdown() 钩子
		inj.Shutdown()
	}
}

// Injector 从 gin.Context 中获取当前请求的 do.Injector。
func Injector(ctx *gin.Context) do.Injector {
	v := ctx.MustGet(doInjectorKey)
	return v.(do.Injector)
}

// Invoke 从 DI 容器中获取指定的 UseCase 实例，并执行其 Handle 方法。
//
// 参数:
//   - ctx: *gin.Context，当前请求的上下文，用于获取 DI 容器。
//
// 返回:
//   - R: UseCase 处理函数的返回值类型。
//   - error: 处理过程中遇到的错误，若成功则为 nil。
func Invoke[T UseCase[R], R any](ctx *gin.Context) (R, error) {
	inj := Injector(ctx)
	useCase := do.MustInvokeStruct[T](inj)
	res, err := useCase.Handle(ctx)
	if err != nil {
		ctx.Error(err)
	}
	return res, err
}
