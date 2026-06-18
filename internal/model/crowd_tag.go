package model

import "time"

// CrowdTag 人群标签
type CrowdTag struct {
	ID         uint64    `gorm:"primaryKey;autoIncrement" json:"-"`
	TagID      string    `gorm:"type:varchar(32);uniqueIndex;not null;column:tag_id" json:"tag_id"`
	TagName    string    `gorm:"type:varchar(64);not null;column:tag_name" json:"tag_name"`
	TagDesc    string    `gorm:"type:varchar(256);not null;default:'';column:tag_desc" json:"tag_desc"`
	Statistics int       `gorm:"type:int;not null;default:0" json:"statistics"`
	CreatedAt  time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt  time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (CrowdTag) TableName() string { return "crowd_tags" }
