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
	"runtime"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	goredis "github.com/redis/go-redis/v9"
	"github.com/robfig/cron/v3"
	"gorm.io/gorm"

	"github.com/reuben/group-buying/internal/cache"
	"github.com/reuben/group-buying/internal/config"
	"github.com/reuben/group-buying/internal/config/dynamic"
	"github.com/reuben/group-buying/internal/handler"
	"github.com/reuben/group-buying/internal/infra/log"
	"github.com/reuben/group-buying/internal/infra/mq"
	"github.com/reuben/group-buying/internal/infra/mysql"
	"github.com/reuben/group-buying/internal/infra/redis"
	"github.com/reuben/group-buying/internal/middleware/admin"
	"github.com/reuben/group-buying/internal/middleware/logging"
	"github.com/reuben/group-buying/internal/middleware/recovery"
	"github.com/reuben/group-buying/internal/middleware/tracing"
	"github.com/reuben/group-buying/internal/pay"
	"github.com/reuben/group-buying/internal/redisx"
	"github.com/reuben/group-buying/internal/repository"
	"github.com/reuben/group-buying/internal/service"
)

// App 聚合所有运行时组件。
type App struct {
	cfg        *config.Config
	logger     *slog.Logger
	srv        *http.Server
	notifySvc  *service.NotifyService
	timeoutSvc *service.TimeoutService
	dynMgr     *dynamic.Manager
	localCache *cache.LocalCache
	ctx        context.Context    // app 级 context，cron job 和 shutdown 使用
	cancel     context.CancelFunc // 触发 graceful shutdown 时 cancel
}

// metrics 全局计数器（atomic，无锁）。
var (
	metricReqTotal  atomic.Int64
	metricReqTrial  atomic.Int64
	metricReqLock   atomic.Int64
	metricReqSettle atomic.Int64
	metricReqRefund atomic.Int64
	metricErr5xx    atomic.Int64
	metricErr4xx    atomic.Int64
	metricCacheHit  atomic.Int64
	metricCacheMiss atomic.Int64
	startTime       = time.Now()
)

