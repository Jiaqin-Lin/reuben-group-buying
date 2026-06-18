package model

import "time"

// CrowdTagDetail 人群标签明细（tag_id + user_id 唯一）
type CrowdTagDetail struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement" json:"-"`
	TagID     string    `gorm:"type:varchar(32);uniqueIndex:uk_tag_user;not null;column:tag_id" json:"tag_id"`
	UserID    string    `gorm:"type:varchar(64);uniqueIndex:uk_tag_user;not null;column:user_id" json:"user_id"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (CrowdTagDetail) TableName() string { return "crowd_tag_details" }
