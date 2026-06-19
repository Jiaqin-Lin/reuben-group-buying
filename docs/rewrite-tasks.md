# Go 重写任务清单

> 基于 Java 参考项目 `group-shopping/` 的拼团营销系统，用 Go 重写。
> 核心原则：**简洁 > 设计模式、缓存优先、扁平化目录**。

---

## Phase 0：项目初始化 ✅

- [x] **0.1** 初始化 Go module（`go mod init`），选定项目名 `github.com/reuben/group-buying`
- [x] **0.2** 确定 Go 框架（gin + gorm + go-redis + viper + slog + robfig/cron）
- [x] **0.3** 搭建项目骨架（目录结构写入 CLAUDE.md，含 infra/ + middleware/ 子包拆分）
- [x] **0.4** 配置 Docker Compose 开发环境（MySQL 8.0 + Redis 7）
- [x] **0.5** 创建建表 SQL → `migrations/001_init.sql`（12 张表：7 核心 + 2 支付 + 3 人群标签；详见 CLAUDE.md 数据模型）
- [x] **0.6** 创建 Makefile（build / run / test / lint / dev / migrate）
- [x] **0.7** 创建 .gitignore
- [x] **0.8** 拆分中间件：HTTP 中间件（`middleware/tracing|logging|ratelimit|recovery`）+ 基础设施（`infra/mysql|redis|mq`）

### 建议 Go 项目结构

```
.
├── cmd/server/main.go          # 入口
├── internal/
│   ├── config/                 # 配置结构 & 加载
│   ├── handler/                # HTTP handler（对应 Java trigger）
│   │   ├── trade.go            # 锁单/结算/退单 接口
│   │   ├── index.go            # 活动首页/商品查询
│   │   └── admin.go            # 管理后台接口
│   ├── service/                # 业务逻辑层
│   │   ├── trial.go            # 试算
│   │   ├── lock.go             # 锁单
│   │   ├── settlement.go       # 结算
│   │   ├── refund.go           # 退单
│   │   ├── notify.go           # 回调通知
│   │   └── timeout.go          # 超时退单
│   ├── repository/             # 数据访问层
│   │   ├── activity.go         # 活动/折扣/活动-商品映射 查询
│   │   ├── order.go            # 订单 CRUD（orders + teams）
│   │   ├── product.go          # 商品查询
│   │   ├── notify_task.go      # 回调任务 CRUD
│   │   ├── crowd.go            # 人群标签 CRUD
│   │   ├── payment.go          # 支付 CRUD
│   │   └── cache.go            # 所有缓存读写操作
│   ├── model/                  # 数据模型（PO/VO 统一放这里）
│   ├── redisx/                 # Redis 操作封装
│   │   ├── stock.go            # 名额占用/释放（go:embed Lua）
│   │   ├── lock.go             # 分布式锁
│   │   ├── cache.go            # 缓存读写
│   │   ├── keys.go             # Redis Key 定义
│   │   └── lua/                # Lua 脚本文件
│   │       ├── occupy_stock.lua
│   │       ├── release_stock.lua
│   │       └── take_limit_incr.lua
│   ├── pay/                    # 支付网关对接（新增）
│   │   ├── gateway.go          # 支付接口定义
│   │   └── mock.go             # Mock 实现
│   └── middleware/             # 中间件（限流、日志、traceID）
├── migrations/                 # 数据库迁移文件
├── Dockerfile
├── docker-compose.yml
├── Makefile
└── CLAUDE.md                   # 项目文档
```

---

## Phase 1：基础设施搭建

- [x] **1.1** MySQL 连接池 + GORM 初始化 → `internal/infra/mysql/mysql.go`
- [x] **1.2** Redis 客户端初始化（推荐 go-redis）→ `internal/infra/redis/redis.go`
- [x] **1.3** 配置管理（环境变量 + YAML 配置文件）→ `internal/config/config.go` + `config.yaml`
- [x] **1.4** 日志框架（结构化日志，含 traceID）→ `internal/infra/log/log.go`
- [x] **1.5** 统一响应格式（对标 Java Response.java）→ `internal/response/response.go`
- [x] **1.6** 统一错误码（对标 Java ResponseCode.java）→ `internal/errcode/errcode.go`
- [x] **1.7** HTTP 中间件：traceID 注入、请求日志、panic recover → `internal/middleware/*/`

---

## Phase 2：数据层（Repository） — 先做无缓存的直读 ✅