// New 从配置构建完整的 App。
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

	// 4. 本地内存缓存（Phase 10：活动/折扣/商品/映射全量加载）
	localCache := cache.New(db)
	if err := localCache.Load(context.Background()); err != nil {
		return nil, fmt.Errorf("init local cache: %w", err)
	}

	// 5. 组装 Repository 层
	activityRepo := repository.NewActivityRepo(db)
	productRepo := repository.NewProductRepo(db)
	crowdRepo := repository.NewCrowdRepo(db)
	orderRepo := repository.NewOrderRepo(db)
	paymentRepo := repository.NewPaymentRepo(db)
	notifyRepo := repository.NewNotifyTaskRepo(db)
	cacheRepo := repository.NewRedisCacheRepo(rdb)

	// 6. 选择支付网关：支付宝配置了 app_id 则用真实网关，否则降级 Mock
	var payGateway pay.Gateway
	if cfg.Alipay.AppID != "" {
		alipayGW, err := pay.NewAlipay(pay.AlipayConfig{
			AppID:           cfg.Alipay.AppID,
			PrivateKey:      cfg.Alipay.PrivateKey,
			AlipayPublicKey: cfg.Alipay.AlipayPublicKey,
			NotifyURL:       cfg.Alipay.NotifyURL,
			ReturnURL:       cfg.Alipay.ReturnURL,
			Sandbox:         cfg.Alipay.Sandbox,
			SignType:        cfg.Alipay.SignType,
		})
		if err != nil {
			return nil, fmt.Errorf("init alipay gateway: %w", err)
		}
		payGateway = alipayGW
		logger.Info("alipay gateway enabled", "app_id", cfg.Alipay.AppID, "sandbox", cfg.Alipay.Sandbox)
	} else {
		payGateway = pay.NewMock()
		logger.Info("using mock payment gateway (alipay.app_id not configured)")
	}

	// 7. 组装 Service 层（注入 localCache + payment）
	trialSvc := service.NewTrialService(activityRepo, productRepo, cacheRepo, crowdRepo, localCache)
	lockSvc := service.NewLockService(
		trialSvc, orderRepo, activityRepo, cacheRepo, localCache,
		payGateway, paymentRepo,
		time.Duration(cfg.App.OrderLockTTL)*time.Second,
		time.Duration(cfg.App.LockResultTTL)*time.Second,
	)
	settlementSvc := service.NewSettlementService(orderRepo, activityRepo, cacheRepo, notifyRepo, localCache)
	refundSvc := service.NewRefundService(orderRepo, paymentRepo, cacheRepo, notifyRepo, payGateway)

	// 8. 组装 MQ 和 Notify 服务（Phase 8）
	mqClient := mq.New(rdb, logger)
	notifySvc := service.NewNotifyService(
		notifyRepo, cacheRepo, mqClient,
		service.NotifyServiceConfig{
			MaxRetry: cfg.App.NotifyMaxRetry,
		},
	)

	// 9. 动态配置（Phase 9a）
	dynMgr := dynamic.New(db, rdb, redisx.ConfigChannel(), logger)

	if err := dynMgr.Register(
		dynamic.TrialCacheTTL,
		dynamic.LockResultTTL,
		dynamic.OrderLockTTL,
		dynamic.NotifyMaxRetry,
		dynamic.NotifyWorkerCount,
		dynamic.TimeoutScanBatch,
		dynamic.FeatureSkipCrowd,
		dynamic.FeatureSkipPayment,
	); err != nil {
		return nil, fmt.Errorf("register dynamic configs: %w", err)
	}

	if err := dynMgr.Load(context.Background()); err != nil {
		return nil, fmt.Errorf("load dynamic configs: %w", err)
	}
	logger.Info("dynamic config loaded", "keys", 8)

	// 10. 超时退单扫描（Phase 9b）
	timeoutSvc := service.NewTimeoutService(
		orderRepo, refundSvc, rdb,
		service.TimeoutServiceConfig{
			BatchSize: dynamic.TimeoutScanBatch.Get(),
		},
		logger,
	)

	// 10. 支付回调 Handler
	payHandler := handler.NewPayHandler(payGateway, paymentRepo, orderRepo, settlementSvc)

	// 11. 组装 Handler 层
	indexHandler := handler.NewIndexHandler(trialSvc, orderRepo)
	tradeHandler := handler.NewTradeHandler(lockSvc, settlementSvc, refundSvc)
	adminHandler := handler.NewAdminHandler(dynMgr)

	// 管理端 CRUD Handler
	adminActivityHandler := handler.NewAdminActivityHandler(db)
	adminProductHandler := handler.NewAdminProductHandler(db)
	adminActivityProductHandler := handler.NewAdminActivityProductHandler(db)
	adminCrowdHandler := handler.NewAdminCrowdHandler(db)
	adminOrderHandler := handler.NewAdminOrderHandler(db)
	adminTeamHandler := handler.NewAdminTeamHandler(db)
	adminDashboardHandler := handler.NewAdminDashboardHandler(db)

	// 12. 构建路由
	gin.SetMode(cfg.Server.Mode)
	router := gin.New()
	router.Use(tracing.Middleware())
	router.Use(bodyLimitMiddleware(1 << 20)) // 1MB request body limit
	router.Use(charsetMiddleware())
	router.Use(metricsMiddleware())
	router.Use(logging.Middleware(logger))
	router.Use(recovery.Middleware(logger))

	router.GET("/health", healthHandler(db, rdb, localCache))
	router.GET("/metrics", metricsHandler())
	router.GET("/ready", readyHandler(db, rdb))
	router.POST("/api/v1/trial", indexHandler.Trial)
	router.POST("/api/v1/trade/lock", tradeHandler.LockOrder)
	router.POST("/api/v1/trade/settlement", tradeHandler.Settlement)
	router.POST("/api/v1/trade/refund", tradeHandler.Refund)
	router.POST("/api/v1/pay/notify", payHandler.Notify)
	router.GET("/api/v1/pay/qr", payHandler.ShowQR)

	// 用户端 — 拼团列表 & 详情
	router.GET("/api/v1/teams", indexHandler.ListTeams)
	router.GET("/api/v1/teams/:team_id", indexHandler.GetTeam)

	// 用户端 — 支付查询
	router.GET("/api/v1/payments/:out_trade_no", payHandler.GetPayment)

	// 用户端 — 订单查询
	router.GET("/api/v1/orders", adminOrderHandler.ListOrdersByUser)
	router.GET("/api/v1/orders/:out_trade_no", adminOrderHandler.GetOrderByOutTradeNo)

	// 管理端 — 统一挂载 admin 鉴权中间件
	adminGroup := router.Group("/api/v1/admin")
	adminGroup.Use(admin.Middleware(cfg.App.AdminToken))
	{
		// 动态配置
		adminGroup.GET("/configs", adminHandler.ListConfigs)
		adminGroup.PUT("/configs/:key", adminHandler.UpdateConfig)

		// 仪表盘
		adminGroup.GET("/dashboard", adminDashboardHandler.GetDashboard)

		// 活动 & 折扣
		adminGroup.GET("/activities", adminActivityHandler.ListActivities)
		adminGroup.GET("/activities/:id", adminActivityHandler.GetActivity)
		adminGroup.POST("/activities", adminActivityHandler.CreateActivity)
		adminGroup.PUT("/activities/:id", adminActivityHandler.UpdateActivity)
		adminGroup.DELETE("/activities/:id", adminActivityHandler.DeleteActivity)
		adminGroup.GET("/discounts", adminActivityHandler.ListDiscounts)
		adminGroup.GET("/discounts/:id", adminActivityHandler.GetDiscount)
		adminGroup.POST("/discounts", adminActivityHandler.CreateDiscount)
		adminGroup.PUT("/discounts/:id", adminActivityHandler.UpdateDiscount)
		adminGroup.DELETE("/discounts/:id", adminActivityHandler.DeleteDiscount)

		// 商品
		adminGroup.GET("/products", adminProductHandler.ListProducts)
		adminGroup.POST("/products", adminProductHandler.CreateProduct)
		adminGroup.PUT("/products/:goods_id", adminProductHandler.UpdateProduct)
		adminGroup.DELETE("/products/:goods_id", adminProductHandler.DeleteProduct)

		// 活动-商品映射
		adminGroup.GET("/activity-products", adminActivityProductHandler.ListActivityProducts)
		adminGroup.POST("/activity-products", adminActivityProductHandler.CreateActivityProduct)
		adminGroup.DELETE("/activity-products", adminActivityProductHandler.DeleteActivityProduct)

		// 人群标签
		adminGroup.GET("/crowd-tags", adminCrowdHandler.ListTags)
		adminGroup.GET("/crowd-tags/:tag_id", adminCrowdHandler.GetTag)
		adminGroup.POST("/crowd-tags", adminCrowdHandler.CreateTag)
		adminGroup.PUT("/crowd-tags/:tag_id", adminCrowdHandler.UpdateTag)
		adminGroup.DELETE("/crowd-tags/:tag_id", adminCrowdHandler.DeleteTag)
		adminGroup.GET("/crowd-tags/:tag_id/members", adminCrowdHandler.ListMembers)
		adminGroup.POST("/crowd-tags/:tag_id/members", adminCrowdHandler.AddMember)
		adminGroup.DELETE("/crowd-tags/:tag_id/members/:user_id", adminCrowdHandler.RemoveMember)

		// 订单监控
		adminGroup.GET("/orders", adminOrderHandler.ListOrders)
		adminGroup.GET("/orders/:order_id", adminOrderHandler.GetOrder)

		// 队伍监控
		adminGroup.GET("/teams", adminTeamHandler.ListTeams)
		adminGroup.GET("/teams/:team_id", adminTeamHandler.GetTeam)
		adminGroup.GET("/teams/:team_id/orders", adminTeamHandler.GetTeamOrders)
	}

	// 静态文件 + SPA fallback（生产模式：编译前 npm run build 到 web/dist/ 即可）
	distPath := "web/dist"
	if _, err := os.Stat(distPath); err == nil {
		router.Static("/assets", distPath+"/assets")
		router.StaticFile("/favicon.svg", distPath+"/favicon.svg")
		router.NoRoute(func(c *gin.Context) {
			// API 路径返回 JSON 404
			if strings.HasPrefix(c.Request.URL.Path, "/api/") {
				c.JSON(http.StatusNotFound, gin.H{"code": "0005", "info": "not found"})
				return
			}
			// SPA fallback：所有其他路径返回 index.html
			c.File(distPath + "/index.html")
		})
		logger.Info("frontend static files served", "path", distPath)
	}

	return &App{
		cfg:        cfg,
		logger:     logger,
		notifySvc:  notifySvc,
		timeoutSvc: timeoutSvc,
		dynMgr:     dynMgr,
		localCache: localCache,
		srv: &http.Server{
			Addr:         fmt.Sprintf(":%d", cfg.Server.Port),
			Handler:      router,
			ReadTimeout:  cfg.Server.ReadTimeout,
			WriteTimeout: cfg.Server.WriteTimeout,
		},
	}, nil
}

