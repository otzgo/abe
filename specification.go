package abe

import (
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"
)

// Controller 定义了控制器的通用接口
//
// 所有控制器都必须实现此接口，以便在路由注册时统一处理
// 这种设计使得路由注册过程更加标准化和可维护
type Controller interface {
	// RegisterRoutes 注册控制器的所有路由到指定的路由组
	// 参数:
	//   - router: gin.IRouter，用于注册路由
	RegisterRoutes(router gin.IRouter, mg *MiddlewareManager, engine *Engine)
}

// ControllerProvider 控制器提供者
//
// 用于创建控制器实例`
type ControllerProvider func() Controller

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

// UseCase 定义了业务用例的通用接口
//
// 每个用例对应一个具体的业务操作，通过 Handle 方法处理请求并返回结果
// 泛型参数 T 表示用例的返回类型
//
// 设计原则：
//   - 用例结构体应包含完成业务所需的最小依赖（如 *gorm.DB，而非 *Engine）
//   - gin.Context 作为 Handle 方法参数传入，而非注入到结构体
//   - 请求参数通过 ctx.ShouldBindJSON 等方法在 Handle 内部获取
//
// 示例:
//
//	type LoginUseCase struct {
//	    db     *gorm.DB
//	    engine *abe.Engine
//	}
//
//	func (uc *LoginUseCase) Handle(ctx *gin.Context) (*dto.LoginResponse, error) {
//	    var req dto.LoginRequest
//	    if err := ctx.ShouldBindJSON(&req); err != nil {
//	        return nil, err
//	    }
//	    // 业务逻辑...
//	}
type UseCase[T any] interface {
	// Handle 处理业务逻辑
	// 参数:
	//   - ctx: gin.Context，用于获取请求数据、用户凭证等
	// 返回:
	//   - T: 业务处理结果
	//   - error: 处理过程中的错误
	Handle(ctx *gin.Context) (T, error)
}

var (
	ErrUnauthorized   = errors.New("unauthorized")          // 未认证
	ErrForbidden      = errors.New("forbidden")             // 无权限
	ErrInternalServer = errors.New("internal server error") // 内部错误
)

// ErrorCode 业务错误码
// 用于表示业务逻辑中的错误码，0 表示成功，非 0 表示具体错误
type ErrorCode int

// Response 定义了统一的响应结构
type Response[T any] struct {
	Code ErrorCode `json:"code"`
	Msg  string    `json:"msg"`
	Data T         `json:"data,omitempty"`
}

// ErrorResponse 定义了统一的错误响应结构
type ErrorResponse = Response[gin.H]

// ErrorHandler 错误处理函数
// 参数:
//   - err: error，要处理的错误
//
// 返回:
//   - *ErrorResponse: 自定义错误响应
//   - int: HTTP 状态码
type ErrorHandler func(err error) (*ErrorResponse, int)

// EncodeUserSub 编码用户主体为 Casbin 格式
// - 用户：user:<userID>
func EncodeUserSub(userID string) string { return "user:" + userID }

// EncodeRoleSub 编码角色主体为 Casbin 格式
// - 角色：role:<role_id>
// 参数 roleID 为角色的数据库ID（uint），使用ID而非名称确保角色改名时权限规则不失效
func EncodeRoleSub(roleID uint) string { return "role:" + strconv.FormatUint(uint64(roleID), 10) }
