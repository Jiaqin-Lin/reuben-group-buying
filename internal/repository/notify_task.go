package repository

import (
	"context"
	"fmt"

	"github.com/reuben/group-buying/internal/model"
	"gorm.io/gorm"
)

// NotifyTaskRepository 回调通知任务的数据访问接口。
//
// 回调通知是成团后的关键流程：
//  1. 结算时创建 notify_task（status=0 待发送）
//  2. 定时任务游标扫描待发送/重试中的任务
//  3. 发送成功后 status=1，失败则 retry_count++ 并置 status=2（重试中）
//  4. 超过最大重试次数（5次）后置 status=3（失败）
type NotifyTaskRepository interface {
	// CreateNotifyTask 创建通知任务。
	// UUID 用于幂等去重，防止成团后重复创建回调任务。
	CreateNotifyTask(ctx context.Context, task *model.NotifyTask) error

	// FindPendingTasks 游标分页扫描待处理任务。
	// status=0（待发送）或 status=2（重试中），按 id 升序，避免大 OFFSET。
	// lastID: 上一页最后一条的 id，首次传 0。
	FindPendingTasks(ctx context.Context, limit int, lastID uint64) ([]model.NotifyTask, error)

	// UpdateTaskStatus 更新任务状态和重试次数。
	// 发送成功：status=1
	// 发送失败未达上限：status=2, retry_count + 1
	// 发送失败达上限：status=3
	UpdateTaskStatus(ctx context.Context, taskID uint64, status int8, retryCount int) error

	// FindTaskByUUID 按 UUID 查任务，用于去重。
	FindTaskByUUID(ctx context.Context, uuid string) (*model.NotifyTask, error)
}

// notifyTaskRepo GORM 实现。
type notifyTaskRepo struct {
	db *gorm.DB
}

// NewNotifyTaskRepo 构造函数。
func NewNotifyTaskRepo(db *gorm.DB) NotifyTaskRepository {
	return &notifyTaskRepo{db: db}
}

// CreateNotifyTask 创建通知任务。
// UUID 有 UK 约束，重复插入会被数据库拒绝。
func (r *notifyTaskRepo) CreateNotifyTask(ctx context.Context, task *model.NotifyTask) error {
	err := r.db.WithContext(ctx).Create(task).Error
	if err != nil {
		return fmt.Errorf("create notify task %s: %w", task.UUID, err)
	}
	return nil
}

// FindPendingTasks 游标分页扫描待处理任务。
// WHERE (status = 0 OR status = 2) AND id > lastID ORDER BY id ASC LIMIT limit
// 游标分页好处：即使有新任务插入，也不会导致重复或漏掉（比 OFFSET 更稳定）。
func (r *notifyTaskRepo) FindPendingTasks(ctx context.Context, limit int, lastID uint64) ([]model.NotifyTask, error) {
	var tasks []model.NotifyTask
	err := r.db.WithContext(ctx).
		Where("(status = ? OR status = ?)", model.NotifyStatusInit, model.NotifyStatusRetry).
		Where("id > ?", lastID).
		Order("id ASC").
		Limit(limit).
		Find(&tasks).Error
	if err != nil {
		return nil, fmt.Errorf("find pending notify tasks: %w", err)
	}
	return tasks, nil
}

// UpdateTaskStatus 更新任务状态和重试次数。
// 原子更新，避免并发重复发送。
func (r *notifyTaskRepo) UpdateTaskStatus(ctx context.Context, taskID uint64, status int8, retryCount int) error {
	result := r.db.WithContext(ctx).
		Model(&model.NotifyTask{}).
		Where("id = ?", taskID).
		Updates(map[string]any{
			"status":      status,
			"retry_count": retryCount,
		})
	if result.Error != nil {
		return fmt.Errorf("update notify task %d: %w", taskID, result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("update notify task %d: %w", taskID, gorm.ErrRecordNotFound)
	}
	return nil
}

// FindTaskByUUID 按 UUID 查任务。
// 用于成团回调去重：同一次成团不应创建多个回调任务。
func (r *notifyTaskRepo) FindTaskByUUID(ctx context.Context, uuid string) (*model.NotifyTask, error) {
	var task model.NotifyTask
	err := r.db.WithContext(ctx).Where("uuid = ?", uuid).First(&task).Error
	if err != nil {
		return nil, fmt.Errorf("find notify task by uuid %s: %w", uuid, err)
	}
	return &task, nil
}
