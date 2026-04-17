package middleware

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/ZebraOps/ZebraGateway/internal/rbac"
	"github.com/ZebraOps/ZebraGateway/internal/types"
	"github.com/ZebraOps/ZebraGateway/pkg/cache"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
)

// WhitelistEntry 白名单条目
type WhitelistEntry struct {
	// Method HTTP 方法，"*" 表示任意方法
	Method string
	// Path 完整请求路径（精确匹配）
	Path string
}

// DynamicWhitelistChecker 动态白名单检查接口（由 router.Manager 实现）。
// 用于检查来自数据库的白名单条目，与静态 YAML 白名单互补。
type DynamicWhitelistChecker interface {
	IsWhitelisted(method, path string) bool
}

// PathRewriter 将网关级路径转换为后端服务路径（由 router.Manager 实现）。
// 用于权限校验时将请求路径映射为 RBAC 中存储的接口路径。
type PathRewriter interface {
	RewritePath(path string) string
}

// Auth 网关鉴权中间件
//
// 鉴权流程：
//  1. 路径在静态白名单（YAML）→ 直接放行
//  2. 路径在动态白名单（DB，dynChecker 不为 nil 时检查）→ 直接放行
//  3. 提取 Authorization: Bearer <token>
//  4. 本地 HS256 验证 JWT 签名，取 sub 字段为 userID
//  5. 查权限缓存（TTL 由 authCache 决定），未命中则调用 ZebraRBAC /api/authorization
//  6. permissions.all == true（超级管理员）→ 放行
//  7. 否则检查 request.path 是否在 permissions.functions 列表中（支持 * 通配符）
//  8. 通过后注入 X-User-Id / X-User-Name 到上游请求头
func Auth(
	jwtSecret string,
	rbacClient *rbac.Client,
	authCache *cache.AuthCache,
	whitelist []WhitelistEntry,
	logger *zap.Logger,
	dynChecker DynamicWhitelistChecker,
	pathRewriter PathRewriter,
) gin.HandlerFunc {
	secretKey := []byte(jwtSecret)

	return func(c *gin.Context) {
		path := c.Request.URL.Path
		method := c.Request.Method

		// --- 安全：移除客户端传入的身份头，防止伪造 ---
		c.Request.Header.Del("X-User-Id")
		c.Request.Header.Del("X-User-Name")

		// --- 白名单检查 ---
		for _, entry := range whitelist {
			if isWhitelisted(entry, method, path) {
				c.Next()
				return
			}
		}
		// 动态白名单（DB 来源）
		if dynChecker != nil && dynChecker.IsWhitelisted(method, path) {
			c.Next()
			return
		}

		// --- 提取 Bearer Token ---
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			types.Error(c, http.StatusUnauthorized, 401, "缺少 Authorization 请求头")
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
			types.Error(c, http.StatusUnauthorized, 401, "Authorization 格式错误，应为 Bearer <token>")
			return
		}
		tokenStr := parts[1]

		// --- 本地 JWT 验证 ---
		token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("不支持的签名算法: %v", t.Header["alg"])
			}
			return secretKey, nil
		})
		if err != nil || !token.Valid {
			logger.Warn("JWT 验证失败",
				zap.String("path", path),
				zap.Error(err),
			)
			types.Error(c, http.StatusUnauthorized, 401, "无效的 Token")
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			types.Error(c, http.StatusUnauthorized, 401, "Token claims 解析失败")
			return
		}

		userID, ok := claims["sub"].(string)
		if !ok || userID == "" {
			types.Error(c, http.StatusUnauthorized, 401, "Token 中缺少用户标识")
			return
		}

		// --- 获取权限（缓存优先） ---
		authData, found := authCache.Get(userID)
		if !found {
			authData, err = rbacClient.GetAuthorization(tokenStr)
			if err != nil {
				logger.Error("从 RBAC 获取权限失败",
					zap.String("userID", userID),
					zap.Error(err),
				)
				types.Error(c, http.StatusUnauthorized, 401, "权限获取失败，请重新登录")
				return
			}
			authCache.Set(userID, authData)
		}

		// --- 权限校验 ---
		if !authData.Permissions.All {
			// 将网关路径转换为后端路径再做权限比对
			checkPath := path
			if pathRewriter != nil {
				checkPath = pathRewriter.RewritePath(path)
			}
			if !hasPermission(authData.Permissions.Functions, method, checkPath) {
				logger.Warn("权限不足",
					zap.String("userID", userID),
					zap.String("method", method),
					zap.String("path", path),
					zap.String("checkPath", checkPath),
				)
				types.Error(c, http.StatusForbidden, 403, "权限不足，无法访问该资源")
				return
			}
		}

		// --- 注入用户信息到请求头（供上游服务使用） ---
		c.Request.Header.Set("X-User-Id", userID)
		c.Request.Header.Set("X-User-Name", authData.UserName)

		// 同时写入 gin Context，方便本地 handler 直接读取
		c.Set(string(types.ContextUserID), userID)
		c.Set(string(types.ContextUserName), authData.UserName)

		c.Next()
	}
}

// isWhitelisted 判断请求是否匹配白名单条目。
//
// 路径匹配规则（优先级从高到低）：
//   - 前缀通配：条目 Path 以 /* 结尾，匹配该前缀下所有子路径（如 "/swagger/*" 匹配 "/swagger/index.html"）
//   - 精确匹配：完整路径相等
func isWhitelisted(entry WhitelistEntry, method, path string) bool {
	methodMatch := entry.Method == "*" || strings.EqualFold(entry.Method, method)
	if !methodMatch {
		return false
	}
	// 前缀通配："/swagger/*" 匹配 "/swagger/任意子路径"
	if strings.HasSuffix(entry.Path, "/*") {
		prefix := strings.TrimSuffix(entry.Path, "*")
		return strings.HasPrefix(path, prefix)
	}
	return entry.Path == path
}

// hasPermission 检查 method + path 是否在权限列表中。
//
// 匹配规则：
//  1. function.method 为空或 "*" 时，仅匹配 path（兼容旧数据）
//  2. 否则同时匹配 method（不区分大小写）和 path
//
// 路径匹配规则（优先级从高到低）：
//   - 精确匹配："/cicd/applications"
//   - 前缀通配："/cicd/applications/*" 匹配 "/cicd/applications/1" 等所有子路径
func hasPermission(functions []types.RBACFunction, method, path string) bool {
	for _, f := range functions {
		// method 校验：空字符串或 * 表示不限方法
		if f.Method != "" && f.Method != "*" {
			if !strings.EqualFold(f.Method, method) {
				continue
			}
		}
		// path 校验
		if f.URI == path {
			return true
		}
		if strings.HasSuffix(f.URI, "*") {
			prefix := strings.TrimSuffix(f.URI, "*")
			if strings.HasPrefix(path, prefix) {
				return true
			}
		}
	}
	return false
}
