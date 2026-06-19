// Package service 业务逻辑层。
// 一个文件一个业务领域，无状态，依赖注入 repository 接口。
package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/reuben/group-buying/internal/errcode"
	"github.com/reuben/group-buying/internal/model"
	"github.com/reuben/group-buying/internal/repository"
)

// TrialService 试算服务。
// 职责：根据 source+channel+goodsId 查找拼团活动，计算折后价格，校验人群标签。
type TrialService struct {
	activityRepo repository.ActivityRepository
	productRepo  repository.ProductRepository
	cacheRepo    repository.CacheRepository
	crowdRepo    repository.CrowdRepository
}

// NewTrialService 构造函数。
func NewTrialService(
	activityRepo repository.ActivityRepository,
	productRepo repository.ProductRepository,
	cacheRepo repository.CacheRepository,
	crowdRepo repository.CrowdRepository,
) *TrialService {
	return &TrialService{
		activityRepo: activityRepo,
		productRepo:  productRepo,
		cacheRepo:    cacheRepo,
		crowdRepo:    crowdRepo,
	}
}

// TrialRequest 试算请求参数。
type TrialRequest struct {
	UserID  string `json:"user_id" binding:"required"`
	GoodsID string `json:"goods_id" binding:"required"`
	Source  string `json:"source" binding:"required"`
	Channel string `json:"channel" binding:"required"`
}

// TrialResult 试算结果。
type TrialResult struct {
	GoodsID        string    `json:"goods_id"`
	GoodsName      string    `json:"goods_name"`
	OriginalPrice  string    `json:"original_price"`
	DeductionPrice string    `json:"deduction_price"` // 优惠金额 = original - pay
	PayPrice       string    `json:"pay_price"`       // 实付金额
	ActivityID     int64     `json:"activity_id"`
	TargetCount    int       `json:"target_count"`
	StartTime      time.Time `json:"start_time"`
	EndTime        time.Time `json:"end_time"`
	IsVisible      bool      `json:"is_visible"` // 用户是否可见此活动
	IsEnable       bool      `json:"is_enable"`  // 用户是否可参与此活动
}

// Trial 试算主流程。
//
// 流程（扁平化，不用责任链）：
//  1. 查 activity_products → 获取 activityId
//  2. 查 activities + discounts（JOIN）→ 校验活动状态
//  3. 查 products → 获取商品原价
//  4. 计算折扣（switch planType: ZJ/MJ/ZK/N）
//  5. 校验活动级人群标签（tagScope 决定 visible/enable）
func (s *TrialService) Trial(ctx context.Context, req TrialRequest) (*TrialResult, error) {
	slog.DebugContext(ctx, "trial start", "user_id", req.UserID, "goods_id", req.GoodsID, "source", req.Source, "channel", req.Channel)

	// 0. 参数校验
	if req.UserID == "" || req.GoodsID == "" || req.Source == "" || req.Channel == "" {
		return nil, &TrialError{Code: errcode.CodeInvalidParam, Err: fmt.Errorf("user_id, goods_id, source, channel are required")}
	}

	// 1. 查活动-商品映射
	ap, err := s.activityRepo.FindActivityProduct(ctx, req.Source, req.Channel, req.GoodsID)
	if err != nil {
		slog.WarnContext(ctx, "trial: no activity mapping", "source", req.Source, "channel", req.Channel, "goods_id", req.GoodsID, "error", err)
		return nil, &TrialError{Code: errcode.CodeTrialFailed, Err: err}
	}

	// 2. 查活动+折扣（JOIN，一次查询）
	awd, err := s.activityRepo.FindActivityWithDiscount(ctx, ap.ActivityID)
	if err != nil {
		slog.WarnContext(ctx, "trial: activity/discount not found", "activity_id", ap.ActivityID, "error", err)
		return nil, &TrialError{Code: errcode.CodeTrialFailed, Err: err}
	}

	// 3. 校验活动状态
	if err := validateActivityStatus(awd.Activity); err != nil {
		slog.WarnContext(ctx, "trial: activity invalid", "activity_id", ap.ActivityID, "status", awd.Status, "error", err)
		return nil, err
	}

	// 4. 查商品
	prod, err := s.productRepo.FindProductByGoodsID(ctx, req.GoodsID)
	if err != nil {
		slog.WarnContext(ctx, "trial: product not found", "goods_id", req.GoodsID, "error", err)
		return nil, &TrialError{Code: errcode.CodeTrialFailed, Err: err}
	}

	// 5. 计算折后价
	originalPrice, err := decimal.NewFromString(prod.OriginalPrice)
	if err != nil {
		return nil, fmt.Errorf("trial: parse original price %q: %w", prod.OriginalPrice, err)
	}

	payPrice, err := calculatePayPrice(ctx, originalPrice, awd.Discount, req.UserID, s.crowdRepo, s.cacheRepo)
	if err != nil {
		slog.WarnContext(ctx, "trial: discount calc failed", "plan_type", awd.Discount.PlanType, "expression", awd.Discount.Expression, "error", err)
		return nil, &TrialError{Code: errcode.CodeNoDiscountService, Err: err}
	}

	deductionPrice := originalPrice.Sub(payPrice)

	// 6. 校验活动级人群标签（visible/enable）
	isVisible, isEnable := resolveTagScope(ctx, awd.TagID, awd.TagScope, req.UserID, s.crowdRepo, s.cacheRepo)

	slog.DebugContext(ctx, "trial done", "user_id", req.UserID, "original", originalPrice, "pay", payPrice, "deduction", deductionPrice)
	return &TrialResult{
		GoodsID:        prod.GoodsID,
		GoodsName:      prod.GoodsName,
		OriginalPrice:  originalPrice.StringFixed(2),
		DeductionPrice: deductionPrice.StringFixed(2),
		PayPrice:       payPrice.StringFixed(2),
		ActivityID:     awd.ActivityID,
		TargetCount:    awd.TargetCount,
		StartTime:      awd.StartTime,
		EndTime:        awd.EndTime,
		IsVisible:      isVisible,
		IsEnable:       isEnable,
	}, nil
}

