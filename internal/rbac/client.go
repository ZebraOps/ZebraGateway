package rbac

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/ZebraOps/ZebraGateway/internal/types"
)

// Client ZebraRBAC HTTP 客户端
type Client struct {
	baseURL    string
	httpClient *http.Client
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

// GetAuthorization 调用 ZebraRBAC GET /api/authorization 获取当前用户的权限信息。
// token 为原始 Bearer token 字符串（不含 "Bearer " 前缀）。
func (c *Client) GetAuthorization(token string) (*types.RBACAuthData, error) {
	url := fmt.Sprintf("%s/api/authorization", c.baseURL)

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
