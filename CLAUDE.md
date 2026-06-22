# 拼团营销系统

## 技术栈

- Go 1.22+
- HTTP: gin
- ORM: gorm + MySQL 8.x driver
- Redis: go-redis/v9
- MQ: RocketMQ（延迟消息处理超时退单）
- Config: Viper（YAML + 环境变量）
- Cron: robfig/cron/v3
- 日志: slog（标准库结构化日志，JSON 格式输出）
- 可观测: Prometheus + Loki + Grafana

## 目录结构

```
.
├── cmd/server/main.go            # 入口
├── internal/
│   ├── config/                   # 配置加载、全局配置 struct
│   ├── infra/                    # 基础设施
│   │   ├── mysql/                # GORM 初始化 + 连接池
│   │   ├── redis/                # go-redis 客户端初始化
│   │   ├── mq/                   # RocketMQ 生产者 + 消费者
│   │   └── log/                  # slog 封装，自动从 context 提取 traceID
│   ├── handler/                  # HTTP handler，参数绑定 + 调用 service + 返回响应
│   │   ├── trade.go              # 锁单/结算/退单
│   │   ├── index.go              # 首页/试算/拼团进度
│   │   ├── pay.go                # 支付下单/回调
│   │   └── admin*.go             # 管理接口（活动/折扣/商品/人群 CRUD）
│   ├── service/                  # 业务逻辑，无状态
│   │   ├── trial.go              # 试算
│   │   ├── lock.go               # 锁单
│   │   ├── settlement.go         # 结算
│   │   ├── refund.go             # 退单
│   │   ├── notify.go             # 回调通知
│   │   └── timeout.go            # 超时扫描
│   ├── repository/               # 数据访问，一个文件一组相关操作
│   │   ├── activity.go           # activities + discounts + activity_products
│   │   ├── order.go              # orders + teams
│   │   ├── product.go            # products
│   │   ├── notify_task.go        # notify_tasks
│   │   ├── payment.go            # payments + payment_logs
│   │   ├── crowd.go              # crowd_tags + crowd_tag_details + crowd_tag_jobs
│   │   └── cache.go              # 所有缓存读写操作
│   ├── model/                    # 数据 struct
│   ├── redisx/                   # Redis 业务操作封装
│   │   ├── stock.go              # 名额占用/释放（Lua 脚本）
│   │   ├── lock.go               # 分布式锁
│   │   └── cache.go              # 通用缓存 set/get
│   ├── pay/                      # 支付网关
│   │   ├── gateway.go            # 接口定义
│   │   └── mock.go               # Mock 实现
│   └── middleware/               # HTTP 中间件
│       ├── tracing/              # traceID 注入
│       ├── logging/              # 请求日志
│       ├── ratelimit/            # 限流
│       └── recovery/             # panic 恢复
├── scripts/lua/                  # Redis Lua 脚本
│   ├── occupy_stock.lua
│   ├── release_stock.lua
│   └── check_stock.lua
├── docker/                       # Docker 配置
│   ├── loki/                     # Loki 日志聚合
│   ├── promtail/                 # Promtail 日志采集
│   ├── prometheus/               # Prometheus 指标采集
│   └── grafana/                  # Grafana 仪表盘 + 数据源
├── migrations/                   # SQL 迁移文件
├── config.yaml                   # 默认配置（敏感值通过 .env 环境变量注入）
├── .env.example                  # 环境变量模板
├── docker-compose.yml            # 本地开发环境（MySQL + Redis + RocketMQ + 可观测）
├── Dockerfile
├── Makefile
└── CLAUDE.md
```

## 核心业务

### outTradeNo 与 orderId

**out_trade_no = 外部交易单号**，由调用方生成，贯穿全链路。

三个用途：
1. **幂等**：同 out_trade_no 重复请求 → 返回缓存结果（`bgm:lock:result:{userId}:{outTradeNo}`，10min TTL），不重复创建订单
2. **全链路关联**：锁单 → 支付 → 结算 → 退单 → 回调，全部通过 out_trade_no 串联
3. **Redis 名额占用标识**：Lua 脚本用 out_trade_no 做 SET 成员，防同一外部单号重复占位

**order_id = 内部订单号**，本系统生成（12 位随机数），用于：
- 内部查询主键
- **发给支付宝的商家订单号**（支付宝接口的 `out_trade_no` 参数传的是我们的 `order_id`）
- 与 `out_trade_no` 一一对应（各自有 UK 约束）

```
调用方传入 out_trade_no = "EXT20260618001"
         │
         ▼
  系统生成 order_id = "384729105638"
         │
         ├── 幂等：SELECT WHERE out_trade_no = 'EXT20260618001'
         ├── 占位：Redis SET SADD bgm:stock:... 'EXT20260618001'
         ├── 支付：Alipay 参数 out_trade_no = '384729105638'
         └── 回调：notify_task.payload 包含 out_trade_no
```

### take_limit 语义

单个用户在单个活动中最多参与拼团的次数。

| 问题 | 答案 |
|------|------|
| 计数维度 | `(userId, activityId)`，不区分商品 |
| 计数时机 | **支付成功时 +1** |
| 退款退次数？ | **退**。退款后递减 Redis take_limit 计数（Lua 保证不降到负数） |
| 并发保护 | 分布式锁 `bgm:lock:order:{userId}:{outTradeNo}` 防同一单号并发；Redis 原子 incr 防跨单号超限 |
| 为什么按活动不按商品？ | 活动是营销单元。"618大促"限制每人参与5次，跟买哪个商品无关 |

### 数据模型（12 张表）

#### 核心业务（7 张）