// validateActivityStatus 校验活动状态和时间范围。
func validateActivityStatus(a model.Activity) error {
	if a.Status != model.ActivityStatusActive {
		return &TrialError{Code: errcode.CodeActivityInactive, Err: fmt.Errorf("activity %d status=%d", a.ActivityID, a.Status)}
	}
	now := time.Now()
	if now.Before(a.StartTime) || now.After(a.EndTime) {
		return &TrialError{Code: errcode.CodeActivityTimeInvalid, Err: fmt.Errorf("activity %d time invalid: now=%v start=%v end=%v", a.ActivityID, now, a.StartTime, a.EndTime)}
	}
	return nil
}

// calculatePayPrice 根据折扣类型计算实付金额。
//
// 折扣类型：
//   - ZJ（直减）：payPrice = originalPrice - expression
//   - MJ（满减）：expression="100.00-20.00" → originalPrice >= 100 时减 20
//   - ZK（折扣）：payPrice = originalPrice * expression（expression="0.8" 即八折）
//   - N（N元购）：payPrice = expression（固定价）
//
// 如果折扣是人群标签类型（discount_type=1），用户不在人群中则原价。
// 最低支付 0.01。
func calculatePayPrice(ctx context.Context, originalPrice decimal.Decimal, d model.Discount, userID string, crowdRepo repository.CrowdRepository, cacheRepo repository.CacheRepository) (decimal.Decimal, error) {
	// 人群标签折扣：用户不在人群中 → 原价
	if d.DiscountType == model.DiscountTypeTag && d.TagID != nil && *d.TagID != "" {
		inCrowd, _ := checkUserInCrowd(ctx, crowdRepo, cacheRepo, *d.TagID, userID)
		if !inCrowd {
			slog.DebugContext(ctx, "trial: user not in discount crowd, original price", "user_id", userID, "tag_id", *d.TagID)
			return originalPrice, nil
		}
	}

	oneCent := decimal.NewFromFloat(0.01)

	switch d.PlanType {
	case model.PlanZJ:
		// 直减：expression = "20.00"
		reduce, err := decimal.NewFromString(strings.TrimSpace(d.Expression))
		if err != nil {
			return decimal.Zero, fmt.Errorf("parse ZJ expression %q: %w", d.Expression, err)
		}
		pay := originalPrice.Sub(reduce)
		if pay.LessThanOrEqual(decimal.Zero) {
			return oneCent, nil
		}
		return pay, nil

	case model.PlanMJ:
		// 满减：expression = "100.00-20.00"
		parts := strings.Split(d.Expression, "-")
		if len(parts) != 2 {
			return decimal.Zero, fmt.Errorf("invalid MJ expression %q", d.Expression)
		}
		threshold, err := decimal.NewFromString(strings.TrimSpace(parts[0]))
		if err != nil {
			return decimal.Zero, fmt.Errorf("parse MJ threshold %q: %w", parts[0], err)
		}
		reduce, err := decimal.NewFromString(strings.TrimSpace(parts[1]))
		if err != nil {
			return decimal.Zero, fmt.Errorf("parse MJ reduce %q: %w", parts[1], err)
		}
		if originalPrice.LessThan(threshold) {
			return originalPrice, nil
		}
		pay := originalPrice.Sub(reduce)
		if pay.LessThanOrEqual(decimal.Zero) {
			return oneCent, nil
		}
		return pay, nil

	case model.PlanZK:
		// 折扣：expression = "0.8"（八折）
		rate, err := decimal.NewFromString(strings.TrimSpace(d.Expression))
		if err != nil {
			return decimal.Zero, fmt.Errorf("parse ZK expression %q: %w", d.Expression, err)
		}
		pay := originalPrice.Mul(rate)
		if pay.LessThanOrEqual(decimal.Zero) {
			return oneCent, nil
		}
		return pay, nil

	case model.PlanN:
		// N元购：expression = "9.90"
		pay, err := decimal.NewFromString(strings.TrimSpace(d.Expression))
		if err != nil {
			return decimal.Zero, fmt.Errorf("parse N expression %q: %w", d.Expression, err)
		}
		if pay.LessThanOrEqual(decimal.Zero) {
			return oneCent, nil
		}
		return pay, nil

	default:
		return decimal.Zero, fmt.Errorf("unknown plan type %q", d.PlanType)
	}
}

