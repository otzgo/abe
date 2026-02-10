# 访问控制机制

## 访问控制概述

ABE 框架提供了完整的用户身份认证和访问权限校验功能，基于 JWT 和 Casbin 技术实现。采用两阶段验证模式：身份认证和权限校验。

## 身份认证 (Authentication)

### JWT 令牌机制

#### 自定义用户声明

```go
type UserClaims struct {
    UID      string   `json:"uid"`
    Username string   `json:"username"`
    MainRole string   `json:"main_role"`
    AllRoles []string `json:"all_roles"`
    jwt.RegisteredClaims
}

// 实现 UserTokenClaims 接口
func (c *UserClaims) UserID() string { 
    return c.UID 
}

func (c *UserClaims) Role() string { 
    return c.MainRole 
}

func (c *UserClaims) Roles() []string { 
    return c.AllRoles 
}
```

#### 生成 JWT 令牌

```go
func generateToken(engine *abe.Engine, user *User) (string, error) {
    claims := UserClaims{
        UID:      fmt.Sprintf("%d", user.ID),
        Username: user.Username,
        MainRole: user.MainRole,
        AllRoles: user.AllRoles,
        RegisteredClaims: jwt.RegisteredClaims{
            ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
            IssuedAt:  jwt.NewNumericDate(time.Now()),
            Subject:   fmt.Sprintf("user:%d", user.ID),
        },
    }
    
    // 从配置获取密钥
    secret := engine.Config().GetString("auth.jwt_secret")
    if secret == "" {
        return "", fmt.Errorf("JWT 密钥未配置")
    }
    
    return abe.NewToken(claims, secret)
}

// 使用示例
func (ac *AuthController) login(ctx *gin.Context) {
    var req LoginRequest
    if err := ctx.ShouldBindJSON(&req); err != nil {
        ctx.JSON(400, gin.H{"error": "参数错误"})
        return
    }
    
    // 验证用户凭据
    user, err := ac.authService.ValidateCredentials(req.Username, req.Password)
    if err != nil {
        ctx.JSON(401, gin.H{"error": "用户名或密码错误"})
        return
    }
    
    // 生成令牌
    token, err := generateToken(ac.engine, user)
    if err != nil {
        ctx.JSON(500, gin.H{"error": "生成令牌失败"})
        return
    }
    
    ctx.JSON(200, gin.H{
        "token": token,
        "user": gin.H{
            "id":       user.ID,
            "username": user.Username,
            "roles":    user.AllRoles,
        },
    })
}
```

### 认证中间件

#### 基础认证中间件

```go
func authenticationMiddleware(engine *abe.Engine) gin.HandlerFunc {
    return func(ctx *gin.Context) {
        // 从 Authorization 头获取令牌
        authHeader := ctx.GetHeader("Authorization")
        if authHeader == "" {
            ctx.JSON(401, gin.H{"error": "缺少认证信息"})
            ctx.Abort()
            return
        }
        
        // 解析 Bearer token
        tokenString := strings.TrimPrefix(authHeader, "Bearer ")
        if tokenString == authHeader {
            ctx.JSON(401, gin.H{"error": "无效的认证格式"})
            ctx.Abort()
            return
        }
        
        // 验证令牌
        secret := engine.Config().GetString("auth.jwt_secret")
        claims, err := abe.ParseToken[*UserClaims](tokenString, secret)
        if err != nil {
            ctx.JSON(401, gin.H{"error": "无效的认证令牌"})
            ctx.Abort()
            return
        }
        
        // 检查令牌是否过期
        if claims.ExpiresAt.Before(time.Now()) {
            ctx.JSON(401, gin.H{"error": "令牌已过期"})
            ctx.Abort()
            return
        }
        
        // 将用户信息存储到上下文
        ctx.Set("user_claims", claims)
        ctx.Set("user_id", claims.UserID())
        ctx.Next()
    }
}

// 在路由中使用
func (ac *AuthController) RegisterRoutes(router gin.IRouter, mg *MiddlewareManager, engine *Engine) {
    // 公开接口不需要认证
    router.POST("/login", ac.login)
    router.POST("/register", ac.register)
    
    // 需要认证的接口组
    authenticated := router.Group("/", authenticationMiddleware(engine))
    {
        authenticated.GET("/profile", ac.getProfile)
        authenticated.PUT("/profile", ac.updateProfile)
        authenticated.POST("/logout", ac.logout)
    }
}
```

