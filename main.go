package main

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ZebraOps/ZebraGateway/config"
	"github.com/ZebraOps/ZebraGateway/internal/api"
	"github.com/ZebraOps/ZebraGateway/internal/handler"
	"github.com/ZebraOps/ZebraGateway/internal/middleware"
	"github.com/ZebraOps/ZebraGateway/internal/model"
	"github.com/ZebraOps/ZebraGateway/internal/rbac"
	"github.com/ZebraOps/ZebraGateway/internal/router"
	"github.com/ZebraOps/ZebraGateway/internal/store"
	"github.com/ZebraOps/ZebraGateway/pkg/cache"
	"github.com/ZebraOps/ZebraGateway/pkg/log"
	nacosClient "github.com/ZebraOps/ZebraGateway/pkg/nacos"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"go.uber.org/zap"
	"gorm.io/gorm"

	_ "github.com/ZebraOps/ZebraGateway/docs" // swagger 文档初始化
)

// @title           ZebraGateway API
// @version         1.0.0
// @description     基于 Gin 的轻量级 API 网关，对接 ZebraRBAC 实现网关层权限校验。\n\n## 鉴权说明\n\n除白名单接口外，所有接口需在请求头中携带 JWT Token：\n```\nAuthorization: Bearer <token>\n```\n\nToken 通过 `POST /rbac/login/access-token` 获取。
// @host            localhost:4121
// @BasePath        /
// @securityDefinitions.apikey  BearerAuth
// @in                          header
// @name                        Authorization
// @description                 格式：Bearer <JWT Token>

const banner = `
███████╗███████╗██████╗ ██████╗  █████╗  ██████╗ ██████╗ ███████╗
╚══███╔╝██╔════╝██╔══██╗██╔══██╗██╔══██╗██╔═══██╗██╔══██╗██╔════╝
  ███╔╝ █████╗  ██████╔╝██████╔╝███████║██║   ██║██████╔╝███████╗
 ███╔╝  ██╔══╝  ██╔══██╗██╔══██╗██╔══██║██║   ██║██╔═══╝ ╚════██║
███████╗███████╗██████╔╝██║  ██║██║  ██║╚██████╔╝██║     ███████║
╚══════╝╚══════╝╚═════╝ ╚═╝  ╚═╝╚═╝  ╚═╝ ╚═════╝ ╚═╝     ╚══════╝
                     [ZebraOps Gateway Platform]
`