// resolveTagScope 解析活动级人群标签范围，返回 (isVisible, isEnable)。
//
// tagScope 语义（2位字符串）：
//   - 第1位 '1' = 可见限制（不在人群中看不到活动）
//   - 第2位 '1' = 参与限制（不在人群中不能锁单）
//   - "0" / "00" → 可见且可参与
//   - "01" → 可见但不可参与（需在人群中才能参与）
//   - "1" / "10" → 不可见也不可参与
//   - "11" → 不可见也不可参与（需在人群中才能看到和参与）
//
// 如果 tagId 为空，直接返回 (true, true)。
// 如果用户在人群中，无论 tagScope 如何都返回 (true, true)。
func resolveTagScope(ctx context.Context, tagID *string, tagScope *string, userID string, crowdRepo repository.CrowdRepository, cacheRepo repository.CacheRepository) (bool, bool) {
	if tagID == nil || *tagID == "" {
		return true, true
	}

	inCrowd, _ := checkUserInCrowd(ctx, crowdRepo, cacheRepo, *tagID, userID)
	if inCrowd {
		// 在人群中 → 不受限制
		return true, true
	}

	scope := ""
	if tagScope != nil {
		scope = *tagScope
	}

	// 默认：不在人群中，根据 tagScope 决定
	isVisible := true
	isEnable := true

	if len(scope) >= 1 && scope[0] == '1' {
		isVisible = false
	}
	if len(scope) >= 2 && scope[1] == '1' {
		isEnable = false
	}
	if !isVisible {
		isEnable = false // 不可见则必然不可参与
	}

	return isVisible, isEnable
}

// checkUserInCrowd 检查用户是否在人群标签中（先查缓存，再查 DB）。
func checkUserInCrowd(ctx context.Context, crowdRepo repository.CrowdRepository, cacheRepo repository.CacheRepository, tagID, userID string) (bool, error) {
	// 先查 Redis 缓存（SISMEMBER）
	// 注意：SISMEMBER 对不存在的 key 也返回 false，无法区分"不在集合中"和"缓存未初始化"。
	// 因此只有 Redis 明确返回 true 时才信任缓存；false 时 fallback 到 DB。
	inCrowd, err := cacheRepo.CheckCrowdMember(ctx, tagID, userID)
	if err == nil && inCrowd {
		return true, nil
	}

	// 缓存未命中或返回 false → 查 DB
	slog.DebugContext(ctx, "trial: crowd cache miss, fallback to db", "tag_id", tagID, "user_id", userID)
	return crowdRepo.IsUserInCrowd(ctx, tagID, userID)
}

// TrialError 试算业务错误。
type TrialError struct {
	Code string
	Err  error
}

func (e *TrialError) Error() string {
	return fmt.Sprintf("trial error [%s]: %v", e.Code, e.Err)
}

func (e *TrialError) Unwrap() error {
	return e.Err
}

// ErrorCode 返回业务错误码。
func (e *TrialError) ErrorCode() string {
	return e.Code
}
