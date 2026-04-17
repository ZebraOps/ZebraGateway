package model

import "gorm.io/gorm"

// ServiceRoute 后端服务路由（DB 表：service_routes）
type ServiceRoute struct {
	gorm.Model
	// Prefix 网关对外暴露的路径前缀，如 "/rbac"
	Prefix string `gorm:"uniqueIndex;not null;size:255" json:"prefix"`
	// Target 后端服务根地址，如 "http://192.168.30.198:8000"
	Target string `gorm:"not null;size:512" json:"target"`
	// Rewrite strip Prefix 后插入的路径前缀，空字符串表示不改写
	Rewrite string `gorm:"size:255" json:"rewrite"`
	// Description 路由描述
	Description string `gorm:"type:text" json:"description"`
	// Enabled 是否启用
	Enabled bool `gorm:"default:true" json:"enabled"`
}

// WhitelistRoute 白名单路由（DB 表：whitelist_routes）
type WhitelistRoute struct {
	gorm.Model
	// Method HTTP 方法，"*" 表示任意方法
	Method string `gorm:"not null;size:10" json:"method"`
	// Path 完整请求路径（精确匹配）或以 /* 结尾的前缀通配
	Path string `gorm:"not null;size:512" json:"path"`
	// Description 描述
	Description string `gorm:"type:text" json:"description"`
}
