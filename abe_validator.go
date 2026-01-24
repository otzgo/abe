package abe

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/gin-gonic/gin/binding"
	ut "github.com/go-playground/universal-translator"
	"github.com/go-playground/validator/v10"
	"github.com/spf13/viper"
)

// ValidationRule 自定义验证规则
// 封装验证函数和多语言翻译，作为独立的可复用单元
type ValidationRule struct {
	tag          string            // 规则标签名（如 "username"）
	fn           validator.Func    // 验证函数
	translations map[string]string // 翻译模板 locale -> template
}

// NewValidationRule 创建自定义验证规则
func NewValidationRule(tag string, fn validator.Func) *ValidationRule {
	return &ValidationRule{
		tag:          tag,
		fn:           fn,
		translations: make(map[string]string),
	}
}

// WithTranslation 添加翻译（链式调用）
func (r *ValidationRule) WithTranslation(locale, template string) *ValidationRule {
	r.translations[locale] = template
	return r
}

// WithZhTranslation 添加中文翻译（便捷方法）
func (r *ValidationRule) WithZhTranslation(template string) *ValidationRule {
	return r.WithTranslation("zh", template)
}

// WithEnTranslation 添加英文翻译（便捷方法）
func (r *ValidationRule) WithEnTranslation(template string) *ValidationRule {
	return r.WithTranslation("en", template)
}

// hasParam 内部方法：检测是否需要参数（自动识别模板中的 {1}）
func (r *ValidationRule) hasParam() bool {
	for _, tmpl := range r.translations {
		if strings.Contains(tmpl, "{1}") {
			return true
		}
	}
	return false
}

// getTranslation 内部方法：获取指定语言的翻译，支持降级到英文
func (r *ValidationRule) getTranslation(locale string) string {
	if tmpl, ok := r.translations[locale]; ok {
		return tmpl
	}
	// 降级为英文
	if tmpl, ok := r.translations["en"]; ok {
		return tmpl
	}
	return ""
}

// check 内部方法：验证规则完整性
func (r *ValidationRule) check() error {
	if r.tag == "" {
		return errors.New("rule tag cannot be empty")
	}
	if r.fn == nil {
		return errors.New("rule validation function cannot be nil")
	}
	if _, hasZh := r.translations["zh"]; !hasZh {
		return fmt.Errorf("rule '%s' missing zh translation", r.tag)
	}
	if _, hasEn := r.translations["en"]; !hasEn {
		return fmt.Errorf("rule '%s' missing en translation", r.tag)
	}
	return nil
}

// Validator 验证器管理器，负责管理验证规则、翻译和配置
type Validator struct {
	instance    *validator.Validate
	customRules map[string]*ValidationRule // 自定义规则集合
	locale      string
}

// newValidator 创建并初始化验证器实例
// - 从配置读取默认语言（validator.locale），默认为 zh
// - 将验证标签从 binding 改为 validate
// - 注册字段名显示优先级：label > json > 字段名
// - 注册 abe 内置通用规则（mobile、idcard）
func newValidator(config *viper.Viper) *Validator {
	gv, ok := binding.Validator.Engine().(*validator.Validate)
	if !ok {
		return nil
	}

	// 从配置读取默认语言，默认 zh
	defaultLocale := config.GetString("validator.locale")
	if defaultLocale == "" {
		defaultLocale = "zh"
	}

	// 将验证标签从 binding 改为 check，提升语义清晰度
	gv.SetTagName("validate")

	// 字段标签名函数：label > json > 字段名
	gv.RegisterTagNameFunc(func(fld reflect.StructField) string {
		if name := fld.Tag.Get("label"); name != "" {
			return name
		}
		if jsonTag := fld.Tag.Get("json"); jsonTag != "" {
			// 去除 ,omitempty 等
			for i, ch := range jsonTag {
				if ch == ',' {
					jsonTag = jsonTag[:i]
					break
				}
			}
			return jsonTag
		}
		return fld.Name
	})

	// 注册 abe 内置通用规则将在返回 Validator 对象后批量注册

	v := &Validator{
		instance:    gv,
		locale:      defaultLocale,
		customRules: make(map[string]*ValidationRule),
	}

	// 批量注册内置规则（使用 Must 版本，初始化失败则 panic）
	for _, rule := range builtinRules() {
		v.MustRegisterCustomRule(rule)
	}

	return v
}

// Instance 返回底层验证器实例
func (v *Validator) Instance() *validator.Validate {
	return v.instance
}

// Locale 返回默认语言
func (v *Validator) Locale() string {
	return v.locale
}

// RegisterCustomRule 注册自定义验证规则
func (v *Validator) RegisterCustomRule(rule *ValidationRule) error {
	// 验证规则完整性
	if err := rule.check(); err != nil {
		return err
	}

	// 注册验证函数到底层验证器
	if err := v.instance.RegisterValidation(rule.tag, rule.fn); err != nil {
		return fmt.Errorf("failed to register validation '%s': %w", rule.tag, err)
	}

	// 存储规则对象（用于后续翻译注册）
	v.customRules[rule.tag] = rule

	return nil
}

// MustRegisterCustomRule 注册自定义规则（panic 版本，用于初始化）
func (v *Validator) MustRegisterCustomRule(rule *ValidationRule) {
	if err := v.RegisterCustomRule(rule); err != nil {
		panic(fmt.Sprintf("failed to register custom rule: %v", err))
	}
}

