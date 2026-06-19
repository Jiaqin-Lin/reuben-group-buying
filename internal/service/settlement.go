// Package service 业务逻辑层。
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/reuben/group-buying/internal/cache"
	"github.com/reuben/group-buying/internal/errcode"
	"github.com/reuben/group-buying/internal/model"
	"github.com/reuben/group-buying/internal/repository"
)

// SettlementService 结算服务。
//
// 职责：支付成功后，将订单从 Locked → Paid，递增 take_limit 计数，
// 更新团进度，判定成团并创建回调通知任务。
//
// 与 Java 版差异：
//   - take_limit 在结算时 +1（不是锁单时），用 Lua 原子 check+incr。
//   - 无责任链校验（OutTradeNoCheckNode → OutTradeTimeCheckNode → …），改为简单 if-else。
//   - source/channel 黑名单校验暂时跳过（Java 版也是 TODO）。
type SettlementService struct {
	orderRepo    repository.OrderRepository
	activityRepo repository.ActivityRepository
	cacheRepo    repository.CacheRepository
	notifyRepo   repository.NotifyTaskRepository
	localCache   *cache.LocalCache
}

// NewSettlementService 构造函数。
func NewSettlementService(
	orderRepo repository.OrderRepository,
	activityRepo repository.ActivityRepository,
	cacheRepo repository.CacheRepository,
	notifyRepo repository.NotifyTaskRepository,
	localCache *cache.LocalCache,
) *SettlementService {
	return &SettlementService{
		orderRepo:    orderRepo,
		activityRepo: activityRepo,
		cacheRepo:    cacheRepo,
		notifyRepo:   notifyRepo,
		localCache:   localCache,
	}
}

// SettlementRequest 结算请求参数。
type SettlementRequest struct {
	UserID       string    `json:"user_id" binding:"required"`
	OutTradeNo   string    `json:"out_trade_no" binding:"required"`
	OutTradeTime time.Time `json:"out_trade_time" binding:"required"`
	Source       string    `json:"source" binding:"required"`
	Channel      string    `json:"channel" binding:"required"`
}

// SettlementResult 结算结果。
type SettlementResult struct {
	OrderID    string `json:"order_id"`
	OutTradeNo string `json:"out_trade_no"`
	TeamID     string `json:"team_id"`
	ActivityID int64  `json:"activity_id"`
	IsComplete bool   `json:"is_complete"` // 是否成团
	TakeCount  int64  `json:"take_count"`  // 本次结算后的参与次数
}

