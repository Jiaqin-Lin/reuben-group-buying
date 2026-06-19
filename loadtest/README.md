# 拼团系统压测

## Vegeta 速成（JMeter 用户视角）

如果你只用过 JMeter，Vegeta 唯一的区别在于**并发模型**：

| | JMeter | Vegeta |
|---|--------|--------|
| 你配置什么 | 线程数 + 循环次数 + 持续时间 | 每秒请求数（rate）+ 持续时间 |
| 实际并发数 | = 你设的线程数 | = 响应时间 × 速率（Little's Law） |
| 并发会变吗 | 基本不变 | 会！响应变慢 → 同时在途请求自动增多 |
| 类比 | 100 个顾客排队买咖啡 | 每秒砸 200 张订单到柜台 |

**rate=200/s, duration=10s** = 以每秒 200 次的速度持续发射 10 秒，总共约 2000 个请求。Vegeta 不管有多少个"线程"，它按需创建 HTTP 连接以达到目标速率。

看到报告里 `Requests [total, rate, throughput]` 三个数：total=发出的请求总数，rate=目标速率，throughput=实际吞吐。throughput 接近 rate 说明服务跟得上。

## 文件说明

```
loadtest/
├── README.md              ← 你正在看的
├── lib.sh                 ← 共享函数（清理/种子/vegeta封装）
├── seed.sql               ← 种子数据（活动/商品/折扣），INSERT IGNORE 幂等
├── generate_targets.go    ← 生成 Vegeta 靶子 JSON 文件
├── targets/               ← 预生成的请求文件（go run generate_targets.go 生成）
│   ├── trial.json         # 10000 个不同用户的试算请求
│   ├── lock.json          # 5000 个不同用户的锁单请求
│   ├── lock_idempotent.json # 1000 个同 outTradeNo 请求
│   ├── lock_same_team.json  # 500 个同团锁单请求
│   └── settlement.json    # 5000 个结算请求
├── results/               ← 压测结果（.txt 人类可读 + .bin 原始数据）
│   ├── trial_100.txt
│   ├── trial_100.bin
│   └── ...
├── 01_smoke.sh            ← 冒烟测试
├── 02_trial.sh            ← 试算压测
├── 03_lock.sh             ← 锁单压测
├── 04_idempotent.sh       ← 幂等性压测
└── 05_seckill.sh          ← 秒杀压测
```

## 前置条件

```bash
# 1. 服务必须运行
cd .. && go run cmd/server/main.go

# 2. 种子数据只需执行一次（INSERT IGNORE，重复执行无副作用）
docker exec -i group-buy-mysql mysql -u dev -pdev123 --default-character-set=utf8mb4 group_buy_market < loadtest/seed.sql

# 3. 生成靶子文件（种子数据不变就不用重新生成）
cd loadtest && go run generate_targets.go
```

## 脚本使用

所有脚本**可单独运行**，不强制按顺序。同脚本同参数多次运行**会覆盖**上次结果。

### 01 — 冒烟测试

快速验证 5 个核心接口功能正常（5 秒跑完）。

```bash
bash 01_smoke.sh
```

测试：试算 → 锁单 → 幂等（同 outTradeNo 重复锁）→ 结算 → 退单。全部返回 `code:0000` 即通过。

### 02 — 试算压测（纯读）

测本地缓存命中下的读吞吐。0 次 DB 查询。

```bash
bash 02_trial.sh            # 默认 4 档: 100 → 500 → 1000 → 2000/s, 每档 10s
bash 02_trial.sh 5000 10s   # 单档: 5000/s 跑 10 秒
bash 02_trial.sh 10000 5s   # 单档: 10000/s 跑 5 秒
```

| 参数 | 说明 | 默认值 |
|------|------|--------|
| $1 | 请求速率/s | 不传则跑 4 档 |
| $2 | 持续时间 | 10s |

### 03 — 锁单压测（完整写路径）

测完整写链路：Redis 幂等检查 → 试算 → Redis Lua 占名额 → DB 事务（INSERT team + INSERT order）→ Redis 缓存结果。

```bash
bash 03_lock.sh             # 默认 4 档: 50 → 100 → 200 → 500/s, 每档 10s
bash 03_lock.sh 1000 10s    # 单档: 1000/s 跑 10 秒
```

5000 个不同用户，team_id 为空 → 各自新建团，无锁竞争。每档结束后自动清理数据，互不污染。

| 参数 | 说明 | 默认值 |
|------|------|--------|
| $1 | 请求速率/s | 不传则跑 4 档 |
| $2 | 持续时间 | 10s |

### 04 — 幂等性压测

1000 个请求同一个 `out_trade_no`，验证只有第 1 个走完整写路径，其余 999 个命中缓存直接返回。

```bash
bash 04_idempotent.sh
```

验证点：DB 里只有 1 条订单 = 幂等正确。

### 05 — 秒杀压测

N 人抢 M 个名额，测 Redis 满标拦截能力。被拦截的请求 0 次 DB 查询。

```bash
bash 05_seckill.sh              # 默认: 500 人抢 3 名额, 200/s
bash 05_seckill.sh 1000 5 500   # 1000 人抢 5 名额, 500/s
bash 05_seckill.sh 5000 3 1000  # 5000 人抢 3 名额, 1000/s
```

为什么这个脚本有 3 个参数？因为秒杀场景的变量比其他场景多——人数、名额数、速率都会显著影响结果。

| 参数 | 说明 | 默认值 |
|------|------|--------|
| $1 | 抢团人数 | 500 |
| $2 | 总名额（含创始人占的 1 个） | 3 |
| $3 | 请求速率/s | 200 |

## 结果文件

每次压测生成两个文件，同脚本同参数重复运行**会覆盖**：

```
results/
├── trial_1000.txt   ← 人类可读
├── trial_1000.bin   ← vegeta 原始数据（可用于 vegeta report -type=json 重放分析）
```

查看历史结果：

```bash
# 列出所有文本报告
ls results/*.txt

# 看某个报告
cat results/trial_1000.txt

# 用 vegeta 重新分析原始数据
vegeta report -type=json results/trial_1000.bin | python3 -m json.tool
```

## Vegeta 报告字段解释

```
Requests      [total, rate, throughput]   1000, 100.10, 100.08
              ↑总请求  ↑目标速率/s         ↑实际吞吐/s
Duration      [total, attack, wait]       9.992s, 9.99s, 1.583ms
              ↑总耗时   ↑发射耗时           ↑平均等待时间
Latencies     [min, mean, 50, 90, 95, 99, max]
              ↑最小     ↑平均   ↑P50  ↑P90  ↑P95  ↑P99  ↑最大
Success       [ratio]                     100.00%
Status Codes  [code:count]                200:1000
```

**核心指标**：Success（成功率，应该 100%）、P99 latency（99% 请求的响应时间上限）、throughput（实际吞吐，接近 rate 说明服务跟得上）。

## 数据清理

每个脚本开头和结尾都会 `clean_scenario`，只删压测产生的数据（orders/teams/payments/notify_tasks + Redis 业务 key），**不删种子数据**。

如果需要手动清理某类数据：

```bash
source lib.sh
clean_scenario "LOADTEST-%"   # 清理锁单测试数据
clean_scenario "SECKILL-%"    # 清理秒杀测试数据
clean_scenario "IDEM-%"       # 清理幂等测试数据
clean_scenario "SMOKE-%"      # 清理冒烟测试数据
clean_all                     # 清空全部业务数据 + Redis（保留种子数据表）
```
