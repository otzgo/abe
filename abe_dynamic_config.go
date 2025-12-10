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
	cache  map[string]interface{} // 内存缓存
}

// SystemConfigModel 系统配置数据模型（简化版，避免循环依赖）
type SystemConfigModel struct {
	ID          uint   `gorm:"primaryKey"`
	Key         string `gorm:"column:key"`
	Value       string `gorm:"column:value"`
	ValueType   string `gorm:"column:value_type"`
	Name        string `gorm:"column:name"`
	Description string `gorm:"column:description"`
	Group       string `gorm:"column:group"`
	Enabled     bool   `gorm:"column:enabled"`
}

func (SystemConfigModel) TableName() string {
	return "system_configs"
}

// newDynamicConfigManager 创建动态配置管理器实例
func newDynamicConfigManager(db *gorm.DB, viper *viper.Viper, logger *slog.Logger) *DynamicConfigManager {
	return &DynamicConfigManager{
		db:     db,
		viper:  viper,
		logger: logger,
		cache:  make(map[string]interface{}),
	}
}

// LoadAll 从数据库加载所有启用的配置项到 Viper
// 在应用启动时调用，将数据库配置同步到 Viper 内存中
func (m *DynamicConfigManager) LoadAll() error {
	var configs []SystemConfigModel
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
		m.cache[cfg.Key] = value

		if m.logger != nil {
			m.logger.Info("加载动态配置", "key", cfg.Key, "value", value, "type", cfg.ValueType)
		}
	}

	return nil
}

// Update 更新单个配置项
// 同时更新数据库和 Viper 内存配置，确保立即生效
func (m *DynamicConfigManager) Update(key, value string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 从数据库查询配置项
	var cfg SystemConfigModel
	if err := m.db.Where("`key` = ?", key).First(&cfg).Error; err != nil {
		return fmt.Errorf("配置项不存在: %w", err)
	}

	// 解析并验证新值
	parsedValue, err := m.parseValue(value, cfg.ValueType)
	if err != nil {
		return fmt.Errorf("配置值格式错误: %w", err)
	}

	// 更新数据库
	if err := m.db.Model(&SystemConfigModel{}).Where("`key` = ?", key).Update("value", value).Error; err != nil {
		return fmt.Errorf("更新配置失败: %w", err)
	}

	// 立即更新 Viper（立即生效）
	m.viper.Set(key, parsedValue)
	m.cache[key] = parsedValue

	if m.logger != nil {
		m.logger.Info("更新动态配置", "key", key, "value", parsedValue)
	}

	return nil
}

// Get 获取配置值（优先从 Viper 读取）
func (m *DynamicConfigManager) Get(key string) interface{} {
	return m.viper.Get(key)
}

// GetString 获取字符串类型配置
func (m *DynamicConfigManager) GetString(key string) string {
	return m.viper.GetString(key)
}

// GetBool 获取布尔类型配置
func (m *DynamicConfigManager) GetBool(key string) bool {
	return m.viper.GetBool(key)
}

// GetInt 获取整数类型配置
func (m *DynamicConfigManager) GetInt(key string) int {
	return m.viper.GetInt(key)
}

// GetDuration 获取时间段类型配置
func (m *DynamicConfigManager) GetDuration(key string) time.Duration {
	return m.viper.GetDuration(key)
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

// Reload 重新加载所有配置（可用于多实例同步）
func (m *DynamicConfigManager) Reload() error {
	return m.LoadAll()
}
