package store

import (
	"github.com/ZebraOps/ZebraGateway/internal/model"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// New 打开 PostgreSQL 连接并自动迁移表结构。
// dsn 示例：postgres://root:pass123@192.168.41.54:5432/zebra_gateway?sslmode=disable
func New(dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, err
	}
	if err := db.AutoMigrate(&model.ServiceRoute{}, &model.WhitelistRoute{}); err != nil {
		return nil, err
	}
	return db, nil
}
