package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"strings"
	"syscall"

	"gorm.io/gorm"

	"github.com/reuben/group-buying/internal/errcode"
	"github.com/reuben/group-buying/internal/model"
	"github.com/reuben/group-buying/internal/pay"
	"github.com/reuben/group-buying/internal/repository"
)

// RefundService 退单服务。
//
// 职责：将订单从 Locked/Paid → Refunded，清理团计数和 Redis 名额，
// 创建回调通知任务。
//
// 三种场景：
//   - 未支付退（order=Locked, team=Forming）：退名额 + lock_count-1
//   - 已支付未成团退（order=Paid, team=Forming）：退名额 + lock_count-1 + complete_count-1
//   - 已成团退（order=Paid, team=Complete/CompleteRefunded）：不退名额，团可能 →CompleteRefunded 或 →Failed
//
// 与 Java 版差异：
//   - 不用责任链+策略模式，改为一个函数 + if-else 分发
//   - Redis 名额释放同步完成（不通过 MQ 异步）
//   - 不用分布式锁，靠 DB WHERE 条件防并发
type RefundService struct {
	orderRepo   repository.OrderRepository
	paymentRepo repository.PaymentRepository
	cacheRepo   repository.CacheRepository
	notifyRepo  repository.NotifyTaskRepository
	payGateway  pay.Gateway
}

// NewRefundService 构造函数。
func NewRefundService(
	orderRepo repository.OrderRepository,
	paymentRepo repository.PaymentRepository,
	cacheRepo repository.CacheRepository,
	notifyRepo repository.NotifyTaskRepository,
	payGateway pay.Gateway,
) *RefundService {
	return &RefundService{
		orderRepo:   orderRepo,
		paymentRepo: paymentRepo,
		cacheRepo:   cacheRepo,
		notifyRepo:  notifyRepo,
		payGateway:  payGateway,
	}
}

// RefundRequest 退单请求参数。
type RefundRequest struct {
	UserID     string `json:"user_id" binding:"required"`
	OutTradeNo string `json:"out_trade_no" binding:"required"`
}

// RefundResult 退单结果。
type RefundResult struct {
	OrderID    string `json:"order_id"`
	OutTradeNo string `json:"out_trade_no"`
	TeamID     string `json:"team_id"`
	ActivityID int64  `json:"activity_id"`
	RefundType string `json:"refund_type"` // "unpaid" | "paid" | "paid_team"
	TeamStatus int8   `json:"team_status"` // 退单后的团状态
}

