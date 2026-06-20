package service

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/reuben/group-buying/internal/infra/mq"
	"github.com/reuben/group-buying/internal/model"
	"github.com/reuben/group-buying/internal/repository"
)

// NotifyService 回调通知执行服务。
//
// 职责：扫描 pending 状态的 notify_task，通过 HTTP 或 MQ 发送回调，
// 并根据结果更新任务状态（成功/重试/失败）。
//
// 与 Java 版差异：
//   - 仅 cron 扫描（不用 eager dispatch），更简单
//   - 仅全局分布式锁防多实例重复扫描（不用 per-team 双重锁）
//   - MQ 用 Redis Pub/Sub（不用 RocketMQ）
type NotifyService struct {
	notifyRepo  repository.NotifyTaskRepository
	cacheRepo   repository.CacheRepository
	mqClient    *mq.Client
	httpClient  *http.Client
	maxRetry    int
	batchSize   int
	concurrency int
}

// NotifyServiceConfig 配置项。
type NotifyServiceConfig struct {
	MaxRetry    int // 最大重试次数（默认 5）
	BatchSize   int // 每批扫描数量（默认 100）
	Concurrency int // 并发发送数（默认 10）
}

// DefaultNotifyConfig 返回默认配置。
func DefaultNotifyConfig() NotifyServiceConfig {
	return NotifyServiceConfig{
		MaxRetry:    5,
		BatchSize:   100,
		Concurrency: 10,
	}
}

// NewNotifyService 构造函数。
func NewNotifyService(
	notifyRepo repository.NotifyTaskRepository,
	cacheRepo repository.CacheRepository,
	mqClient *mq.Client,
	cfg NotifyServiceConfig,
) *NotifyService {
	if cfg.MaxRetry <= 0 {
		cfg.MaxRetry = 5
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 100
	}
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 10
	}

	return &NotifyService{
		notifyRepo: notifyRepo,
		cacheRepo:  cacheRepo,
		mqClient:   mqClient,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
		maxRetry:    cfg.MaxRetry,
		batchSize:   cfg.BatchSize,
		concurrency: cfg.Concurrency,
	}
}

const (
	notifyScannerLockKey = "bgm:lock:notify:scanner"
	notifyScannerLockTTL = 30 * time.Second
)

// ExecPendingTasks 扫描并执行所有待处理的回调任务。
//
// 由 cron job 调用（每 15s 一次）。用分布式锁防多实例同时扫描。
// 游标分页扫描 status=0（待发送）或 status=2（重试中）的任务，
// 每批并发发送，全部处理完才返回。
func (s *NotifyService) ExecPendingTasks(ctx context.Context) error {
	// 获取分布式锁（watch-dog 自动续期，防止大批量通知扫描超过 30s TTL）
	lock, acquired, err := s.cacheRepo.AcquireLockWithExtend(ctx, notifyScannerLockKey, notifyScannerLockTTL)
	if err != nil {
		return fmt.Errorf("notify scanner: acquire lock: %w", err)
	}
	if !acquired {
		slog.DebugContext(ctx, "notify scanner: lock held by another instance, skipping")
		return nil
	}
	defer func() {
		if err := lock.Release(ctx); err != nil {
			slog.WarnContext(ctx, "notify scanner: release lock failed", "error", err)
		}
	}()

	var totalProcessed, totalSuccess, totalFailed int
	var lastID uint64

	for {
		tasks, err := s.notifyRepo.FindPendingTasks(ctx, s.batchSize, lastID)
		if err != nil {
			slog.ErrorContext(ctx, "notify scanner: find pending tasks failed", "last_id", lastID, "error", err)
			break
		}
		if len(tasks) == 0 {
			break
		}

		processed, succeeded, failed := s.execBatch(ctx, tasks)
		totalProcessed += processed
		totalSuccess += succeeded
		totalFailed += failed

		lastID = tasks[len(tasks)-1].ID

		slog.DebugContext(ctx, "notify scanner: batch done",
			"batch_size", len(tasks),
			"processed", processed,
			"succeeded", succeeded,
			"failed", failed,
			"last_id", lastID,
		)
	}

	if totalProcessed > 0 {
		slog.InfoContext(ctx, "notify scanner: all done",
			"total_processed", totalProcessed,
			"total_success", totalSuccess,
			"total_failed", totalFailed,
		)
	}

	return nil
}

// execBatch 并发执行一批通知任务。
// 返回 (processed, succeeded, failed) 计数。
func (s *NotifyService) execBatch(ctx context.Context, tasks []model.NotifyTask) (processed, succeeded, failed int) {
	sem := make(chan struct{}, s.concurrency)
	results := make(chan bool, len(tasks)) // true=success, false=fail

	for i := range tasks {
		task := tasks[i] // capture for goroutine
		sem <- struct{}{}

		go func() {
			defer func() { <-sem }()
			ok := s.execOneTask(ctx, &task)
			results <- ok
		}()
	}

	// 等待所有任务完成
	for range tasks {
		if <-results {
			succeeded++
		} else {
			failed++
		}
		processed++
	}

	return
}