- [x] **2.1** 定义所有数据模型 struct（对应 12 张表）→ `internal/model/*.go`
- [x] **2.2** 实现 activities 查询（按 activity_id）→ `repository/activity.go`
- [x] **2.3** 实现 discounts 查询（按 discount_id）→ `repository/activity.go`
- [x] **2.4** 实现 activity_products 查询（按 source+channel+goods_id）→ `repository/activity.go`
- [x] **2.5** 实现 products 查询（按 goods_id）→ `repository/product.go`
- [x] **2.6** 实现 orders CRUD（查询/插入/更新状态）→ `repository/order.go`
- [x] **2.7** 实现 teams CRUD（查询/插入/更新 lock_count/complete_count/status）→ `repository/order.go`
- [x] **2.8** 实现 notify_tasks CRUD（查询未完成/插入/更新状态）→ `repository/notify_task.go`
- [x] **2.9** 实现 crowd_tags + crowd_tag_details + crowd_tag_jobs 查询 → `repository/crowd.go`
- [x] **2.10** 实现 payments + payment_logs CRUD → `repository/payment.go`
- [x] **2.11** 定义缓存操作接口 → `repository/cache.go`（Phase 3 实现）
- [x] **2.12** 编写数据层集成测试（Docker MySQL，31 个测试全部通过）

---

## Phase 3：Redis 层 ✅

- [x] **3.1** 把 Java 代码中的 Lua 脚本提取到独立文件（go:embed 嵌入）
  - `internal/redisx/lua/occupy_stock.lua` — 原子占用名额（full 哨兵 + 幂等 + TTL 控制）
  - `internal/redisx/lua/release_stock.lua` — 原子释放名额（SREM + 解除满标）
  - `internal/redisx/lua/take_limit_incr.lua` — 原子限购检查+递增（冷启动安全）
- [x] **3.2** 实现名额占用函数 `TryOccupyStock(ctx, rdb, activityID, teamID, outTradeNo, targetCount, ttl)`
- [x] **3.3** 实现名额释放函数 `ReleaseStock(ctx, rdb, activityID, teamID, outTradeNo)`
- [x] **3.4** 实现分布式锁 `AcquireLock(ctx, rdb, key, ttl)` / `ReleaseLock(ctx, rdb, key, token)`（SETNX + Lua 安全释放）
- [x] **3.5** 实现锁单结果缓存 `CacheLockResult(ctx, ...)` / `GetLockResult(ctx, ...)` + 通用 CacheSet/Get/Del
- [x] **3.6** 实现用户限购计数 `TakeLimitCheckAndIncr` / `IncrTakeCount` / `GetTakeCount` / `InitTakeCount`
- [x] **3.7** Redis 层单元测试（30 个测试，含并发测试，-race 通过）

---

## Phase 4：试算（Trial） ✅

- [x] **4.1** 实现折扣计算逻辑（直减 ZJ、满减 MJ、折扣 ZK、N 元购）
- [x] **4.2** 实现试算主流程：
  1. 查 activity_products → 获取 activityId
  2. 查 activities + discounts（JOIN）→ 校验状态
  3. 计算折扣（switch planType: ZJ/MJ/ZK/N）
  4. 查 products → 获取商品信息
- [x] **4.3** 实现人群标签过滤（可选）
- [x] **4.4** 试算接口 HTTP handler：`POST /api/v1/trial`
- [x] **4.5** 试算单元测试（覆盖 4 种折扣类型）

---

## Phase 5：锁单（Lock Order）✅

- [x] **5.1** 实现锁单主流程（`service/lock.go`，约 260 行）：
  1. 幂等检查（缓存 → DB，三层防护：缓存/分布式锁/DB UK）
  2. 分布式锁（`bgm:lock:order:{userId}:{outTradeNo}`，3s TTL）
  3. Double-check 缓存（获取锁后再次查询，防并发窗口）
  4. 试算定价（复用 `TrialService.Trial()`，含活动校验+人群标签）
  5. 限购检查（只查已支付订单数 `CountPaidOrdersByUserActivity`，**不 +1**）
  6. 名额占用（Redis Lua `TryOccupyStock`）
  7. 写库（`CreateTeamWithOrder` 或 `JoinTeamWithOrder`，事务内）
  8. 缓存结果（`CacheLockResult`，10min TTL）
  9. defer 释放分布式锁
- [x] **5.2** 分两条路径实现：
  - **新建团**：teamId 为空 → 生成 8 位 teamId + 12 位 orderId → 建团 + 首单（一个事务）
  - **加入团**：teamId 非空 → 校验团存在+forming+未过期 → Redis 占名额 → `JoinTeamWithOrder`（SELECT FOR UPDATE + 原子 incr lock_count + insert order）