// Refund 退单主流程。
//
// 流程：
//  1. 查订单 → 校验 userId
//  2. 幂等：status=Refunded → 直接返回
//  3. 查团
//  4. 按 (order.Status, team.Status) 分发到具体场景
func (s *RefundService) Refund(ctx context.Context, req RefundRequest) (*RefundResult, error) {
	slog.DebugContext(ctx, "refund start", "user_id", req.UserID, "out_trade_no", req.OutTradeNo)

	// 1. 查订单
	order, err := s.orderRepo.FindOrderByOutTradeNo(ctx, req.OutTradeNo)
	if err != nil {
		slog.WarnContext(ctx, "refund: order not found", "out_trade_no", req.OutTradeNo, "error", err)
		return nil, &RefundError{Code: errcode.CodeOrderNotFound, Err: fmt.Errorf("order not found: %w", err)}
	}
	if order.UserID != req.UserID {
		slog.WarnContext(ctx, "refund: user mismatch", "out_trade_no", req.OutTradeNo, "expected", req.UserID, "got", order.UserID)
		return nil, &RefundError{Code: errcode.CodeOrderNotFound, Err: fmt.Errorf("order user mismatch")}
	}

	// 2. 幂等：已退款的订单直接返回
	if order.Status == model.OrderStatusRefunded {
		slog.DebugContext(ctx, "refund: already refunded, idempotent", "out_trade_no", req.OutTradeNo)
		return s.idempotentResult(ctx, order)
	}

	// 2b. 直接购买订单：不关联团，无需团队操作
	if order.TeamID == "" {
		return s.directRefund(ctx, order)
	}

	// 3. 查团
	team, err := s.orderRepo.FindTeamByID(ctx, order.TeamID)
	if err != nil {
		return nil, fmt.Errorf("refund: find team: %w", err)
	}

	// 4. 按 (order.Status, team.Status) 分发
	switch {
	case order.Status == model.OrderStatusLocked && team.Status == model.TeamStatusForming:
		return s.unpaidRefund(ctx, order)

	case order.Status == model.OrderStatusLocked && team.Status == model.TeamStatusFailed:
		// 团已失败（如超时），订单仍锁定 → 取消订单释放名额
		return s.unpaidRefundFailedTeam(ctx, order)

	case order.Status == model.OrderStatusPaid && team.Status == model.TeamStatusForming:
		return s.paidRefund(ctx, order)

	case order.Status == model.OrderStatusPaid && team.Status == model.TeamStatusFailed:
		// 团已失败但订单已支付 → 退款（需调支付网关），不更新团计数器
		return s.paidRefundFailedTeam(ctx, order)

	case order.Status == model.OrderStatusPaid &&
		(team.Status == model.TeamStatusComplete || team.Status == model.TeamStatusCompleteRefunded):
		return s.paidTeamRefund(ctx, order)

	default:
		slog.WarnContext(ctx, "refund: invalid state", "out_trade_no", req.OutTradeNo,
			"order_status", order.Status, "team_status", team.Status)
		return nil, &RefundError{
			Code: errcode.CodeRefundStateInvalid,
			Err:  fmt.Errorf("invalid state for refund: order=%d team=%d", order.Status, team.Status),
		}
	}
}

// unpaidRefund 未支付退单（order=Locked, team=Forming）。
//
// 操作：
//   - 订单 Locked → Refunded
//   - 团 lock_count - 1
//   - 关闭待支付 payment（最佳努力）
//   - 创建 notify_task (category=trade_unpaid_refund)
//   - 释放 Redis 名额
func (s *RefundService) unpaidRefund(ctx context.Context, order *model.Order) (*RefundResult, error) {
	slog.DebugContext(ctx, "refund: unpaid", "order_id", order.OrderID, "team_id", order.TeamID)

	// 1. 更新订单状态（条件更新，防并发）
	if err := s.orderRepo.UpdateOrderStatusWithCheck(ctx, order.OrderID, model.OrderStatusLocked, model.OrderStatusRefunded); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return s.handleConcurrentRefund(ctx, order)
		}
		return nil, fmt.Errorf("refund unpaid: update order: %w", err)
	}

	// 2. 团 lock_count - 1（带 status=0 检查）
	if err := s.orderRepo.RefundTeamForming(ctx, order.TeamID, -1, 0); err != nil {
		slog.ErrorContext(ctx, "refund unpaid: update team counters failed", "team_id", order.TeamID, "error", err)
		return nil, fmt.Errorf("refund unpaid: update team counters: %w", err)
	}

	// 3. 关闭待支付 payment（最佳努力，payment 可能不存在）
	s.closePayment(ctx, order.OrderID)

	// 4. 创建 notify_task
	s.createRefundNotifyTask(ctx, order, model.NotifyCategoryUnpaidRefund)

	// 5. 释放 Redis 名额
	s.releaseStock(ctx, order.ActivityID, order.TeamID, order.OutTradeNo)

	// 6. 递减活跃订单计数（退款不占 take_limit 名额）
	s.decrActive(ctx, order.ActivityID, order.UserID)

	slog.InfoContext(ctx, "refund unpaid done", "order_id", order.OrderID, "team_id", order.TeamID)

	return &RefundResult{
		OrderID:    order.OrderID,
		OutTradeNo: order.OutTradeNo,
		TeamID:     order.TeamID,
		ActivityID: order.ActivityID,
		RefundType: "unpaid",
		TeamStatus: model.TeamStatusForming,
	}, nil
}

