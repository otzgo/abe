# 表单验证系统

## 验证系统概述

ABE 框架提供了强大的表单验证功能，基于 `go-playground/validator/v10` 库构建，并增加了多语言支持和一系列内置验证规则。

## 配置选项

### 验证器配置

在 `config.yaml` 中配置验证器：

```yaml
validator:
  locale: "zh"  # 默认语言，支持 zh（中文）和 en（英文）
```

## 内置验证规则

### 1. 手机号验证 (mobile)

```go
type User struct {
    Mobile string `json:"mobile" validate:"mobile"`
}

// 错误消息：
// 中文: "手机号必须是有效的手机号码"
// 英文: "Mobile must be a valid mobile number"
```

### 2. 身份证号验证 (idcard)

```go
type User struct {
    IDCard string `json:"id_card" validate:"idcard"`
}

// 错误消息：
// 中文: "身份证号必须是有效的身份证号码"
// 英文: "ID card must be a valid ID card number"
```

### 3. 用户名验证 (username)

```go
type User struct {
    Username string `json:"username" validate:"username"`
}

// 验证规则：3-20位，只能包含字母、数字和下划线
// 错误消息：
// 中文: "用户名必须是3-20位字母数字下划线"
// 英文: "Username must be 3-20 alphanumeric characters or underscore"
```

### 4. 中文姓名验证 (chinese_name)

```go
type User struct {
    RealName string `json:"real_name" validate:"chinese_name"`
}

// 验证规则：2-20个中文字符
// 错误消息：
// 中文: "真实姓名必须是2-20个中文字符"
// 英文: "Real name must be 2-20 Chinese characters"
```

### 5. 强密码验证 (strong_password)

```go
type User struct {
    Password string `json:"password" validate:"strong_password"`
}

// 验证规则：至少8位，必须同时包含大写字母、小写字母和数字
// 错误消息：
// 中文: "密码必须至少8位，且包含大小写字母和数字"
// 英文: "Password must be at least 8 characters with uppercase, lowercase and digits"
```

## 基本使用方法

### 结构体验证

```go
type UserRegistration struct {
    Username string `json:"username" validate:"required,username" label:"用户名"`
    Email    string `json:"email" validate:"required,email" label:"邮箱"`
    Mobile   string `json:"mobile" validate:"required,mobile" label:"手机号"`
    Password string `json:"password" validate:"required,strong_password" label:"密码"`
    RealName string `json:"real_name" validate:"required,chinese_name" label:"真实姓名"`
}

func (uc *UserController) registerUser(ctx *gin.Context) {
    var req UserRegistration
    if err := ctx.ShouldBindJSON(&req); err != nil {
        ctx.JSON(400, gin.H{"error": err.Error()})
        return
    }
    
    // 验证通过，继续处理业务逻辑
    // ...
}
```

### 使用标签优化错误消息

```go
type UserProfile struct {
    RealName   string `json:"real_name" label:"真实姓名" validate:"chinese_name"`
    IDCard     string `json:"id_card" label:"身份证号" validate:"idcard"`
    Birthday   string `json:"birthday" label:"出生日期" validate:"datetime=2006-01-02"`
    Gender     string `json:"gender" label:"性别" validate:"oneof=male female"`
}

// 当验证失败时，错误消息将使用 label 中定义的名称
```

## 自定义验证规则

### 创建自定义规则

```go
// 创建一个验证邮政编码的规则
func createZipCodeRule() *abe.ValidationRule {
    return abe.NewValidationRule("zipcode", func(fl validator.FieldLevel) bool {
        val := fl.Field().String()
        // 简单验证6位数字
        if len(val) != 6 {
            return false
        }
        for _, ch := range val {
            if ch < '0' || ch > '9' {
                return false
            }
        }
        return true
    }).WithZhTranslation("{0}必须是6位数字邮政编码").
       WithEnTranslation("{0} must be a 6-digit postal code")
}

// 注册自定义规则
func setupCustomValidators(engine *abe.Engine) {
    zipCodeRule := createZipCodeRule()
    engine.Validator().MustRegisterCustomRule(zipCodeRule)
}

// 使用自定义规则
type Address struct {
    ZipCode string `json:"zip_code" validate:"zipcode"`
}
```

### 复杂自定义验证

