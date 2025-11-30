package abe

import (
	"github.com/gin-gonic/gin"
	"github.com/samber/do/v2"
)

// Controller 定义了控制器的通用接口
//
// 所有控制器都必须实现此接口，以便在路由注册时统一处理
// 这种设计使得路由注册过程更加标准化和可维护
type Controller interface {
	// RegisterRoutes 注册控制器的所有路由到指定的路由组
	// 参数:
	//   - router: gin.IRouter，用于注册路由
	RegisterRoutes(router gin.IRouter)
}

// ControllerProvider 控制器提供者
//
// 用于创建控制器实例`
type ControllerProvider func() Controller

// ControllerRegistrar 控制器注册器
type ControllerRegistrar interface {
	AddController(providers ...ControllerProvider)
}

// Provider 创建一个控制器提供者
//
// 参数:
//   - controller: 要提供的控制器实例
//
// 返回:
//   - ControllerProvider: 一个函数，当调用时返回传入的控制器实例
func Provider(controller Controller) ControllerProvider {
	return func() Controller {
		return controller
	}
}

// Empty 空结构体，用于表示无请求体的用例
type Empty = struct{}

// UseCase 用户用例接口-应用的服务或业务逻辑层规范
//
// 实现类通过构造函数从 DI 容器中解析依赖，并在 Execute 中执行业务逻辑。
type UseCase[TReq any, TResp any] interface {
	Execute(ctx *gin.Context, req TReq) (TResp, error)
}

// Invoke 统一的控制器辅助：
// - 从请求级 DI 容器解析服务实例
// - 调用服务的 Execute 方法
// - 若发生错误，统一上报到 Gin 错误链，由 ErrorHandlerMiddleware 收敛
// 返回值：成功时返回响应；失败时返回零值与原始错误（并已 ctx.Error）
func Invoke[S UseCase[TReq, TResp], TReq any, TResp any](ctx *gin.Context, req TReq) (TResp, error) {
	inj := Injector(ctx)
	service, err := do.Invoke[S](inj)
	if err != nil {
		// 依赖解析失败：映射为 500，并在错误处理中间件统一输出
		ctx.Error(InternalServerError("依赖解析失败").WithMeta("error", err.Error()))
		var zero TResp
		return zero, err
	}

	resp, err := service.Execute(ctx, req)
	if err != nil {
		// 业务错误由服务实现返回，直接上报，由 ErrorHandlerMiddleware 负责分类与响应
		ctx.Error(err)
		var zero TResp
		return zero, err
	}
	return resp, nil
}

// Call 便捷辅助：用于无需请求体的纯处理型接口。
func Call[S UseCase[Empty, TResp], TResp any](ctx *gin.Context) (TResp, error) {
	return Invoke[S](ctx, Empty{})
}

// EncodeUserSub 编码用户主体为 Casbin 格式
// - 用户：u:<userID>
func EncodeUserSub(userID string) string { return "u:" + userID }

// EncodeRoleSub 编码角色主体为 Casbin 格式
// - 角色：r:<role>
func EncodeRoleSub(role string) string { return "r:" + role }
