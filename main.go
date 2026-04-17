package main

import (
	"fmt"
	"os"

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
// @host            localhost:8080
// @BasePath        /
// @securityDefinitions.apikey  BearerAuth
// @in                          header
// @name                        Authorization
// @description                 格式：Bearer <JWT Token>
func main() {
	defer log.Sync()

	// --- 加载配置 ---
	cfg := config.Load()

	// --- 初始化日志 ---
	if err := log.InitWithConfig(cfg.Logging); err != nil {
		fmt.Fprintln(os.Stderr, "初始化日志失败:", err)
		os.Exit(1)
	}
	logger := log.L()
	logger.Info("ZebraGateway 正在启动",
		zap.String("port", cfg.Port),
		zap.String("rbacURL", cfg.RbacURL),
		zap.Int("cacheTTL", cfg.CacheTTL),
	)

	// --- 连接数据库，初始化动态路由管理器 ---
	if cfg.DatabaseURL == "" {
		logger.Fatal("DatabaseURL 未配置，请在 config/configs.yaml 中设置 app.DatabaseURL")
	}
	db, err := store.New(cfg.DatabaseURL)
	if err != nil {
		logger.Fatal("连接数据库失败", zap.Error(err))
	}

	// 若数据库为空，自动将 YAML 配置中的静态路由和白名单导入数据库
	seedFromConfig(db, cfg, logger)

	routeManager, err := router.New(db, logger)
	if err != nil {
		logger.Fatal("初始化路由管理器失败", zap.Error(err))
	}
	routeManager.StartAutoReload(cfg.RouteReloadInterval)

	// --- 初始化 RBAC 客户端和权限缓存 ---
	rbacClient := rbac.New(cfg.RbacURL)
	authCache := cache.New(cfg.CacheTTL)

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
	r.Use(cors.Default())
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

	addr := fmt.Sprintf(":%s", cfg.Port)
	logger.Info("ZebraGateway 启动成功", zap.String("addr", addr))

	if err := r.Run(addr); err != nil {
		logger.Fatal("服务器启动失败", zap.Error(err))
	}
}

// seedFromConfig 在数据库中没有任何路由时，将 YAML 配置中的服务路由和白名单导入数据库。
// 这样首次启动时无需手动通过 CLI 配置路由。
func seedFromConfig(db *gorm.DB, cfg *config.Config, logger *zap.Logger) {
	var routeCount int64
	db.Model(&model.ServiceRoute{}).Count(&routeCount)
	if routeCount == 0 && len(cfg.Services) > 0 {
		for _, svc := range cfg.Services {
			route := model.ServiceRoute{
				Prefix:      svc.Prefix,
				Target:      svc.Target,
				Rewrite:     svc.Rewrite,
				Description: "从 YAML 配置自动导入",
				Enabled:     true,
			}
			db.Create(&route)
		}
		logger.Info("已将 YAML 服务路由导入数据库", zap.Int("count", len(cfg.Services)))
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
