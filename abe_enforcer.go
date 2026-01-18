package abe

import (
	"fmt"
	"log/slog"

	"github.com/casbin/casbin/v3"
	"github.com/casbin/casbin/v3/model"
	gormadapter "github.com/casbin/gorm-adapter/v3"
	"github.com/spf13/viper"
	"gorm.io/gorm"
)

// newEnforcer 使用 GORM 适配器初始化 Casbin 权限控制器
// 失败时直接 panic，与 newDB 等初始化风格保持一致
func newEnforcer(db *gorm.DB, logger *slog.Logger, cfg *viper.Viper) *casbin.Enforcer {
	m, err := model.NewModelFromString(rbacModel)
	if err != nil {
		panic(fmt.Errorf("加载Casbin模型失败: %w", err))
	}

	table := cfg.GetString("casbin.policy_table")
	if table == "" {
		table = "casbin_rule"
	}

	// 使用 NewAdapterByDBUseTableName 来自定义表名
	// prefix 参数为空字符串表示不使用前缀
	a, err := gormadapter.NewAdapterByDBUseTableName(db, "", table)
	if err != nil {
		panic(fmt.Errorf("创建Casbin适配器失败: %w", err))
	}
	enf, err := casbin.NewEnforcer(m, a)
	if err != nil {
		panic(fmt.Errorf("创建Casbin权限控制器失败: %w", err))
	}
	if err := enf.LoadPolicy(); err != nil {
		panic(fmt.Errorf("加载Casbin策略失败: %w", err))
	}
	if logger != nil {
		logger.Info("Casbin权限控制器已初始化")
	}
	return enf
}

const rbacModel = `
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
`