- [x] **5.3** 异常处理：
  - Redis 满标 → `CodeStockInsufficient` (E0008)
  - DB 团满（lock_count >= target_count）→ 释放名额 + 标记满标 + `CodeTeamFull` (E0006)
  - 限购超限 → `CodeTakeLimitReached` (E0103)，注意只查 status=Paid
  - 人群限制 → `CodeCrowdBlocked` (E0007)
  - DB 失败 → 释放 Redis 名额（补偿）
- [x] **5.4** 锁单接口 HTTP handler：`POST /api/v1/trade/lock`（`handler/trade.go`）
- [x] **5.5** 锁单单元测试（11 个测试：新建团/加入团/幂等缓存/幂等DB/活动过期/限购/团满/团不存在/并发/NotifyURL/活动不匹配/团过期）
- [x] **5.6** 锁单集成测试（需 Docker MySQL + Redis，与 trial_test.go 共享 TestMain）

> **与 Java 版关键差异**：
> - take_limit 锁单时只做 DB 软检查（查已支付订单数），**不递增**。真正 +1 在结算时。
> - 无责任链（ActivityValidityCheckNode + UserTakeLimitCheckNode），改为 if-else。
> - 无热活动优化（isHotTeamFull），Lua 脚本已足够。

---

## Phase 6：结算（Settlement — 支付回调）✅

- [x] **6.1** 实现结算主流程（`service/settlement.go`，约 260 行）：
  1. 查订单 → 校验 userId + status=Locked
  2. 已支付 → 幂等返回（补创建 notify_task）
  3. 查团 → 校验 forming + 未过期
  4. 查活动 → 获取 take_limit
  5. 限购软检查（Redis GET，快速拒绝超限）
  6. **DB 结算先行**（事务，SELECT FOR UPDATE 锁团行，条件更新防重复）
  7. **Redis INCR 后置**（DB 成功才递增，防并发失败污染计数）
  8. 成团 → 创建 notify_task
- [x] **6.2** 结算接口 HTTP handler：`POST /api/v1/trade/settlement`（`handler/trade.go`）
- [x] **6.3** 结算单元测试（9 个测试全部通过，-race 通过）：
  - 基础结算、幂等结算、成团、限购超限、订单不存在、用户不匹配、已退款、团过期、并发结算

> **与 Java 版关键差异**：
> - 无责任链（OutTradeNoCheckNode → OutTradeTimeCheckNode → SourceChannelCheckNode → SettlementEndNode），改为简单 if-else。
> - source/channel 黑名单暂跳过（Java 版也是 TODO）。
> - take_limit 软检查（Redis GET）+ DB 成功后才 INCR，防止并发失败污染计数。
> - DB 结算用 SELECT FOR UPDATE 序列化同团并发，条件 UPDATE（WHERE status=0）防重复结算。

---

## Phase 7：退单（Refund）✅

> **注意**：退款不退 take_limit 次数。支付成功即消耗。

- [x] **7.1** 实现退单（`service/refund.go`，~260 行）：
  - 一个 `Refund()` 主函数 + if-else 分发三种场景（不用责任链+策略模式）
  - 幂等：order.Status==Refunded → 直接返回
  - 并发保护：`UpdateOrderStatusWithCheck`（WHERE status=from）防重复退单，`RefundCompleteTeam` 用 SELECT FOR UPDATE 序列化同团并发退款
  - 新增错误码 `CodeRefundStateInvalid = "E0107"`
  - 新增 3 个 Repository 方法：`UpdateOrderStatusWithCheck`, `RefundTeamForming`, `RefundCompleteTeam`
- [x] **7.2** 实现三种退单场景：
  - **未支付退单（unpaidRefund）**：order Locked→Refunded, team lock_count-1, 关 payment（最佳努力）, 释放 Redis 名额, 建 notify_task (trade_unpaid_refund)
  - **已支付未成团退单（paidRefund）**：order Paid→Refunded, team lock_count-1 & complete_count-1, 释放 Redis 名额, 建 notify_task (trade_paid_refund)
  - **已成团退单（paidTeamRefund）**：order Paid→Refunded, team completeCount>1→CompleteRefunded(3) / =1→Failed(2), 不释放名额, 建 notify_task (trade_paid_team_refund)
