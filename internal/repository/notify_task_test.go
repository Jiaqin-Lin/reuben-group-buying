package repository

import (
	"context"
	"testing"

	"github.com/reuben/group-buying/internal/model"
)

func TestNotifyTask_CreateAndFindByUUID(t *testing.T) {
	if testDB == nil {
		t.Skip("mysql not available")
	}
	repo := NewNotifyTaskRepo(testDB)
	ctx := context.Background()

	task := &model.NotifyTask{
		ActivityID:   100123,
		TeamID:       "T_NOTIFY_001",
		Category:     &[]string{model.NotifyCategorySettlement}[0],
		NotifyType:   model.NotifyTypeHTTP,
		NotifyTarget: &[]string{"http://example.com/callback"}[0],
		RetryCount:   0,
		Status:       model.NotifyStatusInit,
		Payload:      `{"team_id":"T_NOTIFY_001","orders":["ORD001","ORD002"]}`,
		UUID:         "uuid-test-001",
	}

	err := repo.CreateNotifyTask(ctx, task)
	if err != nil {
		t.Fatalf("CreateNotifyTask: %v", err)
	}

	got, err := repo.FindTaskByUUID(ctx, "uuid-test-001")
	if err != nil {
		t.Fatalf("FindTaskByUUID: %v", err)
	}
	if got.TeamID != "T_NOTIFY_001" {
		t.Errorf("expected team_id=T_NOTIFY_001, got %s", got.TeamID)
	}
}

func TestNotifyTask_FindPendingTasks(t *testing.T) {
	if testDB == nil {
		t.Skip("mysql not available")
	}
	repo := NewNotifyTaskRepo(testDB)
	ctx := context.Background()

	// 插入几个不同状态的任务
	testDB.Exec(`INSERT INTO notify_tasks (activity_id, team_id, category, notify_type, retry_count, status, payload, uuid) VALUES
		(100123, 'T_PEND_1', 'trade_settlement', 'HTTP', 0, 0, '{}', 'uuid-pend-1'),
		(100123, 'T_PEND_2', 'trade_settlement', 'HTTP', 0, 2, '{}', 'uuid-pend-2'),
		(100123, 'T_PEND_3', 'trade_settlement', 'HTTP', 0, 1, '{}', 'uuid-pend-3')`)

	// status=0 或 2 的任务应被扫到；status=1（已完成）的不会
	tasks, err := repo.FindPendingTasks(ctx, 100, 0)
	if err != nil {
		t.Fatalf("FindPendingTasks: %v", err)
	}

	found1, found2, found3 := false, false, false
	for _, task := range tasks {
		switch task.UUID {
		case "uuid-pend-1":
			found1 = true
		case "uuid-pend-2":
			found2 = true
		case "uuid-pend-3":
			found3 = true
		}
	}
	if !found1 {
		t.Errorf("status=0 task should be found")
	}
	if !found2 {
		t.Errorf("status=2 (retry) task should be found")
	}
	if found3 {
		t.Errorf("status=1 (success) task should NOT be found")
	}
}

func TestNotifyTask_UpdateStatus(t *testing.T) {
	if testDB == nil {
		t.Skip("mysql not available")
	}
	repo := NewNotifyTaskRepo(testDB)
	ctx := context.Background()

	// 创建一个任务（先查 UUID 获取实际 ID，避免硬编码）
	testDB.Exec(`INSERT INTO notify_tasks (activity_id, team_id, category, notify_type, retry_count, status, payload, uuid) VALUES
		(100123, 'T_UPD', 'trade_settlement', 'HTTP', 0, 0, '{}', 'uuid-upd-001')`)

	task, err := repo.FindTaskByUUID(ctx, "uuid-upd-001")
	if err != nil {
		t.Fatalf("find task by uuid: %v", err)
	}
	taskID := task.ID

	// 模拟重试失败
	err = repo.UpdateTaskStatus(ctx, taskID, model.NotifyStatusRetry, 1)
	if err != nil {
		t.Fatalf("UpdateTaskStatus (retry): %v", err)
	}

	task, err = repo.FindTaskByUUID(ctx, "uuid-upd-001")
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if task.Status != model.NotifyStatusRetry {
		t.Errorf("expected status=2, got %d", task.Status)
	}
	if task.RetryCount != 1 {
		t.Errorf("expected retry_count=1, got %d", task.RetryCount)
	}

	// 模拟最终失败
	err = repo.UpdateTaskStatus(ctx, taskID, model.NotifyStatusFail, 5)
	if err != nil {
		t.Fatalf("UpdateTaskStatus (fail): %v", err)
	}
}

func TestNotifyTask_CursorPagination(t *testing.T) {
	if testDB == nil {
		t.Skip("mysql not available")
	}
	repo := NewNotifyTaskRepo(testDB)
	ctx := context.Background()

	// 插入 5 个待处理任务
	for i := range 5 {
		testDB.Exec(`INSERT INTO notify_tasks (activity_id, team_id, category, notify_type, retry_count, status, payload, uuid) VALUES
			(100123, ?, 'trade_settlement', 'HTTP', 0, 0, '{}', ?)`,
			"T_CURSOR", "uuid-cursor-"+string(rune('a'+i)),
		)
	}

	// 第一次扫 2 条
	page1, err := repo.FindPendingTasks(ctx, 2, 0)
	if err != nil {
		t.Fatalf("page1: %v", err)
	}
	if len(page1) != 2 {
		t.Errorf("expected 2 tasks in page1, got %d", len(page1))
	}

	// 第二次从 page1 最后一条的 id 继续
	lastID := page1[1].ID
	page2, err := repo.FindPendingTasks(ctx, 10, lastID)
	if err != nil {
		t.Fatalf("page2: %v", err)
	}
	if len(page2) < 1 {
		t.Error("expected more tasks in page2")
	}

	// 确认 page2 的 id 都大于 lastID
	for _, task := range page2 {
		if task.ID <= lastID {
			t.Errorf("cursor violation: id=%d <= lastID=%d", task.ID, lastID)
		}
	}
}
