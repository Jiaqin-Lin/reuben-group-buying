package mq

import (
	"context"
	"fmt"
	"log/slog"
	"net"

	"github.com/apache/rocketmq-client-go/v2"
	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/apache/rocketmq-client-go/v2/producer"
)

// RocketMQProducer 基于 RocketMQ 的消息生产者。
type RocketMQProducer struct {
	p   rocketmq.Producer
	log *slog.Logger
}

// RocketMQConfig 生产者配置。
type RocketMQConfig struct {
	NameServer string
	GroupName  string
	WarmTopic  string // 启动时发送空消息暖 topic，触发 RocketMQ 5.x 自动创建
}

// NewRocketMQProducer 创建 RocketMQ 生产者并启动。
// cfg.NameServer 支持 host:port 格式（如 "namesrv:9876"），会自动 DNS 解析为 IP。
func NewRocketMQProducer(cfg RocketMQConfig, log *slog.Logger) (*RocketMQProducer, error) {
	addr, err := resolveAddr(cfg.NameServer)
	if err != nil {
		return nil, fmt.Errorf("create rocketmq producer: resolve %s: %w", cfg.NameServer, err)
	}

	p, err := rocketmq.NewProducer(
		producer.WithNsResolver(primitive.NewPassthroughResolver([]string{addr})),
		producer.WithGroupName(cfg.GroupName),
		producer.WithRetry(2),
	)
	if err != nil {
		return nil, fmt.Errorf("create rocketmq producer: %w", err)
	}

	if err := p.Start(); err != nil {
		return nil, fmt.Errorf("start rocketmq producer: %w", err)
	}

	// 暖 topic：发同步消息触发 RocketMQ 5.x 自动创建 topic（含 RETRY topic）
	if cfg.WarmTopic != "" {
		warmMsg := primitive.NewMessage(cfg.WarmTopic, []byte(`{"__warmup__":true}`))
		warmMsg.WithTag("topic.timeout_payment")
		if _, err := p.SendSync(context.Background(), warmMsg); err != nil {
			log.Warn("rocketmq warm topic failed", "topic", cfg.WarmTopic, "error", err)
		} else {
			log.Info("rocketmq topic warmed", "topic", cfg.WarmTopic)
		}
	}

	log.Info("rocketmq producer started", "name_server", cfg.NameServer, "group", cfg.GroupName)
	return &RocketMQProducer{p: p, log: log}, nil
}

// Publish 发送普通消息到 topic:tag。
func (r *RocketMQProducer) Publish(ctx context.Context, topic, tag string, payload []byte) error {
	msg := primitive.NewMessage(topic, payload)
	msg.WithTag(tag)

	res, err := r.p.SendSync(ctx, msg)
	if err != nil {
		return fmt.Errorf("rocketmq publish %s:%s: %w", topic, tag, err)
	}

	r.log.DebugContext(ctx, "rocketmq published",
		"topic", topic, "tag", tag, "msg_id", res.MsgID, "status", res.Status)
	return nil
}

// PublishDelayed 发送延迟消息。
// delayLevel 使用 RocketMQ 固定级别：1=1s, 5=1m, 9=5m, 14=10m, 16=30m, 17=1h, 18=2h。
func (r *RocketMQProducer) PublishDelayed(ctx context.Context, topic, tag string, payload []byte, delayLevel int) error {
	msg := primitive.NewMessage(topic, payload)
	msg.WithTag(tag)
	msg.WithDelayTimeLevel(delayLevel)

	res, err := r.p.SendSync(ctx, msg)
	if err != nil {
		return fmt.Errorf("rocketmq publish delayed %s:%s (level=%d): %w", topic, tag, delayLevel, err)
	}

	r.log.DebugContext(ctx, "rocketmq delayed published",
		"topic", topic, "tag", tag, "delay_level", delayLevel, "msg_id", res.MsgID)
	return nil
}

// resolveAddr 将 host:port 中的主机名解析为 IP。
// RocketMQ Go SDK 的 verifyIP 只接受 IP 地址，不接受主机名。
func resolveAddr(addr string) (string, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", fmt.Errorf("split host port: %w", err)
	}
	// 已经是 IP 则直接返回
	if net.ParseIP(host) != nil {
		return addr, nil
	}
	// DNS 解析
	ips, err := net.LookupHost(host)
	if err != nil {
		return "", fmt.Errorf("lookup %s: %w", host, err)
	}
	if len(ips) == 0 {
		return "", fmt.Errorf("no IP found for %s", host)
	}
	resolved := net.JoinHostPort(ips[0], port)
	// 只在替换时打印，避免每次调用都打日志
	slog.Info("rocketmq namesrv resolved", "host", addr, "ip", resolved)
	return resolved, nil
}

// Close 关闭生产者。
func (r *RocketMQProducer) Close() error {
	if err := r.p.Shutdown(); err != nil {
		return fmt.Errorf("shutdown rocketmq producer: %w", err)
	}
	r.log.Info("rocketmq producer closed")
	return nil
}
