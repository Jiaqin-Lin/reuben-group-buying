// Package mq 消息队列客户端（预留）。
// 当前为 stub，后续可替换为 Redis Pub/Sub、RabbitMQ 或 Kafka。
package mq

import (
	"context"
	"log/slog"
)

// Config 消息队列配置。
type Config struct {
	Type string // redis_pubsub | rabbitmq | kafka (当前仅 redis_pubsub)
	Addr string // broker 地址
}

// Client 消息队列客户端接口。
type Client struct {
	cfg Config
	log *slog.Logger
}

// New 创建消息队列客户端。
func New(cfg Config, log *slog.Logger) *Client {
	return &Client{cfg: cfg, log: log}
}

// Publish 发送消息到指定 topic。
func (c *Client) Publish(ctx context.Context, topic string, payload []byte) error {
	// TODO: implement via Redis Pub/Sub or real MQ
	c.log.Debug("mq publish (stub)", "topic", topic, "len", len(payload))
	return nil
}

// Subscribe 订阅指定 topic，消息通过回调处理。
func (c *Client) Subscribe(ctx context.Context, topic string, handler func([]byte) error) error {
	// TODO: implement via Redis Pub/Sub or real MQ
	c.log.Debug("mq subscribe (stub)", "topic", topic)
	return nil
}