#### 获取认证用户信息

```go
func getCurrentUser(ctx *gin.Context) (*UserClaims, error) {
    claims, exists := ctx.Get("user_claims")
    if !exists {
        return nil, fmt.Errorf("用户未认证")
    }
    
    userClaims, ok := claims.(*UserClaims)
    if !ok {
        return nil, fmt.Errorf("用户信息格式错误")
    }
    
    return userClaims, nil
}

// 使用示例
func (uc *UserController) getProfile(ctx *gin.Context) {
    userClaims, err := getCurrentUser(ctx)
    if err != nil {
        ctx.JSON(401, gin.H{"error": err.Error()})
        return
    }
    
    // 根据用户ID获取详细信息
    user, err := uc.userService.GetByID(userClaims.UserID())
    if err != nil {
        ctx.JSON(500, gin.H{"error": "获取用户信息失败"})
        return
    }
    
    ctx.JSON(200, user)
}
```

## 权限校验 (Authorization)

### Casbin 权限管理

#### 权限校验中间件

```go
func authorizationMiddleware(engine *abe.Engine, resource, action string) gin.HandlerFunc {
    return func(ctx *gin.Context) {
        // 获取用户信息
        userClaims, err := getCurrentUser(ctx)
        if err != nil {
            ctx.JSON(401, gin.H{"error": "未认证"})
            ctx.Abort()
            return
        }
        
        // 获取权限控制器
        enforcer := engine.Enforcer()
        if enforcer == nil {
            ctx.JSON(500, gin.H{"error": "权限系统未初始化"})
            ctx.Abort()
            return
        }
        
        // 构建主体标识
        subjects := buildSubjects(userClaims)
        
        // 检查权限
        allowed := false
        for _, subject := range subjects {
            permit, err := enforcer.Enforce(subject, resource, action)
            if err != nil {
                log.Printf("权限检查错误: %v", err)
                continue
            }
            if permit {
                allowed = true
                break
            }
        }
        
        if !allowed {
            ctx.JSON(403, gin.H{"error": "权限不足"})
            ctx.Abort()
            return
        }
        
        ctx.Next()
    }
}

func buildSubjects(claims *UserClaims) []string {
    var subjects []string
    
    // 超级管理员特权
    if claims.UserID() == "1" {
        subjects = append(subjects, "role:super_admin")
    }
    
    // 用户特定权限
    subjects = append(subjects, fmt.Sprintf("user:%s", claims.UserID()))
    
    // 角色权限
    for _, role := range claims.Roles() {
        subjects = append(subjects, fmt.Sprintf("role:%s", role))
    }
    
    return subjects
}
```

#### 在路由中使用权限校验

```go
func (uc *UserController) RegisterRoutes(router gin.IRouter, mg *MiddlewareManager, engine *Engine) {
    // 认证中间件
    auth := authenticationMiddleware(engine)
    
    // 权限校验中间件
    canReadUsers := authorizationMiddleware(engine, "/api/users", "read")
    canWriteUsers := authorizationMiddleware(engine, "/api/users", "write")
    canDeleteUsers := authorizationMiddleware(engine, "/api/users", "delete")
    
    userGroup := router.Group("/api/users")
    {
        // 只需认证的接口
        userGroup.GET("/profile", auth, uc.getCurrentUserProfile)
        
        // 需要读取权限的接口
        userGroup.GET("/", auth, canReadUsers, uc.listUsers)
        userGroup.GET("/:id", auth, canReadUsers, uc.getUser)
        
        // 需要写入权限的接口
        userGroup.POST("/", auth, canWriteUsers, uc.createUser)
        userGroup.PUT("/:id", auth, canWriteUsers, uc.updateUser)
        
        // 需要删除权限的接口
        userGroup.DELETE("/:id", auth, canDeleteUsers, uc.deleteUser)
    }
}
```

### 权限策略配置

#### Casbin 模型配置

