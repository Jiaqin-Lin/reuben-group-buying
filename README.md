# 拼团营销系统

锁单、支付、拼团、退单、回调通知。

## 技术栈

| 组件 | 选型 |
|------|------|
| 语言 | Go 1.22+ |
| HTTP | Gin |
| ORM | GORM + MySQL 8.0 |
| 缓存 | go-redis/v9 |
| 消息队列 | RocketMQ（延迟消息处理超时退单） |
| 配置 | Viper（YAML + 环境变量） |
| 日志 | slog（结构化 JSON，自动注入 traceID） |
| 支付 | 支付宝沙箱（可降级 Mock） |
| 可观测 | Prometheus + Loki + Grafana |

## 快速开始

```bash
# 1. 克隆
git clone git@github.com:Jiaqin-Lin/reuben-group-buying.git
cd reuben-group-buying

# 2. 配置环境变量
cp .env.example .env
# 编辑 .env，填入支付宝沙箱密钥（不填则使用 Mock 支付网关）

# 3. 启动
docker compose up -d

# 4. 验证
curl http://localhost:8081/api/v1/index
```

Grafana：`http://localhost:3000`（admin/admin），Loki 和 Prometheus 数据源已预配置。

## 目录结构

```
.
├── cmd/server/main.go         # 入口
├── internal/
│   ├── config/                # 配置加载（Viper）
│   ├── infra/                 # 基础设施
│   │   ├── mysql/             # GORM + 连接池
│   │   ├── redis/             # go-redis 客户端
│   │   ├── mq/                # RocketMQ 生产者/消费者
│   │   └── log/               # slog 封装（context-aware traceID）
│   ├── handler/               # HTTP handler
│   │   ├── trade.go           # 锁单/结算/退单
│   │   ├── index.go           # 首页/试算/拼团进度
│   │   ├── pay.go             # 支付回调
│   │   └── admin*.go          # 管理接口 CRUD
│   ├── service/               # 业务逻辑
│   │   ├── trial.go           # 试算
│   │   ├── lock.go            # 锁单
│   │   ├── settlement.go      # 结算（成团检查 + 创建回调）
│   │   ├── refund.go          # 退单
│   │   ├── notify.go          # 回调通知（HTTP + MQ 重试）
│   │   └── timeout.go         # 超时扫描
│   ├── repository/            # 数据访问
│   ├── model/                 # 数据 struct
│   ├── redisx/                # Redis 业务操作（Lua 脚本）
│   ├── pay/                   # 支付网关接口
│   └── middleware/            # HTTP 中间件
│       ├── tracing/           # traceID 注入
│       ├── logging/           # 请求日志
│       ├── ratelimit/         # 限流
│       └── recovery/          # panic 恢复
├── docker/                    # Docker 配置
│   ├── loki/                  # Loki 日志聚合
│   ├── promtail/              # Promtail 日志采集
│   ├── prometheus/            # Prometheus 指标采集
│   └── grafana/               # Grafana 仪表盘 + 数据源
├── migrations/                # SQL 迁移
├── scripts/lua/               # Redis Lua 脚本
├── docker-compose.yml
├── Dockerfile
└── Makefile
```

## 核心数据模型

| 表 | 用途 |
|---|------|
| `activities` | 拼团活动配置 |
| `discounts` | 折扣规则（直减/满减/折扣/无） |
| `products` | 商品信息 |
| `activity_products` | 商品-活动映射 |
| `teams` | 拼团队伍 |
| `orders` | 用户订单 |
| `payments` | 支付单（对接支付宝） |
| `payment_logs` | 支付回调原始日志 |
| `notify_tasks` | 回调通知任务 |
| `crowd_tags` | 人群标签定义 |
| `crowd_tag_details` | 标签-用户明细 |
| `crowd_tag_jobs` | 人群计算任务 |

## 业务流程

```
试算 → 锁单 → 支付 → 结算(成团) → 回调通知
                ↓ 超时未付
              退单(释放名额)
```

### outTradeNo 与 orderId

- **out_trade_no**：外部交易单号，调用方生成，贯穿全链路幂等
- **order_id**：内部订单号，系统生成，发给支付宝作为商家订单号

### take_limit

单个用户在一个活动中最多参与次数。支付成功 +1，退款 -1。按 `(userId, activityId)` 维度计数。

## 配置

```bash
cp .env.example .env
# 编辑 .env 填入真实值
```

环境变量优先级高于 `config.yaml`。敏感信息（密码、密钥）只放 `.env`，不提交。

## 常用命令

```bash
make dev            # 热重载开发（需要 air）
make test           # 跑测试
make lint           # golangci-lint 检查
make up             # 启动 MySQL + Redis + App
make monitor-up     # 启动 Prometheus + Grafana + Loki
make build          # 编译
```

## License

MIT