// paidRefund 已支付未成团退单（order=Paid, team=Forming）。
//
// 操作：
//   - 先调支付网关退款（外部副作用优先，网关幂等，DB 失败可重试）
//   - 订单 Paid → Refunded
//   - 团 lock_count - 1, complete_count - 1
//   - 创建 notify_task (category=trade_paid_refund)
//   - 释放 Redis 名额
func (s *RefundService) paidRefund(ctx context.Context, order *model.Order) (*RefundResult, error) {
	slog.DebugContext(ctx, "refund: paid", "order_id", order.OrderID, "team_id", order.TeamID)

	// 0. 调用支付网关退款（Mock 支付跳过，支付宝没有这笔交易）
	if s.payGateway != nil && !s.isMockPayment(ctx, order.OrderID) {
		if _, err := s.payGateway.Refund(ctx, order.OrderID, order.PayPrice); err != nil {
			slog.ErrorContext(ctx, "refund paid: gateway refund failed", "order_id", order.OrderID, "error", err)
			return nil, &RefundError{Code: refundErrCode(err), Err: fmt.Errorf("gateway refund: %w", err)}
		}
	}

	// 1. 更新 DB 订单状态（Paid → Refunded）
	if err := s.orderRepo.UpdateOrderStatusWithCheck(ctx, order.OrderID, model.OrderStatusPaid, model.OrderStatusRefunded); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return s.handleConcurrentRefund(ctx, order)
		}
		slog.ErrorContext(ctx, "refund paid: DB update failed after gateway refund",
			"order_id", order.OrderID, "error", err)
		return nil, fmt.Errorf("refund paid: update order: %w", err)
	}

	// 2. 团 lock_count - 1, complete_count - 1
	if err := s.orderRepo.RefundTeamForming(ctx, order.TeamID, -1, -1); err != nil {
		slog.ErrorContext(ctx, "refund paid: update team counters failed", "team_id", order.TeamID, "error", err)
		return nil, fmt.Errorf("refund paid: update team counters: %w", err)
	}

	// 3. 创建 notify_task
	s.createRefundNotifyTask(ctx, order, model.NotifyCategoryPaidRefund)

	// 4. 释放 Redis 名额
	s.releaseStock(ctx, order.ActivityID, order.TeamID, order.OutTradeNo)

	// 5. 递减 take_limit 计数（退款不占参与次数）
	s.decrTakeCount(ctx, order.ActivityID, order.UserID)

	// 6. 递减活跃订单计数（退款不占 take_limit 名额）
	s.decrActive(ctx, order.ActivityID, order.UserID)

	slog.InfoContext(ctx, "refund paid done", "order_id", order.OrderID, "team_id", order.TeamID)

	return &RefundResult{
		OrderID:    order.OrderID,
		OutTradeNo: order.OutTradeNo,
		TeamID:     order.TeamID,
		ActivityID: order.ActivityID,
		RefundType: "paid",
		TeamStatus: model.TeamStatusForming,
	}, nil
}

