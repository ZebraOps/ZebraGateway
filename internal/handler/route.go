package handler

import (
	"net/http"
	"strconv"

	"github.com/ZebraOps/ZebraGateway/internal/model"
	"github.com/ZebraOps/ZebraGateway/internal/router"
	"github.com/ZebraOps/ZebraGateway/internal/types"
	"github.com/gin-gonic/gin"
)

// RouteHandler 提供服务路由和白名单的 CRUD 管理接口。
type RouteHandler struct {
	manager *router.Manager
}

// NewRouteHandler 创建 RouteHandler。
func NewRouteHandler(m *router.Manager) *RouteHandler {
	return &RouteHandler{manager: m}
}

// ─── Service Routes ────────────────────────────────────────────────────────────

// ListRoutes godoc
// @Summary      列出所有服务路由
// @Description  返回所有服务路由，支持按路径前缀（模糊匹配）、目标地址（模糊匹配）、启用状态过滤
// @Tags         admin
// @Security     BearerAuth
// @Produce      json
// @Param        prefix   query     string  false  "路径前缀（模糊匹配，如 /rbac）"
// @Param        target   query     string  false  "目标地址（模糊匹配，如 192.168）"
// @Param        enabled  query     string  false  "是否启用（true 或 false）"  Enums(true, false)
// @Success      200  {object}  types.Response{data=[]swaggerServiceRoute}  "路由列表"
// @Failure      500  {object}  types.Response                             "服务器错误"
// @Router       /admin/routes [get]
func (h *RouteHandler) ListRoutes(c *gin.Context) {
	db := h.manager.DB()
	if prefix := c.Query("prefix"); prefix != "" {
		db = db.Where("prefix LIKE ?", "%"+prefix+"%")
	}
	if target := c.Query("target"); target != "" {
		db = db.Where("target LIKE ?", "%"+target+"%")
	}
	if enabled := c.Query("enabled"); enabled != "" {
		db = db.Where("enabled = ?", enabled == "true")
	}
	var routes []model.ServiceRoute
	if err := db.Find(&routes).Error; err != nil {
		types.Error(c, http.StatusInternalServerError, 500, err.Error())
		return
	}
	types.Success(c, routes)
}

// CreateRoute godoc
// @Summary      新增服务路由
// @Description  新增一条服务路由，网关将匹配到 prefix 的请求反向代理到 target。创建后自动热重载生效
// @Tags         admin
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body      createRouteInput                   true  "路由信息"
// @Success      200   {object}  types.Response{data=swaggerServiceRoute}  "创建的路由"
// @Failure      400   {object}  types.Response                           "参数错误"
// @Failure      500   {object}  types.Response                           "服务器错误"
// @Router       /admin/routes [post]
func (h *RouteHandler) CreateRoute(c *gin.Context) {
	var input createRouteInput
	if err := c.ShouldBindJSON(&input); err != nil {
		types.Error(c, http.StatusBadRequest, 400, err.Error())
		return
	}
	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	route := model.ServiceRoute{
		Prefix:      input.Prefix,
		Target:      input.Target,
		Rewrite:     input.Rewrite,
		Description: input.Description,
		Enabled:     enabled,
	}
	if err := h.manager.DB().Create(&route).Error; err != nil {
		types.Error(c, http.StatusInternalServerError, 500, err.Error())
		return
	}
	_ = h.manager.Reload()
	types.Success(c, route)
}

// UpdateRoute godoc
// @Summary      更新服务路由
// @Description  按 ID 更新路由的前缀、目标地址、重写规则、描述或启用状态。更新后自动热重载生效
// @Tags         admin
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id    path      int                                true   "路由 ID"
// @Param        body  body      updateRouteInput                   true   "路由更新信息"
// @Success      200   {object}  types.Response{data=swaggerServiceRoute}   "更新后的路由"
// @Failure      400   {object}  types.Response                            "参数错误"
// @Failure      404   {object}  types.Response                            "路由不存在"
// @Failure      500   {object}  types.Response                            "服务器错误"
// @Router       /admin/routes/{id} [put]
func (h *RouteHandler) UpdateRoute(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		types.Error(c, http.StatusBadRequest, 400, "无效的路由 ID")
		return
	}
	var route model.ServiceRoute
	if err := h.manager.DB().First(&route, id).Error; err != nil {
		types.Error(c, http.StatusNotFound, 404, "路由不存在")
		return
	}
	var input updateRouteInput
	if err := c.ShouldBindJSON(&input); err != nil {
		types.Error(c, http.StatusBadRequest, 400, err.Error())
		return
	}
	updates := map[string]any{"rewrite": input.Rewrite}
	if input.Prefix != "" {
		updates["prefix"] = input.Prefix
	}
	if input.Target != "" {
		updates["target"] = input.Target
	}
	if input.Description != "" {
		updates["description"] = input.Description
	}
	if input.Enabled != nil {
		updates["enabled"] = *input.Enabled
	}
	if err := h.manager.DB().Model(&route).Updates(updates).Error; err != nil {
		types.Error(c, http.StatusInternalServerError, 500, err.Error())
		return
	}
	_ = h.manager.Reload()
	types.Success(c, route)
}

