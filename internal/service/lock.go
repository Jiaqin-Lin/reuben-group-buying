// Package service 业务逻辑层。
package service

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/big"
	"time"

	"github.com/reuben/group-buying/internal/errcode"
	"github.com/reuben/group-buying/internal/model"
	"github.com/reuben/group-buying/internal/redisx"
	"github.com/reuben/group-buying/internal/repository"
)

// LockService 锁单服务。
//
// 职责：校验参数 → 幂等检查 → 获取分布式锁 → 试算定价 → 限购检查 → 占名额 → 写库 → 缓存结果。
// 分两条路径：新建团（teamId 为空）和加入团（teamId 非空）。
//
// 与 Java 版差异：
//   - take_limit 只做软检查（查已支付订单数），真正 +1 在支付回调时。锁了不支付不该扣次数。
//   - 无责任链、无热活动优化，用简单 if-else 替代。
type LockService struct {
	trialSvc     *TrialService
	orderRepo    repository.OrderRepository
	activityRepo repository.ActivityRepository
	cacheRepo    repository.CacheRepository
	lockTTL      time.Duration
	resultTTL    time.Duration
}

// NewLockService 构造函数。
func NewLockService(
	trialSvc *TrialService,
	orderRepo repository.OrderRepository,
	activityRepo repository.ActivityRepository,
	cacheRepo repository.CacheRepository,
	lockTTL, resultTTL time.Duration,
) *LockService {
	return &LockService{
		trialSvc:     trialSvc,
		orderRepo:    orderRepo,
		activityRepo: activityRepo,
		cacheRepo:    cacheRepo,
		lockTTL:      lockTTL,
		resultTTL:    resultTTL,
	}
}

// LockRequest 锁单请求参数。
type LockRequest struct {
	UserID     string `json:"user_id" binding:"required"`
	ActivityID int64  `json:"activity_id" binding:"required"`
	GoodsID    string `json:"goods_id" binding:"required"`
	Source     string `json:"source" binding:"required"`
	Channel    string `json:"channel" binding:"required"`
	OutTradeNo string `json:"out_trade_no" binding:"required"`
	TeamID     string `json:"team_id"`    // 可选，非空表示加入已有团
	NotifyURL  string `json:"notify_url"` // 可选，成团回调地址
}

// LockResult 锁单结果。
type LockResult struct {
	OrderID        string `json:"order_id"`
	OutTradeNo     string `json:"out_trade_no"`
	TeamID         string `json:"team_id"`
	OriginalPrice  string `json:"original_price"`
	DeductionPrice string `json:"deduction_price"`
	PayPrice       string `json:"pay_price"`
	Status         int    `json:"status"` // 0=锁定待支付
}

