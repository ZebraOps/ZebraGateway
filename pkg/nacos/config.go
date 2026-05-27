package nacos

import (
	"fmt"
	"time"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

// GatewayConfig Gateway 服务的 YAML 配置结构
type GatewayConfig struct {
	Database struct {
		URL          string `yaml:"url"`
		MaxIdleConns int    `yaml:"max_idle_conns"`
		MaxOpenConns int    `yaml:"max_open_conns"`
	} `yaml:"database"`

	JWT struct {
		Secret string `yaml:"secret"`
	} `yaml:"jwt"`

	Cache struct {
		TTL        int `yaml:"ttl"`
		MaxEntries int `yaml:"max_entries"`
	} `yaml:"cache"`

	Route struct {
		ReloadInterval string   `yaml:"reload_interval"`
		Whitelist      []string `yaml:"whitelist"`
	} `yaml:"route"`

	ServiceDiscovery struct {
		Enabled         bool   `yaml:"enabled"`
		RefreshInterval string `yaml:"refresh_interval"`
	} `yaml:"service_discovery"`

	Service struct {
		Name        string `yaml:"name"`
		Version     string `yaml:"version"`
		Description string `yaml:"description"`
	} `yaml:"service"`
}

// ConfigLoader Nacos 配置加载器
type ConfigLoader struct {
	client *Client
	logger *zap.Logger
	config *GatewayConfig
}

// NewConfigLoader 创建配置加载器
func NewConfigLoader(client *Client, logger *zap.Logger) *ConfigLoader {
	loader := &ConfigLoader{
		client: client,
		logger: logger,
	}

	if err := loader.loadYAMLConfig(); err != nil {
		logger.Error("加载 Nacos YAML 配置失败", zap.Error(err))
	}

	return loader
}

// loadYAMLConfig 从 Nacos 加载 YAML 配置文件
func (l *ConfigLoader) loadYAMLConfig() error {
	content, err := l.client.GetConfig("zebra-gateway.yaml", "DEFAULT_GROUP")
	if err != nil {
		return fmt.Errorf("获取 YAML 配置失败: %w", err)
	}

	if content == "" {
		return fmt.Errorf("Nacos 中未找到 zebra-gateway.yaml 配置")
	}

	var config GatewayConfig
	if err := yaml.Unmarshal([]byte(content), &config); err != nil {
		return fmt.Errorf("解析 YAML 配置失败: %w", err)
	}

	l.config = &config
	l.logger.Info("从 Nacos 加载 YAML 配置成功")
	return nil
}

// LoadDatabaseURL 加载数据库连接字符串
func (l *ConfigLoader) LoadDatabaseURL(defaultValue string) string {
	if l.config != nil && l.config.Database.URL != "" {
		l.logger.Info("从 Nacos YAML 加载数据库配置成功")
		return l.config.Database.URL
	}

	l.logger.Warn("从 Nacos 加载数据库配置失败，使用默认值")
	return defaultValue
}

// LoadJWTSecret 加载 JWT 密钥
func (l *ConfigLoader) LoadJWTSecret(defaultValue string) string {
	if l.config != nil && l.config.JWT.Secret != "" {
		l.logger.Info("从 Nacos YAML 加载 JWT 密钥成功")
		return l.config.JWT.Secret
	}

	l.logger.Warn("从 Nacos 加载 JWT 密钥失败，使用默认值")
	return defaultValue
}

// LoadCacheTTL 加载缓存 TTL（秒）
func (l *ConfigLoader) LoadCacheTTL(defaultValue int) int {
	if l.config != nil && l.config.Cache.TTL > 0 {
		l.logger.Info("从 Nacos YAML 加载 CacheTTL 成功", zap.Int("ttl", l.config.Cache.TTL))
		return l.config.Cache.TTL
	}

	return defaultValue
}

// LoadRouteReloadInterval 加载路由重载间隔
func (l *ConfigLoader) LoadRouteReloadInterval(defaultValue time.Duration) time.Duration {
	if l.config != nil && l.config.Route.ReloadInterval != "" {
		interval, err := time.ParseDuration(l.config.Route.ReloadInterval)
		if err == nil {
			l.logger.Info("从 Nacos YAML 加载 RouteReloadInterval 成功", zap.Duration("interval", interval))
			return interval
		}

		l.logger.Warn("解析 RouteReloadInterval 失败", zap.Error(err))
	}

	return defaultValue
}

// DiscoverRBACService 通过服务发现获取 ZebraRBAC 服务地址
// 返回格式: http://ip:port
func (l *ConfigLoader) DiscoverRBACService() (string, error) {
	instance, err := l.client.SelectOneHealthyInstance("zebra-rbac")
	if err != nil {
		return "", fmt.Errorf("discover zebra-rbac: %w", err)
	}

	if instance == nil {
		return "", fmt.Errorf("zebra-rbac service not found")
	}

	url := fmt.Sprintf("http://%s:%d", instance.IP, instance.Port)
	l.logger.Info("通过服务发现获取 ZebraRBAC 地址", zap.String("url", url))
	return url, nil
}

// WatchCacheTTL 监听 CacheTTL 配置变更
func (l *ConfigLoader) WatchCacheTTL(onChange func(int)) error {
	return l.client.ListenConfig("zebra-gateway.yaml", "DEFAULT_GROUP", func(namespace, group, dataID, data string) {
		var config GatewayConfig
		if err := yaml.Unmarshal([]byte(data), &config); err != nil {
			l.logger.Error("解析新的 YAML 配置失败", zap.Error(err))
			return
		}

		if config.Cache.TTL > 0 {
			l.logger.Info("CacheTTL 配置已变更", zap.Int("new_ttl", config.Cache.TTL))
			l.config = &config
			onChange(config.Cache.TTL)
		}
	})
}

// WatchRouteReloadInterval 监听路由重载间隔配置变更
func (l *ConfigLoader) WatchRouteReloadInterval(onChange func(time.Duration)) error {
	return l.client.ListenConfig("zebra-gateway.yaml", "DEFAULT_GROUP", func(namespace, group, dataID, data string) {
		var config GatewayConfig
		if err := yaml.Unmarshal([]byte(data), &config); err != nil {
			l.logger.Error("解析新的 YAML 配置失败", zap.Error(err))
			return
		}

		if config.Route.ReloadInterval != "" {
			interval, err := time.ParseDuration(config.Route.ReloadInterval)
			if err != nil {
				l.logger.Error("解析新的 RouteReloadInterval 失败", zap.Error(err))
				return
			}

			l.logger.Info("RouteReloadInterval 配置已变更", zap.Duration("new_interval", interval))
			l.config = &config
			onChange(interval)
		}
	})
}