// paidTeamRefund 已成团退单（order=Paid, team=Complete 或 CompleteRefunded）。
//
// 操作：
//   - 订单 Paid → Refunded
//   - 团 lock_count - 1, complete_count - 1
//   - completeCount>1 → team status=3 (CompleteRefunded)
//   - completeCount=1 → team status=2 (Failed)
//   - 创建 notify_task (category=trade_paid_team_refund)
//   - 不释放名额（已成团，名额已消耗）
func (s *RefundService) paidTeamRefund(ctx context.Context, order *model.Order) (*RefundResult, error) {
	slog.DebugContext(ctx, "refund: paid team", "order_id", order.OrderID, "team_id", order.TeamID)

	// 0. 调用支付网关退款（Mock 支付跳过）
	if s.payGateway != nil && !s.isMockPayment(ctx, order.OrderID) {
		if _, err := s.payGateway.Refund(ctx, order.OrderID, order.PayPrice); err != nil {
			slog.ErrorContext(ctx, "refund paid team: gateway refund failed", "order_id", order.OrderID, "error", err)
			return nil, &RefundError{Code: refundErrCode(err), Err: fmt.Errorf("gateway refund: %w", err)}
		}
	}

	// 1. 更新订单状态
	if err := s.orderRepo.UpdateOrderStatusWithCheck(ctx, order.OrderID, model.OrderStatusPaid, model.OrderStatusRefunded); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return s.handleConcurrentRefund(ctx, order)
		}
		return nil, fmt.Errorf("refund paid team: update order: %w", err)
	}

	// 2. 更新团计数器和状态（由 repo 方法根据 complete_count 决定新状态）
	newTeamStatus, err := s.orderRepo.RefundCompleteTeam(ctx, order.TeamID, -1, -1)
	if err != nil {
		slog.ErrorContext(ctx, "refund paid team: update team failed", "team_id", order.TeamID, "error", err)
		return nil, fmt.Errorf("refund paid team: update team: %w", err)
	}

	// 3. 创建 notify_task
	s.createRefundNotifyTask(ctx, order, model.NotifyCategoryPaidTeamRefund)

	// 4. 不释放名额（已成团）

	// 5. 递减 take_limit 计数（退款不占参与次数）
	s.decrTakeCount(ctx, order.ActivityID, order.UserID)

	// 6. 递减活跃订单计数（退款不占 take_limit 名额）
	s.decrActive(ctx, order.ActivityID, order.UserID)

	slog.InfoContext(ctx, "refund paid team done", "order_id", order.OrderID, "team_id", order.TeamID, "new_team_status", newTeamStatus)

	return &RefundResult{
		OrderID:    order.OrderID,
		OutTradeNo: order.OutTradeNo,
		TeamID:     order.TeamID,
		ActivityID: order.ActivityID,
		RefundType: "paid_team",
		TeamStatus: newTeamStatus,
	}, nil
}

// unpaidRefundFailedTeam 团已失败时的未支付退单（order=Locked, team=Failed）。
//
// 场景：超时扫描已将团标记为 Failed，但订单仍处于 Locked 状态。
// 操作：
//   - 订单 Locked → Refunded
//   - 团计数器不再更新（团已失败）
//   - 关闭待支付 payment
//   - 释放 Redis 名额
func (s *RefundService) unpaidRefundFailedTeam(ctx context.Context, order *model.Order) (*RefundResult, error) {
	slog.DebugContext(ctx, "refund: unpaid failed team", "order_id", order.OrderID, "team_id", order.TeamID)

	// 1. 更新订单状态
	if err := s.orderRepo.UpdateOrderStatusWithCheck(ctx, order.OrderID, model.OrderStatusLocked, model.OrderStatusRefunded); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return s.handleConcurrentRefund(ctx, order)
		}
		return nil, fmt.Errorf("refund unpaid failed team: update order: %w", err)
	}

	// 2. 关闭待支付 payment
	s.closePayment(ctx, order.OrderID)

	// 3. 创建 notify_task
	s.createRefundNotifyTask(ctx, order, model.NotifyCategoryUnpaidRefund)

	// 4. 释放 Redis 名额
	s.releaseStock(ctx, order.ActivityID, order.TeamID, order.OutTradeNo)

	// 5. 递减活跃订单计数（退款不占 take_limit 名额）
	s.decrActive(ctx, order.ActivityID, order.UserID)

	slog.InfoContext(ctx, "refund unpaid failed team done", "order_id", order.OrderID, "team_id", order.TeamID)

	return &RefundResult{
		OrderID:    order.OrderID,
		OutTradeNo: order.OutTradeNo,
		TeamID:     order.TeamID,
		ActivityID: order.ActivityID,
		RefundType: "unpaid",
		TeamStatus: model.TeamStatusFailed,
	}, nil
}

