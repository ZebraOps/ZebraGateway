// Package api 定义网关自身的 HTTP 处理器（非代理路由），并为 swaggo 提供接口注释。
package api

import (
	"net/http"

	"github.com/ZebraOps/ZebraGateway/internal/types"
	"github.com/gin-gonic/gin"
)

// HealthResponse 健康检查响应数据
type HealthResponse struct {
	Status  string `json:"status" example:"ok"`
	Service string `json:"service" example:"ZebraGateway"`
}

// LoginRequest 登录请求体
type LoginRequest struct {
	Username string `json:"username" example:"admin"`
	Password string `json:"password" example:"password"`
}

// TokenData token 数据
type TokenData struct {
	Token        string `json:"token" example:"eyJhbGci..."`
	RefreshToken string `json:"refreshToken" example:"eyJhbGci..."`
}

// Health 网关健康检查
//
//	@Summary		健康检查
//	@Description	检查网关服务是否正常运行
//	@Tags			Gateway
//	@Produce		json
//	@Success		200	{object}	types.Response{data=HealthResponse}	"服务正常"
//	@Router			/health [get]
func Health(c *gin.Context) {
	types.Success(c, HealthResponse{
		Status:  "ok",
		Service: "ZebraGateway",
	})
}

// Login 用户登录（代理到 ZebraRBAC）
//
//	@Summary		用户登录
//	@Description	通过用户名和密码获取 JWT access token 与 refresh token。请求由网关直接转发到 ZebraRBAC，**无需鉴权**。
//	@Tags			Auth
//	@Accept			json
//	@Produce		json
//	@Param			body	body		LoginRequest									true	"登录信息"
//	@Success		200		{object}	types.Response{data=TokenData}				"登录成功"
//	@Failure		400		{object}	types.Response								"用户名或密码错误"
//	@Router			/rbac/login/access-token [post]
func Login(c *gin.Context) {
	// 此函数仅用于提供 Swagger 文档，实际请求由 ProxyHandler 转发到 ZebraRBAC。
	// 网关层永远不会执行到这里（路由先匹配到代理处理器）。
	c.JSON(http.StatusOK, nil)
}

// ---- ZebraRBAC 代理接口文档 ----

// GetUsers 获取用户列表（代理到 ZebraRBAC）
//
//	@Summary		获取用户列表
//	@Description	获取 RBAC 系统中的用户列表（分页），由网关转发到 ZebraRBAC `/api/users`
//	@Tags			RBAC
//	@Produce		json
//	@Security		BearerAuth
//	@Param			current	query		int								false	"页码（从 0 开始）"	default(0)
//	@Param			size	query		int								false	"每页数量"			default(20)
//	@Param			username	query	string							false	"用户名搜索"
//	@Success		200		{object}	types.Response					"用户列表"
//	@Failure		401		{object}	types.Response					"未认证"
//	@Failure		403		{object}	types.Response					"权限不足"
//	@Router			/rbac/users [get]
func GetUsers(c *gin.Context) { c.JSON(http.StatusOK, nil) }

// GetRoles 获取角色列表（代理到 ZebraRBAC）
//
//	@Summary		获取角色列表
//	@Description	获取 RBAC 系统中所有角色，由网关转发到 ZebraRBAC `/api/roles`
//	@Tags			RBAC
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	types.Response	"角色列表"
//	@Failure		401	{object}	types.Response	"未认证"
//	@Failure		403	{object}	types.Response	"权限不足"
//	@Router			/rbac/roles [get]
func GetRoles(c *gin.Context) { c.JSON(http.StatusOK, nil) }

// GetMenus 获取菜单列表（代理到 ZebraRBAC）
//
//	@Summary		获取菜单列表
//	@Description	获取 RBAC 系统菜单，由网关转发到 ZebraRBAC `/api/menus`
//	@Tags			RBAC
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	types.Response	"菜单列表"
//	@Failure		401	{object}	types.Response	"未认证"
//	@Router			/rbac/menus [get]
func GetMenus(c *gin.Context) { c.JSON(http.StatusOK, nil) }

// GetAuthorization 获取当前用户权限信息（代理到 ZebraRBAC）
//
//	@Summary		获取当前用户权限
//	@Description	返回当前登录用户的角色、菜单路径、功能点、组件权限等完整权限数据，由网关转发到 ZebraRBAC `/api/authorization`
//	@Tags			RBAC
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	types.Response{data=types.RBACAuthData}	"权限信息"
//	@Failure		401	{object}	types.Response							"未认证"
//	@Router			/rbac/authorization [get]
func GetAuthorization(c *gin.Context) { c.JSON(http.StatusOK, nil) }
