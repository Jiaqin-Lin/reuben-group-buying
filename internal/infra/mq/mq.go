// Package mq 消息队列客户端，基于 Redis Pub/Sub。
//
// Redis Pub/Sub 是轻量级选择：
//   - 优点：已运维、无额外依赖、够用
//   - 缺点：fire-and-forget（无持久化）、无 ACK
//   - 补偿：notify_tasks 表提供持久化，cron 扫描兜底重试
package mq

import (
	"context"
	"fmt"
	"log/slog"

	goredis "github.com/redis/go-redis/v9"
)

// Client 消息队列客户端。
type Client struct {
	rdb *goredis.Client
	log *slog.Logger
}

// New 创建消息队列客户端。
func New(rdb *goredis.Client, log *slog.Logger) *Client {
	return &Client{rdb: rdb, log: log}
}

// Publish 发送消息到指定 channel。
//
// Redis Pub/Sub 语义：
//   - 无订阅者时消息直接丢弃（不报错）
//   - 订阅者收到消息的顺序即发送顺序
//   - 消息无持久化，重启/网络断开期间的消息会丢失
//
// 对于回调通知场景，notify_tasks 表是持久化层，cron 重试是兜底，
// 所以 Pub/Sub 的 fire-and-forget 特性可以接受。
func (c *Client) Publish(ctx context.Context, channel string, payload []byte) error {
	result := c.rdb.Publish(ctx, channel, payload)
	if err := result.Err(); err != nil {
		return fmt.Errorf("mq publish to %s: %w", channel, err)
	}

	c.log.DebugContext(ctx, "mq published", "channel", channel, "subscribers", result.Val())
	return nil
}

// Subscribe 订阅指定 channel，消息通过 handler 回调处理。
//
// 返回的 channel 用于通知订阅已就绪（发送一次后关闭）。
// handler 在独立的 goroutine 中串行执行，如果处理慢会阻塞后续消息。
// ctx 取消时订阅自动关闭。
func (c *Client) Subscribe(ctx context.Context, channel string, handler func([]byte) error) (<-chan struct{}, error) {
	pubsub := c.rdb.Subscribe(ctx, channel)

	// 等待订阅确认
	if _, err := pubsub.Receive(ctx); err != nil {
		pubsub.Close()
		return nil, fmt.Errorf("mq subscribe to %s: %w", channel, err)
	}

	ready := make(chan struct{})
	close(ready) // 订阅就绪，立即通知

	go func() {
		defer pubsub.Close()

		ch := pubsub.Channel()
		for {
			select {
			case <-ctx.Done():
				c.log.DebugContext(ctx, "mq subscribe stopped", "channel", channel)
				return
			case msg, ok := <-ch:
				if !ok {
					return
				}
				if err := handler([]byte(msg.Payload)); err != nil {
					c.log.WarnContext(ctx, "mq handler error", "channel", channel, "error", err)
				}
			}
		}
	}()

	c.log.InfoContext(ctx, "mq subscribed", "channel", channel)
	return ready, nil
}