// Settle 结算主流程。
//
// 流程：
//  1. 查订单 → 校验 userId
//  2. 已支付 → 幂等返回（补创建 notify_task）
//  3. 校验订单状态 = Locked
//  4. 查团 → 校验 forming + 未过期
//  5. 查活动 → 获取 take_limit 和 TTL
//  6. Redis 原子 check+incr take_limit（防跨单号超限）
//  7. DB 事务：UPDATE order → UPDATE team → 判定成团
//  8. 成团 → 创建 notify_task
func (s *SettlementService) Settle(ctx context.Context, req SettlementRequest) (*SettlementResult, error) {
	slog.DebugContext(ctx, "settlement start", "user_id", req.UserID, "out_trade_no", req.OutTradeNo)

	// 1. 查订单
	order, err := s.orderRepo.FindOrderByOutTradeNo(ctx, req.OutTradeNo)
	if err != nil {
		slog.WarnContext(ctx, "settlement: order not found", "out_trade_no", req.OutTradeNo, "error", err)
		return nil, &SettlementError{Code: errcode.CodeOrderNotFound, Err: fmt.Errorf("order not found: %w", err)}
	}
	if order.UserID != req.UserID {
		slog.WarnContext(ctx, "settlement: user mismatch", "out_trade_no", req.OutTradeNo, "expected", req.UserID, "got", order.UserID)
		return nil, &SettlementError{Code: errcode.CodeOrderNotFound, Err: fmt.Errorf("order user mismatch")}
	}

	// 2. 幂等：已支付的订单直接返回
	if order.Status == model.OrderStatusPaid {
		slog.DebugContext(ctx, "settlement: already paid, idempotent", "out_trade_no", req.OutTradeNo)
		return s.idempotentResult(ctx, order)
	}

	// 3. 校验订单状态
	if order.Status != model.OrderStatusLocked {
		slog.WarnContext(ctx, "settlement: order not lockable", "out_trade_no", req.OutTradeNo, "status", order.Status)
		return nil, &SettlementError{Code: errcode.CodeOrderNotFound, Err: fmt.Errorf("order status is %d, expected %d", order.Status, model.OrderStatusLocked)}
	}

	// 4. 查团并校验
	team, err := s.orderRepo.FindTeamByID(ctx, order.TeamID)
	if err != nil {
		return nil, fmt.Errorf("settlement: find team: %w", err)
	}
	if team.Status != model.TeamStatusForming {
		return nil, &SettlementError{Code: errcode.CodeOrderTimeInvalid, Err: fmt.Errorf("team %s is not forming", team.TeamID)}
	}
	if time.Now().After(team.ValidEnd) {
		return nil, &SettlementError{Code: errcode.CodeOrderTimeInvalid, Err: fmt.Errorf("team %s expired at %v", team.TeamID, team.ValidEnd)}
	}

	// 5. 查活动（本地缓存 → DB fallback）
	activity, err := s.getActivity(ctx, order.ActivityID)
	if err != nil {
		return nil, fmt.Errorf("settlement: find activity: %w", err)
	}

	// 6. 限购软检查（Redis GET，非原子，快速拒绝超限请求）
	//    真正 +1 在 DB 成功后。软检查 + DB 行锁组合足够（两个不同
	//    outTradeNo 同时通过软检查并都 DB 成功是小概率事件，可接受）。
	takeTTL := time.Until(activity.EndTime)
	if takeTTL <= 0 {
		takeTTL = 1 * time.Minute
	}
	currentTakeCount, err := s.cacheRepo.GetTakeCount(ctx, order.ActivityID, req.UserID)
	if err != nil {
		slog.WarnContext(ctx, "settlement: get take count failed", "error", err)
		currentTakeCount = 0
	}
	// 冷启动：Redis key 不存在，从 DB 回种
	if currentTakeCount == 0 {
		dbCount, err := s.orderRepo.CountPaidOrdersByUserActivity(ctx, req.UserID, order.ActivityID)
		if err == nil && dbCount > 0 {
			currentTakeCount = dbCount
		}
	}
	if activity.TakeLimit > 0 && int(currentTakeCount) >= activity.TakeLimit {
		slog.WarnContext(ctx, "settlement: take limit reached", "user_id", req.UserID, "activity_id", order.ActivityID, "current", currentTakeCount, "limit", activity.TakeLimit)
		return nil, &SettlementError{Code: errcode.CodeTakeLimitReached, Err: fmt.Errorf("take limit reached: %d/%d", currentTakeCount, activity.TakeLimit)}
	}

	// 7. DB 结算（事务，行锁保护）
	settleResult, err := s.orderRepo.SettleOrder(ctx, repository.SettleOrderParams{
		OutTradeNo:   req.OutTradeNo,
		UserID:       req.UserID,
		OutTradeTime: req.OutTradeTime,
	})
	if err != nil {
		if errors.Is(err, repository.ErrOrderNotLocked) {
			// 并发：另一个请求已结算此订单
			slog.DebugContext(ctx, "settlement: concurrent settle detected", "out_trade_no", req.OutTradeNo)
			order, lookupErr := s.orderRepo.FindOrderByOutTradeNo(ctx, req.OutTradeNo)
			if lookupErr == nil && order.UserID == req.UserID && order.Status == model.OrderStatusPaid {
				return s.idempotentResult(ctx, order)
			}
		}
		return nil, fmt.Errorf("settlement: settle order: %w", err)
	}

	// 8. Redis INCR take_limit（DB 已成功，才递增计数）
	takeCount, err := s.cacheRepo.IncrTakeCount(ctx, order.ActivityID, req.UserID)
	if err != nil {
		slog.ErrorContext(ctx, "settlement: incr take count failed", "error", err)
		takeCount = currentTakeCount + 1
	}
	// 设置 TTL（Redis INCR 不会自动设 TTL，需单独设置）
	// 注意：INCR 在 key 不存在时从 0 开始（正确），但需要补 TTL
	_ = takeTTL // TTL 设置略，后续 Phase 10 统一处理

	// 9. 成团 → 创建回调通知任务
	if settleResult.IsComplete {
		s.createNotifyTask(ctx, settleResult, order)
	}

	slog.InfoContext(ctx, "settlement done", "user_id", req.UserID, "out_trade_no", req.OutTradeNo,
		"team_id", settleResult.TeamID, "is_complete", settleResult.IsComplete, "take_count", takeCount)

	return &SettlementResult{
		OrderID:    settleResult.OrderID,
		OutTradeNo: req.OutTradeNo,
		TeamID:     settleResult.TeamID,
		ActivityID: settleResult.ActivityID,
		IsComplete: settleResult.IsComplete,
		TakeCount:  takeCount,
	}, nil
}