// paidRefundFailedTeam 团已失败时的已支付退单（order=Paid, team=Failed）。
//
// 场景：团已失败（如超时），订单已支付但尚未被处理。
// 操作：
//   - 调用支付网关退款
//   - 订单 Paid → Refunded
//   - 团计数器不再更新（团已失败）
//   - 释放 Redis 名额
func (s *RefundService) paidRefundFailedTeam(ctx context.Context, order *model.Order) (*RefundResult, error) {
	slog.DebugContext(ctx, "refund: paid failed team", "order_id", order.OrderID, "team_id", order.TeamID)

	// 0. 调用支付网关退款（Mock 支付跳过）
	if s.payGateway != nil && !s.isMockPayment(ctx, order.OrderID) {
		if _, err := s.payGateway.Refund(ctx, order.OrderID, order.PayPrice); err != nil {
			slog.ErrorContext(ctx, "refund paid failed team: gateway refund failed", "order_id", order.OrderID, "error", err)
			return nil, &RefundError{Code: refundErrCode(err), Err: fmt.Errorf("gateway refund: %w", err)}
		}
	}

	// 1. 更新订单状态
	if err := s.orderRepo.UpdateOrderStatusWithCheck(ctx, order.OrderID, model.OrderStatusPaid, model.OrderStatusRefunded); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return s.handleConcurrentRefund(ctx, order)
		}
		return nil, fmt.Errorf("refund paid failed team: update order: %w", err)
	}

	// 2. 创建 notify_task
	s.createRefundNotifyTask(ctx, order, model.NotifyCategoryPaidRefund)

	// 3. 释放 Redis 名额（团已失败，名额应该还回来）
	s.releaseStock(ctx, order.ActivityID, order.TeamID, order.OutTradeNo)

	// 4. 递减 take_limit 计数（退款不占参与次数）
	s.decrTakeCount(ctx, order.ActivityID, order.UserID)

	// 5. 递减活跃订单计数（退款不占 take_limit 名额）
	s.decrActive(ctx, order.ActivityID, order.UserID)

	slog.InfoContext(ctx, "refund paid failed team done", "order_id", order.OrderID, "team_id", order.TeamID)

	return &RefundResult{
		OrderID:    order.OrderID,
		OutTradeNo: order.OutTradeNo,
		TeamID:     order.TeamID,
		ActivityID: order.ActivityID,
		RefundType: "paid",
		TeamStatus: model.TeamStatusFailed,
	}, nil
}

