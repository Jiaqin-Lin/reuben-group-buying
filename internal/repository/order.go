package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/reuben/group-buying/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// OrderRepository 订单和拼团队伍的数据访问接口。
//
// 核心设计：
//   - out_trade_no = 外部交易单号（调用方生成），用于幂等和全链路关联
//   - order_id = 内部订单号（本系统生成），发给支付宝的商家订单号
//   - 建团+首单、加团+订单 都在事务内完成，保证 teams.lock_count 与 orders 记录一致
type OrderRepository interface {
	// --- 订单查询 ---

	// FindOrderByOrderID 按内部订单号查订单。
	FindOrderByOrderID(ctx context.Context, orderID string) (*model.Order, error)

	// FindOrderByOutTradeNo 按外部交易单号查订单。
	// 幂等检查核心：同 out_trade_no 重复请求不创建新订单。
	FindOrderByOutTradeNo(ctx context.Context, outTradeNo string) (*model.Order, error)

	// FindOrdersByUserAndActivity 查用户在活动下的所有订单。
	// 用于 take_limit 校验：统计用户在该活动下已支付次数。
	FindOrdersByUserAndActivity(ctx context.Context, userID string, activityID int64) ([]model.Order, error)

	// ExistsOrderByUserAndTeam 检查用户是否已在指定团有有效订单（锁定/已支付）。
	// 已退款订单不计入，允许退款后重新加入同一个团。
	ExistsOrderByUserAndTeam(ctx context.Context, userID, teamID string) (bool, error)

	// CountPaidOrdersByUserActivity 统计用户在活动下已支付的订单数。
	// 比 FindOrdersByUserAndActivity 更轻量，只返回计数。
	CountPaidOrdersByUserActivity(ctx context.Context, userID string, activityID int64) (int64, error)

	// CountActiveOrdersByUserActivity 统计用户在活动下有效订单数（锁定+已支付）。
	// 用于锁单时软检查 take_limit，防止只锁不付绕过限购。
	CountActiveOrdersByUserActivity(ctx context.Context, userID string, activityID int64) (int64, error)

	// --- 团队查询 ---

	// FindTeamByID 按 team_id 查团。
	FindTeamByID(ctx context.Context, teamID string) (*model.Team, error)

	// FindTeamForUpdate 行锁读团记录，用于加团时的并发控制。
	// SELECT ... FOR UPDATE，必须在事务内调用。
	FindTeamForUpdate(ctx context.Context, tx *gorm.DB, teamID string) (*model.Team, error)

	// --- 事务写入 ---

	// CreateTeamWithOrder 新建团+首单，一个事务完成。
	// 场景：用户锁单时 teamId 为空，系统新建一个团。
	// 写入 teams + orders 两条记录，保证原子性。
	CreateTeamWithOrder(ctx context.Context, team *model.Team, order *model.Order) error

	// JoinTeamWithOrder 加入已有团+创建订单，一个事务完成。
	// 场景：用户锁单时带 teamId，加入已有队伍。
	// 写入 orders + 原子更新 teams.lock_count += 1。
	// 加入前检查 lock_count < target_count，已满则返回 ErrTeamFull。
	JoinTeamWithOrder(ctx context.Context, teamID string, order *model.Order) error

	// --- 状态更新 ---

	// UpdateOrderStatus 更新订单状态。
	// 用于支付回调（0→1）或退单（0/1→2）。
	UpdateOrderStatus(ctx context.Context, orderID string, status int8) error

	// UpdateOrderStatusWithCheck 条件更新订单状态，带当前状态检查。
	// SQL: UPDATE orders SET status=? WHERE order_id=? AND status=?
	// RowsAffected==0 返回错误，用于并发保护和幂等。
	UpdateOrderStatusWithCheck(ctx context.Context, orderID string, fromStatus, toStatus int8) error

	// UpdateTeamCounters 原子更新团计数器。
	// lockDelta/compleDelta 为增量值（+1 或 -1），用 gorm.Expr 保证原子操作。
	UpdateTeamCounters(ctx context.Context, teamID string, lockDelta, completeDelta int) error

	// UpdateTeamStatus 更新团状态。
	// 成团（0→1）、失败（0→2）、成团含退款（1→3）。
	UpdateTeamStatus(ctx context.Context, teamID string, status int8) error

	// RefundTeamForming 退单时更新 forming 团的计数器（带状态检查）。
	// SQL: UPDATE teams SET lock_count=lock_count+?, complete_count=complete_count+?
	//      WHERE team_id=? AND status=0
	// 比 UpdateTeamCounters 多了 status=0 条件，防止退单时团已发生变化。
	RefundTeamForming(ctx context.Context, teamID string, lockDelta, completeDelta int) error

	// RefundCompleteTeam 退单时更新已成团的计数器并转换团状态。
	// 根据 completeCount 判断团的新状态：
	//   - completeCount > 1 → status=3 (CompleteRefunded)
	//   - completeCount == 1 → status=2 (Failed，最后一人退单)
	// 返回新的团状态。
	RefundCompleteTeam(ctx context.Context, teamID string, lockDelta, completeDelta int) (int8, error)

	// SettleOrder 结算订单（事务内完成订单状态更新+团进度更新+成团判断）。
	// 流程：SELECT FOR UPDATE team → 校验 → UPDATE order status=1 → UPDATE team.complete_count+1 → 判断成团。
	// 返回结算结果，包含是否成团及成团相关信息（用于创建 notify_task）。
	SettleOrder(ctx context.Context, params SettleOrderParams) (*SettleOrderResult, error)

	// --- 超时扫描 ---

	// FindTimeoutOrders 查询超时未支付的订单列表。
	// JOIN orders + teams：orders.status=0（锁定中）且 teams.valid_end < NOW()。
	// 定时任务游标分页扫描，每次限量处理。
	FindTimeoutOrders(ctx context.Context, limit int, lastID uint64) ([]TimeoutOrder, error)

	// FindTeamsByActivityID 查活动下所有 forming 状态的团。
	// 用于用户端首页团列表展示。
	FindTeamsByActivityID(ctx context.Context, activityID int64, offset, limit int) ([]model.Team, int64, error)

	// FindOrdersByTeamID 查团下所有订单。
	// 用于成团后构建回调 payload。
	FindOrdersByTeamID(ctx context.Context, teamID string) ([]model.Order, error)

	// --- 补偿回滚（支付创建失败时撤销已提交的订单/团）---

	// RollbackNewTeam 删除新建团时创建的 team 和 order。
	// 用于支付创建失败时的补偿事务：锁单 DB 事务已提交，需要回滚。
	RollbackNewTeam(ctx context.Context, teamID, orderID string) error

	// RollbackJoinTeam 删除加入团时创建的 order，并回退 team.lock_count -= 1。
	// 用于支付创建失败时的补偿事务。
	RollbackJoinTeam(ctx context.Context, teamID, orderID string) error
}

