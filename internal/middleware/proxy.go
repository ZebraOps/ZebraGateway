package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// ServiceRoute 反向代理路由配置
type ServiceRoute struct {
	// Prefix 网关对外前缀，如 "/rbac"
	Prefix string
	// Target 后端服务根地址，如 "http://localhost:8000"
	Target string
	// Rewrite 路径改写前缀，如 "/api"。
	// 转发路径 = Rewrite + TrimPrefix(originalPath, Prefix)
	Rewrite string
}

// ProxyHandler 创建反向代理 gin.HandlerFunc
//
// 路径转换示例：
//
//	Prefix="/rbac", Rewrite="/api"
//	  /rbac/login/access-token  ->  /api/login/access-token
//
//	Prefix="/cicd", Rewrite=""
//	  /cicd/applications/1  ->  /applications/1
func ProxyHandler(route ServiceRoute, logger *zap.Logger) gin.HandlerFunc {
	targetURL, err := url.Parse(route.Target)
	if err != nil {
		panic("invalid proxy target URL: " + route.Target)
	}

	proxy := &httputil.ReverseProxy{
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
		},
		Director: func(req *http.Request) {
			// 1. 剥离前缀
			newPath := strings.TrimPrefix(req.URL.Path, route.Prefix)
			if newPath == "" || newPath[0] != '/' {
				newPath = "/" + newPath
			}

			// 2. 拼接 rewrite 前缀
			if route.Rewrite != "" {
				newPath = route.Rewrite + newPath
			}

			// 3. 设置目标地址
			req.URL.Scheme = targetURL.Scheme
			req.URL.Host = targetURL.Host
			req.URL.Path = newPath
			req.Host = targetURL.Host

			// 4. 设置 X-Forwarded-Host，转发真实请求 Host
			if req.Header.Get("X-Forwarded-Host") == "" {
				req.Header.Set("X-Forwarded-Host", req.Host)
			}
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			logger.Error("反向代理错误",
				zap.String("target", route.Target),
				zap.String("path", r.URL.Path),
				zap.Error(err),
			)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			body, _ := json.Marshal(map[string]interface{}{
				"code":    502,
				"message": "上游服务暂时不可用，请稍后重试",
			})
			_, _ = w.Write(body)
		},
	}

	return func(c *gin.Context) {
		proxy.ServeHTTP(c.Writer, c.Request)
	}
}