// Run 启动 HTTP 服务、定时任务，并等待退出信号。
func (a *App) Run() error {
	// 创建 app 级 context，cron job 和 shutdown 共用
	a.ctx, a.cancel = context.WithCancel(context.Background())
	defer a.cancel()

	// 启动动态配置 Watch（Redis Pub/Sub）
	a.dynMgr.Watch(context.Background())
	defer a.dynMgr.Stop()

	// 启动 HTTP 服务
	go func() {
		a.logger.Info("server starting", "port", a.cfg.Server.Port, "mode", a.cfg.Server.Mode)
		if err := a.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			a.logger.Error("server listen", "error", err)
			os.Exit(1)
		}
	}()

	// 启动定时任务（使用 app 级 ctx，shutdown 时 cancel 可中断正在运行的 job）
	c := cron.New()

	// 每 15s 扫描待发送的回调通知
	_, err := c.AddFunc("@every 15s", func() {
		if err := a.notifySvc.ExecPendingTasks(a.ctx); err != nil {
			a.logger.Error("notify cron job failed", "error", err)
		}
	})
	if err != nil {
		return fmt.Errorf("add notify cron job: %w", err)
	}

	// 超时退单扫描
	timeoutSpec := fmt.Sprintf("@every %ds", a.cfg.App.TimeoutScanInterval)
	_, err = c.AddFunc(timeoutSpec, func() {
		scanned, refunded, failed, scanErr := a.timeoutSvc.ScanAndRefund(a.ctx)
		if scanErr != nil {
			a.logger.Error("timeout scan cron failed", "error", scanErr)
		} else if scanned > 0 {
			a.logger.Info("timeout scan done", "scanned", scanned, "refunded", refunded, "failed", failed)
		}
	})
	if err != nil {
		return fmt.Errorf("add timeout cron job: %w", err)
	}

	// 本地缓存定时刷新（每 5 分钟）
	_, err = c.AddFunc("@every 300s", func() {
		if err := a.localCache.Load(a.ctx); err != nil {
			a.logger.Error("local cache refresh failed", "error", err)
		}
	})
	if err != nil {
		return fmt.Errorf("add cache refresh cron job: %w", err)
	}

	c.Start()
	a.logger.Info("cron jobs started",
		"jobs", fmt.Sprintf("notify-scanner@15s, timeout-scanner@%s, cache-refresh@300s", timeoutSpec),
	)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	a.logger.Info("server shutting down")

	// 1. 先 cancel app context → 正在运行的 cron job 收到 ctx.Done() 尽快退出
	a.cancel()

	// 2. 停止 cron scheduler（等待正在运行的 job 完成）
	cronCtx := c.Stop()

	// 3. HTTP server graceful shutdown（10s 超时）
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := a.srv.Shutdown(ctx); err != nil {
		a.logger.Error("server shutdown", "error", err)
		return err
	}

	// 4. 等待 cron job 全部完成
	<-cronCtx.Done()

	a.logger.Info("server stopped")
	return nil
}

