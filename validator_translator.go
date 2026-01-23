package abe

import (
	"github.com/gin-gonic/gin"
	"github.com/go-playground/locales/en"
	"github.com/go-playground/locales/zh"
	ut "github.com/go-playground/universal-translator"
	entrans "github.com/go-playground/validator/v10/translations/en"
	zhtrans "github.com/go-playground/validator/v10/translations/zh"
)

const translatorContextKey = "abe.translator"

// validationTranslatorMiddleware 注入翻译器到请求上下文
// 从 Engine 中获取验证器和默认语言配置
func validationTranslatorMiddleware(e *Engine) gin.HandlerFunc {
	var translator ut.Translator

	// 获取验证器
	validate := e.validator.Instance()
	defaultLocale := e.validator.DefaultLocale()

	// 初始化多语言
	zhCn := zh.New()
	enUs := en.New()
	uni := ut.New(zhCn, zhCn, enUs)

	if defaultLocale != "en" { // 默认 zh
		translator, _ = uni.GetTranslator("zh")
		_ = zhtrans.RegisterDefaultTranslations(validate, translator)
		e.validator.registerCustomRuleTranslations(translator, "zh")
	} else {
		translator, _ = uni.GetTranslator("en")
		_ = entrans.RegisterDefaultTranslations(validate, translator)
		e.validator.registerCustomRuleTranslations(translator, "en")
	}

	return func(ctx *gin.Context) {
		ctx.Set(translatorContextKey, translator)
		ctx.Next()
	}
}

// Translator 从上下文获取翻译器
func Translator(c *gin.Context) ut.Translator {
	v, ok := c.Get(translatorContextKey)
	if !ok {
		return nil
	}
	if t, ok := v.(ut.Translator); ok {
		return t
	}
	return nil
}