// DeleteRoute godoc
// @Summary      删除服务路由
// @Description  按 ID 删除服务路由，删除后自动热重载生效
// @Tags         admin
// @Security     BearerAuth
// @Produce      json
// @Param        id  path      int              true  "路由 ID"
// @Success      200 {object}  types.Response        "删除成功"
// @Failure      400 {object}  types.Response        "ID 无效"
// @Failure      500 {object}  types.Response        "服务器错误"
// @Router       /admin/routes/{id} [delete]
func (h *RouteHandler) DeleteRoute(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		types.Error(c, http.StatusBadRequest, 400, "无效的路由 ID")
		return
	}
	if err := h.manager.DB().Delete(&model.ServiceRoute{}, id).Error; err != nil {
		types.Error(c, http.StatusInternalServerError, 500, err.Error())
		return
	}
	_ = h.manager.Reload()
	types.Success(c, nil)
}

// EnableRoute godoc
// @Summary      启用服务路由
// @Description  将指定路由的 enabled 字段设为 true，热重载后立即生效
// @Tags         admin
// @Security     BearerAuth
// @Produce      json
// @Param        id  path      int              true  "路由 ID"
// @Success      200 {object}  types.Response        "操作成功"
// @Failure      400 {object}  types.Response        "ID 无效"
// @Failure      500 {object}  types.Response        "服务器错误"
// @Router       /admin/routes/{id}/enable [post]
func (h *RouteHandler) EnableRoute(c *gin.Context) {
	h.setEnabled(c, true)
}

// DisableRoute godoc
// @Summary      禁用服务路由
// @Description  将指定路由的 enabled 字段设为 false，热重载后立即停止转发
// @Tags         admin
// @Security     BearerAuth
// @Produce      json
// @Param        id  path      int              true  "路由 ID"
// @Success      200 {object}  types.Response        "操作成功"
// @Failure      400 {object}  types.Response        "ID 无效"
// @Failure      500 {object}  types.Response        "服务器错误"
// @Router       /admin/routes/{id}/disable [post]
func (h *RouteHandler) DisableRoute(c *gin.Context) {
	h.setEnabled(c, false)
}

func (h *RouteHandler) setEnabled(c *gin.Context, enabled bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		types.Error(c, http.StatusBadRequest, 400, "无效的路由 ID")
		return
	}
	if err := h.manager.DB().Model(&model.ServiceRoute{}).Where("id = ?", id).Update("enabled", enabled).Error; err != nil {
		types.Error(c, http.StatusInternalServerError, 500, err.Error())
		return
	}
	_ = h.manager.Reload()
	types.Success(c, nil)
}

// ReloadRoutes godoc
// @Summary      手动触发路由热重载
// @Description  从数据库重新加载所有启用的服务路由和白名单，无需重启进程即可生效
// @Tags         admin
// @Security     BearerAuth
// @Produce      json
// @Success      200 {object}  types.Response  "重载成功"
// @Failure      500 {object}  types.Response  "重载失败"
// @Router       /admin/routes/reload [post]
func (h *RouteHandler) ReloadRoutes(c *gin.Context) {
	if err := h.manager.Reload(); err != nil {
		types.Error(c, http.StatusInternalServerError, 500, err.Error())
		return
	}
	types.Success(c, gin.H{"message": "路由重载成功"})
}

// ─── Whitelist ────────────────────────────────────────────────────────────────

// ListWhitelists godoc
// @Summary      列出所有白名单
// @Description  返回所有白名单路由，支持按 HTTP 方法（精确匹配）和路径（模糊匹配）过滤
// @Tags         admin
// @Security     BearerAuth
// @Produce      json
// @Param        method  query     string  false  "HTTP 方法（精确匹配，如 GET、POST、*）"  Enums(GET, POST, PUT, DELETE, PATCH, *)
// @Param        path    query     string  false  "路径（模糊匹配，如 /rbac）"
// @Success      200     {object}  types.Response{data=[]swaggerWhitelistRoute}  "白名单列表"
// @Failure      500     {object}  types.Response                               "服务器错误"
// @Router       /admin/whitelists [get]
func (h *RouteHandler) ListWhitelists(c *gin.Context) {
	db := h.manager.DB()
	if method := c.Query("method"); method != "" {
		db = db.Where("method = ?", method)
	}
	if path := c.Query("path"); path != "" {
		db = db.Where("path LIKE ?", "%"+path+"%")
	}
	var items []model.WhitelistRoute
	if err := db.Find(&items).Error; err != nil {
		types.Error(c, http.StatusInternalServerError, 500, err.Error())
		return
	}
	types.Success(c, items)
}