// Lock 锁单主流程。
//
// 三层层幂等防护：
//  1. 缓存层（Redis，10min TTL）
//  2. 分布式锁层（Redis SETNX，3s TTL）
//  3. 数据库层（out_trade_no 唯一索引）
func (s *LockService) Lock(ctx context.Context, req LockRequest) (*LockResult, error) {
	slog.DebugContext(ctx, "lock start", "user_id", req.UserID, "out_trade_no", req.OutTradeNo, "team_id", req.TeamID)

	// 1. 幂等检查：缓存
	if cached, err := s.getCachedResult(ctx, req.UserID, req.OutTradeNo); err != nil {
		slog.WarnContext(ctx, "lock: cache lookup failed", "out_trade_no", req.OutTradeNo, "error", err)
	} else if cached != nil {
		slog.DebugContext(ctx, "lock: cache hit, return cached", "out_trade_no", req.OutTradeNo)
		return cached, nil
	}

	// 2. 幂等检查：数据库
	if existing, err := s.findExistingOrder(ctx, req.UserID, req.OutTradeNo); err != nil {
		slog.WarnContext(ctx, "lock: db lookup failed", "out_trade_no", req.OutTradeNo, "error", err)
	} else if existing != nil {
		s.cacheResult(ctx, req.UserID, req.OutTradeNo, existing)
		return existing, nil
	}

	// 3. 获取分布式锁
	lockKey := redisx.LockOrderKey(req.UserID, req.OutTradeNo)
	acquired, err := s.cacheRepo.AcquireLock(ctx, lockKey, s.lockTTL)
	if err != nil {
		return nil, fmt.Errorf("lock: acquire lock: %w", err)
	}
	if !acquired {
		return nil, &LockError{Code: errcode.CodeUnknownErr, Err: fmt.Errorf("system busy, please retry")}
	}
	defer func() {
		if err := s.cacheRepo.ReleaseLock(ctx, lockKey); err != nil {
			slog.WarnContext(ctx, "lock: release lock failed", "key", lockKey, "error", err)
		}
	}()

	// 4. 获取锁后再次检查缓存（防止并发窗口）
	if cached, err := s.getCachedResult(ctx, req.UserID, req.OutTradeNo); err != nil {
		slog.WarnContext(ctx, "lock: double-check cache failed", "error", err)
	} else if cached != nil {
		return cached, nil
	}

	// 5. 试算：定价 + 活动校验 + 人群标签
	trialReq := TrialRequest{
		UserID:  req.UserID,
		GoodsID: req.GoodsID,
		Source:  req.Source,
		Channel: req.Channel,
	}
	trialResult, err := s.trialSvc.Trial(ctx, trialReq)
	if err != nil {
		return nil, fmt.Errorf("lock: trial: %w", err)
	}
	if trialResult.ActivityID != req.ActivityID {
		return nil, &LockError{
			Code: errcode.CodeTrialFailed,
			Err:  fmt.Errorf("activity mismatch: expected %d, got %d", req.ActivityID, trialResult.ActivityID),
		}
	}
	if !trialResult.IsEnable {
		return nil, &LockError{Code: errcode.CodeCrowdBlocked, Err: fmt.Errorf("user not in required crowd for activity %d", req.ActivityID)}
	}

	// 6. 限购检查（只查已支付订单数，不计锁定的）
	activity, err := s.activityRepo.FindActivityByID(ctx, req.ActivityID)
	if err != nil {
		return nil, fmt.Errorf("lock: find activity: %w", err)
	}
	paidCount, err := s.orderRepo.CountPaidOrdersByUserActivity(ctx, req.UserID, req.ActivityID)
	if err != nil {
		return nil, fmt.Errorf("lock: check take_limit: %w", err)
	}
	if activity.TakeLimit > 0 && int(paidCount) >= activity.TakeLimit {
		return nil, &LockError{
			Code: errcode.CodeTakeLimitReached,
			Err:  fmt.Errorf("take limit reached: %d/%d", paidCount, activity.TakeLimit),
		}
	}

	// 7. 执行锁单（新建团 / 加入团）
	stockTTL := time.Duration(activity.ValidMinutes) * time.Minute
	var result *LockResult
	if req.TeamID == "" {
		result, err = s.lockNewTeam(ctx, req, trialResult, activity, stockTTL)
	} else {
		result, err = s.lockJoinTeam(ctx, req, trialResult, activity, stockTTL)
	}
	if err != nil {
		return nil, err
	}

	// 8. 缓存结果
	s.cacheResult(ctx, req.UserID, req.OutTradeNo, result)

	slog.DebugContext(ctx, "lock done", "user_id", req.UserID, "order_id", result.OrderID, "team_id", result.TeamID)
	return result, nil
}

