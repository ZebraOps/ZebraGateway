package router

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ZebraOps/ZebraGateway/internal/middleware"
	"github.com/ZebraOps/ZebraGateway/internal/model"
	"github.com/ZebraOps/ZebraGateway/internal/types"
	nacosClient "github.com/ZebraOps/ZebraGateway/pkg/nacos"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Manager 动态路由管理器，线程安全。
// 维护一份从数据库同步的路由快照，支持热更新（API 触发 + 定时自动刷新）。
type Manager struct {
	mu          sync.RWMutex
	routes      []model.ServiceRoute // 按前缀长度降序，最长优先匹配
	whitelists  []model.WhitelistRoute
	proxies     map[string]*httputil.ReverseProxy // key: resolved target URL
	db          *gorm.DB
	logger      *zap.Logger
	nacosLoader *nacosClient.ConfigLoader // 可选，用于服务发现
}

// New 创建 Manager 并执行初次加载。
func New(db *gorm.DB, logger *zap.Logger, nacosLoader *nacosClient.ConfigLoader) (*Manager, error) {
	m := &Manager{
		db:          db,
		logger:      logger,
		proxies:     make(map[string]*httputil.ReverseProxy),
		nacosLoader: nacosLoader,
	}
	if err := m.Reload(); err != nil {
		return nil, fmt.Errorf("初始加载路由失败: %w", err)
	}
	return m, nil
}

// DB 返回底层 *gorm.DB（供 handler 层使用）。
func (m *Manager) DB() *gorm.DB { return m.db }

// AllRoutes 返回当前内存快照的只读副本（含禁用路由，用于管理接口展示）。
func (m *Manager) AllRoutes() []model.ServiceRoute {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]model.ServiceRoute, len(m.routes))
	copy(result, m.routes)
	return result
}

// AllWhitelists 返回当前白名单快照的只读副本。
func (m *Manager) AllWhitelists() []model.WhitelistRoute {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]model.WhitelistRoute, len(m.whitelists))
	copy(result, m.whitelists)
	return result
}

// Reload 从数据库重新加载路由和白名单，原子替换内存快照。
func (m *Manager) Reload() error {
	var routes []model.ServiceRoute
	if err := m.db.Where("enabled = ?", true).Find(&routes).Error; err != nil {
		return fmt.Errorf("查询服务路由失败: %w", err)
	}
	// 最长前缀优先（longest-prefix-match）
	sort.Slice(routes, func(i, j int) bool {
		return len(routes[i].Prefix) > len(routes[j].Prefix)
	})

	var whitelists []model.WhitelistRoute
	if err := m.db.Find(&whitelists).Error; err != nil {
		return fmt.Errorf("查询白名单失败: %w", err)
	}

	// 构建或复用 ReverseProxy 实例（按 resolved target URL 去重）
	newProxies := make(map[string]*httputil.ReverseProxy)
	for _, r := range routes {
		target := r.Target
		// 如果配置了 Nacos 服务名，优先通过服务发现获取地址
		if r.ServiceName != "" && m.nacosLoader != nil {
			if discovered, ok := m.resolveService(r.ServiceName); ok {
				target = discovered
				m.logger.Info("服务发现解析路由目标",
					zap.String("prefix", r.Prefix),
					zap.String("service", r.ServiceName),
					zap.String("static_target", r.Target),
					zap.String("discovered", target),
				)
			} else {
				m.logger.Warn("服务发现失败，使用静态目标地址",
					zap.String("prefix", r.Prefix),
					zap.String("service", r.ServiceName),
					zap.String("fallback", r.Target),
				)
			}
		}
		if _, ok := newProxies[target]; ok {
			continue
		}
		targetURL, err := url.Parse(target)
		if err != nil {
			m.logger.Warn("无效的后端地址，跳过该路由",
				zap.String("target", target),
				zap.Error(err),
			)
			continue
		}
		newProxies[target] = httputil.NewSingleHostReverseProxy(targetURL)
	}

	m.mu.Lock()
	m.routes = routes
	m.whitelists = whitelists
	m.proxies = newProxies
	m.mu.Unlock()

	m.logger.Info("路由快照更新完成",
		zap.Int("routes", len(routes)),
		zap.Int("whitelists", len(whitelists)),
	)
	return nil
}

