// Package mq 消息队列抽象层。
//
// 提供 Producer 接口用于消息发送，以及 TimeoutConsumer 用于支付超时退单。
// 动态配置传播继续使用 Redis Pub/Sub（bgm:config:updates），不走此抽象。
package mq

import "context"

// Producer 消息生产者接口。
// 当前实现：RocketMQ（rocketmq.go），旧有 Redis Pub/Sub 实现（mq.go）保留用于测试。
type Producer interface {
	// Publish 发送普通消息到指定 topic:tag。
	Publish(ctx context.Context, topic, tag string, payload []byte) error

	// PublishDelayed 发送延迟消息。delayLevel 使用 RocketMQ 固定级别（1=1s, 5=1m, 9=5m）。
	PublishDelayed(ctx context.Context, topic, tag string, payload []byte, delayLevel int) error

	// Close 优雅关闭生产者。
	Close() error
}
