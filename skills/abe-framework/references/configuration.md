# 配置管理系统

## 配置系统概述

ABE 框架采用多层配置系统，支持命令行参数、环境变量和配置文件三种配置方式，按照优先级从高到低排序：

1. **命令行参数 (CLI Flags)** - 优先级最高
2. **环境变量 (Environment Variables)** - 优先级中等  
3. **配置文件 (Configuration Files)** - 优先级最低

## 配置文件管理

### 文件格式和位置

- 文件名: `config.yaml`
- 格式: YAML
- 搜索路径（按优先级顺序）:
  1. `./configs` (当前目录下的 configs 子目录)
  2. `~/.<config-dir>/` (用户主目录下的配置目录，默认为 ~/.abe/)
  3. `/etc/<config-dir>/` (系统级配置目录，<config-dir> 为 --config-dir 参数指定的目录名，默认为 abe)

### 完整配置文件示例

```yaml
# 应用程序配置
app:
  name: "my-app"                      # 应用程序名称
  debug: false                        # 是否开启调试模式
  
# 服务器配置
server:
  address: ":8080"                    # 服务器监听地址
  shutdown_timeout: "5s"              # 服务器优雅关闭超时时间
  
# 跨域资源共享配置
cors:                               
  allow_origins:                    # 允许的源
    - "*"
  allow_methods:                    # 允许的 HTTP 方法
    - "GET"
    - "POST"
    - "PUT"
    - "DELETE"
    - "OPTIONS"
  allow_headers:                    # 允许的请求头
    - "Content-Type"
    - "Content-Length"
    - "Accept"
    - "Accept-Encoding"
    - "Authorization"
    - "Origin"
    - "Cache-Control"
    - "X-Requested-With"
  expose_headers:                   # 暴露给浏览器的响应头
    - ""
  allow_credentials: false          # 是否允许凭据
  max_age_seconds: 86400            # 预检请求缓存时间（秒）

# 数据库配置
database:
  type: "mysql"                       # 数据库类型 (mysql, postgres)
  host: "localhost"                   # 数据库主机地址
  port: 3306                          # 数据库端口
  user: "root"                        # 数据库用户名
  password: "password"                # 数据库密码
  dbname: "myapp"                     # 数据库名称
  charset: "utf8mb4"                  # 字符集
  parse_time: "True"                  # 解析时间格式
  loc: "Local"                        # 时区

# 日志配置
logger:
  level: "info"                       # 日志级别 (debug, info, warn, error)
  format: "json"                      # 日志格式 (text, json)
  type: "console"                     # 输出类型 (console, file)
  file:                               # 文件日志配置（当 type 为 file 时有效）
    path: "./logs/app.log"            # 日志文件路径
    max_size: 100                     # 单个日志文件最大尺寸（MB）
    max_backups: 10                   # 保留的旧日志文件最大数量
    max_age: 30                       # 保留旧日志文件的最大天数
    compress: true                    # 是否压缩旧日志文件

# Swagger 配置
swagger:
  enabled: true                       # 是否启用 Swagger 文档
  url: ""                             # Swagger 文档的自定义 URL
  instance: ""                        # Swagger 实例名称

# 协程池配置
pool:
  size: 50000                         # 协程池大小
  expiry_duration: "10s"              # 协程过期时间
  pre_alloc: true                     # 是否预分配内存
  max_blocking_tasks: 10000           # 最大阻塞任务数
  nonblocking: false                  # 是否为非阻塞模式

# 验证器配置
validator:
  locale: "zh"                        # 默认语言 (zh, en)

# 多语言配置
i18n:
  default_language: "zh"              # 默认语言
  lang_query_key: "lang"              # 语言查询参数键名
  lang_header: "Accept-Language"      # 语言请求头键名
  message_paths:                      # 翻译文件路径列表
    - "./configs/i18n/locales"
    - "./custom_locales"

# 事件系统配置
event:
  output_buffer: 128                  # 事件输出缓冲区大小

# Casbin 权限配置
casbin:
  policy_table: "casbin_rule"         # 策略表名
```

## 环境变量使用

环境变量使用 `ABE_` 前缀，并将配置键中的点（`.`）替换为下划线（`_`）。

### 环境变量示例

```bash
# 服务器配置
export ABE_SERVER_ADDRESS=":8080"
export ABE_SERVER_SHUTDOWN_TIMEOUT="10s"

# 应用配置
export ABE_APP_NAME="my-app"
export ABE_APP_DEBUG="true"

# 数据库配置
export ABE_DATABASE_TYPE="mysql"
export ABE_DATABASE_HOST="localhost"
export ABE_DATABASE_PORT="3306"
export ABE_DATABASE_USER="root"
export ABE_DATABASE_PASSWORD="password"
export ABE_DATABASE_DBNAME="myapp"

# 日志配置
export ABE_LOGGER_LEVEL="debug"
export ABE_LOGGER_FORMAT="json"
export ABE_LOGGER_TYPE="file"
```

## 命令行参数

命令行参数具有最高优先级，可以直接覆盖配置文件和环境变量中的设置。

### 常用命令行参数

```bash
# 查看所有可用参数
./myapp -h
./myapp --help

# 启动应用并指定配置
./myapp --server.address=:9090 --app.debug=true

# 指定配置目录
./myapp --config-dir=myconfig --database.host=prod-db.example.com
```

### 主要参数列表