// handleConcurrentRefund 处理并发退单。
// 订单状态更新失败（RowsAffected=0）时，重新查询订单状态：
//   - 已退款 → 幂等返回
//   - 其他状态 → 返回错误（被别的操作改动了）
// directRefund 直接购买退单——不关联团，跳过团队操作。
func (s *RefundService) directRefund(ctx context.Context, order *model.Order) (*RefundResult, error) {
	slog.DebugContext(ctx, "refund: direct buy", "order_id", order.OrderID)

	// 已支付的需要调支付网关退款（Mock 支付跳过）
	if order.Status == model.OrderStatusPaid && s.payGateway != nil && !s.isMockPayment(ctx, order.OrderID) {
		if _, err := s.payGateway.Refund(ctx, order.OrderID, order.PayPrice); err != nil {
			slog.ErrorContext(ctx, "refund direct: gateway refund failed", "order_id", order.OrderID, "error", err)
			return nil, &RefundError{Code: refundErrCode(err), Err: fmt.Errorf("gateway refund: %w", err)}
		}
	}

	// 更新订单状态
	fromStatus := order.Status
	if err := s.orderRepo.UpdateOrderStatusWithCheck(ctx, order.OrderID, fromStatus, model.OrderStatusRefunded); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return s.handleConcurrentRefund(ctx, order)
		}
		return nil, fmt.Errorf("refund direct: update order: %w", err)
	}

	// 未支付的关闭 payment
	if fromStatus == model.OrderStatusLocked {
		s.closePayment(ctx, order.OrderID)
	}

	slog.InfoContext(ctx, "refund direct done", "order_id", order.OrderID)
	return &RefundResult{
		OrderID:    order.OrderID,
		OutTradeNo: order.OutTradeNo,
		TeamID:     "",
		ActivityID: 0,
		RefundType: "direct",
		TeamStatus: 0,
	}, nil
}
func (s *RefundService) handleConcurrentRefund(ctx context.Context, order *model.Order) (*RefundResult, error) {
	fresh, lookupErr := s.orderRepo.FindOrderByOutTradeNo(ctx, order.OutTradeNo)
	if lookupErr != nil {
		return nil, &RefundError{Code: errcode.CodeUnknownErr, Err: fmt.Errorf("re-query after concurrent update: %w", lookupErr)}
	}
	if fresh.Status == model.OrderStatusRefunded {
		// 并发退单，另一请求已成功
		slog.DebugContext(ctx, "refund: concurrent refund detected, idempotent", "out_trade_no", order.OutTradeNo)
		return s.idempotentResult(ctx, fresh)
	}
	return nil, &RefundError{
		Code: errcode.CodeRefundStateInvalid,
		Err:  fmt.Errorf("concurrent state change: expected refunded, got status=%d", fresh.Status),
	}
}

// idempotentResult 构建幂等返回结果。
func (s *RefundService) idempotentResult(ctx context.Context, order *model.Order) (*RefundResult, error) {
	team, err := s.orderRepo.FindTeamByID(ctx, order.TeamID)
	if err != nil {
		slog.WarnContext(ctx, "refund idempotent: find team failed", "team_id", order.TeamID, "error", err)
		return &RefundResult{
			OrderID:    order.OrderID,
			OutTradeNo: order.OutTradeNo,
			TeamID:     order.TeamID,
			ActivityID: order.ActivityID,
			RefundType: "idempotent",
		}, nil
	}

	return &RefundResult{
		OrderID:    order.OrderID,
		OutTradeNo: order.OutTradeNo,
		TeamID:     order.TeamID,
		ActivityID: order.ActivityID,
		RefundType: "idempotent",
		TeamStatus: team.Status,
	}, nil
}

// closePayment 关闭待支付 payment（最佳努力）。
// 支付不存在或已支付/已关闭则静默跳过。
func (s *RefundService) closePayment(ctx context.Context, orderID string) {
	if err := s.paymentRepo.UpdatePaymentClosed(ctx, orderID); err != nil {
		// 支付可能不存在（锁单时未创建 payment），或已处于非 pending 状态
		slog.DebugContext(ctx, "refund: close payment skipped", "order_id", orderID, "reason", err)
	}
}

// isMockPayment 检查是否为 Mock 支付（TradeNo 以 MOCK_ 开头）。
// Mock 支付的退款不调真实网关（支付宝没有这笔交易），直接跳过。
func (s *RefundService) isMockPayment(ctx context.Context, orderID string) bool {
	pm, err := s.paymentRepo.FindPaymentByOrderID(ctx, orderID)
	if err != nil {
		// 查不到 payment 也跳过网关退款（安全侧：宁可少退网关，不卡死退款流程）
		slog.WarnContext(ctx, "refund: find payment for mock check failed, skip gateway", "order_id", orderID, "error", err)
		return true
	}
	if pm.TradeNo != nil && strings.HasPrefix(*pm.TradeNo, "MOCK_") {
		return true
	}
	return false
}

// decrTakeCount 退款后递减 take_limit 计数（最佳努力）。
// 退款订单不应占用用户的参与次数。Lua 保证不会减到负数。
// 失败不影响退款主流程。
func (s *RefundService) decrTakeCount(ctx context.Context, activityID int64, userID string) {
	if err := s.cacheRepo.DecrTakeCount(ctx, activityID, userID); err != nil {
		slog.WarnContext(ctx, "refund: decr take count failed", "activity_id", activityID, "user_id", userID, "error", err)
	}
}

