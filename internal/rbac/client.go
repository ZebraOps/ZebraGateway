package rbac

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/ZebraOps/ZebraGateway/internal/types"
)

// Client ZebraRBAC HTTP 客户端
type Client struct {
	baseURL    string
	httpClient *http.Client
	mu         sync.RWMutex // 保护 baseURL 的并发访问
}

// New 创建 RBAC 客户端
func New(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// UpdateBaseURL 更新 RBAC 服务地址（用于服务发现场景）
func (c *Client) UpdateBaseURL(newURL string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.baseURL = newURL
}

// GetBaseURL 获取当前 RBAC 服务地址
func (c *Client) GetBaseURL() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.baseURL
}

// GetAuthorization 调用 ZebraRBAC GET /api/authorization 获取当前用户的权限信息。
// token 为原始 Bearer token 字符串（不含 "Bearer " 前缀）。
func (c *Client) GetAuthorization(token string) (*types.RBACAuthData, error) {
	c.mu.RLock()
	url := fmt.Sprintf("%s/api/authorization", c.baseURL)
	c.mu.RUnlock()

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("rbac returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result types.RBACAuthResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if result.Code != 200 {
		return nil, fmt.Errorf("rbac error code=%d message=%s", result.Code, result.Message)
	}

	return &result.Data, nil
}