```ini
# casbin_model.conf
[request_definition]
r = sub, obj, act

[policy_definition]
p = sub, obj, act

[role_definition]
g = _, _

[policy_effect]
e = some(where (p.eft == allow))

[matchers]
m = g(r.sub, p.sub) && keyMatch2(r.obj, p.obj) && (r.act == p.act || p.act == "*")
```

#### 策略管理

```go
type PolicyService struct {
    enforcer *casbin.Enforcer
}

func (ps *PolicyService) AddPolicy(subject, object, action string) error {
    return ps.enforcer.AddPolicy(subject, object, action)
}

func (ps *PolicyService) RemovePolicy(subject, object, action string) error {
    return ps.enforcer.RemovePolicy(subject, object, action)
}

func (ps *PolicyService) AddRoleForUser(user, role string) error {
    return ps.enforcer.AddRoleForUser(user, role)
}

func (ps *PolicyService) GetPermissionsForUser(userID string) [][]string {
    return ps.enforcer.GetPermissionsForUser(fmt.Sprintf("user:%s", userID))
}

// 初始化策略
func setupDefaultPolicies(enforcer *casbin.Enforcer) error {
    policies := [][]string{
        // 管理员权限
        {"role:admin", "/api/users", "read"},
        {"role:admin", "/api/users", "write"},
        {"role:admin", "/api/users", "delete"},
        
        // 普通用户权限
        {"role:user", "/api/users/profile", "read"},
        {"role:user", "/api/users/profile", "write"},
        
        // 特定用户权限
        {"user:123", "/api/special", "read"},
        {"user:123", "/api/special", "write"},
    }
    
    for _, policy := range policies {
        if _, err := enforcer.AddPolicy(policy); err != nil {
            return fmt.Errorf("添加策略失败 %v: %w", policy, err)
        }
    }
    
    return nil
}
```

## 高级访问控制

### 基于角色的访问控制 (RBAC)

```go
type RBACService struct {
    enforcer *casbin.Enforcer
    logger   *slog.Logger
}

func (rbac *RBACService) CheckPermission(userID, resource, action string) (bool, error) {
    // 构建主体标识
    userSubject := fmt.Sprintf("user:%s", userID)
    
    // 直接权限检查
    if permit, err := rbac.enforcer.Enforce(userSubject, resource, action); err == nil && permit {
        return true, nil
    }
    
    // 角色权限检查
    roles, err := rbac.enforcer.GetRolesForUser(userSubject)
    if err != nil {
        return false, fmt.Errorf("获取用户角色失败: %w", err)
    }
    
    for _, role := range roles {
        if permit, err := rbac.enforcer.Enforce(role, resource, action); err == nil && permit {
            return true, nil
        }
    }
    
    return false, nil
}

func (rbac *RBACService) GrantRoleToUser(userID, role string) error {
    userSubject := fmt.Sprintf("user:%s", userID)
    roleSubject := fmt.Sprintf("role:%s", role)
    
    return rbac.enforcer.AddRoleForUser(userSubject, roleSubject)
}

func (rbac *RBACService) RevokeRoleFromUser(userID, role string) error {
    userSubject := fmt.Sprintf("user:%s", userID)
    roleSubject := fmt.Sprintf("role:%s", role)
    
    return rbac.enforcer.DeleteRoleForUser(userSubject, roleSubject)
}
```

### 动态权限管理

```go
type PermissionManager struct {
    enforcer *casbin.Enforcer
    cache    *sync.Map // 权限缓存
}

func (pm *PermissionManager) HasPermission(userID, resource, action string) bool {
    cacheKey := fmt.Sprintf("%s:%s:%s", userID, resource, action)
    
    // 检查缓存
    if cached, ok := pm.cache.Load(cacheKey); ok {
        return cached.(bool)
    }
    
    // 实时检查权限
    permit, err := pm.checkPermission(userID, resource, action)
    if err != nil {
        pm.logger.Error("权限检查失败", "error", err)
        return false
    }
    
    // 缓存结果（带过期时间）
    pm.cache.Store(cacheKey, permit)
    time.AfterFunc(5*time.Minute, func() {
        pm.cache.Delete(cacheKey)
    })
    
    return permit
}

func (pm *PermissionManager) checkPermission(userID, resource, action string) (bool, error) {
    userSubject := fmt.Sprintf("user:%s", userID)
    
    // 超级管理员检查
    if userID == "1" {
        return true, nil
    }
    
    // 直接权限检查
    directPermit, err := pm.enforcer.Enforce(userSubject, resource, action)
    if err != nil {
        return false, err
    }
    if directPermit {
        return true, nil
    }
    
    // 角色权限检查
    roles, err := pm.enforcer.GetRolesForUser(userSubject)
    if err != nil {
        return false, err
    }
    
    for _, role := range roles {
        rolePermit, err := pm.enforcer.Enforce(role, resource, action)
        if err != nil {
            continue
        }
        if rolePermit {
            return true, nil
        }
    }
    
    return false, nil
}
```

