// Package redis go-redis 客户端初始化与连接管理。
package redis

import (
	"context"
	"log/slog"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// Config Redis 连接配置。
type Config struct {
	Addr     string // host:port
	Password string
	DB       int
}

// New 创建 go-redis 客户端并验证连接。
func New(cfg Config, log *slog.Logger) (*goredis.Client, error) {
	rdb := goredis.NewClient(&goredis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, err
	}

	log.Info("redis connected", "addr", cfg.Addr, "db", cfg.DB)
	return rdb, nil
}
