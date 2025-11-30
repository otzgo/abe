package abe

// RunOption 运行选项
type RunOption func(*Engine)

// WithBasePath 设置 API 基础路径
//
// basePath: API 基础路径，默认值为空字符串
func WithBasePath(basePath string) RunOption {
	return func(e *Engine) {
		e.basePath = basePath
	}
}
