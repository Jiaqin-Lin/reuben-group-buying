package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/reuben/group-buying/internal/redisx"
	"github.com/reuben/group-buying/internal/repository"
)

// TimeoutService 超时退单扫描器。
//
// 定时扫描 orders.status=Locked(0) 且 teams.valid_end < NOW() 的订单，
// 逐条触发退单。用分布式锁保证多实例部署时只有一个扫描器执行。
//
// 与 Phase 8 NotifyService 的模式完全一致：定时 cron → 分布式锁 → 游标分页 → 调用 service。
type TimeoutService struct {
	orderRepo repository.OrderRepository
	refundSvc *RefundService
	rdb       *redis.Client
	batchSize int
	logger    *slog.Logger
}

// TimeoutServiceConfig 超时扫描配置。
type TimeoutServiceConfig struct {
	BatchSize int // 每批扫描数量，默认 100
}

// NewTimeoutService 构造函数。
func NewTimeoutService(
	orderRepo repository.OrderRepository,
	refundSvc *RefundService,
	rdb *redis.Client,
	cfg TimeoutServiceConfig,
	logger *slog.Logger,
) *TimeoutService {
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 100
	}
	return &TimeoutService{
		orderRepo: orderRepo,
		refundSvc: refundSvc,
		rdb:       rdb,
		batchSize: cfg.BatchSize,
		logger:    logger,
	}
}

// ScanAndRefund 扫描超时订单并逐条退单。
//
// 流程：
//  1. 获取分布式锁 bgm:lock:timeout:scanner（60s TTL，防多实例）
//  2. 游标分页循环：FindTimeoutOrders(limit=batchSize, lastID)
//  3. 逐条调用 RefundService.Refund(ctx, {UserId, OutTradeNo})
//  4. 无更多结果时退出
//  5. defer 释放锁
//
// 返回统计：(扫描总数, 退单成功数, 退单失败数)。
func (s *TimeoutService) ScanAndRefund(ctx context.Context) (scanned, refunded, failed int, err error) {
	// 1. 获取分布式锁
	lockKey := redisx.TimeoutScanLockKey()
	locked, err := redisx.AcquireLockSimple(ctx, s.rdb, lockKey, lockTTL)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("timeout scan: acquire lock: %w", err)
	}
	if !locked {
		s.logger.DebugContext(ctx, "timeout scan: lock not acquired, another instance is scanning")
		return 0, 0, 0, nil
	}
	defer func() {
		if relErr := redisx.ReleaseLockSimple(ctx, s.rdb, lockKey); relErr != nil {
			s.logger.WarnContext(ctx, "timeout scan: release lock failed", "error", relErr)
		}
	}()

	s.logger.InfoContext(ctx, "timeout scan started", "batch_size", s.batchSize)

	// 2. 游标分页循环
	var lastID uint64
	for {
		orders, findErr := s.orderRepo.FindTimeoutOrders(ctx, s.batchSize, lastID)
		if findErr != nil {
			return scanned, refunded, failed, fmt.Errorf("timeout scan: find orders: %w", findErr)
		}
		if len(orders) == 0 {
			break
		}

		scanned += len(orders)

		// 3. 逐条退单
		for _, o := range orders {
			req := RefundRequest{
				UserID:     o.UserID,
				OutTradeNo: o.OutTradeNo,
			}
			if _, refErr := s.refundSvc.Refund(ctx, req); refErr != nil {
				s.logger.WarnContext(ctx, "timeout scan: refund failed",
					"out_trade_no", o.OutTradeNo,
					"order_id", o.OrderID,
					"team_id", o.TeamID,
					"error", refErr,
				)
				failed++
			} else {
				refunded++
			}

			// 更新游标
			if o.ID > lastID {
				lastID = o.ID
			}
		}

		// 更新游标到本批最后一条
		if len(orders) > 0 {
			lastID = orders[len(orders)-1].ID
		}

		// 本批不足 batchSize，说明已扫完
		if len(orders) < s.batchSize {
			break
		}
	}

	s.logger.InfoContext(ctx, "timeout scan completed",
		"scanned", scanned,
		"refunded", refunded,
		"failed", failed,
	)
	return scanned, refunded, failed, nil
}

var lockTTL = 60 * time.Second // 分布式锁 TTL，远大于一次扫描时间