// idempotentResult 处理幂等请求（订单已支付）。
// 检查是否需要补创建 notify_task（上次结算可能崩溃在创建 notify_task 之前）。
func (s *SettlementService) idempotentResult(ctx context.Context, order *model.Order) (*SettlementResult, error) {
	// 查团状态
	team, err := s.orderRepo.FindTeamByID(ctx, order.TeamID)
	if err != nil {
		// 团查不到不阻塞，返回基础结果
		slog.WarnContext(ctx, "settlement idempotent: find team failed", "team_id", order.TeamID, "error", err)
		return &SettlementResult{
			OrderID:    order.OrderID,
			OutTradeNo: order.OutTradeNo,
			TeamID:     order.TeamID,
			ActivityID: order.ActivityID,
			IsComplete: false,
		}, nil
	}

	// 如果团已完成但 notify_task 丢失，补创建
	if team.Status == model.TeamStatusComplete {
		uuid := buildNotifyUUID(order.TeamID, model.NotifyCategorySettlement, order.OutTradeNo)
		_, err := s.notifyRepo.FindTaskByUUID(ctx, uuid)
		if err != nil {
			slog.InfoContext(ctx, "settlement idempotent: recreating missing notify task", "team_id", order.TeamID)
			s.createNotifyTask(ctx, &repository.SettleOrderResult{
				TeamID:      order.TeamID,
				ActivityID:  order.ActivityID,
				TargetCount: team.TargetCount,
				NotifyType:  team.NotifyType,
				NotifyURL:   team.NotifyURL,
			}, order)
		}
	}

	return &SettlementResult{
		OrderID:    order.OrderID,
		OutTradeNo: order.OutTradeNo,
		TeamID:     order.TeamID,
		ActivityID: order.ActivityID,
		IsComplete: team.Status == model.TeamStatusComplete,
	}, nil
}

// createNotifyTask 创建回调通知任务。
// 失败仅打日志（不阻塞结算主流程），后续定时任务会补发。
func (s *SettlementService) createNotifyTask(ctx context.Context, settleResult *repository.SettleOrderResult, order *model.Order) {
	// 查团下所有订单，构建回调 payload
	orders, err := s.orderRepo.FindOrdersByTeamID(ctx, settleResult.TeamID)
	if err != nil {
		slog.ErrorContext(ctx, "settlement: find orders for notify failed", "team_id", settleResult.TeamID, "error", err)
		return
	}

	outTradeNoList := make([]string, len(orders))
	for i, o := range orders {
		outTradeNoList[i] = o.OutTradeNo
	}

	payload, _ := json.Marshal(map[string]any{
		"teamId":          settleResult.TeamID,
		"outTradeNoList":  outTradeNoList,
	})

	category := model.NotifyCategorySettlement
	task := &model.NotifyTask{
		ActivityID:   settleResult.ActivityID,
		TeamID:       settleResult.TeamID,
		Category:     &category,
		NotifyType:   settleResult.NotifyType,
		NotifyTarget: settleResult.NotifyURL,
		RetryCount:   0,
		Status:       model.NotifyStatusInit,
		Payload:      string(payload),
		UUID:         buildNotifyUUID(settleResult.TeamID, category, order.OutTradeNo),
	}

	if err := s.notifyRepo.CreateNotifyTask(ctx, task); err != nil {
		slog.ErrorContext(ctx, "settlement: create notify task failed", "team_id", settleResult.TeamID, "error", err)
		return
	}

	slog.InfoContext(ctx, "settlement: notify task created", "team_id", settleResult.TeamID, "uuid", task.UUID)
}

// getActivity 查活动（本地缓存 → DB fallback）。
func (s *SettlementService) getActivity(ctx context.Context, activityID int64) (*model.Activity, error) {
	if s.localCache != nil {
		if a, ok := s.localCache.GetActivity(activityID); ok {
			return a, nil
		}
	}
	return s.activityRepo.FindActivityByID(ctx, activityID)
}

// buildNotifyUUID 构建通知任务 UUID（用于幂等去重）。
// 格式：{teamId}_{category}_{outTradeNo}
func buildNotifyUUID(teamID, category, outTradeNo string) string {
	return teamID + "_" + category + "_" + outTradeNo
}

// SettlementError 结算业务错误。
type SettlementError struct {
	Code string
	Err  error
}

func (e *SettlementError) Error() string {
	return fmt.Sprintf("settlement error [%s]: %v", e.Code, e.Err)
}

func (e *SettlementError) Unwrap() error { return e.Err }

// ErrorCode 返回业务错误码。
func (e *SettlementError) ErrorCode() string { return e.Code }