```go
// 验证业务逻辑规则
func createBusinessRule() *abe.ValidationRule {
    return abe.NewValidationRule("business_hours", func(fl validator.FieldLevel) bool {
        hours := fl.Field().Int()
        return hours >= 0 && hours <= 24
    }).WithZhTranslation("{0}必须在0-24小时范围内").
       WithEnTranslation("{0} must be between 0-24 hours")
}

// 条件验证规则
func createConditionalRule() *abe.ValidationRule {
    return abe.NewValidationRule("conditional_required", func(fl validator.FieldLevel) bool {
        // 只有当某个条件满足时才需要验证
        parent := fl.Parent()
        requiredField := parent.FieldByName("IsRequired")
        if !requiredField.Bool() {
            return true // 不需要验证
        }
        
        fieldValue := fl.Field().String()
        return fieldValue != ""
    }).WithZhTranslation("{0}是必填项").
       WithEnTranslation("{0} is required")
}
```

## 验证错误处理

### 基本错误处理

```go
func handleValidationError(ctx *gin.Context, err error) {
    // 检查是否为验证错误
    if validationErrs, ok := err.(validator.ValidationErrors); ok {
        // 获取翻译器
        translator := abe.Translator(ctx)
        if translator != nil {
            // 翻译验证错误
            translatedErrors := validationErrs.Translate(translator)
            
            ctx.JSON(400, gin.H{
                "code":   400,
                "msg":    "参数验证失败",
                "errors": translatedErrors,
            })
            return
        }
    }
    
    // 其他类型错误
    ctx.JSON(400, gin.H{
        "code": 400,
        "msg":  err.Error(),
    })
}
```

### 详细错误信息

```go
func detailedValidationError(ctx *gin.Context, err error) {
    if validationErrs, ok := err.(validator.ValidationErrors); ok {
        var errorDetails []map[string]interface{}
        
        for _, fieldErr := range validationErrs {
            errorDetail := map[string]interface{}{
                "field":       fieldErr.Field(),
                "tag":         fieldErr.Tag(),
                "value":       fieldErr.Value(),
                "translation": fieldErr.Translate(abe.Translator(ctx)),
            }
            errorDetails = append(errorDetails, errorDetail)
        }
        
        ctx.JSON(400, gin.H{
            "code":   400,
            "msg":    "参数验证失败",
            "detail": errorDetails,
        })
        return
    }
    
    ctx.JSON(400, gin.H{"error": err.Error()})
}
```

## 验证器 API 使用

### 获取验证器实例

```go
func main() {
    engine := abe.NewEngine()
    
    // 获取验证器实例
    validator := engine.Validator()
    
    // 获取底层验证器
    validate := validator.Instance()
    
    // 注册自定义验证规则
    setupCustomRules(validator)
    
    engine.Run()
}

func setupCustomRules(validator *abe.Validator) {
    // 注册多个自定义规则
    rules := []*abe.ValidationRule{
        createZipCodeRule(),
        createBusinessRule(),
        createConditionalRule(),
    }
    
    for _, rule := range rules {
        validator.MustRegisterCustomRule(rule)
    }
}
```

### 验证器方法

```go
type ValidationService struct {
    validator *validator.Validate
}

func NewValidationService(v *validator.Validate) *ValidationService {
    return &ValidationService{validator: v}
}

func (vs *ValidationService) ValidateStruct(s interface{}) error {
    return vs.validator.Struct(s)
}

func (vs *ValidationService) ValidateVar(field interface{}, tag string) error {
    return vs.validator.Var(field, tag)
}

func (vs *ValidationService) ValidateMap(data map[string]interface{}, rules map[string]interface{}) error {
    return vs.validator.ValidateMap(data, rules)
}
```

## 高级验证技巧

### 1. 条件验证

```go
type ConditionalValidation struct {
    Type     string `json:"type" validate:"required,oneof=basic premium"`
    PremiumFeatures bool `json:"premium_features"`
    // 条件验证：只有当 Type 为 premium 时，PremiumFeatures 才需要为 true
    FeaturesEnabled bool `json:"features_enabled" validate:"required_if=Type premium"`
}

// 或者使用自定义验证
func createConditionalFeatureRule() *abe.ValidationRule {
    return abe.NewValidationRule("premium_required", func(fl validator.FieldLevel) bool {
        parent := fl.Parent()
        userType := parent.FieldByName("Type").String()
        
        if userType == "premium" {
            featuresEnabled := fl.Field().Bool()
            return featuresEnabled
        }
        
        return true // 非 premium 用户不需要验证
    })
}
```

### 2. 跨字段验证

```go
type PasswordChange struct {
    OldPassword string `json:"old_password" validate:"required"`
    NewPassword string `json:"new_password" validate:"required,strong_password"`
    ConfirmPassword string `json:"confirm_password" validate:"required,eqfield=NewPassword"`
}

// 自定义跨字段验证
func createPasswordMatchRule() *abe.ValidationRule {
    return abe.NewValidationRule("password_match", func(fl validator.FieldLevel) bool {
        parent := fl.Parent()
        newPassword := parent.FieldByName("NewPassword").String()
        confirmPassword := fl.Field().String()
        
        return newPassword == confirmPassword
    }).WithZhTranslation("确认密码必须与新密码一致").
       WithEnTranslation("Confirm password must match new password")
}
```

