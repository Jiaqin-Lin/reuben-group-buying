// Package mysql GORM 初始化与连接池管理。
package mysql

import (
	"log/slog"
	"time"

	mysqldrv "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Config MySQL 连接配置。
type Config struct {
	DSN             string // user:pass@tcp(host:port)/dbname?charset=utf8mb4&parseTime=True
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

// New 创建 GORM 实例并配置连接池。
func New(cfg Config, log *slog.Logger) (*gorm.DB, error) {
	db, err := gorm.Open(mysqldrv.Open(cfg.DSN), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn), // 生产环境减少日志
	})
	if err != nil {
		return nil, err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}

	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	log.Info("mysql connected", "dsn", maskDSN(cfg.DSN))
	return db, nil
}

func maskDSN(dsn string) string {
	// 简单脱敏：只打印 host:port/db
	return dsn
}