// registerCustomRuleTranslations 注册自定义规则的翻译
func (v *Validator) registerCustomRuleTranslations(trans ut.Translator, locale string) {
	for _, rule := range v.customRules {
		template := rule.getTranslation(locale)
		if template == "" {
			continue
		}

		// 自动检测是否需要参数
		if rule.hasParam() {
			v.registerTranslationWithParam(trans, rule.tag, template)
		} else {
			v.registerTranslation(trans, rule.tag, template)
		}
	}
}

// registerTranslation 辅助：注册标准翻译（无参数）
func (v *Validator) registerTranslation(trans ut.Translator, tag string, template string) {
	_ = v.instance.RegisterTranslation(tag, trans, func(ut ut.Translator) error {
		return ut.Add(tag, template, true)
	}, func(ut ut.Translator, fe validator.FieldError) string {
		msg, _ := ut.T(tag, fe.Field())
		return msg
	})
}

// registerTranslationWithParam 辅助：注册标准翻译（带参数）
func (v *Validator) registerTranslationWithParam(trans ut.Translator, tag string, template string) {
	_ = v.instance.RegisterTranslation(tag, trans, func(ut ut.Translator) error {
		return ut.Add(tag, template, true)
	}, func(ut ut.Translator, fe validator.FieldError) string {
		// {0} = 字段名, {1} = 参数值
		msg, _ := ut.T(tag, fe.Field(), fe.Param())
		return msg
	})
}

// --- 内置通用规则实现（简化版正则） ---

func validateMobile(fl validator.FieldLevel) bool {
	val := fl.Field().String()
	// 中国大陆手机号（简化）：以 1 开头，次位 3-9，后续 9 位数字
	if len(val) != 11 {
		return false
	}
	if val[0] != '1' {
		return false
	}
	if val[1] < '3' || val[1] > '9' {
		return false
	}
	for i := 2; i < 11; i++ {
		if val[i] < '0' || val[i] > '9' {
			return false
		}
	}
	return true
}

func validateIDCard(fl validator.FieldLevel) bool {
	val := fl.Field().String()
	// 18 位：前 17 位数字 + 最后一位数字或 X/x（简化校验）
	if len(val) != 18 {
		return false
	}
	for i := range 17 {
		if val[i] < '0' || val[i] > '9' {
			return false
		}
	}
	last := val[17]
	return (last >= '0' && last <= '9') || last == 'X' || last == 'x'
}

// validateUsername 验证用户名格式：3-20位字母数字下划线
func validateUsername(fl validator.FieldLevel) bool {
	val := fl.Field().String()
	if len(val) < 3 || len(val) > 20 {
		return false
	}
	for _, ch := range val {
		if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
			(ch >= '0' && ch <= '9') || ch == '_') {
			return false
		}
	}
	return true
}

// validateChineseName 验证中文姓名：2-20个中文字符
func validateChineseName(fl validator.FieldLevel) bool {
	val := fl.Field().String()
	runes := []rune(val)
	if len(runes) < 2 || len(runes) > 20 {
		return false
	}
	for _, r := range runes {
		// 中文字符范围：\u4e00-\u9fa5
		if r < 0x4e00 || r > 0x9fa5 {
			return false
		}
	}
	return true
}

// validateStrongPassword 验证强密码：至少8位，包含大小写字母和数字
func validateStrongPassword(fl validator.FieldLevel) bool {
	val := fl.Field().String()
	if len(val) < 8 {
		return false
	}

	hasUpper := false
	hasLower := false
	hasDigit := false

	for _, ch := range val {
		if ch >= 'A' && ch <= 'Z' {
			hasUpper = true
		} else if ch >= 'a' && ch <= 'z' {
			hasLower = true
		} else if ch >= '0' && ch <= '9' {
			hasDigit = true
		}
	}

	return hasUpper && hasLower && hasDigit
}

// --- 内置验证规则定义（导出，可供其他包使用） ---

var (
	// BuiltinRuleMobile 手机号验证规则
	BuiltinRuleMobile = NewValidationRule("mobile", validateMobile).
				WithZhTranslation("{0}必须是有效的手机号码").
				WithEnTranslation("{0} must be a valid mobile number")

	// BuiltinRuleIDCard 身份证号验证规则
	BuiltinRuleIDCard = NewValidationRule("idcard", validateIDCard).
				WithZhTranslation("{0}必须是有效的身份证号码").
				WithEnTranslation("{0} must be a valid ID card number")

	// BuiltinRuleUsername 用户名验证规则
	BuiltinRuleUsername = NewValidationRule("username", validateUsername).
				WithZhTranslation("{0}必须是3-20位字母数字下划线").
				WithEnTranslation("{0} must be 3-20 alphanumeric characters or underscore")

	// BuiltinRuleChineseName 中文姓名验证规则
	BuiltinRuleChineseName = NewValidationRule("chinese_name", validateChineseName).
				WithZhTranslation("{0}必须是2-20个中文字符").
				WithEnTranslation("{0} must be 2-20 Chinese characters")

	// BuiltinRuleStrongPassword 强密码验证规则
	BuiltinRuleStrongPassword = NewValidationRule("strong_password", validateStrongPassword).
					WithZhTranslation("{0}必须至少8位，且包含大小写字母和数字").
					WithEnTranslation("{0} must be at least 8 characters with uppercase, lowercase and digits")
)

// builtinRules 返回所有内置规则
func builtinRules() []*ValidationRule {
	return []*ValidationRule{
		BuiltinRuleMobile,
		BuiltinRuleIDCard,
		BuiltinRuleUsername,
		BuiltinRuleChineseName,
		BuiltinRuleStrongPassword,
	}
}
