package abe

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/nicksnyder/go-i18n/v2/i18n"
)

const contextKeyI18nLocalizer = "abe.i18n.localizer"

// i18nMiddleware 根据配置解析语言偏好，在请求上下文中注入 Localizer
func i18nMiddleware(e *Engine) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		cfg := e.Config()
		langQueryKey := cfg.GetString("i18n.lang_query_key")
		if langQueryKey == "" {
			langQueryKey = "lang"
		}
		langHeaderKey := cfg.GetString("i18n.lang_header")
		if langHeaderKey == "" {
			langHeaderKey = "Accept-Language"
		}
		langCookieKey := cfg.GetString("i18n.lang_cookie")
		if langCookieKey == "" {
			langCookieKey = "lang"
		}
		defaultLang := cfg.GetString("i18n.default_language")
		fallbacks := cfg.GetStringSlice("i18n.fallback_languages")

		var candidates []string
		if qv := strings.TrimSpace(ctx.Query(langQueryKey)); qv != "" {
			candidates = append(candidates, qv)
		}
		if cv, err := ctx.Cookie(langCookieKey); err == nil {
			cv = strings.TrimSpace(cv)
			if cv != "" {
				candidates = append(candidates, cv)
			}
		}
		if hv := strings.TrimSpace(ctx.GetHeader(langHeaderKey)); hv != "" {
			candidates = append(candidates, hv)
		}
		if defaultLang != "" {
			candidates = append(candidates, defaultLang)
		}
		if len(fallbacks) > 0 {
			candidates = append(candidates, fallbacks...)
		}

		localizer := i18n.NewLocalizer(e.i18nBundle, candidates...)
		ctx.Set(contextKeyI18nLocalizer, localizer)
		ctx.Next()
	}
}

// GetLocalizer 从 gin.Context 中获取 Localizer
func GetLocalizer(ctx *gin.Context) (*i18n.Localizer, bool) {
	v, ok := ctx.Get(contextKeyI18nLocalizer)
	if !ok {
		return nil, false
	}
	loc, ok := v.(*i18n.Localizer)
	return loc, ok
}

// T 根据消息 ID 获取翻译文本
//
// 参数说明：
//   - ctx: 请求上下文，用于获取注入的 Localizer
//   - id: 消息 ID（定义于 YAML 消息文件）
//   - args: 可选参数，最多两个；用于模板数据与复数计数
//
// 行为说明：
//   - 未获取到 Localizer 或翻译失败时，回退返回消息 ID
//   - 超过两个的参数会被忽略，仅取前两项
//
// 使用示例：
//   - T(ctx, "user.welcome")
//   - T(ctx, "user.info", gin.H{"Name": name})
//   - T(ctx, "cart.items", nil, n)
func T(ctx *gin.Context, id string, args ...any) string {
	cfg := &i18n.LocalizeConfig{MessageID: id}

	if len(args) > 0 {
		cfg.TemplateData = args[0]
	}

	if len(args) > 1 {
		cfg.PluralCount = args[1]
	}

	return TWithConfig(ctx, cfg)
}

// TWithConfig 根据 LocalizeConfig 获取翻译文本
func TWithConfig(ctx *gin.Context, cfg *i18n.LocalizeConfig) string {
	loc, ok := GetLocalizer(ctx)
	if !ok || loc == nil {
		return cfg.MessageID
	}
	msg, err := loc.Localize(cfg)
	if err != nil || msg == "" {
		return cfg.MessageID
	}
	return msg
}
