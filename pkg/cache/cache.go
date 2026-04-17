package cache

import (
	"fmt"
	"time"

	gocache "github.com/patrickmn/go-cache"

	"github.com/ZebraOps/ZebraGateway/internal/types"
)

// AuthCache 用户权限缓存，基于 patrickmn/go-cache 实现内存缓存 + TTL
type AuthCache struct {
	c   *gocache.Cache
	ttl time.Duration
}

// New 创建权限缓存实例，ttlSeconds 为缓存有效期（秒）
func New(ttlSeconds int) *AuthCache {
	ttl := time.Duration(ttlSeconds) * time.Second
	// 清理间隔设为 TTL 的 2 倍，避免频繁 GC
	return &AuthCache{
		c:   gocache.New(ttl, ttl*2),
		ttl: ttl,
	}
}

// Set 将用户权限数据写入缓存
func (a *AuthCache) Set(userID string, data *types.RBACAuthData) {
	a.c.Set(cacheKey(userID), data, a.ttl)
}

// Get 从缓存读取用户权限数据，第二个返回值表示是否命中
func (a *AuthCache) Get(userID string) (*types.RBACAuthData, bool) {
	val, ok := a.c.Get(cacheKey(userID))
	if !ok {
		return nil, false
	}
	data, ok := val.(*types.RBACAuthData)
	return data, ok
}

// Delete 主动删除用户权限缓存（如用户角色变更时调用）
func (a *AuthCache) Delete(userID string) {
	a.c.Delete(cacheKey(userID))
}

func cacheKey(userID string) string {
	return fmt.Sprintf("auth:%s", userID)
}
