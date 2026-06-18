package model

import "time"

// CrowdTagJob 人群标签任务（定时计算人群并写入 Redis BitSet）
type CrowdTagJob struct {
	ID            uint64    `gorm:"primaryKey;autoIncrement" json:"-"`
	TagID         string    `gorm:"type:varchar(32);not null;column:tag_id" json:"tag_id"`
	BatchID       string    `gorm:"type:varchar(16);uniqueIndex;not null;column:batch_id" json:"batch_id"`
	TagType       int8      `gorm:"type:tinyint;not null;default:1;column:tag_type" json:"tag_type"`
	TagRule       string    `gorm:"type:varchar(16);not null;column:tag_rule" json:"tag_rule"`
	StatStartTime time.Time `gorm:"type:datetime;not null;column:stat_start_time" json:"stat_start_time"`
	StatEndTime   time.Time `gorm:"type:datetime;not null;column:stat_end_time" json:"stat_end_time"`
	Status        int8      `gorm:"type:tinyint;not null;default:0" json:"status"`
	CreatedAt     time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt     time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (CrowdTagJob) TableName() string { return "crowd_tag_jobs" }

// 标签任务状态常量
const (
	TagJobStatusInit    = 0 // 初始
	TagJobStatusRunning = 1 // 执行中
	TagJobStatusReset   = 2 // 重置
	TagJobStatusDone    = 3 // 完成
)

// 标签类型常量
const (
	TagTypeParticipate = 1 // 参与量
	TagTypeConsume     = 2 // 消费金额
)
