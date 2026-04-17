package config

import (
	"time"

	"github.com/ZebraOps/ZebraGateway/internal/types"
	"github.com/spf13/viper"
)

// ServiceConfig 后端服务路由配置
type ServiceConfig struct {
	// Prefix 网关监听前缀，如 "/rbac"
	Prefix string `mapstructure:"prefix"`
	// Target 后端服务地址，如 "http://localhost:8000"
	Target string `mapstructure:"target"`
	// Rewrite 路径改写前缀，如 "/api"（strip Prefix 后拼接 Rewrite 作为新路径）
	Rewrite string `mapstructure:"rewrite"`
}

// WhitelistConfig 白名单条目（不需要 JWT 鉴权）
type WhitelistConfig struct {
	Method string `mapstructure:"method"`
	Path   string `mapstructure:"path"`
}

// Config 全局配置
type Config struct {
	// Port 网关监听端口
	Port string
	// JWTSecret 与 ZebraRBAC 共享的 JWT 签名密钥
	JWTSecret string
	// RbacURL ZebraRBAC 服务地址（用于获取用户权限）
	RbacURL string
	// CacheTTL 权限缓存 TTL（秒）
	CacheTTL int
	// DatabaseURL PostgreSQL DSN（动态路由持久化存储）
	DatabaseURL string
	// RouteReloadInterval 定时重载路由间隔
	RouteReloadInterval time.Duration
	// Services 初始后端服务路由表（启动时若 DB 为空则自动导入）
	Services []ServiceConfig
	// Whitelist 初始白名单路由（启动时若 DB 为空则自动导入）
	Whitelist []WhitelistConfig
	// Logging 日志配置
	Logging types.LoggingConfig
}

func Load() *Config {
	viper.SetConfigName("configs")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("./config")

	// 支持通过环境变量覆盖（前缀 ZEBRA_GW_）
	viper.SetEnvPrefix("ZEBRA_GW")
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			panic(err)
		}
	}

	// 默认值
	viper.SetDefault("app.Port", "8080")
	viper.SetDefault("app.CacheTTL", 300)
	viper.SetDefault("app.RouteReloadInterval", "30s")
	viper.SetDefault("logging.level", "info")
	viper.SetDefault("logging.encoding", "json")
	viper.SetDefault("logging.output_paths", []string{"stdout"})
	viper.SetDefault("logging.error_output_paths", []string{"stderr"})

	reloadInterval, err := time.ParseDuration(viper.GetString("app.RouteReloadInterval"))
	if err != nil {
		reloadInterval = 30 * time.Second
	}

	cfg := &Config{
		Port:                viper.GetString("app.Port"),
		JWTSecret:           viper.GetString("app.JWTSecret"),
		RbacURL:             viper.GetString("app.RbacURL"),
		CacheTTL:            viper.GetInt("app.CacheTTL"),
		DatabaseURL:         viper.GetString("app.DatabaseURL"),
		RouteReloadInterval: reloadInterval,
		Logging: types.LoggingConfig{
			Level:            viper.GetString("logging.level"),
			Encoding:         viper.GetString("logging.encoding"),
			OutputPaths:      viper.GetStringSlice("logging.output_paths"),
			ErrorOutputPaths: viper.GetStringSlice("logging.error_output_paths"),
			MaxSize:          viper.GetInt("logging.max_size"),
			MaxAge:           viper.GetInt("logging.max_age"),
			MaxBackups:       viper.GetInt("logging.max_backups"),
			Compress:         viper.GetBool("logging.compress"),
		},
	}

	// 反序列化嵌套切片配置
	if err := viper.UnmarshalKey("services", &cfg.Services); err != nil {
		panic("failed to parse services config: " + err.Error())
	}
	if err := viper.UnmarshalKey("whitelist", &cfg.Whitelist); err != nil {
		panic("failed to parse whitelist config: " + err.Error())
	}

	return cfg
}
