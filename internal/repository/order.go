package repository

import (
	"context"
	"fmt"

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

	// CountPaidOrdersByUserActivity 统计用户在活动下已支付的订单数。
	// 比 FindOrdersByUserAndActivity 更轻量，只返回计数。
	CountPaidOrdersByUserActivity(ctx context.Context, userID string, activityID int64) (int64, error)

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

	// UpdateTeamCounters 原子更新团计数器。
	// lockDelta/compleDelta 为增量值（+1 或 -1），用 gorm.Expr 保证原子操作。
	UpdateTeamCounters(ctx context.Context, teamID string, lockDelta, completeDelta int) error

	// UpdateTeamStatus 更新团状态。
	// 成团（0→1）、失败（0→2）、成团含退款（1→3）。
	UpdateTeamStatus(ctx context.Context, teamID string, status int8) error

	// --- 超时扫描 ---

	// FindTimeoutOrders 查询超时未支付的订单列表。
	// JOIN orders + teams：orders.status=0（锁定中）且 teams.valid_end < NOW()。
	// 定时任务游标分页扫描，每次限量处理。
	FindTimeoutOrders(ctx context.Context, limit int, lastID uint64) ([]TimeoutOrder, error)

	// FindOrdersByTeamID 查团下所有订单。
	// 用于成团后构建回调 payload。
	FindOrdersByTeamID(ctx context.Context, teamID string) ([]model.Order, error)
}

// TimeoutOrder 超时订单联表查询结果。
// 包含订单信息和团有效期，用于超时退单处理。
type TimeoutOrder struct {
	model.Order
	TeamValidEnd string `gorm:"column:team_valid_end"` // teams.valid_end
}

// 业务错误（哨兵错误，service 层判断用）
var (
	ErrTeamFull  = fmt.Errorf("team is full")
	ErrOrderDup  = fmt.Errorf("order duplicate")
	ErrTeamDup   = fmt.Errorf("team duplicate")
)

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

// --- 超时扫描 ---

func (r *orderRepo) FindTimeoutOrders(ctx context.Context, limit int, lastID uint64) ([]TimeoutOrder, error) {
	// 游标分页：用 orders.id > lastID 代替 OFFSET，避免大偏移量性能问题。
	// 联表找 orders.status=0 且 teams.valid_end < NOW() 的订单。
	var orders []TimeoutOrder
	err := r.db.WithContext(ctx).
		Table("orders").
		Select("orders.*, teams.valid_end AS team_valid_end").
		Joins("JOIN teams ON teams.team_id = orders.team_id").
		Where("orders.status = ?", model.OrderStatusLocked).
		Where("orders.id > ?", lastID).
		Where("teams.valid_end < NOW()").
		Order("orders.id ASC").
		Limit(limit).
		Find(&orders).Error
	if err != nil {
		return nil, fmt.Errorf("find timeout orders: %w", err)
	}
	return orders, nil
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
