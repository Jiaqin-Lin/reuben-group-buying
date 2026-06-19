// Package service 业务逻辑层。
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"time"

	"github.com/reuben/group-buying/internal/cache"
	"github.com/reuben/group-buying/internal/config/dynamic"
	"github.com/reuben/group-buying/internal/errcode"
	"github.com/reuben/group-buying/internal/model"
	"github.com/reuben/group-buying/internal/pay"
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
	localCache   *cache.LocalCache
	payGateway   pay.Gateway
	paymentRepo  repository.PaymentRepository
	lockTTL      time.Duration
	resultTTL    time.Duration
}

// NewLockService 构造函数。
func NewLockService(
	trialSvc *TrialService,
	orderRepo repository.OrderRepository,
	activityRepo repository.ActivityRepository,
	cacheRepo repository.CacheRepository,
	localCache *cache.LocalCache,
	payGateway pay.Gateway,
	paymentRepo repository.PaymentRepository,
	lockTTL, resultTTL time.Duration,
) *LockService {
	return &LockService{
		trialSvc:     trialSvc,
		orderRepo:    orderRepo,
		activityRepo: activityRepo,
		cacheRepo:    cacheRepo,
		localCache:   localCache,
		payGateway:   payGateway,
		paymentRepo:  paymentRepo,
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
	UserID         string `json:"user_id"`
	TeamID         string `json:"team_id"`
	OriginalPrice  string `json:"original_price"`
	DeductionPrice string `json:"deduction_price"`
	PayPrice       string `json:"pay_price"`
	PayURL         string `json:"pay_url,omitempty"`
	Status         int    `json:"status"`
}

// Lock 锁单主流程。
//
// 幂等防护（对齐 Java 版三层架构）：
//  1. 缓存层（Redis，10min TTL）— 快速路径
//  2. 分布式锁层（Redis SETNX，3s TTL）— 序列化同 outTradeNo
//  3. 数据库层（out_trade_no 唯一索引）— 最后防线
//
// 与 Java 版对齐：锁前不做 DB 查询。缓存未命中直接抢锁，DB 唯一索引兜底。
// 压测场景（全部唯一 outTradeNo）可省掉每次锁单 1 次无效 DB SELECT。
func (s *LockService) Lock(ctx context.Context, req LockRequest) (*LockResult, error) {
	slog.DebugContext(ctx, "lock start", "user_id", req.UserID, "out_trade_no", req.OutTradeNo, "team_id", req.TeamID)

	// 1. 幂等检查：缓存（快速路径）
	if cached, err := s.getCachedResult(ctx, req.UserID, req.OutTradeNo); err != nil {
		slog.WarnContext(ctx, "lock: cache lookup failed", "out_trade_no", req.OutTradeNo, "error", err)
	} else if cached != nil {
		slog.DebugContext(ctx, "lock: cache hit, return cached", "out_trade_no", req.OutTradeNo)
		return cached, nil
	}

	// 2. 获取分布式锁
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

	// 3. 获取锁后再次检查缓存+DB（防止并发窗口）
	//    缓存优先（快），未命中才查 DB（慢，但锁内串行，QPS 可控）
	if cached, err := s.getCachedResult(ctx, req.UserID, req.OutTradeNo); err != nil {
		slog.WarnContext(ctx, "lock: double-check cache failed", "error", err)
	} else if cached != nil {
		return cached, nil
	}
	if existing, err := s.findExistingOrder(ctx, req.UserID, req.OutTradeNo); err != nil {
		slog.WarnContext(ctx, "lock: db lookup failed", "out_trade_no", req.OutTradeNo, "error", err)
	} else if existing != nil {
		s.cacheResult(ctx, req.UserID, req.OutTradeNo, existing)
		return existing, nil
	}

	// 4. 试算：定价 + 活动校验 + 人群标签（本地缓存命中，不查 Redis/DB）
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

	// 5. 限购检查（只查已支付订单数，不计锁定的）
	activity, err := s.getActivity(ctx, req.ActivityID)
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

	// 6. 执行锁单（新建团 / 加入团）
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

	// 7. 创建支付单（压测/测试时可降级跳过）
	if s.payGateway != nil && !dynamic.FeatureSkipPayment.Get() {
		payURL, paymentErr := s.createPayment(ctx, result)
		if paymentErr != nil {
			slog.WarnContext(ctx, "lock: create payment failed", "order_id", result.OrderID, "error", paymentErr)
		} else {
			result.PayURL = payURL
		}
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
//
// 秒杀优化：先 Redis Lua 占名额（满标快速拒绝，~0.1ms），成功后再查 DB 校验团状态。
// 被拒绝的请求只做 1 次 Redis 往返，不碰 DB。
func (s *LockService) lockJoinTeam(ctx context.Context, req LockRequest, trial *TrialResult, activity *model.Activity, stockTTL time.Duration) (*LockResult, error) {
	orderID := generateNumericID(12)

	// 1. Redis Lua 原子占名额（优先，满标直接拒绝，不查 DB）
	ok, err := s.cacheRepo.TryOccupyStock(ctx, req.ActivityID, req.TeamID, req.OutTradeNo, activity.TargetCount, stockTTL)
	if err != nil {
		return nil, fmt.Errorf("lock: occupy stock join team: %w", err)
	}
	if !ok {
		return nil, &LockError{Code: errcode.CodeStockInsufficient, Err: fmt.Errorf("team %s stock full", req.TeamID)}
	}

	// 2. 占位成功后校验团状态（DB 查询）
	team, err := s.orderRepo.FindTeamByID(ctx, req.TeamID)
	if err != nil {
		s.cacheRepo.ReleaseStock(ctx, req.ActivityID, req.TeamID, req.OutTradeNo)
		return nil, &LockError{Code: errcode.CodeOrderNotFound, Err: fmt.Errorf("team %s not found: %w", req.TeamID, err)}
	}
	if team.Status != model.TeamStatusForming {
		s.cacheRepo.ReleaseStock(ctx, req.ActivityID, req.TeamID, req.OutTradeNo)
		return nil, &LockError{Code: errcode.CodeTeamFull, Err: fmt.Errorf("team %s is not forming (status=%d)", req.TeamID, team.Status)}
	}
	if time.Now().After(team.ValidEnd) {
		s.cacheRepo.ReleaseStock(ctx, req.ActivityID, req.TeamID, req.OutTradeNo)
		return nil, &LockError{Code: errcode.CodeActivityTimeInvalid, Err: fmt.Errorf("team %s expired at %v", req.TeamID, team.ValidEnd)}
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

// createPayment 创建支付单并调用支付网关获取 payUrl。
func (s *LockService) createPayment(ctx context.Context, result *LockResult) (string, error) {
	order := &model.Order{
		OrderID:    result.OrderID,
		OutTradeNo: result.OutTradeNo,
		PayPrice:   result.PayPrice,
	}

	payResult, err := s.payGateway.CreateOrder(ctx, order)
	if err != nil {
		return "", fmt.Errorf("gateway create order: %w", err)
	}

	payURL := payResult.PayURL
	payment := &model.Payment{
		OrderID:    result.OrderID,
			OutTradeNo: result.OutTradeNo,
			UserID:     result.UserID,
			TeamID:     result.TeamID,
		Amount:     result.PayPrice,
		Subject:    "拼团订单",
		PayURL:     &payURL,
		Status:     model.PaymentStatusPending,
		ExpireAt:   time.Now().Add(15 * time.Minute),
	}
	if err := s.paymentRepo.CreatePayment(ctx, payment); err != nil {
		slog.WarnContext(ctx, "lock: save payment record failed", "order_id", result.OrderID, "error", err)
	}

	return payResult.PayURL, nil
}

// getActivity 查活动（本地缓存 → DB fallback）。
func (s *LockService) getActivity(ctx context.Context, activityID int64) (*model.Activity, error) {
	if s.localCache != nil {
		if a, ok := s.localCache.GetActivity(activityID); ok {
			return a, nil
		}
	}
	return s.activityRepo.FindActivityByID(ctx, activityID)
}

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
		UserID:         order.UserID,
		TeamID:         teamID,
		OriginalPrice:  order.OriginalPrice,
		DeductionPrice: order.DeductionPrice,
		PayPrice:       order.PayPrice,
		Status:         int(order.Status),
	}
}

// generateNumericID 生成指定长度的随机数字字符串。
// 用 math/rand/v2（PCG 算法，无锁，~3ns/op），非安全场景不需要 crypto/rand。
func generateNumericID(length int) string {
	b := make([]byte, length)
	for i := range b {
		b[i] = byte('0' + rand.IntN(10))
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