// releaseStock 释放 Redis 名额（最佳努力）。
// ReleaseStock 操作是幂等的（SREM + DEL full），失败不影响退款主流程。
func (s *RefundService) releaseStock(ctx context.Context, activityID int64, teamID, outTradeNo string) {
	if err := s.cacheRepo.ReleaseStock(ctx, activityID, teamID, outTradeNo); err != nil {
		slog.WarnContext(ctx, "refund: release stock failed", "activity_id", activityID, "team_id", teamID, "error", err)
	}
}

// decrActive 退单后递减活跃订单计数（最佳努力）。
// 失败不影响退款主流程，下次锁单会从 DB 重建。
func (s *RefundService) decrActive(ctx context.Context, activityID int64, userID string) {
	if err := s.cacheRepo.DecrActiveCount(ctx, activityID, userID); err != nil {
		slog.WarnContext(ctx, "refund: decr active count failed", "activity_id", activityID, "user_id", userID, "error", err)
	}
}

// createRefundNotifyTask 创建退单回调通知任务。
// 失败仅打日志，不阻塞退单主流程。
func (s *RefundService) createRefundNotifyTask(ctx context.Context, order *model.Order, category string) {
	payload, _ := json.Marshal(map[string]any{
		"teamId":     order.TeamID,
		"outTradeNo": order.OutTradeNo,
		"orderId":    order.OrderID,
		"userId":     order.UserID,
		"activityId": order.ActivityID,
	})

	// 查团获取 notify 配置
	team, err := s.orderRepo.FindTeamByID(ctx, order.TeamID)
	if err != nil {
		slog.ErrorContext(ctx, "refund: find team for notify failed", "team_id", order.TeamID, "error", err)
		return
	}

	task := &model.NotifyTask{
		ActivityID:   order.ActivityID,
		TeamID:       order.TeamID,
		Category:     &category,
		NotifyType:   team.NotifyType,
		NotifyTarget: team.NotifyURL,
		RetryCount:   0,
		Status:       model.NotifyStatusInit,
		Payload:      string(payload),
		UUID:         buildNotifyUUID(order.TeamID, category, order.OutTradeNo),
	}

	if err := s.notifyRepo.CreateNotifyTask(ctx, task); err != nil {
		slog.ErrorContext(ctx, "refund: create notify task failed", "team_id", order.TeamID, "uuid", task.UUID, "error", err)
		return
	}

	slog.InfoContext(ctx, "refund: notify task created", "team_id", order.TeamID, "category", category, "uuid", task.UUID)
}

// RefundError 退单业务错误。
type RefundError struct {
	Code string
	Err  error
}

func (e *RefundError) Error() string {
	return fmt.Sprintf("refund error [%s]: %v", e.Code, e.Err)
}

func (e *RefundError) Unwrap() error { return e.Err }

// ErrorCode 返回业务错误码。
func (e *RefundError) ErrorCode() string { return e.Code }

// refundErrCode 根据网关错误类型返回合适的错误码。
// 超时/网络错误 → P0005（可重试），其他 → P0004（业务失败）。
func refundErrCode(err error) string {
	if isTimeoutErr(err) {
		return errcode.CodeRefundGatewayErr
	}
	return errcode.CodeRefundFailed
}

// isTimeoutErr 判断是否为超时或网络不可达错误。
func isTimeoutErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) { //nolint:errorsastype // stdlib errors.As pattern
		if urlErr.Timeout() {
			return true
		}
		if urlErr.Err != nil {
			return isTimeoutErr(urlErr.Err)
		}
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	if errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.ECONNRESET) {
		return true
	}
	if errors.Is(err, os.ErrDeadlineExceeded) {
		return true
	}
	return false
}