// execOneTask 执行单个通知任务并更新状态。
//
// 流程：
//  1. dispatch（HTTP POST 或 MQ publish）
//  2. 成功 → status=1（Success）
//  3. 失败 → retry_count+1：
//     - 未达上限 → status=2（Retry），等下次扫描
//     - 已达上限 → status=3（Fail）
func (s *NotifyService) execOneTask(ctx context.Context, task *model.NotifyTask) bool {
	err := s.dispatch(ctx, task)
	if err == nil {
		// 发送成功
		if updateErr := s.notifyRepo.UpdateTaskStatus(ctx, task.ID, model.NotifyStatusSuccess, task.RetryCount); updateErr != nil {
			slog.WarnContext(ctx, "notify: update success status failed", "task_id", task.ID, "error", updateErr)
		}
		return true
	}

	// 发送失败
	slog.WarnContext(ctx, "notify: dispatch failed",
		"task_id", task.ID,
		"team_id", task.TeamID,
		"notify_type", task.NotifyType,
		"retry_count", task.RetryCount,
		"error", err,
	)

	newRetryCount := task.RetryCount + 1
	if newRetryCount < s.maxRetry {
		if updateErr := s.notifyRepo.UpdateTaskStatus(ctx, task.ID, model.NotifyStatusRetry, newRetryCount); updateErr != nil {
			slog.WarnContext(ctx, "notify: update retry status failed", "task_id", task.ID, "error", updateErr)
		}
	} else {
		if updateErr := s.notifyRepo.UpdateTaskStatus(ctx, task.ID, model.NotifyStatusFail, newRetryCount); updateErr != nil {
			slog.WarnContext(ctx, "notify: update fail status failed", "task_id", task.ID, "error", updateErr)
		}
		slog.ErrorContext(ctx, "notify: max retry exhausted",
			"task_id", task.ID,
			"team_id", task.TeamID,
			"retry_count", newRetryCount,
		)
	}

	return false
}

// dispatch 按 NotifyType 分发执行回调。
//
// HTTP 模式：POST JSON 到 notify_url，2xx = 成功。
// MQ 模式：Publish JSON 到 Redis Pub/Sub channel。
func (s *NotifyService) dispatch(ctx context.Context, task *model.NotifyTask) error {
	switch task.NotifyType {
	case model.NotifyTypeHTTP:
		return s.httpNotify(ctx, task)
	case model.NotifyTypeMQ:
		return s.mqNotify(ctx, task)
	default:
		return fmt.Errorf("unknown notify type: %s", task.NotifyType)
	}
}

// httpNotify HTTP 回调通知。
//
// POST {notify_url}，Content-Type: application/json，body = task.Payload。
// 成功条件：HTTP 2xx 状态码。
// notify_url 为空时视为成功（直接跳过，避免阻塞重试队列）。
func (s *NotifyService) httpNotify(ctx context.Context, task *model.NotifyTask) error {
	target := ""
	if task.NotifyTarget != nil {
		target = *task.NotifyTarget
	}
	if target == "" {
		slog.DebugContext(ctx, "notify http: empty url, treating as success", "task_id", task.ID)
		return nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader([]byte(task.Payload)))
	if err != nil {
		return fmt.Errorf("http notify: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("http notify: %s: %w", target, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("http notify: %s: status %d", target, resp.StatusCode)
	}

	slog.DebugContext(ctx, "notify http success", "task_id", task.ID, "url", target, "status", resp.StatusCode)
	return nil
}

// mqNotify Redis Pub/Sub 回调通知。
//
// Publish 到 task.NotifyTarget（即 MQ topic/channel），payload 为消息体。
// notify_target 为空时视为成功。
func (s *NotifyService) mqNotify(ctx context.Context, task *model.NotifyTask) error {
	target := ""
	if task.NotifyTarget != nil {
		target = *task.NotifyTarget
	}
	if target == "" {
		slog.DebugContext(ctx, "notify mq: empty channel, treating as success", "task_id", task.ID)
		return nil
	}

	if err := s.mqClient.Publish(ctx, target, []byte(task.Payload)); err != nil {
		return fmt.Errorf("mq notify: %s: %w", target, err)
	}

	slog.DebugContext(ctx, "notify mq success", "task_id", task.ID, "channel", target)
	return nil
}

// NotifyError 通知业务错误。
type NotifyError struct {
	Code string
	Err  error
}

func (e *NotifyError) Error() string {
	return fmt.Sprintf("notify error [%s]: %v", e.Code, e.Err)
}

func (e *NotifyError) Unwrap() error { return e.Err }

// ErrorCode 返回业务错误码。
func (e *NotifyError) ErrorCode() string { return e.Code }
