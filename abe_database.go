package abe

import (
	"fmt"

	"github.com/spf13/viper"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// DbConfig 数据库配置
type DbConfig struct {
	Type      string `mapstructure:"type"`       // 数据库类型，目前仅支持 mysql
	Host      string `mapstructure:"host"`       // 数据库主机地址
	Port      int    `mapstructure:"port"`       // 数据库端口号
	User      string `mapstructure:"user"`       // 数据库用户名
	Password  string `mapstructure:"password"`   // 数据库密码
	DBName    string `mapstructure:"dbname"`     // 数据库名称
	Charset   string `mapstructure:"charset"`    // 字符集
	ParseTime string `mapstructure:"parse_time"` // 解析时间格式
	Loc       string `mapstructure:"loc"`        // 时间区域
}

func newDB(cfg *viper.Viper) *gorm.DB {
	var dbCfg DbConfig
	err := cfg.UnmarshalKey("database", &dbCfg)
	if err != nil {
		panic(fmt.Errorf("fatal error database config: %w", err))
	}
	sdn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=%s&parseTime=%s&loc=%s",
		dbCfg.User,
		dbCfg.Password,
		dbCfg.Host,
		dbCfg.Port,
		dbCfg.DBName,
		dbCfg.Charset,
		dbCfg.ParseTime,
		dbCfg.Loc,
	)
	db, err := gorm.Open(mysql.Open(sdn), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		panic(fmt.Errorf("致命错误数据库连接：%w", err))
	}
	// 如果是开发模式，则打印 SQL
	if cfg.GetBool("app.debug") {
		db = db.Debug() // 打印 SQL
	}
	return db
}
