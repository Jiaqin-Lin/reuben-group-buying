package service

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/reuben/group-buying/internal/infra/mq"
	"github.com/reuben/group-buying/internal/model"
	"github.com/reuben/group-buying/internal/repository"
)

// testLogger 返回丢弃所有输出的 logger（测试用）。
func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// newTestNotifyService 创建测试用 NotifyService。
func newTestNotifyService(t *testing.T) *NotifyService {
	t.Helper()
	if testDB == nil || testRDB == nil {
		t.Skip("mysql or redis not available")
	}
	mqClient := mq.New(testRDB, testLogger())
	return NewNotifyService(
		repository.NewNotifyTaskRepo(testDB),
		repository.NewRedisCacheRepo(testRDB),
		mqClient,
		NotifyServiceConfig{
			MaxRetry:    3,
			BatchSize:   10,
			Concurrency: 5,
		},
	)
}

// clearNotifyTestData 清理通知测试产生的数据。
func clearNotifyTestData(t *testing.T) {
	t.Helper()
	if testDB == nil {
		return
	}
	testDB.Exec("DELETE FROM notify_tasks")
	if testRDB != nil {
		ctx := context.Background()
		keys, _ := testRDB.Keys(ctx, "bgm:*").Result()
		for _, k := range keys {
			testRDB.Del(ctx, k)
		}
	}
}

// TestNotifyHTTPSuccess 测试 HTTP 回调成功（200 → status=1）。
func TestNotifyHTTPSuccess(t *testing.T) {
	defer clearNotifyTestData(t)
	svc := newTestNotifyService(t)
	ctx := context.Background()

	// 启动 mock HTTP 服务器，捕获请求
	var receivedPayload string
	var receivedContentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedContentType = r.Header.Get("Content-Type")
		buf := make([]byte, 1024)
		n, _ := r.Body.Read(buf)
		receivedPayload = string(buf[:n])
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// 创建 HTTP 类型的待发送任务
	target := srv.URL + "/callback/team"
	category := model.NotifyCategorySettlement
	task := &model.NotifyTask{
		ActivityID:   800001,
		TeamID:       "TEAM_NTF_001",
		Category:     &category,
		NotifyType:   model.NotifyTypeHTTP,
		NotifyTarget: &target,
		RetryCount:   0,
		Status:       model.NotifyStatusInit,
		Payload:      `{"teamId":"TEAM_NTF_001","outTradeNoList":["EXT001"]}`,
		UUID:         "uuid-http-success-001",
	}
	repo := repository.NewNotifyTaskRepo(testDB)
	if err := repo.CreateNotifyTask(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	// 执行扫描
	if err := svc.ExecPendingTasks(ctx); err != nil {
		t.Fatalf("exec pending tasks: %v", err)
	}

	// 验证：HTTP 服务器收到了请求
	if receivedContentType != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", receivedContentType)
	}
	if receivedPayload != task.Payload {
		t.Errorf("payload mismatch: got %q, want %q", receivedPayload, task.Payload)
	}

	// 验证：任务状态变为成功
	result, err := repo.FindTaskByUUID(ctx, "uuid-http-success-001")
	if err != nil {
		t.Fatalf("find task: %v", err)
	}
	if result.Status != model.NotifyStatusSuccess {
		t.Errorf("expected status=%d (Success), got %d", model.NotifyStatusSuccess, result.Status)
	}
}

