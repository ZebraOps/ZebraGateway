package types

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Response 统一 API 响应结构
type Response struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// RBACFunction 功能点（method + uri）
type RBACFunction struct {
	Method string `json:"method"`
	URI    string `json:"uri"`
}

// RBACPermissions 权限信息
type RBACPermissions struct {
	All        bool            `json:"all"`
	Functions  []RBACFunction  `json:"functions"`
	Components map[string]bool `json:"components"`
}

// RBACMenus 菜单信息
type RBACMenus struct {
	All  bool            `json:"all"`
	Data map[string]bool `json:"data"`
}

// RBACRoles 角色信息
type RBACRoles struct {
	All  bool     `json:"all"`
	Data []string `json:"data"`
}

// RBACAuthData RBAC 授权数据（对应 /api/authorization 响应中的 data）
type RBACAuthData struct {
	UserID      interface{}     `json:"userId"`
	UserName    string          `json:"userName"`
	Email       string          `json:"email"`
	NickName    string          `json:"nickName"`
	Avatar      string          `json:"avatar"`
	Roles       RBACRoles       `json:"roles"`
	Menus       RBACMenus       `json:"menus"`
	Permissions RBACPermissions `json:"permissions"`
}

// RBACAuthResponse RBAC 接口外层响应
type RBACAuthResponse struct {
	Code    int          `json:"code"`
	Message string       `json:"message"`
	Data    RBACAuthData `json:"data"`
}

// LoggingConfig 日志配置结构
type LoggingConfig struct {
	Level            string   `mapstructure:"level"`
	Encoding         string   `mapstructure:"encoding"`
	OutputPaths      []string `mapstructure:"output_paths"`
	ErrorOutputPaths []string `mapstructure:"error_output_paths"`
	MaxSize          int      `mapstructure:"max_size"`
	MaxAge           int      `mapstructure:"max_age"`
	MaxBackups       int      `mapstructure:"max_backups"`
	Compress         bool     `mapstructure:"compress"`
}

// ContextKey 用于 gin.Context 的键类型（防止 key 冲突）
type ContextKey string

const (
	ContextUserID   ContextKey = "userID"
	ContextUserName ContextKey = "userName"
)

// Success 成功响应
func Success(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, Response{
		Code:    200,
		Message: "success",
		Data:    data,
	})
}

// Error 以指定 HTTP 状态码返回错误，并终止后续处理
func Error(c *gin.Context, httpCode int, code int, message string) {
	c.AbortWithStatusJSON(httpCode, Response{
		Code:    code,
		Message: message,
	})
}
