package abe

import (
	"errors"
	"fmt"
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
		// 当参数为 -h 或 --help 时,flags.Parse() 返回 pflag.ErrHelp,这不是错误
		if errors.Is(err, pflag.ErrHelp) {
			os.Exit(0)
		}
		panic(fmt.Errorf("致命错误解析命令行参数：%w", err))
	}

	// 从 flag 中获取 configDir
	configDir, _ := flags.GetString("config-dir")

	// 加载 .env 文件
	loadEnvFiles(configDir)

	// 创建 viper 实例
	config := viper.New()

	// 配置文件设置
	config.SetConfigName(configName)
	config.SetConfigType(configType)
	// 添加配置文件搜索路径
	for _, path := range getConfigPaths(configDir) {
		config.AddConfigPath(path)
	}

	// 读取配置文件
	handleConfigFileRead(config)

	// 配置环境变量支持
	setupEnvConfig(config)

	// 绑定 flags 到 viper（优先级最高）
	// 注意：flag 名称使用点号分隔的嵌套格式（如 "server.address"）
	// 这样可以直接映射到配置文件的嵌套结构，确保 flag 优先级高于配置文件
	if err := config.BindPFlags(flags); err != nil {
		panic(fmt.Errorf("致命错误绑定命令行参数到配置：%w", err))
	}

	return config
}

// getConfigPaths 获取配置文件搜索路径列表
// 根据 configDir 参数返回统一的搜索路径列表
func getConfigPaths(configDir string) []string {
	var paths []string

	paths = append(paths, "./configs")
	if configDir != "" {
		homeDir, err := os.UserHomeDir()
		if err == nil {
			paths = append(paths, filepath.Join(homeDir, fmt.Sprintf(".%s", configDir)))
		}
		paths = append(paths, fmt.Sprintf("/etc/%s/", configDir))
	}

	return paths
}

// createFlags 创建并定义所有命令行 flags
func createFlags() *pflag.FlagSet {
	flags := pflag.NewFlagSet("abe", pflag.ContinueOnError)

	// 配置目录 flag
	flags.String("config-dir", defaultConfigDir, "config directory")

	// 服务器配置 flags（使用嵌套键格式以匹配配置文件结构）
	flags.String("server.address", "", "server listen address (e.g., :8080)")
	flags.String("server.mode", "", "server mode (debug, release)")
	flags.String("server.shutdown_timeout", "", "server graceful shutdown timeout (e.g., 5s)")

	// 应用配置 flags
	flags.String("app.name", "", "application name")
	flags.Bool("app.debug", false, "enable debug mode")

	// 日志配置 flags
	flags.String("logger.level", "", "log level (debug, info, warn, error)")
	flags.String("logger.format", "", "log format (text, json)")
	flags.String("logger.type", "", "log output type (console, file)")

	// 数据库配置 flags
	flags.String("database.type", "", "database type (mysql, postgres)")
	flags.String("database.host", "", "database host")
	flags.Int("database.port", 0, "database port")
	flags.String("database.user", "", "database username")
	flags.String("database.password", "", "database password")
	flags.String("database.dbname", "", "database name")

	return flags
}

// setupEnvConfig 配置 viper 的环境变量支持
func setupEnvConfig(config *viper.Viper) {
	config.SetEnvPrefix(envPrefix)                          // 环境变量前缀
	config.AutomaticEnv()                                   // 自动读取环境变量
	config.SetEnvKeyReplacer(strings.NewReplacer(".", "_")) // 支持嵌套配置，如 server.address -> ABE_SERVER_ADDRESS
}

// handleConfigFileRead 读取配置文件并处理错误
func handleConfigFileRead(config *viper.Viper) {
	err := config.ReadInConfig()
	if err != nil {
		// 判断是否是配置文件不存在的错误
		var configFileNotFoundError viper.ConfigFileNotFoundError
		if errors.As(err, &configFileNotFoundError) {
			panic(fmt.Errorf("致命错误配置文件未找到：%w", err))
		}
		panic(fmt.Errorf("读取配置文件失败：%w", err))
	}
}

// loadEnvFiles 加载 .env 文件
// 从多个位置查找 .env 文件并加载到环境变量中
func loadEnvFiles(configDir string) {
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
				return
			}
		}
	}

	// 未找到 .env 文件，这是正常情况，不需要记录日志
}