// TimeoutOrder 超时订单联表查询结果。
// 包含订单信息和团有效期，用于超时退单处理。
type TimeoutOrder struct {
	model.Order
	TeamValidEnd string `gorm:"column:team_valid_end"` // teams.valid_end
}

// 业务错误（哨兵错误，service 层判断用）
var (
	ErrTeamFull       = fmt.Errorf("team is full")
	ErrOrderDup       = fmt.Errorf("order duplicate")
	ErrTeamDup        = fmt.Errorf("team duplicate")
	ErrTeamNotForming = fmt.Errorf("team is not forming")
	ErrTeamExpired    = fmt.Errorf("team has expired")
	ErrOrderNotLocked = fmt.Errorf("order is not in locked status")
)

// SettleOrderParams 结算订单参数。
type SettleOrderParams struct {
	OutTradeNo   string
	UserID       string
	OutTradeTime time.Time
}

// SettleOrderResult 结算订单结果。
type SettleOrderResult struct {
	OrderID       string  // 内部订单号
	TeamID        string  // 团 ID
	ActivityID    int64   // 活动 ID
	IsComplete    bool    // 本次结算后成团
	TargetCount   int     // 目标人数
	CompleteCount int     // 结算后的 complete_count
	NotifyType    string  // 通知类型（来自 team）
	NotifyURL     *string // 通知地址（来自 team）
}

// orderRepo GORM 实现。
type orderRepo struct {
	db *gorm.DB
}

// NewOrderRepo 构造函数。
func NewOrderRepo(db *gorm.DB) OrderRepository {
	return &orderRepo{db: db}
}