func main() {
	defer log.Sync()

	// 打印启动 Banner
	fmt.Print(banner)

	// --- 加载配置 ---
	cfg := config.Load()

	// --- 初始化日志 ---
	if err := log.InitWithConfig(cfg.Logging); err != nil {
		fmt.Fprintln(os.Stderr, "初始化日志失败:", err)
		os.Exit(1)
	}
	logger := log.L()
	
	logger.Info("========================================")
	logger.Info("ZebraGateway 正在启动...")
	logger.Info("========================================")

	// --- 初始化 Nacos 客户端（可选） ---
	var nacos *nacosClient.Client
	var nacosLoader *nacosClient.ConfigLoader
	
	if cfg.NacosServerAddr != "" {
		logger.Info("检测到 Nacos 配置，开始初始化 Nacos 客户端",
			zap.String("server", cfg.NacosServerAddr),
			zap.String("namespace", cfg.NacosNamespace),
		)
		
		nc, err := nacosClient.NewClient(nacosClient.Config{
			ServerAddr: cfg.NacosServerAddr,
			Namespace:  cfg.NacosNamespace,
			Username:   cfg.NacosUsername,
			Password:   cfg.NacosPassword,
			Group:      cfg.NacosGroup,
			LogLevel:   cfg.Logging.Level,
		}, logger)
		
		if err != nil {
			logger.Error("Nacos 客户端初始化失败，将使用本地配置", zap.Error(err))
		} else {
			nacos = nc
			nacosLoader = nacosClient.NewConfigLoader(nc, logger)
			
			// 从 Nacos 加载配置（覆盖本地配置）
			logger.Info("正在从 Nacos 加载配置...")
			
			// 加载数据库配置
			if dbURL := nacosLoader.LoadDatabaseURL(cfg.DatabaseURL); dbURL != "" {
				cfg.DatabaseURL = dbURL
			}
			
			// 加载 JWT 密钥
			if jwtSecret := nacosLoader.LoadJWTSecret(cfg.JWTSecret); jwtSecret != "" {
				cfg.JWTSecret = jwtSecret
			}
			
			// 加载缓存 TTL
			cfg.CacheTTL = nacosLoader.LoadCacheTTL(cfg.CacheTTL)
			
			// 加载路由重载间隔
			cfg.RouteReloadInterval = nacosLoader.LoadRouteReloadInterval(cfg.RouteReloadInterval)
			
			// 如果启用服务发现，从 Nacos 获取 RBAC 服务地址
			if cfg.UseServiceDiscovery {
				logger.Info("已启用服务发现，尝试从 Nacos 发现 ZebraRBAC 服务...")
				rbacURL, err := nacosLoader.DiscoverRBACService()
				if err != nil {
					logger.Warn("服务发现失败，使用配置中的 RBAC 地址", zap.Error(err))
				} else {
					cfg.RbacURL = rbacURL
					logger.Info("✓ 服务发现成功", zap.String("rbacURL", rbacURL))
				}
			}
			
			logger.Info("✓ Nacos 配置加载完成")
		}
	} else {
		logger.Info("未配置 Nacos，使用本地配置")
	}
	
	logger.Info("当前配置",
		zap.String("port", cfg.Port),
		zap.String("rbacURL", cfg.RbacURL),
		zap.Int("cacheTTL", cfg.CacheTTL),
		zap.Duration("routeReloadInterval", cfg.RouteReloadInterval),
	)

	// --- 连接数据库，初始化动态路由管理器 ---
	if cfg.DatabaseURL == "" {
		logger.Fatal("DatabaseURL 未配置，请在 config/configs.yaml 中设置 app.DatabaseURL")
	}
	db, err := store.New(cfg.DatabaseURL)
	if err != nil {
		logger.Fatal("连接数据库失败", zap.Error(err))
	}

	// 同步 YAML 配置中的路由和白名单到数据库（首次全量导入，后续增量更新 service_name）
	syncRoutesFromConfig(db, cfg, logger)

	routeManager, err := router.New(db, logger, nacosLoader)
	if err != nil {
		logger.Fatal("初始化路由管理器失败", zap.Error(err))
	}
	routeManager.StartAutoReload(cfg.RouteReloadInterval)

	// --- 初始化 RBAC 客户端和权限缓存 ---
	rbacClient := rbac.New(cfg.RbacURL)
	authCache := cache.New(cfg.CacheTTL)
	
	// 如果启用了服务发现，启动定时任务刷新 RBAC 服务地址
	if nacos != nil && cfg.UseServiceDiscovery {
		go func() {
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()
			
			for range ticker.C {
				rbacURL, err := nacosLoader.DiscoverRBACService()
				if err != nil {
					logger.Warn("定时服务发现失败", zap.Error(err))
					continue
				}
				
				// 如果地址变更，更新 RBAC 客户端
				if rbacURL != rbacClient.GetBaseURL() {
					logger.Info("RBAC 服务地址已变更",
						zap.String("old", rbacClient.GetBaseURL()),
						zap.String("new", rbacURL),
					)
					rbacClient.UpdateBaseURL(rbacURL)
				}
			}
		}()
		logger.Info("✓ 已启动 RBAC 服务发现定时任务（30s 刷新）")
	}

	// --- 构建静态白名单（YAML 来源） ---
	var whitelist []middleware.WhitelistEntry
	for _, w := range cfg.Whitelist {
		whitelist = append(whitelist, middleware.WhitelistEntry{
			Method: w.Method,
			Path:   w.Path,
		})
	}

	// --- 初始化 Gin ---
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(cors.New(cors.Config{
		AllowOrigins: []string{
			"http://127.0.0.1:4120",
			"http://localhost:4120",
			"http://192.168.3.15:4120",
			"https://*.trycloudflare.com",
		},
		AllowMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders: []string{"Origin", "Content-Type", "Accept", "Authorization", "X-Requested-With"},
		ExposeHeaders: []string{"Content-Length", "Content-Type"},
		AllowCredentials: false,
		MaxAge: 12 * time.Hour,
	}))
	r.Use(middleware.RequestLogger(logger))
	r.Use(middleware.Auth(cfg.JWTSecret, rbacClient, authCache, whitelist, logger, routeManager, routeManager))

	// --- Swagger UI（静态白名单内，无需鉴权） ---
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// --- 网关自身接口 ---
	r.GET("/health", api.Health)

	// --- 管理接口（路由和白名单 CRUD） ---
	rh := handler.NewRouteHandler(routeManager)
	admin := r.Group("/admin")
	{
		routes := admin.Group("/routes")
		routes.GET("", rh.ListRoutes)
		routes.POST("", rh.CreateRoute)
		routes.POST("/reload", rh.ReloadRoutes)
		routes.PUT("/:id", rh.UpdateRoute)
		routes.DELETE("/:id", rh.DeleteRoute)
		routes.POST("/:id/enable", rh.EnableRoute)
		routes.POST("/:id/disable", rh.DisableRoute)

		wl := admin.Group("/whitelists")
		wl.GET("", rh.ListWhitelists)
		wl.POST("", rh.CreateWhitelist)
		wl.DELETE("/:id", rh.DeleteWhitelist)
	}

	// --- 动态反向代理（NoRoute 捕获所有未匹配路径） ---
	r.NoRoute(routeManager.ServeProxy)

	// --- 注册服务到 Nacos ---
	if nacos != nil {
		serviceIP := getLocalIP()
		servicePort := getPortNumber(cfg.Port)
		
		err := nacos.RegisterInstance("zebra-gateway", serviceIP, uint64(servicePort), map[string]string{
			"version":   "1.0.0",
			"endpoints": "/admin,/swagger,/health",
			"description": "ZebraGateway API 网关服务",
		})
		
		if err != nil {
			logger.Error("服务注册失败", zap.Error(err))
		} else {
			logger.Info("✓ 服务注册成功",
				zap.String("service", "zebra-gateway"),
				zap.String("ip", serviceIP),
				zap.Uint64("port", uint64(servicePort)),
			)
		}
	}

	addr := fmt.Sprintf(":%s", cfg.Port)
	logger.Info("========================================")
	logger.Info("ZebraGateway 启动成功", zap.String("addr", addr))
	logger.Info("========================================")

	// --- 启动服务器，支持优雅关闭 ---
	srv := make(chan error, 1)
	go func() {
		srv <- r.Run(addr)
	}()

	// 等待中断信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-srv:
		logger.Fatal("服务器启动失败", zap.Error(err))
	case sig := <-quit:
		logger.Info("收到退出信号，开始优雅关闭...", zap.String("signal", sig.String()))
		
		// 注销 Nacos 服务
		if nacos != nil {
			serviceIP := getLocalIP()
			servicePort := getPortNumber(cfg.Port)
			
			err := nacos.DeregisterInstance("zebra-gateway", serviceIP, uint64(servicePort))
			if err != nil {
				logger.Error("服务注销失败", zap.Error(err))
			} else {
				logger.Info("✓ 服务注销成功")
			}
		}
		
		logger.Info("ZebraGateway 已关闭")
	}
}