// CreateWhitelist godoc
// @Summary      新增白名单条目
// @Description  新增一条白名单路由，匹配到该方法+路径的请求将绕过 JWT 鉴权。创建后自动热重载生效
// @Tags         admin
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body      createWhitelistInput                      true  "白名单条目"
// @Success      200   {object}  types.Response{data=swaggerWhitelistRoute}       "创建的白名单条目"
// @Failure      400   {object}  types.Response                                  "参数错误"
// @Failure      500   {object}  types.Response                                  "服务器错误"
// @Router       /admin/whitelists [post]
func (h *RouteHandler) CreateWhitelist(c *gin.Context) {
	var input createWhitelistInput
	if err := c.ShouldBindJSON(&input); err != nil {
		types.Error(c, http.StatusBadRequest, 400, err.Error())
		return
	}
	item := model.WhitelistRoute{
		Method:      input.Method,
		Path:        input.Path,
		Description: input.Description,
	}
	if err := h.manager.DB().Create(&item).Error; err != nil {
		types.Error(c, http.StatusInternalServerError, 500, err.Error())
		return
	}
	_ = h.manager.Reload()
	types.Success(c, item)
}

// DeleteWhitelist godoc
// @Summary      删除白名单条目
// @Description  按 ID 删除白名单条目，删除后该路径将重新受 JWT 鉴权保护。自动热重载生效
// @Tags         admin
// @Security     BearerAuth
// @Produce      json
// @Param        id  path      int              true  "白名单 ID"
// @Success      200 {object}  types.Response        "删除成功"
// @Failure      400 {object}  types.Response        "ID 无效"
// @Failure      500 {object}  types.Response        "服务器错误"
// @Router       /admin/whitelists/{id} [delete]
func (h *RouteHandler) DeleteWhitelist(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		types.Error(c, http.StatusBadRequest, 400, "无效的白名单 ID")
		return
	}
	if err := h.manager.DB().Delete(&model.WhitelistRoute{}, id).Error; err != nil {
		types.Error(c, http.StatusInternalServerError, 500, err.Error())
		return
	}
	_ = h.manager.Reload()
	types.Success(c, nil)
}

// ─── Input types (for swagger) ─────────────────────────────────────────────────

// swaggerServiceRoute Swagger 文档用的服务路由响应模型。
type swaggerServiceRoute struct {
	ID          uint    `json:"ID" example:"1"`
	CreatedAt   string  `json:"CreatedAt" example:"2026-03-23T23:05:00.567525+08:00"`
	UpdatedAt   string  `json:"UpdatedAt" example:"2026-03-23T23:05:00.567525+08:00"`
	DeletedAt   *string `json:"DeletedAt" example:"null"`
	Prefix      string  `json:"prefix" example:"/rbac"`
	Target      string  `json:"target" example:"http://192.168.30.198:8000"`
	Rewrite     string  `json:"rewrite" example:""`
	Description string  `json:"description" example:"ZebraRBAC 权限服务"`
	Enabled     bool    `json:"enabled" example:"true"`
}

// swaggerWhitelistRoute Swagger 文档用的白名单响应模型。
type swaggerWhitelistRoute struct {
	ID          uint    `json:"ID" example:"1"`
	CreatedAt   string  `json:"CreatedAt" example:"2026-03-23T23:05:00.567525+08:00"`
	UpdatedAt   string  `json:"UpdatedAt" example:"2026-03-23T23:05:00.567525+08:00"`
	DeletedAt   *string `json:"DeletedAt" example:"null"`
	Method      string  `json:"method" example:"POST"`
	Path        string  `json:"path" example:"/rbac/login/access-token"`
	Description string  `json:"description" example:"登录接口，无需鉴权"`
}

// createRouteInput 新增服务路由请求体
type createRouteInput struct {
	// Prefix 网关对外暴露的路径前缀，如 /rbac（必填，全局唯一）
	Prefix string `json:"prefix" binding:"required" example:"/rbac"`
	// Target 后端服务根地址，如 http://192.168.30.198:8000（必填）
	Target string `json:"target" binding:"required" example:"http://192.168.30.198:8000"`
	// Rewrite strip Prefix 后插入的路径前缀，空字符串表示不改写
	Rewrite string `json:"rewrite" example:""`
	// Description 路由描述
	Description string `json:"description" example:"ZebraRBAC 权限服务"`
	// Enabled 是否启用，默认 true
	Enabled *bool `json:"enabled" example:"true"`
}

// updateRouteInput 更新服务路由请求体（所有字段均可选）
type updateRouteInput struct {
	// Prefix 路径前缀（为空则不更新）
	Prefix string `json:"prefix" example:"/rbac"`
	// Target 目标地址（为空则不更新）
	Target string `json:"target" example:"http://192.168.30.198:8000"`
	// Rewrite 重写前缀
	Rewrite string `json:"rewrite" example:""`
	// Description 描述（为空则不更新）
	Description string `json:"description" example:"ZebraRBAC 权限服务"`
	// Enabled 启用状态
	Enabled *bool `json:"enabled" example:"true"`
}

// createWhitelistInput 新增白名单请求体
type createWhitelistInput struct {
	// Method HTTP 方法，* 表示任意方法（必填）
	Method string `json:"method" binding:"required" example:"POST"`
	// Path 完整请求路径或 /* 结尾的前缀通配（必填）
	Path string `json:"path" binding:"required" example:"/rbac/login/access-token"`
	// Description 描述
	Description string `json:"description" example:"登录接口，无需鉴权"`
}