| 表 | 用途 | 关键 UK |
|---|------|--------|
| `activities` | 拼团活动配置 | activity_id |
| `discounts` | 折扣规则（ZJ/MJ/ZK/N） | discount_id |
| `products` | 商品信息 | goods_id |
| `activity_products` | 商品-活动映射 | (source, channel, goods_id) |
| `teams` | 拼团队伍 | team_id |
| `orders` | 用户订单 | order_id, out_trade_no |
| `notify_tasks` | 回调通知任务 | uuid |

#### 支付（2 张）

| 表 | 用途 | 关键 UK |
|---|------|--------|
| `payments` | 支付宝支付单（order_id 发给支付宝，trade_no 支付宝回填） | order_id |
| `payment_logs` | 支付宝回调原始日志（调试/审计） | notify_id |

#### 人群标签（3 张）

| 表 | 用途 | 关键 UK |
|---|------|--------|
| `crowd_tags` | 人群标签定义 | tag_id |
| `crowd_tag_details` | 标签-用户明细 | (tag_id, user_id) |
| `crowd_tag_jobs` | 人群计算任务 | batch_id |

人群标签两个使用场景：
- **活动级**（`activities.tag_id` + `tag_scope`）：控制谁看得见活动、谁能参与活动
- **折扣级**（`discounts.tag_id` + `discount_type=1`）：特定人群额外优惠（如 VIP 额外减 5 元）

#### 实体关系

```
crowd_tags ──→ activities.tag_id (活动人群限制)
    │       ──→ discounts.tag_id (折扣人群定向)
    │ 1:N
    ▼
crowd_tag_details(tag_id, user_id)

activity_products(source, channel, goods_id) → activity_id  ← 试算入口
                                                    │
products(goods_id) ←────────────────────────────────┤
discounts(discount_id) ←────────────────────────────┘
                    │
                    ▼
              activities(activity_id, discount_id, target_count, take_limit, valid_minutes)
                    │
                    ▼
              teams(team_id, activity_id, target_count, lock_count, complete_count, status)
                    │ 1:N
                    ▼
              orders(order_id, out_trade_no, team_id, user_id, status, prices)
                    │ 1:1                    │ 1:N
                    ▼                        ▼
              payments(order_id,        notify_tasks(uuid, team_id,
                       trade_no,                     category, payload,
                       qr_code_url)                  status)
                    │ 1:N
                    ▼
              payment_logs(notify_id, notify_raw)
```

### 业务流程概览

1. **试算**：查 activity_products → activities → discounts → products → 计算折后价。走缓存。
2. **锁单**：试算 → 检查 take_limit（`bgm:take:{activityId}:{userId}` < activities.take_limit）→ Redis Lua 占名额 → 写 orders + teams（事务）。分新建团/加入团两条路径。幂等靠 out_trade_no。
3. **支付**：创建 payments 记录 → 调用支付宝下单（传 order_id 作商家订单号）→ 返回 QR/支付链接。支付宝异步通知 → 验签 → 记录 payment_logs → 更新 payments.status + trade_no → 触发结算。
4. **结算**：支付成功 → 更新 orders.status=1 → Redis incr take_limit 计数 → 更新 teams.complete_count +1 → 成团则创建 notify_task → 执行回调。
5. **退单**：三种场景——未支付退（order=Locked）、已支付未成团退（order=Paid, team=Forming）、已成团退（order=Paid, team=Complete）。dispatch 在 `refund.go:117` 的 switch。释放名额用 Lua 脚本（SREM out_trade_no）。
6. **超时退单**：锁单时发送 RocketMQ 延迟消息 → 延迟后 consumer 收到 → 查订单仍为 Locked → 调 refundSvc.Refund() 退单释放名额。另有 timeout 定时扫描做低频兜底。
7. **回调通知**：支持 HTTP POST 和 MQ。失败重试（最多 5 次），定时任务游标扫描 status=0 或 2 的任务。

## Redis Key 约定

| 用途 | Key 格式 | 类型 | TTL |
|------|---------|------|-----|
| 团名额占用 | `bgm:stock:{activityId}:{teamId}:holders` | SET | 拼团有效期 |
| 团满标 | `bgm:stock:{activityId}:{teamId}:full` | String | 拼团有效期 |
| 用户限购计数 | `bgm:take:{activityId}:{userId}` | int64 | 活动结束时间 |
| 锁单结果缓存 | `bgm:lock:result:{userId}:{outTradeNo}` | JSON | 10min |
| 分布式锁 | `bgm:lock:order:{userId}:{outTradeNo}` | lock | 15s |
| 人群标签成员 | `bgm:tag:{tagId}:members` | BitSet | 长期 |

## 配置管理

- **默认值**：`config.go` 中 `setDefaults()` 提供
- **YAML 覆盖**：`config.yaml`（非敏感默认值）
- **环境变量覆盖**：`.env` 文件（敏感值：密码、密钥、Token），优先级最高
- **映射规则**：Viper 将 `.` 替换为 `_`，如 `mysql.password` ← `MYSQL_PASSWORD`

## 编码约定

- **一个文件不超过 300 行**：超过就拆。repository 文件不超过 200 行。
- **函数不超过 50 行**：超过说明该拆子函数了。
- **错误处理**：用 `fmt.Errorf("lock order: %w", err)` 包装，不要吞掉。handler 层统一翻译为 HTTP 状态码。
- **日志**：用 `slog.InfoContext(ctx, ...)`，带 traceID。关键节点必须打日志（锁单开始/完成/失败、Redis 异常、DB 慢查询）。
- **测试**：service 层必须有单元测试，用 go-sqlmock + miniredis 模拟依赖。
- **不要用 panic**：除了 main 和 init，其他地方一律 return error。