// getLocalIP 获取本机 IP 地址
func getLocalIP() string {
	// 优先使用环境变量
	if ip := os.Getenv("SERVICE_IP"); ip != "" {
		return ip
	}
	
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "127.0.0.1"
	}
	defer conn.Close()
	
	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String()
}

// getPortNumber 从端口字符串提取端口号
func getPortNumber(port string) int {
	var p int
	fmt.Sscanf(port, "%d", &p)
	if p == 0 {
		return 4121
	}
	return p
}

// syncRoutesFromConfig 将 YAML 配置中的路由和白名单同步到数据库。
// - DB 为空时：全量导入（首次启动）。
// - DB 已有路由时：仅同步 service_name 字段（增量迁移），不覆盖已有的 Target/Rewrite 等人为修改。
func syncRoutesFromConfig(db *gorm.DB, cfg *config.Config, logger *zap.Logger) {
	var routeCount int64
	db.Model(&model.ServiceRoute{}).Count(&routeCount)

	if routeCount == 0 && len(cfg.Services) > 0 {
		// 首次启动：全量导入
		for _, svc := range cfg.Services {
			route := model.ServiceRoute{
				Prefix:      svc.Prefix,
				Target:      svc.Target,
				Rewrite:     svc.Rewrite,
				ServiceName: svc.ServiceName,
				Description: "从 YAML 配置自动导入",
				Enabled:     true,
			}
			db.Create(&route)
		}
		logger.Info("已将 YAML 服务路由导入数据库", zap.Int("count", len(cfg.Services)))
	} else {
		// 已有路由：增量同步 service_name（避免覆盖用户手动修改的 Target/Rewrite）
		for _, svc := range cfg.Services {
			if svc.ServiceName == "" {
				continue
			}
			result := db.Model(&model.ServiceRoute{}).
				Where("prefix = ? AND (service_name = '' OR service_name IS NULL)", svc.Prefix).
				Update("service_name", svc.ServiceName)
			if result.RowsAffected > 0 {
				logger.Info("已同步路由 service_name",
					zap.String("prefix", svc.Prefix),
					zap.String("service_name", svc.ServiceName),
				)
			}
		}
	}

	var wlCount int64
	db.Model(&model.WhitelistRoute{}).Count(&wlCount)
	if wlCount == 0 && len(cfg.Whitelist) > 0 {
		for _, w := range cfg.Whitelist {
			item := model.WhitelistRoute{
				Method:      w.Method,
				Path:        w.Path,
				Description: "从 YAML 配置自动导入",
			}
			db.Create(&item)
		}
		logger.Info("已将 YAML 白名单导入数据库", zap.Int("count", len(cfg.Whitelist)))
	}
}
