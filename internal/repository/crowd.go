package repository

import (
	"context"
	"fmt"

	"github.com/reuben/group-buying/internal/model"
	"gorm.io/gorm"
)

// CrowdRepository 人群标签相关数据访问接口。
//
// 人群标签有两个使用场景：
//  1. 活动级（activities.tag_id + tag_scope）：控制谁可见活动、谁可参与
//  2. 折扣级（discounts.tag_id + discount_type=1）：特定人群额外优惠
//
// tag_scope 语义（2位字符串）：
//   - 第1位 '1' = 可见限制（不在人群中看不到活动）
//   - 第2位 '1' = 参与限制（不在人群中不能锁单）
type CrowdRepository interface {
	// FindCrowdTagByID 按 tag_id 查人群标签定义。
	FindCrowdTagByID(ctx context.Context, tagID string) (*model.CrowdTag, error)

	// IsUserInCrowd 判断用户是否在人群标签中。
	// 用于试算/锁单时的人群限制校验。
	IsUserInCrowd(ctx context.Context, tagID, userID string) (bool, error)

	// FindCrowdTagDetails 查询标签下所有用户。
	// 用于缓存预热：加载到 Redis BitSet（bgm:tag:{tagId}:members）。
	FindCrowdTagDetails(ctx context.Context, tagID string) ([]model.CrowdTagDetail, error)

	// FindCrowdTagJobs 按状态查询人群计算任务。
	// 用于定时任务调度：找到待执行的任务。
	FindCrowdTagJobsByStatus(ctx context.Context, status int8) ([]model.CrowdTagJob, error)

	// UpdateCrowdTagJobStatus 更新人群计算任务状态。
	UpdateCrowdTagJobStatus(ctx context.Context, batchID string, status int8) error

	// UpdateCrowdTagStatistics 更新人群标签统计数。
	UpdateCrowdTagStatistics(ctx context.Context, tagID string, count int) error
}

// crowdRepo GORM 实现。
type crowdRepo struct {
	db *gorm.DB
}

// NewCrowdRepo 构造函数。
func NewCrowdRepo(db *gorm.DB) CrowdRepository {
	return &crowdRepo{db: db}
}

// FindCrowdTagByID 按 tag_id 查人群标签。
func (r *crowdRepo) FindCrowdTagByID(ctx context.Context, tagID string) (*model.CrowdTag, error) {
	var ct model.CrowdTag
	err := r.db.WithContext(ctx).Where("tag_id = ?", tagID).First(&ct).Error
	if err != nil {
		return nil, fmt.Errorf("find crowd tag %s: %w", tagID, err)
	}
	return &ct, nil
}

// IsUserInCrowd 判断用户是否在人群标签中。
// 查询 crowd_tag_details 表的 UK (tag_id, user_id)。
func (r *crowdRepo) IsUserInCrowd(ctx context.Context, tagID, userID string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.CrowdTagDetail{}).
		Where("tag_id = ? AND user_id = ?", tagID, userID).
		Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("check user in crowd (%s,%s): %w", tagID, userID, err)
	}
	return count > 0, nil
}

// FindCrowdTagDetails 查询标签下所有用户。
func (r *crowdRepo) FindCrowdTagDetails(ctx context.Context, tagID string) ([]model.CrowdTagDetail, error) {
	var details []model.CrowdTagDetail
	err := r.db.WithContext(ctx).
		Where("tag_id = ?", tagID).
		Find(&details).Error
	if err != nil {
		return nil, fmt.Errorf("find crowd tag details %s: %w", tagID, err)
	}
	return details, nil
}

// FindCrowdTagJobsByStatus 按状态查询人群计算任务。
func (r *crowdRepo) FindCrowdTagJobsByStatus(ctx context.Context, status int8) ([]model.CrowdTagJob, error) {
	var jobs []model.CrowdTagJob
	err := r.db.WithContext(ctx).
		Where("status = ?", status).
		Find(&jobs).Error
	if err != nil {
		return nil, fmt.Errorf("find crowd tag jobs by status %d: %w", status, err)
	}
	return jobs, nil
}

// UpdateCrowdTagJobStatus 更新任务状态。
func (r *crowdRepo) UpdateCrowdTagJobStatus(ctx context.Context, batchID string, status int8) error {
	result := r.db.WithContext(ctx).
		Model(&model.CrowdTagJob{}).
		Where("batch_id = ?", batchID).
		Update("status", status)
	if result.Error != nil {
		return fmt.Errorf("update crowd tag job %s: %w", batchID, result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("update crowd tag job %s: %w", batchID, gorm.ErrRecordNotFound)
	}
	return nil
}

// UpdateCrowdTagStatistics 更新人群标签统计数。
func (r *crowdRepo) UpdateCrowdTagStatistics(ctx context.Context, tagID string, count int) error {
	result := r.db.WithContext(ctx).
		Model(&model.CrowdTag{}).
		Where("tag_id = ?", tagID).
		Update("statistics", count)
	if result.Error != nil {
		return fmt.Errorf("update crowd tag statistics %s: %w", tagID, result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("update crowd tag statistics %s: %w", tagID, gorm.ErrRecordNotFound)
	}
	return nil
}
