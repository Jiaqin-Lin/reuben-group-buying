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
│   │   ├── activity.go         # 活动/折扣/sc_sku_activity 查询
│   │   ├── order.go            # 订单 CRUD（order_list + order）
│   │   ├── sku.go              # 商品查询
│   │   ├── notify_task.go      # 回调任务 CRUD
│   │   └── cache.go            # 所有缓存读写操作
│   ├── model/                  # 数据模型（PO/VO 统一放这里）
│   ├── redisx/                 # Redis 操作封装
│   │   ├── stock.go            # 名额占用/释放 Lua 脚本
│   │   ├── lock.go             # 分布式锁
│   │   └── cache.go            # 缓存读写
│   ├── pay/                    # 支付网关对接（新增）
│   │   ├── gateway.go          # 支付接口定义
│   │   └── mock.go             # Mock 实现
│   └── middleware/             # 中间件（限流、日志、traceID）
├── scripts/lua/                # Lua 脚本文件（从 Java 代码中提取）
│   ├── occupy_stock.lua
│   ├── release_stock.lua
│   └── check_stock.lua
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
- [x] **1.5** 统一响应格式（对标 Java Response.java）→ `internal/response.go`
- [x] **1.6** 统一错误码（对标 Java ResponseCode.java）→ `internal/errors.go`
- [x] **1.7** HTTP 中间件：traceID 注入、请求日志、panic recover → `internal/middleware/*/`

---

## Phase 2：数据层（Repository） — 先做无缓存的直读

- [ ] **2.1** 定义所有数据模型 struct（对应 7 张表）
- [ ] **2.2** 实现 `group_buy_activity` 查询（按 activity_id）
- [ ] **2.3** 实现 `group_buy_discount` 查询（按 discount_id）
- [ ] **2.4** 实现 `sc_sku_activity` 查询（按 source+channel+goods_id）
- [ ] **2.5** 实现 `sku` 查询（按 goods_id）
- [ ] **2.6** 实现 `group_buy_order_list` CRUD（查询/插入/更新状态）
- [ ] **2.7** 实现 `group_buy_order` CRUD（查询/插入/更新 lock_count/complete_count/status）
- [ ] **2.8** 实现 `notify_task` CRUD（查询未完成/插入/更新状态）
- [ ] **2.9** 实现 `crowd_tags` + `crowd_tags_detail` 查询（人群标签过滤）
- [ ] **2.10** 编写数据层单元测试

---

## Phase 3：Redis 层

- [ ] **3.1** 把 Java 代码中的 Lua 脚本提取到独立 `.lua` 文件
  - `scripts/lua/occupy_stock.lua` — 占用名额
  - `scripts/lua/release_stock.lua` — 释放名额
  - `scripts/lua/check_stock.lua` — 检查是否已占用
- [ ] **3.2** 实现名额占用函数 `TryOccupyTeamStock(ctx, key, permitId, total, ttl)`
- [ ] **3.3** 实现名额释放函数 `ReleaseTeamStock(ctx, key, permitId)`
- [ ] **3.4** 实现分布式锁 `AcquireLock(ctx, key, ttl)` / `ReleaseLock(ctx, key)`
- [ ] **3.5** 实现锁单结果缓存 `CacheLockResult(ctx, key, data, ttl)` / `GetLockResult(ctx, key)`
- [ ] **3.6** 实现用户限购计数 `IncrUserTakeCount(ctx, activityId, userId)` / `GetUserTakeCount(ctx, activityId, userId)`
- [ ] **3.7** Redis 层单元测试（用 miniredis mock）

---

## Phase 4：试算（Trial）

- [ ] **4.1** 实现折扣计算逻辑（直减 ZJ、满减 MJ、折扣 ZK、N 元购）
- [ ] **4.2** 实现试算主流程：
  1. 查 sc_sku_activity → 获取 activityId
  2. 查 group_buy_activity → 校验状态
  3. 查 group_buy_discount → 计算优惠
  4. 查 sku → 获取商品信息
- [ ] **4.3** 实现人群标签过滤（可选）
- [ ] **4.4** 试算接口 HTTP handler：`POST /api/v1/trial`
- [ ] **4.5** 试算单元测试（覆盖 4 种折扣类型）

---