// --- 订单查询 ---

func (r *orderRepo) FindOrderByOrderID(ctx context.Context, orderID string) (*model.Order, error) {
	var o model.Order
	err := r.db.WithContext(ctx).Where("order_id = ?", orderID).First(&o).Error
	if err != nil {
		return nil, fmt.Errorf("find order by id %s: %w", orderID, err)
	}
	return &o, nil
}

func (r *orderRepo) FindOrderByOutTradeNo(ctx context.Context, outTradeNo string) (*model.Order, error) {
	var o model.Order
	err := r.db.WithContext(ctx).Where("out_trade_no = ?", outTradeNo).First(&o).Error
	if err != nil {
		return nil, fmt.Errorf("find order by out_trade_no %s: %w", outTradeNo, err)
	}
	return &o, nil
}

func (r *orderRepo) FindOrdersByUserAndActivity(ctx context.Context, userID string, activityID int64) ([]model.Order, error) {
	var orders []model.Order
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND activity_id = ?", userID, activityID).
		Find(&orders).Error
	if err != nil {
		return nil, fmt.Errorf("find orders by user %s activity %d: %w", userID, activityID, err)
	}
	return orders, nil
}

// ExistsOrderByUserAndTeam 检查用户是否已在指定团有有效订单（锁定/已支付，不含已退款）。
// 已退款订单不计入，允许退款后重新加入同一个团。
func (r *orderRepo) ExistsOrderByUserAndTeam(ctx context.Context, userID, teamID string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.Order{}).
		Where("user_id = ? AND team_id = ? AND status IN ?",
			userID, teamID, []int8{model.OrderStatusLocked, model.OrderStatusPaid}).
		Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("check order exists user %s team %s: %w", userID, teamID, err)
	}
	return count > 0, nil
}

func (r *orderRepo) CountPaidOrdersByUserActivity(ctx context.Context, userID string, activityID int64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.Order{}).
		Where("user_id = ? AND activity_id = ? AND status = ?",
			userID, activityID, model.OrderStatusPaid).
		Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("count paid orders user %s activity %d: %w", userID, activityID, err)
	}
	return count, nil
}

func (r *orderRepo) CountActiveOrdersByUserActivity(ctx context.Context, userID string, activityID int64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.Order{}).
		Where("user_id = ? AND activity_id = ? AND status IN ?",
			userID, activityID, []int8{model.OrderStatusLocked, model.OrderStatusPaid}).
		Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("count active orders user %s activity %d: %w", userID, activityID, err)
	}
	return count, nil
}

// --- 团队查询 ---

func (r *orderRepo) FindTeamByID(ctx context.Context, teamID string) (*model.Team, error) {
	var t model.Team
	err := r.db.WithContext(ctx).Where("team_id = ?", teamID).First(&t).Error
	if err != nil {
		return nil, fmt.Errorf("find team %s: %w", teamID, err)
	}
	return &t, nil
}

func (r *orderRepo) FindTeamForUpdate(ctx context.Context, tx *gorm.DB, teamID string) (*model.Team, error) {
	var t model.Team
	err := tx.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("team_id = ?", teamID).
		First(&t).Error
	if err != nil {
		return nil, fmt.Errorf("find team for update %s: %w", teamID, err)
	}
	return &t, nil
}

// --- 事务写入 ---

func (r *orderRepo) CreateTeamWithOrder(ctx context.Context, team *model.Team, order *model.Order) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(team).Error; err != nil {
			return fmt.Errorf("create team: %w", err)
		}
		if err := tx.Create(order).Error; err != nil {
			return fmt.Errorf("create order: %w", err)
		}
		return nil
	})
}

func (r *orderRepo) JoinTeamWithOrder(ctx context.Context, teamID string, order *model.Order) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 行锁读团，防并发超加
		team, err := r.FindTeamForUpdate(ctx, tx, teamID)
		if err != nil {
			return err
		}

		// 满员检查：已锁人数 >= 目标人数则拒绝
		if team.LockCount >= team.TargetCount {
			return ErrTeamFull
		}

		// 原子增加 lock_count
		result := tx.Model(&model.Team{}).
			Where("team_id = ? AND lock_count < target_count", teamID).
			Update("lock_count", gorm.Expr("lock_count + 1"))
		if result.Error != nil {
			return fmt.Errorf("incr lock_count: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return ErrTeamFull
		}

		if err := tx.Create(order).Error; err != nil {
			return fmt.Errorf("create order: %w", err)
		}
		return nil
	})
}

