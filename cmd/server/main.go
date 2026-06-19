// Package main 拼团营销系统入口。
// 职责：加载配置 → 初始化基础设施 → 注册中间件 → 注册路由 → 启动 HTTP 服务。
package main

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

	"github.com/reuben/group-buying/internal/config"
	"github.com/reuben/group-buying/internal/handler"
	"github.com/reuben/group-buying/internal/infra/log"
	"github.com/reuben/group-buying/internal/infra/mysql"
	"github.com/reuben/group-buying/internal/infra/redis"
	"github.com/reuben/group-buying/internal/middleware/logging"
	"github.com/reuben/group-buying/internal/middleware/recovery"
	"github.com/reuben/group-buying/internal/middleware/tracing"
	"github.com/reuben/group-buying/internal/repository"
	"github.com/reuben/group-buying/internal/service"
)

func main() {
	// 1. 加载配置
	cfg, err := config.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}

	// 2. 初始化日志
	logger := log.New(cfg.Log.Level, cfg.Log.Format)
	slog.SetDefault(logger) // 让 gorm/gin 等库也能用上

	// 3. 初始化 MySQL
	db, err := mysql.New(mysql.Config{
		DSN:             cfg.MySQL.DSN(),
		MaxOpenConns:    cfg.MySQL.MaxOpenConns,
		MaxIdleConns:    cfg.MySQL.MaxIdleConns,
		ConnMaxLifetime: cfg.MySQL.ConnMaxLifetime,
	}, logger)
	if err != nil {
		logger.Error("init mysql", "error", err)
		os.Exit(1)
	}
	// 4. 初始化 Redis
	rdb, err := redis.New(redis.Config{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	}, logger)
	if err != nil {
		logger.Error("init redis", "error", err)
		os.Exit(1)
	}

	// 5. 初始化 Repository 层
	activityRepo := repository.NewActivityRepo(db)
	productRepo := repository.NewProductRepo(db)
	crowdRepo := repository.NewCrowdRepo(db)
	orderRepo := repository.NewOrderRepo(db)
	paymentRepo := repository.NewPaymentRepo(db) // Phase 11 支付对接用
	notifyRepo := repository.NewNotifyTaskRepo(db)
	cacheRepo := repository.NewRedisCacheRepo(rdb)

	// 6. 初始化 Service 层
	trialSvc := service.NewTrialService(activityRepo, productRepo, cacheRepo, crowdRepo)
	lockSvc := service.NewLockService(
		trialSvc, orderRepo, activityRepo, cacheRepo,
		time.Duration(cfg.App.OrderLockTTL)*time.Second,
		time.Duration(cfg.App.LockResultTTL)*time.Second,
	)
	settlementSvc := service.NewSettlementService(orderRepo, activityRepo, cacheRepo, notifyRepo)
	refundSvc := service.NewRefundService(orderRepo, paymentRepo, cacheRepo, notifyRepo)

	// 7. 初始化 Handler 层
	indexHandler := handler.NewIndexHandler(trialSvc)
	tradeHandler := handler.NewTradeHandler(lockSvc, settlementSvc, refundSvc)

	// 8. 初始化 Gin 路由
	gin.SetMode(cfg.Server.Mode)
	router := gin.New()

	// 9. 注册全局中间件（按顺序）
	router.Use(tracing.Middleware())
	router.Use(logging.Middleware(logger))
	router.Use(recovery.Middleware(logger))

	// 10. 注册路由
	router.GET("/health", healthHandler(db, rdb))
	router.POST("/api/v1/trial", indexHandler.Trial)
	router.POST("/api/v1/trade/lock", tradeHandler.LockOrder)
	router.POST("/api/v1/trade/settlement", tradeHandler.Settlement)
	router.POST("/api/v1/trade/refund", tradeHandler.Refund)

	// 8. 启动 HTTP 服务（优雅退出）
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	go func() {
		logger.Info("server starting", "port", cfg.Server.Port, "mode", cfg.Server.Mode)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server listen", "error", err)
			os.Exit(1)
		}
	}()

	// 9. 等待退出信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("server shutting down")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("server shutdown", "error", err)
	}

	logger.Info("server stopped")
}

// healthHandler 健康检查，同时验证 MySQL 和 Redis 连接。
func healthHandler(db any, rdb any) gin.HandlerFunc {
	return func(c *gin.Context) {
		// TODO: 实际检查 db.Ping() 和 rdb.Ping()。Phase 2 接入具体类型后再完善。
		c.JSON(http.StatusOK, gin.H{
			"status": "ok",
			"time":   time.Now().Format(time.RFC3339),
		})
	}
}