## Phase 5：锁单（Lock Order）

- [ ] **5.1** 实现锁单主流程（`service/lock.go`）：
  1. 参数校验
  2. outTradeNo 幂等检查
  3. 同团复用检查（有 teamId 时）
  4. 分布式锁
  5. 调用试算获取价格
  6. 名额占用（Redis Lua）
  7. 写入订单 & 团表（DB 事务）
  8. 缓存锁单结果
  9. 释放分布式锁
- [ ] **5.2** 分两条路径实现：
  - **新建团**：teamId 为空 → 生成 teamId → 建团 + 首单
  - **加入团**：teamId 非空 → 加团 + 增量更新
- [ ] **5.3** 异常处理：
  - DuplicateKey → 返回已有订单
  - 名额满 → 返回"团已满"
  - Redis 异常 → 确认 token 存在 or 快速失败
- [ ] **5.4** 锁单接口 HTTP handler：`POST /api/v1/trade/lock`
- [ ] **5.5** 锁单单元测试（含并发测试、幂等测试）
- [ ] **5.6** 锁单集成测试（真实 MySQL + Redis）

---

## Phase 6：结算（Settlement — 支付回调）

- [ ] **6.1** 实现结算主流程（`service/settlement.go`）：
  1. 校验订单（outTradeNo + userId 查询）
  2. 更新 order_list 状态为 COMPLETE
  3. 更新 order complete_count + 1
  4. 判断是否成团（complete_count == target_count）
  5. 成团后创建 notify_task
  6. 执行回调通知
- [ ] **6.2** 结算接口 HTTP handler：`POST /api/v1/trade/settlement`
- [ ] **6.3** 结算单元测试

---

## Phase 7：退单（Refund）

- [ ] **7.1** 实现退单责任链（简化版：条件判断 + 提前返回）
- [ ] **7.2** 实现三种退单策略：
  - **未支付退单**：更新订单 status=REFUND, 退名额(release stock), 更新团 lock_count-1
  - **已支付未成团退单**：更新订单 status=REFUND, 退名额, 更新团 lock_count-1 和 complete_count-1
  - **已成团退单**：更新订单 status=REFUND, 更新团状态为 COMPLETE_REFUND
- [ ] **7.3** 退单接口 HTTP handler：`POST /api/v1/trade/refund`
- [ ] **7.4** 退单单元测试

---

## Phase 8：回调通知（Notify Task）

- [ ] **8.1** 实现 HTTP 回调：POST 到 notify_url，带 body（teamId + outTradeNoList）
- [ ] **8.2** 实现 MQ 回调（暂用 Redis Pub/Sub 或 mock，后续接真实 MQ）
- [ ] **8.3** 实现回调重试机制（成功 → status=1, 失败 → retry_count+1)
- [ ] **8.4** 定时扫描未完成任务（游标分页，每次限量处理）
- [ ] **8.5** 回调任务单元测试

---

## Phase 9：超时退单定时任务

- [ ] **9.1** 实现超时扫描（查询 status=0 且超过 valid_end_time 的订单）
- [ ] **9.2** 批量触发退单（每个超时订单调用退单服务）
- [ ] **9.3** 定时任务调度（Go cron 库或 time.Ticker）
- [ ] **9.4** 多实例防重复（分布式锁或 DB 抢占）

---

## Phase 10：缓存预热 & 性能优化

> Java 版 DCC 的坑：用 `BeanPostProcessor` + 反射 + `@DCCValue` 注解 + Fastjson 反序列化来做配置热更新，极度复杂。Go 版直接读 Redis/DB 更新本地变量即可。

- [ ] **10.1** 启动时预加载活动/折扣/SKU 到本地缓存（sync.Map 或 go-cache）
- [ ] **10.2** 配置热更新接口：`POST /api/v1/admin/config/reload`，从 DB/Redis 重新加载本地缓存（对标 Java DCC，但实现只需几十行）
- [ ] **10.3** 试算结果短 TTL 缓存（3~10 秒，key=`trial:{userId}:{outTradeNo}`，防重试穿透）
- [ ] **10.4** 报价上下文缓存（活动+折扣+SKU+渠道映射，key=`bgm:quote:{source}:{channel}:{goodsId}`，分钟级 TTL）
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