| 参数 | 类型 | 默认值 | 描述 |
|------|------|--------|------|
| `--config-dir` | string | `abe` | 配置目录 |
| `--server.address` | string | `` | 服务器监听地址 |
| `--server.shutdown_timeout` | string | `` | 优雅关闭超时时间 |
| `--app.name` | string | `` | 应用程序名称 |
| `--app.debug` | bool | `false` | 启用调试模式 |
| `--logger.level` | string | `` | 日志级别 |
| `--logger.format` | string | `` | 日志格式 |
| `--logger.type` | string | `` | 日志输出类型 |
| `--database.type` | string | `` | 数据库类型 |
| `--database.host` | string | `` | 数据库主机 |
| `--database.port` | int | `0` | 数据库端口 |
| `--database.user` | string | `` | 数据库用户名 |
| `--database.password` | string | `` | 数据库密码 |
| `--database.dbname` | string | `` | 数据库名称 |

## .env 文件支持

框架支持从 `.env` 文件加载环境变量，`.env` 文件的搜索路径与配置文件相同。

### .env 文件示例

```bash
# .env 文件
ABE_SERVER_ADDRESS=:8080
ABE_DATABASE_HOST=localhost
ABE_DATABASE_PORT=3306
ABE_DATABASE_USER=root
ABE_DATABASE_PASSWORD=password
ABE_DATABASE_DBNAME=myapp
ABE_APP_NAME=my-application
ABE_APP_DEBUG=true
```

## 配置读取和使用

### 在代码中读取配置

```go
func main() {
    engine := abe.NewEngine()
    
    // 读取配置值
    config := engine.Config()
    
    // 基本类型读取
    serverAddr := config.GetString("server.address")
    debugMode := config.GetBool("app.debug")
    dbPort := config.GetInt("database.port")
    timeout := config.GetDuration("server.shutdown_timeout")
    
    // 设置默认值
    if serverAddr == "" {
        serverAddr = ":8080" // 默认端口
    }
    
    // 读取嵌套配置
    dbConfig := config.Sub("database")
    if dbConfig != nil {
        host := dbConfig.GetString("host")
        user := dbConfig.GetString("user")
    }
    
    engine.Run()
}
```

### 配置验证

```go
// 在应用启动时验证必要配置
func validateConfig(config *viper.Viper) error {
    requiredConfigs := []string{
        "server.address",
        "database.host",
        "database.user",
        "database.dbname",
    }
    
    for _, key := range requiredConfigs {
        if config.GetString(key) == "" {
            return fmt.Errorf("缺少必要配置: %s", key)
        }
    }
    
    return nil
}
```

## 动态配置

框架支持运行时动态修改配置，这些配置存储在数据库的 `system_configs` 表中。

### 系统配置表结构

| 字段 | 类型 | 描述 |
|------|------|------|
| `key` | string | 配置键 |
| `value` | string | 配置值 |
| `value_type` | string | 值类型 |
| `name` | string | 配置项名称 |
| `description` | string | 配置项描述 |
| `group` | string | 配置分组 |
| `enabled` | bool | 是否启用 |

## 最佳实践

### 1. 配置组织原则
```go
// ✅ 好的做法：按功能模块组织配置
type AppConfig struct {
    Name  string `mapstructure:"name"`
    Debug bool   `mapstructure:"debug"`
}

type DatabaseConfig struct {
    Host     string `mapstructure:"host"`
    Port     int    `mapstructure:"port"`
    User     string `mapstructure:"user"`
    Password string `mapstructure:"password"`
    DBName   string `mapstructure:"dbname"`
}

// 在初始化时统一加载配置
func loadConfig() (*AppConfig, *DatabaseConfig, error) {
    config := engine.Config()
    
    var appCfg AppConfig
    var dbCfg DatabaseConfig
    
    if err := config.UnmarshalKey("app", &appCfg); err != nil {
        return nil, nil, err
    }
    
    if err := config.UnmarshalKey("database", &dbCfg); err != nil {
        return nil, nil, err
    }
    
    return &appCfg, &dbCfg, nil
}
```

### 2. 环境区分配置
```go
// 根据环境加载不同配置
func setupEnvironmentConfig(config *viper.Viper) {
    env := config.GetString("app.env")
    if env == "" {
        env = "development" // 默认开发环境
    }
    
    switch env {
    case "production":
        config.Set("logger.level", "error")
        config.Set("app.debug", false)
    case "staging":
        config.Set("logger.level", "warn")
        config.Set("app.debug", false)
    default: // development
        config.Set("logger.level", "debug")
        config.Set("app.debug", true)
    }
}
```

### 3. 敏感信息处理
```go
// ❌ 避免：在配置文件中存储敏感信息
/*
database:
  password: "mysecretpassword"  # 不安全
*/

// ✅ 推荐：使用环境变量或密钥管理
func getDatabasePassword() string {
    // 优先从环境变量获取
    if password := os.Getenv("DATABASE_PASSWORD"); password != "" {
        return password
    }
    
    // 回退到配置文件（仅用于开发环境）
    return engine.Config().GetString("database.password")
}
```

## 故障排除

### 常见配置问题

1. **配置文件未找到**
   - 检查配置文件是否存在指定路径
   - 确认文件名是否正确（config.yaml）
   - 验证文件格式是否为有效的 YAML

2. **配置值为空**
   - 检查配置键名是否正确
   - 确认配置层级是否正确
   - 验证环境变量前缀是否正确

3. **配置优先级问题**
   - 理解配置优先级顺序
   - 使用 `config.Debug()` 查看配置加载详情
   - 检查是否有重复的配置源

4. **类型转换错误**
   - 确保配置值与期望类型匹配
   - 使用适当的 Get 方法（GetString, GetInt, GetBool等）
   - 为数值类型配置设置合理的默认值