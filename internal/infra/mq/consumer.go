package mq

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/apache/rocketmq-client-go/v2"
	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
)

// TimeoutMessage 延迟消息载荷。
type TimeoutMessage struct {
	OutTradeNo string `json:"out_trade_no"`
	UserID     string `json:"user_id"`
}

// TimeoutHandler 超时消息处理回调。
// 返回 error 时消息会重试（Clustering 模式下由 RocketMQ 重新投递）。
type TimeoutHandler func(ctx context.Context, msg TimeoutMessage) error

// TimeoutConsumer 订阅支付超时延迟消息的消费者。
type TimeoutConsumer struct {
	c       rocketmq.PushConsumer
	log     *slog.Logger
	handler TimeoutHandler
}

// TimeoutConsumerConfig 消费者配置。
type TimeoutConsumerConfig struct {
	NameServer string
	GroupName  string
	Topic      string
	Tag        string
}

// NewTimeoutConsumer 创建超时消费者（需调用 Start 才会开始消费）。
func NewTimeoutConsumer(cfg TimeoutConsumerConfig, handler TimeoutHandler, log *slog.Logger) (*TimeoutConsumer, error) {
	addr, resolveErr := resolveAddr(cfg.NameServer)
	if resolveErr != nil {
		return nil, fmt.Errorf("create timeout consumer: resolve %s: %w", cfg.NameServer, resolveErr)
	}
	c, err := rocketmq.NewPushConsumer(
		consumer.WithGroupName(cfg.GroupName),
		consumer.WithNsResolver(primitive.NewPassthroughResolver([]string{addr})),
		consumer.WithConsumeFromWhere(consumer.ConsumeFromFirstOffset),
		consumer.WithConsumerModel(consumer.Clustering),
	)
	if err != nil {
		return nil, fmt.Errorf("create timeout consumer: %w", err)
	}

	tc := &TimeoutConsumer{c: c, log: log, handler: handler}

	selector := consumer.MessageSelector{
		Type:       consumer.TAG,
		Expression: cfg.Tag,
	}

	err = c.Subscribe(cfg.Topic, selector, tc.onMessage)
	if err != nil {
		return nil, fmt.Errorf("subscribe timeout consumer: %w", err)
	}

	return tc, nil
}

// Start 启动消费。
func (t *TimeoutConsumer) Start() error {
	if err := t.c.Start(); err != nil {
		return fmt.Errorf("start timeout consumer: %w", err)
	}
	t.log.Info("timeout consumer started")
	return nil
}

// Shutdown 优雅关闭消费者。
func (t *TimeoutConsumer) Shutdown() error {
	if err := t.c.Shutdown(); err != nil {
		return fmt.Errorf("shutdown timeout consumer: %w", err)
	}
	t.log.Info("timeout consumer closed")
	return nil
}

// onMessage RocketMQ 回调，解析消息并委托给 handler。
func (t *TimeoutConsumer) onMessage(ctx context.Context, msgs ...*primitive.MessageExt) (consumer.ConsumeResult, error) {
	for _, msg := range msgs {
		var tm TimeoutMessage
		if err := json.Unmarshal(msg.Body, &tm); err != nil {
			t.log.WarnContext(ctx, "timeout message unmarshal failed, skipping",
				"msg_id", msg.MsgId, "error", err)
			continue
		}

		if err := t.handler(ctx, tm); err != nil {
			t.log.WarnContext(ctx, "timeout handler failed, will retry",
				"out_trade_no", tm.OutTradeNo, "error", err)
			return consumer.ConsumeRetryLater, nil
		}
	}

	return consumer.ConsumeSuccess, nil
}
