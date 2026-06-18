package repository

import (
	"context"
	"testing"

	"github.com/reuben/group-buying/internal/model"
)

func TestCrowdRepo_FindByID(t *testing.T) {
	if testDB == nil {
		t.Skip("mysql not available")
	}
	repo := NewCrowdRepo(testDB)
	ctx := context.Background()

	ct, err := repo.FindCrowdTagByID(ctx, "TAG001")
	if err != nil {
		t.Fatalf("FindCrowdTagByID: %v", err)
	}
	if ct.TagName != "VIP用户" {
		t.Errorf("expected tag_name=VIP用户, got %s", ct.TagName)
	}
}

func TestCrowdRepo_IsUserInCrowd(t *testing.T) {
	if testDB == nil {
		t.Skip("mysql not available")
	}
	repo := NewCrowdRepo(testDB)
	ctx := context.Background()

	// USER999 在 TAG001 中（seed data）
	in, err := repo.IsUserInCrowd(ctx, "TAG001", "USER999")
	if err != nil {
		t.Fatalf("IsUserInCrowd: %v", err)
	}
	if !in {
		t.Error("USER999 should be in TAG001")
	}

	// USER000 不在 TAG001 中
	in, err = repo.IsUserInCrowd(ctx, "TAG001", "USER000")
	if err != nil {
		t.Fatalf("IsUserInCrowd (not in): %v", err)
	}
	if in {
		t.Error("USER000 should NOT be in TAG001")
	}
}

func TestCrowdRepo_FindCrowdTagDetails(t *testing.T) {
	if testDB == nil {
		t.Skip("mysql not available")
	}
	repo := NewCrowdRepo(testDB)
	ctx := context.Background()

	// 多插入几个用户
	testDB.Exec(`INSERT INTO crowd_tag_details (tag_id, user_id) VALUES
		('TAG001', 'USER998'), ('TAG001', 'USER997')
		ON DUPLICATE KEY UPDATE user_id=user_id`)

	details, err := repo.FindCrowdTagDetails(ctx, "TAG001")
	if err != nil {
		t.Fatalf("FindCrowdTagDetails: %v", err)
	}
	if len(details) < 3 { // seed 1 + 2 new
		t.Errorf("expected at least 3 details, got %d", len(details))
	}
}

func TestCrowdRepo_Jobs(t *testing.T) {
	if testDB == nil {
		t.Skip("mysql not available")
	}
	repo := NewCrowdRepo(testDB)
	ctx := context.Background()

	// 插入测试任务
	testDB.Exec(`INSERT INTO crowd_tag_jobs (tag_id, batch_id, tag_type, tag_rule, stat_start_time, stat_end_time, status) VALUES
		('TAG001', 'BATCH001', 1, 'VIP>=3', NOW(), NOW(), 0)`)

	jobs, err := repo.FindCrowdTagJobsByStatus(ctx, model.TagJobStatusInit)
	if err != nil {
		t.Fatalf("FindCrowdTagJobsByStatus: %v", err)
	}
	if len(jobs) < 1 {
		t.Fatal("expected at least 1 job")
	}

	// 更新状态
	err = repo.UpdateCrowdTagJobStatus(ctx, "BATCH001", model.TagJobStatusRunning)
	if err != nil {
		t.Fatalf("UpdateCrowdTagJobStatus: %v", err)
	}

	// 更新统计
	err = repo.UpdateCrowdTagStatistics(ctx, "TAG001", 200)
	if err != nil {
		t.Fatalf("UpdateCrowdTagStatistics: %v", err)
	}

	ct, err := repo.FindCrowdTagByID(ctx, "TAG001")
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if ct.Statistics != 200 {
		t.Errorf("expected statistics=200, got %d", ct.Statistics)
	}
}