### 资源级权限控制

```go
type ResourcePermissionChecker struct {
    permissionManager *PermissionManager
}

func (rpc *ResourcePermissionChecker) CheckUserResourceAccess(
    userID string, 
    resourceType string, 
    resourceID string, 
    action string,
) bool {
    // 构建资源路径
    resourcePath := fmt.Sprintf("/%s/%s", resourceType, resourceID)
    
    // 检查通用权限
    if rpc.permissionManager.HasPermission(userID, resourcePath, action) {
        return true
    }
    
    // 检查资源类型权限
    typePath := fmt.Sprintf("/%s/*", resourceType)
    if rpc.permissionManager.HasPermission(userID, typePath, action) {
        return true
    }
    
    // 检查所有资源权限
    allPath := "/*"
    return rpc.permissionManager.HasPermission(userID, allPath, action)
}

// 使用示例
func (oc *OrderController) getOrder(ctx *gin.Context) {
    orderID := ctx.Param("id")
    userID := getCurrentUserID(ctx)
    
    // 检查是否有权访问该订单
    if !resourceChecker.CheckUserResourceAccess(userID, "orders", orderID, "read") {
        ctx.JSON(403, gin.H{"error": "无权访问此订单"})
        return
    }
    
    // 获取订单详情
    order, err := oc.orderService.GetByID(orderID)
    if err != nil {
        ctx.JSON(500, gin.H{"error": "获取订单失败"})
        return
    }
    
    ctx.JSON(200, order)
}
```

## 最佳实践

### 1. 分层权限设计

```go
// ✅ 推荐的权限分层设计
type PermissionLayer struct {
    Resource string   // 资源路径
    Actions  []string // 允许的操作
    Roles    []string // 允许的角色
}

var permissionLayers = []PermissionLayer{
    {
        Resource: "/api/admin/*",
        Actions:  []string{"*"}, // 所有操作
        Roles:    []string{"admin", "super_admin"},
    },
    {
        Resource: "/api/users/*",
        Actions:  []string{"read"},
        Roles:    []string{"user", "admin"},
    },
    {
        Resource: "/api/users/*/profile",
        Actions:  []string{"read", "write"},
        Roles:    []string{"user", "admin"}, // 用户可以管理自己的资料
    },
}

func setupLayeredPermissions(enforcer *casbin.Enforcer) error {
    for _, layer := range permissionLayers {
        for _, role := range layer.Roles {
            roleSubject := fmt.Sprintf("role:%s", role)
            for _, action := range layer.Actions {
                if err := enforcer.AddPolicy(roleSubject, layer.Resource, action); err != nil {
                    return err
                }
            }
        }
    }
    return nil
}
```

### 2. 权限缓存策略

```go
type CachedPermissionService struct {
    permissionManager *PermissionManager
    cache             *redis.Client // 使用 Redis 缓存
    ttl               time.Duration
}

func (cps *CachedPermissionService) HasPermission(userID, resource, action string) bool {
    cacheKey := fmt.Sprintf("perm:%s:%s:%s", userID, resource, action)
    
    // 尝试从缓存获取
    if val, err := cps.cache.Get(context.Background(), cacheKey).Result(); err == nil {
        return val == "true"
    }
    
    // 实时检查权限
    hasPerm := cps.permissionManager.HasPermission(userID, resource, action)
    
    // 缓存结果
    permStr := "false"
    if hasPerm {
        permStr = "true"
    }
    
    cps.cache.Set(context.Background(), cacheKey, permStr, cps.ttl)
    
    return hasPerm
}

func (cps *CachedPermissionService) InvalidateUserCache(userID string) {
    pattern := fmt.Sprintf("perm:%s:*", userID)
    cps.cache.Del(context.Background(), pattern)
}
```

