// Package config 负责读取、合并、转换并校验 API 启动所需的配置。
package config

import (
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"
)

// Config 是阶段 1 的总配置。
// 它只包含应用信息、HTTP Server 和日志配置，不提前加入数据库等后续阶段内容。
type Config struct {
	App  AppConfig  `mapstructure:"app"`
	HTTP HTTPConfig `mapstructure:"http"`
	Log  LogConfig  `mapstructure:"log"`
}

// AppConfig 描述应用自身的信息。
//
// mapstructure 标签告诉 Viper 应读取哪个 YAML 字段；
// validate 标签则描述最终值必须满足的规则。
type AppConfig struct {
	// Env 决定程序采用开发模式还是生产模式。
	Env string `mapstructure:"env" validate:"required,oneof=development production"`
	// Name 会作为 service 字段附加到每一条结构化日志中。
	Name string `mapstructure:"name" validate:"required"`
}

// HTTPConfig 描述监听地址以及 HTTP 生命周期中的各种超时。
type HTTPConfig struct {
	// Addr 是 net/http 使用的监听地址，例如 ":8080"。
	Addr string `mapstructure:"addr" validate:"required"`
	// ReadHeaderTimeout 限制读取请求头可花费的最长时间。
	ReadHeaderTimeout time.Duration `mapstructure:"read_header_timeout" validate:"gt=0"`
	// ReadTimeout 限制读取整个请求可花费的最长时间。
	ReadTimeout time.Duration `mapstructure:"read_timeout" validate:"gt=0"`
	// WriteTimeout 限制服务器写回响应可花费的最长时间。
	WriteTimeout time.Duration `mapstructure:"write_timeout" validate:"gt=0"`
	// IdleTimeout 限制 Keep-Alive 连接在空闲状态下可保留的时间。
	IdleTimeout time.Duration `mapstructure:"idle_timeout" validate:"gt=0"`
	// ShutdownTimeout 是收到退出信号后等待现有请求完成的最长时间。
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout" validate:"gt=0"`
}

// LogConfig 描述日志输出级别。
type LogConfig struct {
	// Level 越高输出越少；当前允许 debug、info、warn 和 error。
	Level string `mapstructure:"level" validate:"required,oneof=debug info warn error"`
}

// Load 从后端约定路径读取阶段 1 配置。
//
// 调用者应从 backend 目录启动程序，因此相对路径会指向
// backend/configs/default.yaml。
func Load() (*Config, error) {
	return loadFile("./configs/default.yaml")
}

// loadFile 执行完整配置流水线：
//
//	代码默认值 < YAML 文件 < 进程环境变量
//
// 参数化文件路径便于测试使用临时 YAML，而不依赖真实开发配置。
func loadFile(configFile string) (*Config, error) {
	// 使用独立 Viper 实例，避免包级全局状态污染其他测试或调用者。
	v := viper.New()
	setDefaults(v)
	v.SetConfigFile(configFile)

	// ReadInConfig 只负责读取 YAML；此时还没有写入强类型 Config。
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config file %q: %w", configFile, err)
	}

	// 左侧是 Viper/YAML 中的点分配置键，右侧是操作系统环境变量名。
	// BindEnv 只建立对应关系，不会主动读取项目根目录的 .env 文件。
	bindings := map[string]string{
		"app.env":                  "APP_ENV",
		"app.name":                 "APP_NAME",
		"http.addr":                "HTTP_ADDR",
		"log.level":                "LOG_LEVEL",
		"http.read_header_timeout": "HTTP_READ_HEADER_TIMEOUT",
		"http.read_timeout":        "HTTP_READ_TIMEOUT",
		"http.write_timeout":       "HTTP_WRITE_TIMEOUT",
		"http.idle_timeout":        "HTTP_IDLE_TIMEOUT",
		"http.shutdown_timeout":    "HTTP_SHUTDOWN_TIMEOUT",
	}

	for key, envName := range bindings {
		if err := v.BindEnv(key, envName); err != nil {
			return nil, fmt.Errorf("bind environment variable %s: %w", envName, err)
		}
	}

	// Unmarshal 按 mapstructure 标签把配置写入 cfg。
	// DecodeHook 把 "5s"、"10m" 等字符串转换成 time.Duration。
	var cfg Config
	if err := v.Unmarshal(
		&cfg,
		viper.DecodeHook(mapstructure.StringToTimeDurationHookFunc()),
	); err != nil {
		return nil, fmt.Errorf("decode configuration: %w", err)
	}

	// validate.Struct 真正执行结构体标签中的 required、oneof 和 gt 等规则。
	// 仅仅写 validate 标签并不会自动触发校验。
	validate := validator.New()
	if err := validate.Struct(&cfg); err != nil {
		return nil, fmt.Errorf("validate configuration: %w", err)
	}

	// required 只能判断地址非空，无法确认它是否真的是 host:port 格式，
	// 因此还要进行一层明确的网络地址校验。
	if err := validateHTTPAddress(cfg.HTTP.Addr); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// setDefaults 注册代码级安全默认值。
// default.yaml 通常会覆盖这些值；保留代码默认值可明确最低优先级配置。
func setDefaults(v *viper.Viper) {
	v.SetDefault("app.env", "development")
	v.SetDefault("app.name", "Muggle")
	v.SetDefault("log.level", "debug")
	v.SetDefault("http.addr", ":8080")
	v.SetDefault("http.read_header_timeout", "5s")
	v.SetDefault("http.read_timeout", "10s")
	v.SetDefault("http.write_timeout", "15s")
	v.SetDefault("http.idle_timeout", "60s")
	v.SetDefault("http.shutdown_timeout", "10s")
}

// validateHTTPAddress 验证监听地址使用 host:port 格式，且端口位于有效范围内。
//
// ":8080" 的 host 为空是合法的，表示监听本机所有可用网络接口。
func validateHTTPAddress(address string) error {
	_, portText, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("validate HTTP address %q: %w", address, err)
	}

	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("validate HTTP address %q: port must be between 1 and 65535", address)
	}

	return nil
}
