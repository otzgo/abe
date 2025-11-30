package abe

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// 配置相关常量
const (
	envPrefix        = "ABE"    // 环境变量前缀
	configName       = "config" // 配置文件名称（不含扩展名）
	configType       = "yaml"   // 配置文件类型
	defaultConfigDir = "abe"    // 默认配置目录
)

func newConfig() *viper.Viper {
	// 创建并解析 flags
	flags := createFlags()
	if err := flags.Parse(os.Args[1:]); err != nil {
		// 解析错误，记录日志并使用默认值继续
		_getBasicLogger(slog.LevelWarn).Warn("解析命令行参数失败，将忽略 CLI 配置", "error", err.Error())
	}

	// 从 flag 中获取 configDir
	configDir, _ := flags.GetString("config-dir")

	// 加载 .env 文件
	_loadEnvFiles(configDir)

	// 创建 viper 实例
	config := viper.New()

	// 绑定 flags 到 viper（优先级最高）
	if err := config.BindPFlags(flags); err != nil {
		// 绑定失败，记录警告但继续运行
		_getBasicLogger(slog.LevelWarn).Warn("绑定命令行参数到配置失败", "error", err.Error())
	}

	// 配置环境变量支持
	_setupEnvConfig(config)

	// 配置文件设置
	config.SetConfigName(configName)
	config.SetConfigType(configType)
	// 添加配置文件搜索路径
	for _, path := range getConfigPaths(configDir) {
		config.AddConfigPath(path)
	}

	// 读取配置文件
	_handleConfigFileRead(config)

	return config
}

// getConfigPaths 获取配置文件搜索路径列表
// 根据 configDir 参数返回统一的搜索路径列表
func getConfigPaths(configDir string) []string {
	var paths []string

	if configDir != "" {
		paths = append(paths, fmt.Sprintf("/etc/%s/", configDir))
		homeDir, err := os.UserHomeDir()
		if err == nil {
			paths = append(paths, filepath.Join(homeDir, fmt.Sprintf(".%s", configDir)))
		}
	}
	paths = append(paths, "./configs")
	paths = append(paths, ".")

	return paths
}

// createFlags 创建并定义所有命令行 flags
func createFlags() *pflag.FlagSet {
	flags := pflag.NewFlagSet("abe", pflag.ContinueOnError)

	// 配置目录 flag
	flags.String("config-dir", defaultConfigDir, "config directory")

	// 服务器配置 flags
	flags.String("server-address", "", "server listen address (e.g., :8080)")
	flags.String("server-mode", "", "server mode (debug, release)")
	flags.String("server-shutdown-timeout", "", "server graceful shutdown timeout (e.g., 5s)")

	// 应用配置 flags
	flags.String("app-name", "", "application name")
	flags.Bool("app-debug", false, "enable debug mode")

	// 日志配置 flags
	flags.String("logger-level", "", "log level (debug, info, warn, error)")
	flags.String("logger-format", "", "log format (text, json)")
	flags.String("logger-type", "", "log output type (console, file)")

	// 数据库配置 flags
	flags.String("database-type", "", "database type (mysql, postgres)")
	flags.String("database-host", "", "database host")
	flags.Int("database-port", 0, "database port")
	flags.String("database-user", "", "database username")
	flags.String("database-password", "", "database password")
	flags.String("database-dbname", "", "database name")

	return flags
}

// _setupEnvConfig 配置 viper 的环境变量支持
func _setupEnvConfig(config *viper.Viper) {
	config.SetEnvPrefix(envPrefix)                                    // 环境变量前缀
	config.AutomaticEnv()                                             // 自动读取环境变量
	config.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_")) // 支持嵌套配置，如 server.address -> ABE_SERVER_ADDRESS
}

// _handleConfigFileRead 读取配置文件并处理错误
func _handleConfigFileRead(config *viper.Viper) {
	err := config.ReadInConfig()
	if err != nil {
		// 判断是否是配置文件不存在的错误
		var configFileNotFoundError viper.ConfigFileNotFoundError
		if errors.As(err, &configFileNotFoundError) {
			// 配置文件不存在，记录警告日志
			_getBasicLogger(slog.LevelWarn).Warn("配置文件未找到，将使用环境变量和默认值", "error", err.Error())
		} else {
			// 其他错误，panic
			panic(fmt.Errorf("致命错误读取配置文件：%w", err))
		}
	}
}

// _loadEnvFiles 加载 .env 文件
// 从多个位置查找 .env 文件并加载到环境变量中
func _loadEnvFiles(configDir string) {
	// 获取配置文件搜索路径列表
	configPaths := getConfigPaths(configDir)

	// 构建 .env 文件路径列表
	var envPaths []string
	for _, path := range configPaths {
		envPaths = append(envPaths, filepath.Join(path, ".env"))
	}

	// 尝试加载第一个存在的 .env 文件
	for _, envPath := range envPaths {
		if _, err := os.Stat(envPath); err == nil {
			// 文件存在，尝试加载
			if err := godotenv.Load(envPath); err == nil {
				// 加载成功，记录日志并返回
				_getBasicLogger(slog.LevelInfo).Info("成功加载 .env 文件", "path", envPath)
				return
			}
		}
	}

	// 未找到 .env 文件，这是正常情况，不需要记录日志
}

// _getBasicLogger 创建基础的日志记录器
// 用于在配置系统初始化期间记录日志
func _getBasicLogger(level slog.Level) *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	}))
}