// TestNotifyHTTPFail 测试 HTTP 回调失败（500 → status=2, retry_count+1）。
func TestNotifyHTTPFail(t *testing.T) {
	defer clearNotifyTestData(t)
	svc := newTestNotifyService(t)
	ctx := context.Background()

	// mock 服务器返回 500
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	target := srv.URL + "/callback"
	category := model.NotifyCategorySettlement
	task := &model.NotifyTask{
		ActivityID:   800002,
		TeamID:       "TEAM_NTF_002",
		Category:     &category,
		NotifyType:   model.NotifyTypeHTTP,
		NotifyTarget: &target,
		RetryCount:   0,
		Status:       model.NotifyStatusInit,
		Payload:      `{"teamId":"TEAM_NTF_002"}`,
		UUID:         "uuid-http-fail-002",
	}
	repo := repository.NewNotifyTaskRepo(testDB)
	if err := repo.CreateNotifyTask(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	if err := svc.ExecPendingTasks(ctx); err != nil {
		t.Fatalf("exec pending tasks: %v", err)
	}

	result, err := repo.FindTaskByUUID(ctx, "uuid-http-fail-002")
	if err != nil {
		t.Fatalf("find task: %v", err)
	}
	if result.Status != model.NotifyStatusRetry {
		t.Errorf("expected status=%d (Retry), got %d", model.NotifyStatusRetry, result.Status)
	}
	if result.RetryCount != 1 {
		t.Errorf("expected retry_count=1, got %d", result.RetryCount)
	}
}

// TestNotifyHTTPExhausted 测试重试次数耗尽（已达 max retry → status=3）。
func TestNotifyHTTPExhausted(t *testing.T) {
	defer clearNotifyTestData(t)
	svc := newTestNotifyService(t)
	ctx := context.Background()

	// mock 服务器返回 500
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	target := srv.URL + "/callback"
	category := model.NotifyCategorySettlement
	// retry_count 已经是 maxRetry-1 = 2，再失败一次就耗尽
	task := &model.NotifyTask{
		ActivityID:   800003,
		TeamID:       "TEAM_NTF_003",
		Category:     &category,
		NotifyType:   model.NotifyTypeHTTP,
		NotifyTarget: &target,
		RetryCount:   2, // maxRetry=3, 所以这是最后一次重试机会
		Status:       model.NotifyStatusRetry,
		Payload:      `{"teamId":"TEAM_NTF_003"}`,
		UUID:         "uuid-http-exhausted-003",
	}
	repo := repository.NewNotifyTaskRepo(testDB)
	if err := repo.CreateNotifyTask(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	if err := svc.ExecPendingTasks(ctx); err != nil {
		t.Fatalf("exec pending tasks: %v", err)
	}

	result, err := repo.FindTaskByUUID(ctx, "uuid-http-exhausted-003")
	if err != nil {
		t.Fatalf("find task: %v", err)
	}
	if result.Status != model.NotifyStatusFail {
		t.Errorf("expected status=%d (Fail), got %d", model.NotifyStatusFail, result.Status)
	}
	if result.RetryCount != 3 {
		t.Errorf("expected retry_count=3, got %d", result.RetryCount)
	}
}

// TestNotifyMQ 测试 MQ 回调成功。
func TestNotifyMQ(t *testing.T) {
	defer clearNotifyTestData(t)
	svc := newTestNotifyService(t)
	ctx := context.Background()

	channel := "topic_team_notify_test"
	payload := `{"teamId":"TEAM_NTF_MQ01","outTradeNoList":["EXT_MQ01"]}`

	// 订阅 channel，验证能收到消息
	var received atomic.Bool
	ready, err := svc.mqClient.Subscribe(ctx, channel, func(msg []byte) error {
		if string(msg) == payload {
			received.Store(true)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	<-ready                           // 等待订阅就绪
	time.Sleep(50 * time.Millisecond) // 给 Redis 一点时间

	target := channel
	category := model.NotifyCategorySettlement
	task := &model.NotifyTask{
		ActivityID:   800004,
		TeamID:       "TEAM_NTF_MQ01",
		Category:     &category,
		NotifyType:   model.NotifyTypeMQ,
		NotifyTarget: &target,
		RetryCount:   0,
		Status:       model.NotifyStatusInit,
		Payload:      payload,
		UUID:         "uuid-mq-success-004",
	}
	repo := repository.NewNotifyTaskRepo(testDB)
	if err := repo.CreateNotifyTask(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	if err := svc.ExecPendingTasks(ctx); err != nil {
		t.Fatalf("exec pending tasks: %v", err)
	}

	// 等消息到达
	time.Sleep(200 * time.Millisecond)

	if !received.Load() {
		t.Error("MQ message was not received by subscriber")
	}

	// 验证任务状态变为成功
	result, err := repo.FindTaskByUUID(ctx, "uuid-mq-success-004")
	if err != nil {
		t.Fatalf("find task: %v", err)
	}
	if result.Status != model.NotifyStatusSuccess {
		t.Errorf("expected status=%d (Success), got %d", model.NotifyStatusSuccess, result.Status)
	}
}

// TestNotifyEmptyTarget 测试空 URL/channel 直接视为成功。
func TestNotifyEmptyTarget(t *testing.T) {
	defer clearNotifyTestData(t)
	svc := newTestNotifyService(t)
	ctx := context.Background()

	category := model.NotifyCategorySettlement
	task := &model.NotifyTask{
		ActivityID:   800005,
		TeamID:       "TEAM_NTF_EMPTY",
		Category:     &category,
		NotifyType:   model.NotifyTypeHTTP,
		NotifyTarget: nil, // 空 URL
		RetryCount:   0,
		Status:       model.NotifyStatusInit,
		Payload:      `{"teamId":"TEAM_NTF_EMPTY"}`,
		UUID:         "uuid-empty-target-005",
	}
	repo := repository.NewNotifyTaskRepo(testDB)
	if err := repo.CreateNotifyTask(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	if err := svc.ExecPendingTasks(ctx); err != nil {
		t.Fatalf("exec pending tasks: %v", err)
	}

	result, err := repo.FindTaskByUUID(ctx, "uuid-empty-target-005")
	if err != nil {
		t.Fatalf("find task: %v", err)
	}
	if result.Status != model.NotifyStatusSuccess {
		t.Errorf("expected status=%d (Success) for empty target, got %d", model.NotifyStatusSuccess, result.Status)
	}
}

// TestNotifySkipsCompleted 测试已成功的任务不会被重复处理。
func TestNotifySkipsCompleted(t *testing.T) {
	defer clearNotifyTestData(t)
	svc := newTestNotifyService(t)
	ctx := context.Background()

	category := model.NotifyCategorySettlement
	task := &model.NotifyTask{
		ActivityID:   800006,
		TeamID:       "TEAM_NTF_SKIP",
		Category:     &category,
		NotifyType:   model.NotifyTypeHTTP,
		NotifyTarget: nil,
		RetryCount:   0,
		Status:       model.NotifyStatusSuccess, // 已成功
		Payload:      `{"teamId":"TEAM_NTF_SKIP"}`,
		UUID:         "uuid-already-success-006",
	}
	repo := repository.NewNotifyTaskRepo(testDB)
	if err := repo.CreateNotifyTask(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	if err := svc.ExecPendingTasks(ctx); err != nil {
		t.Fatalf("exec pending tasks: %v", err)
	}

	// 状态应保持 success，retry_count 不变
	result, err := repo.FindTaskByUUID(ctx, "uuid-already-success-006")
	if err != nil {
		t.Fatalf("find task: %v", err)
	}
	if result.Status != model.NotifyStatusSuccess {
		t.Errorf("status changed from Success to %d", result.Status)
	}
	if result.RetryCount != 0 {
		t.Errorf("retry_count changed from 0 to %d", result.RetryCount)
	}
}

// TestExecPendingTasksBatch 测试批量处理多个任务。
func TestExecPendingTasksBatch(t *testing.T) {
	defer clearNotifyTestData(t)
	svc := newTestNotifyService(t)
	ctx := context.Background()

	// mock HTTP 服务器（记录请求次数）
	var requestCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	repo := repository.NewNotifyTaskRepo(testDB)

	// 创建 5 个 HTTP 待发送任务
	for i := 1; i <= 5; i++ {
		target := fmt.Sprintf("%s/callback/%d", srv.URL, i)
		category := model.NotifyCategorySettlement
		task := &model.NotifyTask{
			ActivityID:   int64(800010 + i),
			TeamID:       fmt.Sprintf("TEAM_BATCH_%02d", i),
			Category:     &category,
			NotifyType:   model.NotifyTypeHTTP,
			NotifyTarget: &target,
			RetryCount:   0,
			Status:       model.NotifyStatusInit,
			Payload:      fmt.Sprintf(`{"teamId":"TEAM_BATCH_%02d"}`, i),
			UUID:         fmt.Sprintf("uuid-batch-%02d", i),
		}
		if err := repo.CreateNotifyTask(ctx, task); err != nil {
			t.Fatalf("create task %d: %v", i, err)
		}
	}

	if err := svc.ExecPendingTasks(ctx); err != nil {
		t.Fatalf("exec pending tasks: %v", err)
	}

	// 验证所有 5 个请求都发送了
	if count := requestCount.Load(); count != 5 {
		t.Errorf("expected 5 HTTP requests, got %d", count)
	}

	// 验证所有任务状态都是 success
	for i := 1; i <= 5; i++ {
		uuid := fmt.Sprintf("uuid-batch-%02d", i)
		result, err := repo.FindTaskByUUID(ctx, uuid)
		if err != nil {
			t.Errorf("find task %s: %v", uuid, err)
			continue
		}
		if result.Status != model.NotifyStatusSuccess {
			t.Errorf("task %s: expected status=%d, got %d", uuid, model.NotifyStatusSuccess, result.Status)
		}
	}
}

// TestNotifyScannerLock 测试分布式锁：同时两个扫描器只有一个能执行。
func TestNotifyScannerLock(t *testing.T) {
	defer clearNotifyTestData(t)
	svc := newTestNotifyService(t)
	ctx := context.Background()

	// 先手动获取扫描器锁
	lock, acquired, err := svc.cacheRepo.AcquireLock(ctx, notifyScannerLockKey, notifyScannerLockTTL)
	if err != nil {
		t.Fatalf("acquire lock: %v", err)
	}
	if !acquired {
		t.Fatal("first acquire should succeed")
	}
	defer lock.Release(ctx)

	// 锁已被持有，ExecPendingTasks 应该直接跳过（不报错）
	if err := svc.ExecPendingTasks(ctx); err != nil {
		t.Fatalf("exec pending tasks should not error when lock held: %v", err)
	}
	// 成功：不报错，只是 skip
}

// TestNotifyHTTPConnectionRefused 测试连接失败（无法连接的 URL → retry）。
func TestNotifyHTTPConnectionRefused(t *testing.T) {
	defer clearNotifyTestData(t)
	svc := newTestNotifyService(t)
	ctx := context.Background()

	// 使用一个几乎不可能在监听的端口
	target := "http://127.0.0.1:19999/callback"
	category := model.NotifyCategorySettlement
	task := &model.NotifyTask{
		ActivityID:   800007,
		TeamID:       "TEAM_NTF_CONN",
		Category:     &category,
		NotifyType:   model.NotifyTypeHTTP,
		NotifyTarget: &target,
		RetryCount:   0,
		Status:       model.NotifyStatusInit,
		Payload:      `{"teamId":"TEAM_NTF_CONN"}`,
		UUID:         "uuid-conn-refused-007",
	}
	repo := repository.NewNotifyTaskRepo(testDB)
	if err := repo.CreateNotifyTask(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	if err := svc.ExecPendingTasks(ctx); err != nil {
		t.Fatalf("exec pending tasks: %v", err)
	}

	result, err := repo.FindTaskByUUID(ctx, "uuid-conn-refused-007")
	if err != nil {
		t.Fatalf("find task: %v", err)
	}
	if result.Status != model.NotifyStatusRetry {
		t.Errorf("expected status=%d (Retry) for connection refused, got %d", model.NotifyStatusRetry, result.Status)
	}
	if result.RetryCount != 1 {
		t.Errorf("expected retry_count=1, got %d", result.RetryCount)
	}
}
