package abe

import (
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/spf13/viper"
	"gorm.io/gorm"
)

// DynamicConfigManager 动态配置管理器
// 负责从数据库加载配置项到 Viper，并提供统一的配置访问接口
type DynamicConfigManager struct {
	db     *gorm.DB
	viper  *viper.Viper
	logger *slog.Logger
	mu     sync.RWMutex
}

// SystemConfig 系统配置表
// 用于存储可通过管理后台动态修改的系统配置项
type SystemConfig struct {
	gorm.Model
	Key         string `gorm:"column:key;uniqueIndex;size:100;not null;comment:配置键(如 business.recharge.reversal_window)"` // 配置键
	Value       string `gorm:"column:value;size:500;not null;comment:配置值"`                                                // 配置值
	ValueType   string `gorm:"column:value_type;size:20;not null;default:'string';comment:值类型"`                           // 值类型
	Name        string `gorm:"column:name;size:100;not null;comment:配置项名称"`                                               // 配置项名称
	Description string `gorm:"column:description;size:500;comment:配置项描述"`                                                 // 配置项描述
	Group       string `gorm:"column:group;size:50;index;comment:配置分组"`                                                   // 配置分组 (如 business, system)
	Enabled     bool   `gorm:"column:enabled;not null;default:true;comment:是否启用"`                                         // 是否启用
}

func (SystemConfig) TableName() string {
	return "system_configs"
}

// newDynamicConfigManager 创建动态配置管理器实例
func newDynamicConfigManager(db *gorm.DB, viper *viper.Viper, logger *slog.Logger) *DynamicConfigManager {
	return &DynamicConfigManager{
		db:     db,
		viper:  viper,
		logger: logger,
	}
}

// LoadAll 从数据库加载所有启用的配置项到 Viper
// 在应用启动时调用，将数据库配置同步到 Viper 内存中
func (m *DynamicConfigManager) LoadAll() error {
	var configs []SystemConfig
	if err := m.db.Where("enabled = ?", true).Find(&configs).Error; err != nil {
		return fmt.Errorf("加载动态配置失败: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, cfg := range configs {
		// 解析并验证配置值
		value, err := m.parseValue(cfg.Value, cfg.ValueType)
		if err != nil {
			if m.logger != nil {
				m.logger.Warn("解析配置值失败，跳过该配置", "key", cfg.Key, "value", cfg.Value, "type", cfg.ValueType, "error", err)
			}
			continue
		}

		// 设置到 Viper（立即生效）
		m.viper.Set(cfg.Key, value)

		if m.logger != nil {
			m.logger.Info("加载动态配置", "key", cfg.Key, "value", value, "type", cfg.ValueType)
		}
	}

	return nil
}

func (m *DynamicConfigManager) Reload() error {
	return m.LoadAll()
}

// Update 更新单个配置项
// 同时更新数据库和 Viper 内存配置，确保立即生效
func (m *DynamicConfigManager) Update(key, value string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 从数据库查询配置项
	var cfg SystemConfig
	if err := m.db.Where("`key` = ?", key).First(&cfg).Error; err != nil {
		return fmt.Errorf("配置项不存在: %w", err)
	}

	// 解析并验证新值
	parsedValue, err := m.parseValue(value, cfg.ValueType)
	if err != nil {
		return fmt.Errorf("配置值格式错误: %w", err)
	}

	// 更新数据库
	if err := m.db.Model(&SystemConfig{}).Where("`key` = ?", key).Update("value", value).Error; err != nil {
		return fmt.Errorf("更新配置失败: %w", err)
	}

	// 立即更新 Viper（立即生效）
	m.viper.Set(key, parsedValue)

	if m.logger != nil {
		m.logger.Info("更新动态配置", "key", key, "value", parsedValue)
	}

	return nil
}

// parseValue 根据类型解析配置值
func (m *DynamicConfigManager) parseValue(value, valueType string) (interface{}, error) {
	switch valueType {
	case "string":
		return value, nil
	case "bool":
		return strconv.ParseBool(value)
	case "int":
		return strconv.Atoi(value)
	case "float":
		return strconv.ParseFloat(value, 64)
	case "duration":
		// 验证 duration 格式
		d, err := time.ParseDuration(value)
		if err != nil {
			return nil, fmt.Errorf("无效的 duration 格式: %w", err)
		}
		return d, nil
	default:
		return value, nil
	}
}
