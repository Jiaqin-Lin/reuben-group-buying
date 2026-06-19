// Package app 应用组装层。
// 职责：加载配置 → 初始化基础设施 → 依赖注入 → 注册路由 → 启动 HTTP 服务。
// main 包只调用 app.New(cfg).Run()，不感知任何内部组件。
package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/robfig/cron/v3"

	"github.com/reuben/group-buying/internal/config"
	"github.com/reuben/group-buying/internal/handler"
	"github.com/reuben/group-buying/internal/infra/log"
	"github.com/reuben/group-buying/internal/infra/mq"
	"github.com/reuben/group-buying/internal/infra/mysql"
	"github.com/reuben/group-buying/internal/infra/redis"
	"github.com/reuben/group-buying/internal/middleware/logging"
	"github.com/reuben/group-buying/internal/middleware/recovery"
	"github.com/reuben/group-buying/internal/middleware/tracing"
	"github.com/reuben/group-buying/internal/repository"
	"github.com/reuben/group-buying/internal/service"
)

// App 聚合所有运行时组件。
type App struct {
	cfg       *config.Config
	logger    *slog.Logger
	srv       *http.Server
	notifySvc *service.NotifyService
}

// New 从配置构建完整的 App。
// 包含：日志、MySQL、Redis、Repository、Service、Handler、路由、中间件。
func New(cfg *config.Config) (*App, error) {
	// 1. 初始化日志
	logger := log.New(cfg.Log.Level, cfg.Log.Format)
	slog.SetDefault(logger)

	// 2. 初始化 MySQL
	db, err := mysql.New(mysql.Config{
		DSN:             cfg.MySQL.DSN(),
		MaxOpenConns:    cfg.MySQL.MaxOpenConns,
		MaxIdleConns:    cfg.MySQL.MaxIdleConns,
		ConnMaxLifetime: cfg.MySQL.ConnMaxLifetime,
	}, logger)
	if err != nil {
		return nil, fmt.Errorf("init mysql: %w", err)
	}

	// 3. 初始化 Redis
	rdb, err := redis.New(redis.Config{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	}, logger)
	if err != nil {
		return nil, fmt.Errorf("init redis: %w", err)
	}

	// 4. 组装 Repository 层
	activityRepo := repository.NewActivityRepo(db)
	productRepo := repository.NewProductRepo(db)
	crowdRepo := repository.NewCrowdRepo(db)
	orderRepo := repository.NewOrderRepo(db)
	paymentRepo := repository.NewPaymentRepo(db)
	notifyRepo := repository.NewNotifyTaskRepo(db)
	cacheRepo := repository.NewRedisCacheRepo(rdb)

	// 5. 组装 Service 层
	trialSvc := service.NewTrialService(activityRepo, productRepo, cacheRepo, crowdRepo)
	lockSvc := service.NewLockService(
		trialSvc, orderRepo, activityRepo, cacheRepo,
		time.Duration(cfg.App.OrderLockTTL)*time.Second,
		time.Duration(cfg.App.LockResultTTL)*time.Second,
	)
	settlementSvc := service.NewSettlementService(orderRepo, activityRepo, cacheRepo, notifyRepo)
	refundSvc := service.NewRefundService(orderRepo, paymentRepo, cacheRepo, notifyRepo)

	// 8. 组装 MQ 和 Notify 服务（Phase 8）
	mqClient := mq.New(rdb, logger)
	notifySvc := service.NewNotifyService(
		notifyRepo, cacheRepo, mqClient,
		service.NotifyServiceConfig{
			MaxRetry: cfg.App.NotifyMaxRetry,
		},
	)

	// 6. 组装 Handler 层
	indexHandler := handler.NewIndexHandler(trialSvc)
	tradeHandler := handler.NewTradeHandler(lockSvc, settlementSvc, refundSvc)

	// 7. 构建路由
	gin.SetMode(cfg.Server.Mode)
	router := gin.New()
	router.Use(tracing.Middleware())
	router.Use(logging.Middleware(logger))
	router.Use(recovery.Middleware(logger))

	router.GET("/health", healthHandler(db, rdb))
	router.POST("/api/v1/trial", indexHandler.Trial)
	router.POST("/api/v1/trade/lock", tradeHandler.LockOrder)
	router.POST("/api/v1/trade/settlement", tradeHandler.Settlement)
	router.POST("/api/v1/trade/refund", tradeHandler.Refund)

	return &App{
		cfg:       cfg,
		logger:    logger,
		notifySvc: notifySvc,
		srv: &http.Server{
			Addr:         fmt.Sprintf(":%d", cfg.Server.Port),
			Handler:      router,
			ReadTimeout:  cfg.Server.ReadTimeout,
			WriteTimeout: cfg.Server.WriteTimeout,
		},
	}, nil
}

// Run 启动 HTTP 服务、定时任务，并等待退出信号，收到 SIGINT/SIGTERM 后优雅关闭。
func (a *App) Run() error {
	// 启动 HTTP 服务
	go func() {
		a.logger.Info("server starting", "port", a.cfg.Server.Port, "mode", a.cfg.Server.Mode)
		if err := a.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			a.logger.Error("server listen", "error", err)
			os.Exit(1)
		}
	}()

	// 启动定时任务：每 15s 扫描待发送的回调通知
	c := cron.New()
	_, err := c.AddFunc("@every 15s", func() {
		ctx := context.Background()
		if err := a.notifySvc.ExecPendingTasks(ctx); err != nil {
			a.logger.Error("notify cron job failed", "error", err)
		}
	})
	if err != nil {
		return fmt.Errorf("add cron job: %w", err)
	}
	c.Start()
	a.logger.Info("cron jobs started", "jobs", "notify-scanner@15s")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	a.logger.Info("server shutting down")

	// 先停定时任务
	cronCtx := c.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := a.srv.Shutdown(ctx); err != nil {
		a.logger.Error("server shutdown", "error", err)
		return err
	}

	// 等待 cron 任务执行完毕
	<-cronCtx.Done()

	a.logger.Info("server stopped")
	return nil
}

// healthHandler 健康检查，验证 MySQL 和 Redis 连接。
func healthHandler(_, _ any) gin.HandlerFunc {
	return func(c *gin.Context) {
		// TODO: 实际检查 db.Ping() 和 rdb.Ping()
		c.JSON(http.StatusOK, gin.H{
			"status": "ok",
			"time":   time.Now().Format(time.RFC3339),
		})
	}
}