// healthHandler 健康检查，验证 MySQL、Redis 和本地缓存。
func healthHandler(db *gorm.DB, rdb *goredis.Client, lc *cache.LocalCache) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		status := "ok"
		httpStatus := http.StatusOK
		details := gin.H{
			"mysql": "ok",
			"redis": "ok",
			"cache": "ok",
			"time":  time.Now().Format(time.RFC3339),
		}

		// Ping MySQL
		sqlDB, err := db.DB()
		if err != nil {
			details["mysql"] = "error"
			status = "degraded"
		} else if err := sqlDB.PingContext(ctx); err != nil {
			details["mysql"] = fmt.Sprintf("error: %v", err)
			status = "degraded"
		}

		// Ping Redis
		if err := rdb.Ping(ctx).Err(); err != nil {
			details["redis"] = fmt.Sprintf("error: %v", err)
			status = "degraded"
		}

		// 检查本地缓存是否已加载（activities > 0 表示已加载）
		stats := lc.Stats()
		if stats.Activities == 0 {
			details["cache"] = "stale (no data loaded)"
			status = "degraded"
		}

		if status != "ok" {
			httpStatus = http.StatusServiceUnavailable
		}
		c.JSON(httpStatus, gin.H{"status": status, "details": details})
	}
}

// readyHandler 就绪检查，只有 MySQL 和 Redis 都连通才返回 200。
// K8s readinessProbe 用此端点：不通过则摘除 Pod，防止流量打到不可用实例。
func readyHandler(db *gorm.DB, rdb *goredis.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()

		// MySQL must be reachable
		sqlDB, err := db.DB()
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "not ready",
				"reason": fmt.Sprintf("mysql connection pool: %v", err),
				"time":   time.Now().Format(time.RFC3339),
			})
			return
		}
		if err := sqlDB.PingContext(ctx); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "not ready",
				"reason": fmt.Sprintf("mysql unreachable: %v", err),
				"time":   time.Now().Format(time.RFC3339),
			})
			return
		}

		// Redis must be reachable
		if err := rdb.Ping(ctx).Err(); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "not ready",
				"reason": fmt.Sprintf("redis unreachable: %v", err),
				"time":   time.Now().Format(time.RFC3339),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{"status": "ready", "time": time.Now().Format(time.RFC3339)})
	}
}