// --- 状态更新 ---

func (r *orderRepo) UpdateOrderStatus(ctx context.Context, orderID string, status int8) error {
	result := r.db.WithContext(ctx).
		Model(&model.Order{}).
		Where("order_id = ?", orderID).
		Update("status", status)
	if result.Error != nil {
		return fmt.Errorf("update order status %s: %w", orderID, result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("update order status %s: %w", orderID, gorm.ErrRecordNotFound)
	}
	return nil
}

func (r *orderRepo) UpdateTeamCounters(ctx context.Context, teamID string, lockDelta, completeDelta int) error {
	// 用 gorm.Expr 生成原子 SQL：UPDATE teams SET lock_count = lock_count + ?, complete_count = complete_count + ? WHERE team_id = ?
	result := r.db.WithContext(ctx).
		Model(&model.Team{}).
		Where("team_id = ?", teamID).
		Updates(map[string]any{
			"lock_count":     gorm.Expr("lock_count + ?", lockDelta),
			"complete_count": gorm.Expr("complete_count + ?", completeDelta),
		})
	if result.Error != nil {
		return fmt.Errorf("update team counters %s: %w", teamID, result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("update team counters %s: %w", teamID, gorm.ErrRecordNotFound)
	}
	return nil
}

func (r *orderRepo) UpdateTeamStatus(ctx context.Context, teamID string, status int8) error {
	result := r.db.WithContext(ctx).
		Model(&model.Team{}).
		Where("team_id = ?", teamID).
		Update("status", status)
	if result.Error != nil {
		return fmt.Errorf("update team status %s: %w", teamID, result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("update team status %s: %w", teamID, gorm.ErrRecordNotFound)
	}
	return nil
}

// UpdateOrderStatusWithCheck 条件更新订单状态，带当前状态检查。
// RowsAffected==0 表示不存在该状态下的订单（可能已被并发修改）。
func (r *orderRepo) UpdateOrderStatusWithCheck(ctx context.Context, orderID string, fromStatus, toStatus int8) error {
	result := r.db.WithContext(ctx).
		Model(&model.Order{}).
		Where("order_id = ? AND status = ?", orderID, fromStatus).
		Update("status", toStatus)
	if result.Error != nil {
		return fmt.Errorf("update order status with check %s: %w", orderID, result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("update order status with check %s: %w", orderID, gorm.ErrRecordNotFound)
	}
	return nil
}

// RefundTeamForming 退单时更新 forming 团的计数器。
// 带 status=0 条件，防止退单时团已经变成其他状态。
func (r *orderRepo) RefundTeamForming(ctx context.Context, teamID string, lockDelta, completeDelta int) error {
	result := r.db.WithContext(ctx).
		Model(&model.Team{}).
		Where("team_id = ? AND status = ?", teamID, model.TeamStatusForming).
		Updates(map[string]any{
			"lock_count":     gorm.Expr("lock_count + ?", lockDelta),
			"complete_count": gorm.Expr("complete_count + ?", completeDelta),
		})
	if result.Error != nil {
		return fmt.Errorf("refund team forming %s: %w", teamID, result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("refund team forming %s: team not forming or not found", teamID)
	}
	return nil
}

// RefundCompleteTeam 退单时更新已成团的计数器并转换团状态。
//
// 分两种情况：
//  1. 团剩余人数 > 1 → status=3 (CompleteRefunded)，团还活着
//  2. 团剩余人数 = 1 → status=2 (Failed)，最后一人退单，团解散
//
// 使用事务 + SELECT FOR UPDATE 序列化并发请求，确保两个 goroutine
// 同时退同一团的成员时，一个看到原始 complete_count 并匹配 >1 分支，
// 另一个看到减 1 后的 complete_count 并匹配 =1 分支。
func (r *orderRepo) RefundCompleteTeam(ctx context.Context, teamID string, lockDelta, completeDelta int) (int8, error) {
	var newStatus int8

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// SELECT FOR UPDATE 锁行，序列化同团的并发退款
		var team model.Team
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("team_id = ?", teamID).
			First(&team).Error; err != nil {
			return fmt.Errorf("find team for update: %w", err)
		}

		// 校验团状态
		if team.Status != model.TeamStatusComplete && team.Status != model.TeamStatusCompleteRefunded {
			return fmt.Errorf("team %s status is %d, expected Complete(1) or CompleteRefunded(3)", teamID, team.Status)
		}

		newCompleteCount := team.CompleteCount + completeDelta
		newLockCount := team.LockCount + lockDelta

		// 根据锁行读到的 complete_count 决定新状态
		if newCompleteCount > 0 {
			newStatus = model.TeamStatusCompleteRefunded
		} else {
			newStatus = model.TeamStatusFailed
		}

		result := tx.Model(&model.Team{}).
			Where("team_id = ?", teamID).
			Updates(map[string]any{
				"lock_count":     newLockCount,
				"complete_count": newCompleteCount,
				"status":         newStatus,
			})
		if result.Error != nil {
			return fmt.Errorf("update team: %w", result.Error)
		}
		return nil
	})

	if err != nil {
		return 0, fmt.Errorf("refund complete team %s: %w", teamID, err)
	}

	return newStatus, nil
}

// SettleOrder 结算订单（事务内完成）。
//
// 流程：
//  1. 查订单，校验 userId 和 status=Locked
//  2. SELECT FOR UPDATE 锁团行
//  3. 校验团状态（forming + 未过期）
//  4. UPDATE order SET status=1, out_trade_time=? WHERE order_id=? AND status=0
//  5. UPDATE team SET complete_count = complete_count + 1
//  6. 判断成团（old.complete_count + 1 >= target_count）→ 更新 team status=1
//
// 并发安全：
//   - SELECT FOR UPDATE 串行化同团的并发结算，保证只有一个请求判定"成团"
//   - order 状态条件更新（WHERE status=0）防重复结算
func (r *orderRepo) SettleOrder(ctx context.Context, params SettleOrderParams) (*SettleOrderResult, error) {
	var result *SettleOrderResult

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. 查订单
		var order model.Order
		if err := tx.Where("out_trade_no = ?", params.OutTradeNo).First(&order).Error; err != nil {
			return fmt.Errorf("find order by out_trade_no %s: %w", params.OutTradeNo, err)
		}
		if order.UserID != params.UserID {
			return fmt.Errorf("order user mismatch: expected %s, got %s", params.UserID, order.UserID)
		}
		if order.Status != model.OrderStatusLocked {
			return ErrOrderNotLocked
		}

		// 2. SELECT FOR UPDATE 锁团行
		var team model.Team
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("team_id = ?", order.TeamID).
			First(&team).Error; err != nil {
			return fmt.Errorf("find team for update %s: %w", order.TeamID, err)
		}

		// 3. 校验团状态
		if team.Status != model.TeamStatusForming {
			return ErrTeamNotForming
		}
		if time.Now().After(team.ValidEnd) {
			return ErrTeamExpired
		}

		// 4. 更新订单状态（条件更新，防重复结算）
		orderUpdate := tx.Model(&model.Order{}).
			Where("order_id = ? AND status = ?", order.OrderID, model.OrderStatusLocked).
			Updates(map[string]any{
				"status":         model.OrderStatusPaid,
				"out_trade_time": params.OutTradeTime,
			})
		if orderUpdate.Error != nil {
			return fmt.Errorf("update order status: %w", orderUpdate.Error)
		}
		if orderUpdate.RowsAffected == 0 {
			return ErrOrderNotLocked
		}

		// 5. 增加团完成人数
		teamUpdate := tx.Model(&model.Team{}).
			Where("team_id = ?", order.TeamID).
			Update("complete_count", gorm.Expr("complete_count + 1"))
		if teamUpdate.Error != nil {
			return fmt.Errorf("incr team complete_count: %w", teamUpdate.Error)
		}

		newCompleteCount := team.CompleteCount + 1

		// 6. 判断成团
		isComplete := newCompleteCount >= team.TargetCount
		if isComplete {
			statusUpdate := tx.Model(&model.Team{}).
				Where("team_id = ? AND status = ?", order.TeamID, model.TeamStatusForming).
				Update("status", model.TeamStatusComplete)
			if statusUpdate.Error != nil {
				return fmt.Errorf("update team status to complete: %w", statusUpdate.Error)
			}
		}

		result = &SettleOrderResult{
			OrderID:       order.OrderID,
			TeamID:        order.TeamID,
			ActivityID:    order.ActivityID,
			IsComplete:    isComplete,
			TargetCount:   team.TargetCount,
			CompleteCount: newCompleteCount,
			NotifyType:    team.NotifyType,
			NotifyURL:     team.NotifyURL,
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("settle order: %w", err)
	}
	return result, nil
}

// --- 超时扫描 ---

func (r *orderRepo) FindTimeoutOrders(ctx context.Context, limit int, lastID uint64) ([]TimeoutOrder, error) {
	// 游标分页：用 orders.id > lastID 代替 OFFSET，避免大偏移量性能问题。
	// 两种超时条件（满足其一即退单）：
	//   1. 订单支付超时：orders.created_at + 5 分钟 < NOW()（锁单不付钱 5 分钟退单）
	//   2. 拼团过期：teams.valid_end < NOW()（团超过拼团有效期解散）
	var orders []TimeoutOrder
	err := r.db.WithContext(ctx).
		Table("orders").
		Select("orders.*, teams.valid_end AS team_valid_end").
		Joins("JOIN teams ON teams.team_id = orders.team_id").
		Where("orders.status = ?", model.OrderStatusLocked).
		Where("orders.id > ?", lastID).
		Where("(orders.created_at + INTERVAL 5 MINUTE < NOW() OR teams.valid_end < NOW())").
		Order("orders.id ASC").
		Limit(limit).
		Find(&orders).Error
	if err != nil {
		return nil, fmt.Errorf("find timeout orders: %w", err)
	}
	return orders, nil
}

// FindTeamsByActivityID 查活动下所有 forming 状态的团，带分页。
func (r *orderRepo) FindTeamsByActivityID(ctx context.Context, activityID int64, offset, limit int) ([]model.Team, int64, error) {
	var total int64
	if err := r.db.WithContext(ctx).Model(&model.Team{}).
		Where("activity_id = ? AND status = ?", activityID, model.TeamStatusForming).
		Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count teams by activity %d: %w", activityID, err)
	}

	var teams []model.Team
	err := r.db.WithContext(ctx).
		Where("activity_id = ? AND status = ?", activityID, model.TeamStatusForming).
		Order("id DESC").
		Offset(offset).
		Limit(limit).
		Find(&teams).Error
	if err != nil {
		return nil, 0, fmt.Errorf("find teams by activity %d: %w", activityID, err)
	}
	return teams, total, nil
}

// FindOrdersByTeamID 查团下所有订单。
func (r *orderRepo) FindOrdersByTeamID(ctx context.Context, teamID string) ([]model.Order, error) {
	var orders []model.Order
	err := r.db.WithContext(ctx).
		Where("team_id = ?", teamID).
		Find(&orders).Error
	if err != nil {
		return nil, fmt.Errorf("find orders by team %s: %w", teamID, err)
	}
	return orders, nil
}

// --- 补偿回滚 ---

// RollbackNewTeam 删除新建团时创建的 team 和 order（补偿事务）。
func (r *orderRepo) RollbackNewTeam(ctx context.Context, teamID, orderID string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("order_id = ?", orderID).Delete(&model.Order{}).Error; err != nil {
			return fmt.Errorf("delete order %s: %w", orderID, err)
		}
		if err := tx.Where("team_id = ?", teamID).Delete(&model.Team{}).Error; err != nil {
			return fmt.Errorf("delete team %s: %w", teamID, err)
		}
		return nil
	})
}

// RollbackJoinTeam 删除加入团时创建的 order，并回退 team.lock_count（补偿事务）。
func (r *orderRepo) RollbackJoinTeam(ctx context.Context, teamID, orderID string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("order_id = ?", orderID).Delete(&model.Order{}).Error; err != nil {
			return fmt.Errorf("delete order %s: %w", orderID, err)
		}
		result := tx.Model(&model.Team{}).
			Where("team_id = ? AND lock_count > 0", teamID).
			Update("lock_count", gorm.Expr("lock_count - 1"))
		if result.Error != nil {
			return fmt.Errorf("decrement lock_count for team %s: %w", teamID, result.Error)
		}
		return nil
	})
}
