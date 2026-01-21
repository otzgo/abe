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
		defaultLang := cfg.GetString("i18n.default_language")

		var candidates []string
		if qv := strings.TrimSpace(ctx.Query(langQueryKey)); qv != "" {
			candidates = append(candidates, qv)
		}
		if hv := strings.TrimSpace(ctx.GetHeader(langHeaderKey)); hv != "" {
			candidates = append(candidates, hv)
		}
		if defaultLang != "" {
			candidates = append(candidates, defaultLang)
		}

		localizer := i18n.NewLocalizer(e.i18nBundle, candidates...)
		ctx.Set(contextKeyI18nLocalizer, localizer)
		ctx.Next()
	}
}

// Localizer 从 gin.Context 中获取 Localizer
func Localizer(ctx *gin.Context) *i18n.Localizer {
	v, ok := ctx.Get(contextKeyI18nLocalizer)
	if !ok {
		return nil
	}
	loc, ok := v.(*i18n.Localizer)
	return loc
}

// Localize 根据 LocalizeConfig 获取翻译文本
func Localize(ctx *gin.Context, config *i18n.LocalizeConfig) string {
	localizer := Localizer(ctx)
	if localizer == nil {
		return config.MessageID
	}
	msg, err := localizer.Localize(config)
	if err != nil || msg == "" {
		return config.MessageID
	}
	return msg
}