// lockNewTeam 新建团 + 首单。
func (s *LockService) lockNewTeam(ctx context.Context, req LockRequest, trial *TrialResult, activity *model.Activity, stockTTL time.Duration) (*LockResult, error) {
	teamID := generateNumericID(8)
	orderID := generateNumericID(12)

	// Redis Lua 原子占名额
	ok, err := s.cacheRepo.TryOccupyStock(ctx, req.ActivityID, teamID, req.OutTradeNo, activity.TargetCount, stockTTL)
	if err != nil {
		return nil, fmt.Errorf("lock: occupy stock new team: %w", err)
	}
	if !ok {
		return nil, &LockError{Code: errcode.CodeStockInsufficient, Err: fmt.Errorf("team stock insufficient for activity %d", req.ActivityID)}
	}

	// 构建 team 和 order
	now := time.Now()
	validEnd := now.Add(stockTTL)

	var notifyURL *string
	if req.NotifyURL != "" {
		notifyURL = &req.NotifyURL
	}

	team := &model.Team{
		TeamID:         teamID,
		ActivityID:     req.ActivityID,
		Source:         req.Source,
		Channel:        req.Channel,
		OriginalPrice:  trial.OriginalPrice,
		DeductionPrice: trial.DeductionPrice,
		PayPrice:       trial.PayPrice,
		TargetCount:    activity.TargetCount,
		LockCount:      1,
		CompleteCount:  0,
		Status:         model.TeamStatusForming,
		ValidStart:     now,
		ValidEnd:       validEnd,
		NotifyType:     model.NotifyTypeHTTP,
		NotifyURL:      notifyURL,
	}

	order := &model.Order{
		UserID:         req.UserID,
		TeamID:         teamID,
		OrderID:        orderID,
		ActivityID:     req.ActivityID,
		GoodsID:        req.GoodsID,
		Source:         req.Source,
		Channel:        req.Channel,
		OriginalPrice:  trial.OriginalPrice,
		DeductionPrice: trial.DeductionPrice,
		PayPrice:       trial.PayPrice,
		Status:         model.OrderStatusLocked,
		OutTradeNo:     req.OutTradeNo,
	}

	if err := s.orderRepo.CreateTeamWithOrder(ctx, team, order); err != nil {
		s.cacheRepo.ReleaseStock(ctx, req.ActivityID, teamID, req.OutTradeNo)
		return nil, fmt.Errorf("lock: create team with order: %w", err)
	}

	slog.InfoContext(ctx, "lock: new team created", "team_id", teamID, "order_id", orderID, "user_id", req.UserID)
	return buildLockResult(order, teamID), nil
}

// lockJoinTeam 加入已有团。
func (s *LockService) lockJoinTeam(ctx context.Context, req LockRequest, trial *TrialResult, activity *model.Activity, stockTTL time.Duration) (*LockResult, error) {
	// 校验团存在且仍在拼团中
	team, err := s.orderRepo.FindTeamByID(ctx, req.TeamID)
	if err != nil {
		return nil, &LockError{Code: errcode.CodeOrderNotFound, Err: fmt.Errorf("team %s not found: %w", req.TeamID, err)}
	}
	if team.Status != model.TeamStatusForming {
		return nil, &LockError{Code: errcode.CodeTeamFull, Err: fmt.Errorf("team %s is not forming (status=%d)", req.TeamID, team.Status)}
	}
	if time.Now().After(team.ValidEnd) {
		return nil, &LockError{Code: errcode.CodeActivityTimeInvalid, Err: fmt.Errorf("team %s expired at %v", req.TeamID, team.ValidEnd)}
	}

	orderID := generateNumericID(12)

	// Redis Lua 原子占名额
	ok, err := s.cacheRepo.TryOccupyStock(ctx, req.ActivityID, req.TeamID, req.OutTradeNo, activity.TargetCount, stockTTL)
	if err != nil {
		return nil, fmt.Errorf("lock: occupy stock join team: %w", err)
	}
	if !ok {
		return nil, &LockError{Code: errcode.CodeStockInsufficient, Err: fmt.Errorf("team %s stock full", req.TeamID)}
	}

	order := &model.Order{
		UserID:         req.UserID,
		TeamID:         req.TeamID,
		OrderID:        orderID,
		ActivityID:     req.ActivityID,
		GoodsID:        req.GoodsID,
		Source:         req.Source,
		Channel:        req.Channel,
		OriginalPrice:  trial.OriginalPrice,
		DeductionPrice: trial.DeductionPrice,
		PayPrice:       trial.PayPrice,
		Status:         model.OrderStatusLocked,
		OutTradeNo:     req.OutTradeNo,
	}

	if err := s.orderRepo.JoinTeamWithOrder(ctx, req.TeamID, order); err != nil {
		s.cacheRepo.ReleaseStock(ctx, req.ActivityID, req.TeamID, req.OutTradeNo)
		if err == repository.ErrTeamFull {
			s.cacheRepo.MarkTeamFull(ctx, req.ActivityID, req.TeamID, stockTTL)
			return nil, &LockError{Code: errcode.CodeTeamFull, Err: err}
		}
		return nil, fmt.Errorf("lock: join team with order: %w", err)
	}

	slog.InfoContext(ctx, "lock: joined team", "team_id", req.TeamID, "order_id", orderID, "user_id", req.UserID)
	return buildLockResult(order, req.TeamID), nil
}

