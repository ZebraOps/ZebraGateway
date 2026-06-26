package middleware

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/ZebraOps/ZebraGateway/internal/model"
	"github.com/gorilla/websocket"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

var wsUpgrader = websocket.Upgrader{
	CheckOrigin:       func(r *http.Request) bool { return true },
	HandshakeTimeout:  10 * time.Second,
}

// wsDialer 用于建立网关到后端的 WebSocket 连接。
var wsDialer = websocket.Dialer{
	HandshakeTimeout: 10 * time.Second,
}

// IsWebSocketUpgrade 判断 gin.Context 是否为 WebSocket 升级请求。
// 复用 auth.go 中的逻辑，统一检测标准。
func IsWebSocketUpgrade(c *gin.Context) bool {
	return strings.EqualFold(c.GetHeader("Connection"), "upgrade") &&
		strings.EqualFold(c.GetHeader("Upgrade"), "websocket")
}

// ProxyWebSocket 将 WebSocket 升级请求桥接到后端服务。
//
// 流程：
//  1. 升级客户端连接（浏览器 → 网关）为 WebSocket
//  2. 根据路由配置将 HTTP 目标地址转换为 WebSocket URL
//  3. 建立网关 → 后端的 WebSocket 连接
//  4. 双向桥接：两个 goroutine 分别转发 client→backend 和 backend→client
//  5. 任一端断开则关闭另一端，清理资源
func ProxyWebSocket(c *gin.Context, route *model.ServiceRoute, target string, logger *zap.Logger) {
	// --- 1. 升级客户端连接 ---
	clientConn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		logger.Error("WebSocket 客户端升级失败",
			zap.String("path", c.Request.URL.Path),
			zap.Error(err),
		)
		return
	}
	defer clientConn.Close()

	// --- 2. 构建后端 WebSocket URL ---
	// 路径改写：与 ServeProxy 保持一致的逻辑
	reqPath := c.Request.URL.Path
	newPath := strings.TrimPrefix(reqPath, route.Prefix)
	if route.Rewrite != "" {
		newPath = route.Rewrite + newPath
	}
	if newPath == "" {
		newPath = "/"
	}

	// 目标地址 http → ws, https → wss
	wsTarget := target
	if after, ok := strings.CutPrefix(wsTarget, "https://"); ok {
		wsTarget = "wss://" + after
	} else if after, ok := strings.CutPrefix(wsTarget, "http://"); ok {
		wsTarget = "ws://" + after
	}

	// 保留原始查询参数（namespace、token、container 等）
	wsURL := fmt.Sprintf("%s%s?%s", wsTarget, newPath, c.Request.URL.RawQuery)

	logger.Debug("WebSocket 代理桥接",
		zap.String("client_path", reqPath),
		zap.String("backend_path", newPath),
		zap.String("backend_ws_url", wsURL),
	)

	// --- 3. 建立后端 WebSocket 连接 ---
	// 将客户端请求的 Authorization 头传递给后端（用于后端鉴权）
	reqHeader := http.Header{}
	if auth := c.Request.Header.Get("Authorization"); auth != "" {
		reqHeader.Set("Authorization", auth)
	}
	// 网关 auth 中间件已注入的 X-User-Id / X-User-Name 也传递
	if uid := c.Request.Header.Get("X-User-Id"); uid != "" {
		reqHeader.Set("X-User-Id", uid)
	}
	if uname := c.Request.Header.Get("X-User-Name"); uname != "" {
		reqHeader.Set("X-User-Name", uname)
	}

	backendConn, _, err := wsDialer.Dial(wsURL, reqHeader)
	if err != nil {
		logger.Error("WebSocket 后端连接失败",
			zap.String("ws_url", wsURL),
			zap.Error(err),
		)
		// 向客户端发送错误关闭帧
		clientConn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseInternalServerErr, "后端服务连接失败"))
		return
	}
	defer backendConn.Close()

	// --- 4. 双向桥接 ---
	done := make(chan struct{}, 2)

	// client → backend
	go func() {
		defer func() { done <- struct{}{} }()
		wsCopy(clientConn, backendConn, logger, "client→backend")
	}()

	// backend → client
	go func() {
		defer func() { done <- struct{}{} }()
		wsCopy(backendConn, clientConn, logger, "backend→client")
	}()

	// 等待任一方向结束
	<-done

	logger.Debug("WebSocket 代理桥接结束",
		zap.String("path", reqPath),
	)
}

// wsCopy 将 WebSocket 消息从 src 逐帧转发到 dst。
// 保留原始消息类型（TextMessage / BinaryMessage），不做内容修改。
// src 读取失败时向 dst 发送 Close 帧，确保对方也能优雅退出。
func wsCopy(src, dst *websocket.Conn, logger *zap.Logger, direction string) {
	for {
		msgType, data, err := src.ReadMessage()
		if err != nil {
			// 读失败：连接已关闭或出错
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				logger.Debug("WebSocket 读结束",
					zap.String("direction", direction),
					zap.Error(err),
				)
			}
			// 向对方发送 Close 帧（若对方还活着）
			dst.WriteMessage(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			return
		}

		if err := dst.WriteMessage(msgType, data); err != nil {
			logger.Debug("WebSocket 写失败",
				zap.String("direction", direction),
				zap.Error(err),
			)
			return
		}
	}
}