- [x] **7.3** 退单接口 HTTP handler：`POST /api/v1/trade/refund`（`handler/trade.go`）
- [x] **7.4** 退单单元测试（9 个测试全部通过，-race 通过）：
  - 未支付退、已支付退、已成团退（多人+最后一人）
  - 幂等退、并发退（5 goroutines + 2 goroutines 已成团并发）
  - 订单不存在、用户不匹配、无效状态

> **与 Java 版关键差异**：
> - 不用责任链+策略模式，改为一个函数 + if-else 分发。
> - Redis 名额释放改为同步（Java 通过 MQ listener 异步），ReleaseStock Lua 已是幂等的。
> - 已成团退单用 SELECT FOR UPDATE 序列化（非两步 WHERE 条件），更安全可靠。
> - 不用分布式锁，DB 条件 WHERE 子句足够。

---

## Phase 8：回调通知（Notify Task）

- [ ] **8.1** 实现 HTTP 回调：POST 到 notify_url，带 body（teamId + outTradeNoList）
- [ ] **8.2** 实现 MQ 回调
- [ ] **8.3** 实现回调重试机制（成功 → status=1, 失败 → retry_count+1)
- [ ] **8.4** 定时扫描未完成任务（游标分页，每次限量处理）
- [ ] **8.5** 回调任务单元测试

---

## Phase 9：超时退单定时任务

- [ ] **9.1** 实现超时扫描（查询 orders.status=0 且 teams.valid_end < NOW()）
- [ ] **9.2** 批量触发退单（每个超时订单调用退单服务）
- [ ] **9.3** 定时任务调度（Go cron 库或 time.Ticker）
- [ ] **9.4** 多实例防重复（分布式锁或 DB 抢占）

---

## Phase 10：缓存预热 & 性能优化

> Java 版 DCC 的坑：用 `BeanPostProcessor` + 反射 + `@DCCValue` 注解 + Fastjson 反序列化来做配置热更新，极度复杂。Go 版直接读 Redis/DB 更新本地变量即可。

- [ ] **10.1** 启动时预加载活动/折扣/商品到本地缓存（sync.Map 或 go-cache）
- [ ] **10.2** 配置热更新接口：`POST /api/v1/admin/config/reload`，从 DB/Redis 重新加载本地缓存（对标 Java DCC，但实现只需几十行）
- [ ] **10.3** 试算结果短 TTL 缓存（3~10 秒，key=`trial:{userId}:{outTradeNo}`，防重试穿透）
- [ ] **10.4** 报价上下文缓存（活动+折扣+商品+渠道映射，key=`bgm:quote:{source}:{channel}:{goodsId}`，分钟级 TTL）
- [ ] **10.5** 压测验证（wrk / vegeta），目标 QPS > 5000（锁单）、> 20000（试算）

---

## Phase 11：支付对接（新增）

- [ ] **11.1** 定义支付网关接口（`internal/pay/gateway.go`）
  - `CreateOrder(ctx, req) → payUrl` — 创建支付单
  - `QueryOrder(ctx, outTradeNo) → status` — 查询支付状态
  - `Refund(ctx, outTradeNo) → refundId` — 退款
  - `VerifyNotify(ctx, raw) → notify` — 验签支付回调
- [ ] **11.2** 实现 Mock 支付网关（开发/测试用）
- [ ] **11.3** 支付回调接口 `POST /api/v1/pay/notify`
- [ ] **11.4** 在锁单响应中返回支付链接（payUrl）
- [ ] **11.5** 预留真实支付网关实现（微信/支付宝）

---

## Phase 12：可观测性 & 运维

- [ ] **12.1** 关键指标埋点（Prometheus metrics）
  - 锁单 QPS / P99 延迟 / 错误码分布
  - 试算缓存命中率
  - Redis 操作成功/失败次数
  - DB 慢查询次数
- [ ] **12.2** 健康检查接口 `/health`、`/ready`
- [ ] **12.3** Dockerfile + docker-compose 一键启动
- [ ] **12.4** 压测脚本 & 压测报告

---

## 待明确 / 待决策

- [ ] **Q1** 是否保留人群标签（crowd_tags）功能？目前业务场景似乎未大量使用
- [ ] **Q2** 消息队列选型：RabbitMQ / Kafka / Redis Stream / 先不做？
- [ ] **Q3** 是否需要管理后台接口（CRUD 活动/折扣/商品）？
- [ ] **Q4** 支付网关先对接微信还是支付宝？还是先做 Mock 够用？

---

> 勾选标记：复制此文件到项目中，完成一项打 `[x]`。
