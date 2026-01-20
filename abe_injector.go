package abe

import (
	"log/slog"

	"github.com/casbin/casbin/v3"
	"github.com/panjf2000/ants/v2"
	"github.com/samber/do/v2"
	"github.com/spf13/viper"
	"gorm.io/gorm"
)

func newRootScope(
	config *viper.Viper,
	db *gorm.DB,
	eventBus EventBus,
	pool *ants.Pool,
	logger *slog.Logger,
	enforcer *casbin.Enforcer,
) *do.RootScope {
	rs := do.New()

	do.ProvideValue(rs, config)   // *viper.Viper
	do.ProvideValue(rs, logger)   // *slog.Logger
	do.ProvideValue(rs, db)       // *gorm.DB
	do.ProvideValue(rs, eventBus) // EventBus
	do.ProvideValue(rs, pool)     // *ants.Pool
	do.ProvideValue(rs, enforcer) // *casbin.Enforcer

	return rs
}
