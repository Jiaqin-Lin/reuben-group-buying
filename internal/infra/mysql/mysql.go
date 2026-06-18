// Package mysql GORM 初始化与连接池管理。
package mysql

import (
	"fmt"
	"log/slog"
	"time"

	mysqldrv "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Config MySQL 连接配置。
type Config struct {
	DSN             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

// New 创建 GORM 实例并配置连接池。
func New(cfg Config, log *slog.Logger) (*gorm.DB, error) {
	db, err := gorm.Open(mysqldrv.Open(cfg.DSN), &gorm.Config{
		Logger:      logger.Default.LogMode(logger.Warn),
		PrepareStmt: true,
		// 禁用外键约束（应用层保证一致性）
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		return nil, fmt.Errorf("mysql open: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("mysql get db: %w", err)
	}

	// 连接池配置
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	sqlDB.SetConnMaxIdleTime(5 * time.Minute)

	// 验证连接
	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("mysql ping: %w", err)
	}

	log.Info("mysql connected",
		"max_open", cfg.MaxOpenConns,
		"max_idle", cfg.MaxIdleConns,
	)
	return db, nil
}