// --- 辅助函数 ---

// getCachedResult 从缓存获取锁单结果。
func (s *LockService) getCachedResult(ctx context.Context, userID, outTradeNo string) (*LockResult, error) {
	raw, err := s.cacheRepo.GetLockResult(ctx, userID, outTradeNo)
	if err != nil {
		return nil, err
	}
	if raw == nil {
		return nil, nil
	}
	var result LockResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("unmarshal cached result: %w", err)
	}
	return &result, nil
}

// findExistingOrder 从数据库查找已有订单（幂等兜底）。
func (s *LockService) findExistingOrder(ctx context.Context, userID, outTradeNo string) (*LockResult, error) {
	order, err := s.orderRepo.FindOrderByOutTradeNo(ctx, outTradeNo)
	if err != nil {
		return nil, fmt.Errorf("find existing order: %w", err)
	}
	// 校验 userId 一致
	if order.UserID != userID {
		return nil, &LockError{Code: errcode.CodeOrderNotFound, Err: fmt.Errorf("out_trade_no %s belongs to different user", outTradeNo)}
	}
	return buildLockResult(order, order.TeamID), nil
}

// cacheResult 缓存锁单结果。
func (s *LockService) cacheResult(ctx context.Context, userID, outTradeNo string, result *LockResult) {
	raw, err := json.Marshal(result)
	if err != nil {
		slog.WarnContext(ctx, "lock: marshal result failed", "error", err)
		return
	}
	if err := s.cacheRepo.CacheLockResult(ctx, userID, outTradeNo, raw, s.resultTTL); err != nil {
		slog.WarnContext(ctx, "lock: cache result failed", "out_trade_no", outTradeNo, "error", err)
	}
}

// buildLockResult 从 Order 构造 LockResult。
func buildLockResult(order *model.Order, teamID string) *LockResult {
	return &LockResult{
		OrderID:        order.OrderID,
		OutTradeNo:     order.OutTradeNo,
		TeamID:         teamID,
		OriginalPrice:  order.OriginalPrice,
		DeductionPrice: order.DeductionPrice,
		PayPrice:       order.PayPrice,
		Status:         int(order.Status),
	}
}

// generateNumericID 生成指定长度的随机数字字符串（crypto/rand）。
func generateNumericID(length int) string {
	b := make([]byte, length)
	for i := range b {
		n, _ := cryptorand.Int(cryptorand.Reader, big.NewInt(10))
		b[i] = byte('0' + n.Int64())
	}
	return string(b)
}

// LockError 锁单业务错误。
type LockError struct {
	Code string
	Err  error
}

func (e *LockError) Error() string {
	return fmt.Sprintf("lock error [%s]: %v", e.Code, e.Err)
}

func (e *LockError) Unwrap() error { return e.Err }

// ErrorCode 返回业务错误码。
func (e *LockError) ErrorCode() string { return e.Code }