### 3. 审计日志

```go
type AccessLog struct {
    ID          uint      `gorm:"primaryKey"`
    UserID      string    `json:"user_id"`
    Resource    string    `json:"resource"`
    Action      string    `json:"action"`
    IP          string    `json:"ip"`
    UserAgent   string    `json:"user_agent"`
    Success     bool      `json:"success"`
    ErrorMessage string   `json:"error_message,omitempty"`
    Timestamp   time.Time `json:"timestamp"`
}

func logAccessAttempt(
    db *gorm.DB,
    ctx *gin.Context,
    resource, action string,
    success bool,
    errorMessage string,
) {
    userID := getCurrentUserID(ctx)
    
    accessLog := AccessLog{
        UserID:       userID,
        Resource:     resource,
        Action:       action,
        IP:           ctx.ClientIP(),
        UserAgent:    ctx.Request.UserAgent(),
        Success:      success,
        ErrorMessage: errorMessage,
        Timestamp:    time.Now(),
    }
    
    db.Create(&accessLog)
}

// 在权限检查中间件中使用
func auditedAuthorizationMiddleware(
    engine *abe.Engine, 
    resource, action string,
    db *gorm.DB,
) gin.HandlerFunc {
    return func(ctx *gin.Context) {
        userClaims, err := getCurrentUser(ctx)
        if err != nil {
            logAccessAttempt(db, ctx, resource, action, false, "未认证")
            ctx.JSON(401, gin.H{"error": "未认证"})
            ctx.Abort()
            return
        }
        
        enforcer := engine.Enforcer()
        allowed, err := enforcer.Enforce(
            fmt.Sprintf("user:%s", userClaims.UserID()),
            resource,
            action,
        )
        
        if err != nil || !allowed {
            logAccessAttempt(db, ctx, resource, action, false, "权限不足")
            ctx.JSON(403, gin.H{"error": "权限不足"})
            ctx.Abort()
            return
        }
        
        logAccessAttempt(db, ctx, resource, action, true, "")
        ctx.Next()
    }
}
```

## 故障排除

### 常见权限问题

1. **权限检查不生效**
   ```go
   // 调试权限检查
   func debugPermissionCheck(enforcer *casbin.Enforcer, userID, resource, action string) {
       userSubject := fmt.Sprintf("user:%s", userID)
       
       // 检查直接权限
       directPermit, _ := enforcer.Enforce(userSubject, resource, action)
       fmt.Printf("直接权限: %v\n", directPermit)
       
       // 检查角色权限
       roles, _ := enforcer.GetRolesForUser(userSubject)
       fmt.Printf("用户角色: %v\n", roles)
       
       for _, role := range roles {
           rolePermit, _ := enforcer.Enforce(role, resource, action)
           fmt.Printf("角色 %s 权限: %v\n", role, rolePermit)
       }
   }
   ```

2. **策略加载问题**
   ```go
   // 验证策略加载
   func validatePolicies(enforcer *casbin.Enforcer) {
       // 列出所有策略
       policies := enforcer.GetPolicy()
       fmt.Printf("加载的策略: %v\n", policies)
       
       // 列出所有角色关系
       roles := enforcer.GetGroupingPolicy()
       fmt.Printf("角色关系: %v\n", roles)
   }
   ```

3. **缓存一致性问题**
   ```go
   // 清理权限缓存
   func clearPermissionCache(userID string) {
       // 清理内存缓存
       permissionCache.Delete(fmt.Sprintf("user:%s:permissions", userID))
       
       // 清理 Redis 缓存
       pattern := fmt.Sprintf("perm:%s:*", userID)
       redisClient.Del(context.Background(), pattern)
   }
   
   // 在权限变更时调用
   func onPermissionChanged(userID string) {
       clearPermissionCache(userID)
   }
   ```