// metricsHandler Prometheus 兼容的指标端点。
func metricsHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		uptime := time.Since(startTime).Seconds()
		var mem runtime.MemStats
		runtime.ReadMemStats(&mem)

		c.String(http.StatusOK,
			`# HELP groupbuy_uptime_seconds Server uptime in seconds
# TYPE groupbuy_uptime_seconds gauge
groupbuy_uptime_seconds %.0f

# HELP groupbuy_requests_total Total requests
# TYPE groupbuy_requests_total counter
groupbuy_requests_total %d
groupbuy_requests_total{endpoint="trial"} %d
groupbuy_requests_total{endpoint="lock"} %d
groupbuy_requests_total{endpoint="settlement"} %d
groupbuy_requests_total{endpoint="refund"} %d

# HELP groupbuy_errors_total Total errors
# TYPE groupbuy_errors_total counter
groupbuy_errors_total{code="4xx"} %d
groupbuy_errors_total{code="5xx"} %d

# HELP groupbuy_cache_total Cache operations
# TYPE groupbuy_cache_total counter
groupbuy_cache_total{result="hit"} %d
groupbuy_cache_total{result="miss"} %d

# HELP groupbuy_memory_bytes Memory usage
# TYPE groupbuy_memory_bytes gauge
groupbuy_memory_bytes{type="alloc"} %d
groupbuy_memory_bytes{type="sys"} %d
groupbuy_memory_bytes{type="heap_objects"} %d
`,
			uptime,
			metricReqTotal.Load(),
			metricReqTrial.Load(),
			metricReqLock.Load(),
			metricReqSettle.Load(),
			metricReqRefund.Load(),
			metricErr4xx.Load(),
			metricErr5xx.Load(),
			metricCacheHit.Load(),
			metricCacheMiss.Load(),
			mem.Alloc,
			mem.Sys,
			mem.HeapObjects,
		)
	}
}


// bodyLimitMiddleware 限制请求体大小，防止大 payload 导致 OOM。
func bodyLimitMiddleware(maxBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		c.Next()
	}
}

// metricsMiddleware 请求计数中间件。
func metricsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		metricReqTotal.Add(1)

		c.Next()

		// 按路径分类
		switch {
		case c.FullPath() == "/api/v1/trial":
			metricReqTrial.Add(1)
		case c.FullPath() == "/api/v1/trade/lock":
			metricReqLock.Add(1)
		case c.FullPath() == "/api/v1/trade/settlement":
			metricReqSettle.Add(1)
		case c.FullPath() == "/api/v1/trade/refund":
			metricReqRefund.Add(1)
		}

		// 错误分类
		status := c.Writer.Status()
		if status >= 500 {
			metricErr5xx.Add(1)
		} else if status >= 400 {
			metricErr4xx.Add(1)
		}
	}
}

// IncrCacheHit 缓存命中计数（供 service 层调用）。
func IncrCacheHit() { metricCacheHit.Add(1) }

// IncrCacheMiss 缓存未命中计数（供 service 层调用）。
func IncrCacheMiss() { metricCacheMiss.Add(1) }

// charsetMiddleware 确保所有响应都带上 charset=utf-8。
// Gin 的 c.JSON/c.String 已自带 charset，但静态文件服务（c.File/router.Static）
// 依赖 http.ServeContent 的 MIME 检测，部分浏览器可能解析异常。
// 此中间件为所有 text/* 类型响应补充 charset=utf-8。
func charsetMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		ct := c.Writer.Header().Get("Content-Type")
		if ct != "" && !strings.Contains(ct, "charset") {
			if strings.HasPrefix(ct, "text/") ||
				strings.HasPrefix(ct, "application/json") ||
				strings.HasPrefix(ct, "application/javascript") {
				c.Writer.Header().Set("Content-Type", ct+"; charset=utf-8")
			}
		}
	}
}