### 3. 动态验证规则

```go
func dynamicValidation(ctx *gin.Context) {
    // 根据用户角色动态设置验证规则
    userRole := getUserRole(ctx)
    
    var rules string
    switch userRole {
    case "admin":
        rules = "required,min=1,max=1000"
    case "user":
        rules = "required,min=1,max=100"
    default:
        rules = "required,min=1,max=50"
    }
    
    // 动态验证
    var value string
    if err := ctx.ShouldBindJSON(&value); err != nil {
        ctx.JSON(400, gin.H{"error": err.Error()})
        return
    }
    
    if err := engine.Validator().Instance().Var(value, rules); err != nil {
        ctx.JSON(400, gin.H{"error": "验证失败: " + err.Error()})
        return
    }
    
    ctx.JSON(200, gin.H{"message": "验证通过"})
}
```

## 最佳实践

### 1. 验证层分离

```go
// ✅ 推荐：将验证逻辑分离到专门的服务层
type ValidationService struct {
    validator *validator.Validate
}

func (vs *ValidationService) ValidateUserRegistration(req *UserRegistration) error {
    // 预处理数据
    vs.normalizeData(req)
    
    // 执行验证
    if err := vs.validator.Struct(req); err != nil {
        return err
    }
    
    // 业务逻辑验证
    return vs.businessValidation(req)
}

func (vs *ValidationService) normalizeData(req *UserRegistration) {
    // 数据标准化
    req.Username = strings.TrimSpace(req.Username)
    req.Email = strings.ToLower(strings.TrimSpace(req.Email))
}

func (vs *ValidationService) businessValidation(req *UserRegistration) error {
    // 检查用户名是否已存在
    if vs.userExists(req.Username) {
        return fmt.Errorf("用户名已存在")
    }
    
    // 检查邮箱是否已注册
    if vs.emailRegistered(req.Email) {
        return fmt.Errorf("邮箱已被注册")
    }
    
    return nil
}
```

### 2. 统一验证响应格式

```go
type ValidationErrorResponse struct {
    Code    int                    `json:"code"`
    Message string                 `json:"message"`
    Errors  map[string]interface{} `json:"errors,omitempty"`
}

func createValidationErrorResponse(err error, translator ut.Translator) *ValidationErrorResponse {
    response := &ValidationErrorResponse{
        Code:    400,
        Message: "参数验证失败",
        Errors:  make(map[string]interface{}),
    }
    
    if validationErrs, ok := err.(validator.ValidationErrors); ok {
        for _, fieldErr := range validationErrs {
            fieldName := fieldErr.Field()
            response.Errors[fieldName] = map[string]interface{}{
                "code":    fieldErr.Tag(),
                "message": fieldErr.Translate(translator),
                "value":   fieldErr.Value(),
            }
        }
    }
    
    return response
}
```

### 3. 验证性能优化

```go
// ❌ 避免：重复编译验证规则
func inefficientValidation() {
    for i := 0; i < 1000; i++ {
        validate := validator.New()
        validate.Struct(user) // 每次都重新编译规则
    }
}

// ✅ 推荐：复用验证器实例
var validate *validator.Validate

func init() {
    validate = validator.New()
    // 注册自定义验证规则...
}

func efficientValidation(users []User) {
    for _, user := range users {
        validate.Struct(user) // 复用同一个验证器
    }
}
```

## 故障排除

### 常见验证问题

1. **验证规则不生效**
   ```go
   // 检查验证标签格式
   type User struct {
       Name string `validate:"required"` // 正确
       Age  int    `valid:"required"`    // 错误：应该是 validate
   }
   ```

2. **翻译不工作**
   ```go
   // 确保正确设置翻译器
   func setupTranslator() ut.Translator {
       // 正确初始化翻译器...
       return translator
   }
   ```

3. **自定义规则注册失败**
   ```go
   // 确保规则名称唯一且符合规范
   rule := abe.NewValidationRule("my_rule", validationFunc)
   validator.MustRegisterCustomRule(rule) // 使用 MustRegister 确保注册成功
   ```

4. **复杂结构体验证**
   ```go
   // 对嵌套结构体也需要添加 validate 标签
   type Address struct {
       Street string `validate:"required"`
       City   string `validate:"required"`
   }
   
   type User struct {
       Name    string  `validate:"required"`
       Address Address `validate:"required,dive"` // dive 标签用于验证嵌套结构
   }
   ```