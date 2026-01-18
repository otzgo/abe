package abe

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
	"gopkg.in/natefinch/lumberjack.v2"
)

// LogConfig 日志配置结构体
// 定义日志级别、格式、输出类型和文件相关配置
type LogConfig struct {
	Level  string `mapstructure:"level"`  // 日志级别，如 "debug", "info", "warn", "error"
	Format string `mapstructure:"format"` // 日志格式，如 "json" 或 "text"
	Type   string `mapstructure:"type"`   // 日志输出类型，如 "console" 或 "file"
	File   struct {
		Path       string `mapstructure:"path"`        // 日志文件路径，仅在 type 为 "file" 时有效
		MaxSize    int    `mapstructure:"max_size"`    // 每个日志文件最大尺寸，单位为MB
		MaxBackups int    `mapstructure:"max_backups"` // 保留的旧日志文件最大数量
		MaxAge     int    `mapstructure:"max_age"`     // 保留旧日志文件的最大天数
		Compress   bool   `mapstructure:"compress"`    // 是否压缩旧日志文件
	} `mapstructure:"file"` // 文件日志配置，仅在 type 为 "file" 时有效
}

// newLogger 获取日志记录器
// 根据配置初始化日志系统
// 支持控制台和文件日志输出，根据环境自动配置日志级别和格式
func newLogger(cfg *viper.Viper) *slog.Logger {
	var lc LogConfig
	err := cfg.UnmarshalKey("logger", &lc)
	if err != nil {
		panic(fmt.Sprintf("解析日志配置失败: %v", err))
	}
	// 根据环境设置默认配置
	setDefaultLogConfig(cfg, &lc)

	// 解析日志级别
	level, err := LevelFromString(lc.Level)
	if err != nil {
		panic(fmt.Sprintf("解析日志级别失败: %v", err))
	}

	// 确定日志输出目标
	var logWriter io.Writer
	if lc.Type == "file" && lc.File.Path != "" {
		// 确保日志目录存在
		logDir := filepath.Dir(lc.File.Path)
		if err := os.MkdirAll(logDir, 0755); err != nil {
			panic(fmt.Sprintf("创建日志目录失败: %v", err))
		}

		// 使用 lumberjack 进行日志切割
		logWriter = &lumberjack.Logger{
			Filename:   lc.File.Path,
			MaxSize:    lc.File.MaxSize,    // 每个日志文件最大尺寸，单位为MB
			MaxBackups: lc.File.MaxBackups, // 保留的旧日志文件最大数量
			MaxAge:     lc.File.MaxAge,     // 保留旧日志文件的最大天数
			Compress:   lc.File.Compress,   // 是否压缩旧日志文件
		}
	} else {
		// 默认输出到控制台
		logWriter = os.Stdout
	}

	// 创建日志处理器
	var handler slog.Handler
	if lc.Format == "json" {
		handler = slog.NewJSONHandler(logWriter, &slog.HandlerOptions{
			Level: level,
		})
	} else {
		handler = slog.NewTextHandler(logWriter, &slog.HandlerOptions{
			Level: level,
		})
	}

	// 创建日志记录器
	logger := slog.New(handler)

	return logger
}

// LevelFromString 将字符串解析为 slog.Level
// 支持: "debug", "info", "warn", "warning", "error"
// 如果级别无效，返回 LevelInfo 和错误
func LevelFromString(levelStr string) (slog.Level, error) {
	switch strings.ToLower(levelStr) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("无效的日志级别: %q，使用默认级别 info", levelStr)
	}
}

// setDefaultLogConfig 根据运行环境设置默认日志配置
// 开发环境：日志级别为 Debug，输出到控制台
// 生产环境：日志级别默认为 Info，输出到文件，文件路径为用户家目录下的特定目录
func setDefaultLogConfig(cfg *viper.Viper, lc *LogConfig) {
	// 如果是开发环境
	if cfg.GetBool("app.debug") {
		lc.Level = "debug"
	}
	if lc.Level == "" {
		lc.Level = "info"
	}
	// 如果日志类型为空，则设置为文件
	if lc.Type == "" {
		lc.Type = "console"
	}
	// 如果输出到文件但没有设置文件路径，则设置默认路径
	if lc.Type == "file" && lc.File.Path == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			panic(fmt.Sprintf("无法获取用户家目录: %v", err))
		}
		lc.File.Path = filepath.Join(homeDir, cfg.GetString("app.name"), "logs", "app.log")
	}
}
