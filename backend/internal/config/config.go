// Package config 负责读取、合并、转换并校验 API 启动所需的配置。
package config

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"
)

// Config 是 API 进程的总配置。
// 阶段二在阶段一已有的应用、HTTP 和日志配置基础上加入 MySQL，
// 阶段三继续加入管理员认证所需的 JWT 与 Refresh Cookie 配置。
type Config struct {
	App   AppConfig   `mapstructure:"app"`
	HTTP  HTTPConfig  `mapstructure:"http"`
	Log   LogConfig   `mapstructure:"log"`
	MySQL MySQLConfig `mapstructure:"mysql"`
	Auth  AuthConfig  `mapstructure:"auth"`
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

// MySQLConfig 描述 API 连接 MySQL 以及管理连接池所需的配置。
//
// 密码没有代码默认值，必须由进程环境变量提供；这样即使误启动生产进程，
// 也不会悄悄使用仓库中的示例密码连接数据库。
type MySQLConfig struct {
	Host string `mapstructure:"host" validate:"required"`
	Port int    `mapstructure:"port" validate:"gte=1,lte=65535"`

	Database string `mapstructure:"database" validate:"required"`
	User     string `mapstructure:"user" validate:"required"`
	Password string `mapstructure:"password" validate:"required"`

	MaxOpenConns    int           `mapstructure:"max_open_conns" validate:"gte=1"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns" validate:"gte=0,ltefield=MaxOpenConns"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime" validate:"gt=0"`
}

// AuthConfig 描述管理员认证所需的全部配置。
//
// 这些值在进程启动时一次性读取并校验，运行期间不热更新；
// Handler、Service 和 Repository 都不直接读取环境变量或 Viper。
type AuthConfig struct {
	// JWTSecret 是签发与验证 Access JWT 使用的 HMAC-SHA256 密钥。
	// 与数据库密码同理：没有代码默认值，必须由 JWT_SECRET 环境变量提供，
	// 防止误用仓库示例密钥签发真实 Token。
	JWTSecret string `mapstructure:"jwt_secret" validate:"required"`

	// AccessTokenTTL 是 Access Token 的有效期。
	// 设计上它很短（15 分钟左右），泄露后造成的风险窗口有限；
	// 长时间登录依赖 Refresh Token 轮转，而不是给 Access Token 更长的有效期。
	AccessTokenTTL time.Duration `mapstructure:"access_token_ttl" validate:"gt=0"`

	// RefreshSessionTTL 是 Refresh Token 的有效期，也用作 Cookie 的 Max-Age。
	// 比 Access Token 长得多（例如 7 天），让用户不必频繁重新输入密码。
	RefreshSessionTTL time.Duration `mapstructure:"refresh_session_ttl" validate:"gt=0"`

	// RefreshCookieName 是保存 Refresh Token 的 Cookie 名称。
	RefreshCookieName string `mapstructure:"refresh_cookie_name" validate:"required"`

	// RefreshCookieSecure 为 true 时 Cookie 只在 HTTPS 连接上发送。
	// 本地开发使用 HTTP 因此为 false；生产环境启动校验会强制为 true。
	RefreshCookieSecure bool `mapstructure:"refresh_cookie_secure"`

	// RefreshCookieSameSite 控制跨站请求是否携带 Refresh Cookie，
	// 合法取值是 lax、strict 或 none（none 只应配合 Secure 使用）。
	RefreshCookieSameSite string `mapstructure:"refresh_cookie_same_site" validate:"required,oneof=lax strict none"`

	// PublicOrigin 是前端站点的可信来源。Refresh 和退出接口会校验请求的
	// Origin 请求头与它一致，降低跨站请求伪造（CSRF）风险。
	PublicOrigin string `mapstructure:"public_origin" validate:"required"`
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
		"app.env":                       "APP_ENV",
		"app.name":                      "APP_NAME",
		"http.addr":                     "HTTP_ADDR",
		"log.level":                     "LOG_LEVEL",
		"http.read_header_timeout":      "HTTP_READ_HEADER_TIMEOUT",
		"http.read_timeout":             "HTTP_READ_TIMEOUT",
		"http.write_timeout":            "HTTP_WRITE_TIMEOUT",
		"http.idle_timeout":             "HTTP_IDLE_TIMEOUT",
		"http.shutdown_timeout":         "HTTP_SHUTDOWN_TIMEOUT",
		"mysql.host":                    "MYSQL_HOST",
		"mysql.port":                    "MYSQL_PORT",
		"mysql.database":                "MYSQL_DATABASE",
		"mysql.user":                    "MYSQL_USER",
		"mysql.password":                "MYSQL_PASSWORD",
		"mysql.max_open_conns":          "MYSQL_MAX_OPEN_CONNS",
		"mysql.max_idle_conns":          "MYSQL_MAX_IDLE_CONNS",
		"mysql.conn_max_lifetime":       "MYSQL_CONN_MAX_LIFETIME",
		"auth.jwt_secret":               "JWT_SECRET",
		"auth.access_token_ttl":         "ACCESS_TOKEN_TTL",
		"auth.refresh_session_ttl":      "REFRESH_SESSION_TTL",
		"auth.refresh_cookie_name":      "REFRESH_COOKIE_NAME",
		"auth.refresh_cookie_secure":    "REFRESH_COOKIE_SECURE",
		"auth.refresh_cookie_same_site": "REFRESH_COOKIE_SAME_SITE",
		"auth.public_origin":            "PUBLIC_ORIGIN",
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

	// 可信来源必须是合法的 http/https URL，防止配置写成任意字符串。
	if err := validatePublicOrigin(cfg.Auth.PublicOrigin); err != nil {
		return nil, err
	}

	// 生产环境额外拒绝危险配置：
	// 弱密钥、仓库示例密钥、通配来源以及未启用 Secure 的 Refresh Cookie。
	// 开发环境保持宽松，方便本地快速启动。
	if cfg.App.Env == "production" {
		if err := validateProductionAuth(cfg.Auth); err != nil {
			return nil, err
		}
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
	v.SetDefault("mysql.host", "127.0.0.1")
	v.SetDefault("mysql.port", 3306)
	v.SetDefault("mysql.database", "tiny_blog")
	v.SetDefault("mysql.user", "blog")
	// 密码故意留空，使缺少 MYSQL_PASSWORD 的启动在校验阶段失败。
	v.SetDefault("mysql.password", "")
	v.SetDefault("mysql.max_open_conns", 20)
	v.SetDefault("mysql.max_idle_conns", 10)
	v.SetDefault("mysql.conn_max_lifetime", "30m")

	// JWT 密钥与数据库密码同理，必须来自环境变量，代码里不留任何默认值。
	v.SetDefault("auth.jwt_secret", "")
	v.SetDefault("auth.access_token_ttl", "15m")
	v.SetDefault("auth.refresh_session_ttl", "168h")
	v.SetDefault("auth.refresh_cookie_name", "muggle_refresh")
	// 本地开发走 HTTP，Cookie 不启用 Secure；生产环境由校验强制开启。
	v.SetDefault("auth.refresh_cookie_secure", false)
	v.SetDefault("auth.refresh_cookie_same_site", "lax")
	v.SetDefault("auth.public_origin", "http://localhost:5173")
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

// validatePublicOrigin 校验可信前端来源必须是带 http/https 协议和主机名的 URL。
// 后续 Refresh 和退出接口会把请求的 Origin 请求头与它做精确比较，
// 因此配置必须足够严格，不能是 "*" 之类无法精确匹配的值。
func validatePublicOrigin(origin string) error {
	parsed, err := url.Parse(origin)
	if err != nil {
		return fmt.Errorf("validate public origin %q: %w", origin, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("validate public origin %q: scheme must be http or https", origin)
	}
	if parsed.Host == "" {
		return fmt.Errorf("validate public origin %q: host must not be empty", origin)
	}
	return nil
}

// validateProductionAuth 在生产环境下拒绝不安全的认证配置。
// 开发环境允许宽松配置以便本地调试，但生产必须：
//   - JWT 密钥足够长，且不是仓库示例值；
//   - Refresh Cookie 启用 Secure（只在 HTTPS 下发送）；
//   - 可信来源不是通配值。
func validateProductionAuth(auth AuthConfig) error {
	if len(auth.JWTSecret) < 32 {
		return fmt.Errorf("validate production auth: jwt_secret must be at least 32 characters")
	}
	if strings.Contains(auth.JWTSecret, "change-me") ||
		strings.Contains(auth.JWTSecret, "replace-with") {
		return fmt.Errorf("validate production auth: jwt_secret must not be a placeholder value")
	}
	if !auth.RefreshCookieSecure {
		return fmt.Errorf("validate production auth: refresh cookie must be secure in production")
	}
	if auth.PublicOrigin == "*" {
		return fmt.Errorf("validate production auth: public origin must not be a wildcard")
	}
	return nil
}