// RewritePath 将网关级路径（如 /rbac/organizations）转换为后端服务路径（如 /api/organizations）。
// 用于权限校验时将请求路径映射为 RBAC 中存储的接口路径。
// 若无匹配路由则原样返回。
func (m *Manager) RewritePath(path string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for i := range m.routes {
		if after, ok := strings.CutPrefix(path, m.routes[i].Prefix); ok {
			newPath := after
			if m.routes[i].Rewrite != "" {
				newPath = m.routes[i].Rewrite + newPath
			}
			if newPath == "" {
				newPath = "/"
			}
			return newPath
		}
	}
	return path
}

// IsWhitelisted 检查请求是否命中动态白名单（数据库来源）。
// 实现 middleware.DynamicWhitelistChecker 接口。
func (m *Manager) IsWhitelisted(method, path string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, entry := range m.whitelists {
		if matchWhitelist(entry, method, path) {
			return true
		}
	}
	return false
}

func matchWhitelist(entry model.WhitelistRoute, method, path string) bool {
	methodMatch := entry.Method == "*" || strings.EqualFold(entry.Method, method)
	if !methodMatch {
		return false
	}
	if strings.HasSuffix(entry.Path, "/*") {
		prefix := strings.TrimSuffix(entry.Path, "*")
		return strings.HasPrefix(path, prefix)
	}
	return entry.Path == path
}

// ServeProxy 作为 gin.NoRoute 处理器，按最长前缀匹配转发请求到后端服务。
func (m *Manager) ServeProxy(c *gin.Context) {
	reqPath := c.Request.URL.Path
	method := c.Request.Method

	m.mu.RLock()
	var matched *model.ServiceRoute
	var proxy *httputil.ReverseProxy
	for i := range m.routes {
		if strings.HasPrefix(reqPath, m.routes[i].Prefix) {
			// 解析目标地址（优先 Nacos 服务发现）
			target := m.resolveTarget(&m.routes[i])
			if p, ok := m.proxies[target]; ok {
				matched = &m.routes[i]
				proxy = p
				break
			}
		}
	}
	m.mu.RUnlock()

	if matched == nil {
		types.Error(c, http.StatusNotFound, 404, "路由不存在")
		return
	}

	// WebSocket 升级请求：使用专用 WebSocket 代理桥接
	// httputil.ReverseProxy 对 WebSocket 支持有限（hop-by-hop 头清理、路径双重改写等）
	// 独立桥接更可靠，且保留原始消息帧类型
	if middleware.IsWebSocketUpgrade(c) {
		target := m.resolveTarget(matched)
		middleware.ProxyWebSocket(c, matched, target, m.logger)
		return
	}

	// 路径改写：strip prefix，prepend rewrite
	newPath := strings.TrimPrefix(reqPath, matched.Prefix)
	if matched.Rewrite != "" {
		newPath = matched.Rewrite + newPath
	}
	if newPath == "" {
		newPath = "/"
	}
	c.Request.URL.Path = newPath

	m.logger.Debug("转发请求",
		zap.String("method", method),
		zap.String("original", reqPath),
		zap.String("rewritten", newPath),
		zap.String("target", m.resolveTarget(matched)),
	)

	proxy.ServeHTTP(c.Writer, c.Request)
}

// StartAutoReload 启动后台定时重载 goroutine，每隔 interval 从数据库刷新路由。
// 同时定时刷新通过 Nacos 服务发现解析的路由代理。
func (m *Manager) StartAutoReload(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			if err := m.Reload(); err != nil {
				m.logger.Error("定时重载路由失败", zap.Error(err))
			}
		}
	}()
}

// resolveTarget 解析路由的实际目标地址，优先使用 Nacos 服务发现。
func (m *Manager) resolveTarget(r *model.ServiceRoute) string {
	if r.ServiceName != "" && m.nacosLoader != nil {
		if discovered, ok := m.resolveService(r.ServiceName); ok {
			return discovered
		}
	}
	return r.Target
}

// resolveService 通过 Nacos 服务发现获取服务实例地址。
// 注意：Docker 容器注册的 IP 可能为 0.0.0.0（监听所有接口），此时回退为 127.0.0.1。
func (m *Manager) resolveService(serviceName string) (string, bool) {
	instance, err := m.nacosLoader.SelectOneHealthyInstance(serviceName)
	if err != nil || instance == nil {
		m.logger.Warn("Nacos 服务发现失败",
			zap.String("service", serviceName),
			zap.Error(err),
		)
		return "", false
	}
	ip := instance.IP
	if ip == "0.0.0.0" || ip == "::" {
		ip = "127.0.0.1"
		m.logger.Info("Nacos 实例 IP 为通配地址，回退为 localhost",
			zap.String("service", serviceName),
			zap.String("original_ip", instance.IP),
		)
	}
	return fmt.Sprintf("http://%s:%d", ip, instance.Port), true
}
