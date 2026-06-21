# 压测脚本

## 环境要求

- `vegeta`（HTTP 压测工具）
- `python3`（生成目标文件）
- Docker（MySQL + Redis 容器运行中）
- 应用服务运行中（`http://localhost:8080`）

## 文件说明

```
loadtest/
├── lib.sh                   # 共享函数库（所有脚本 source 这个）
├── generate_targets.sh      # 生成 vegeta 目标文件
├── seed.sql                 # 种子数据（INSERT IGNORE，幂等）
├── 01_smoke.sh              # 冒烟测试（5 个接口功能验证）
├── 02_trial.sh              # 试算压测（纯读，本地缓存）
├── 03_lock.sh               # 锁单压测（完整写路径）
├── 04_idempotent.sh         # 幂等压测（同 out_trade_no）
├── 05_seckill.sh            # 秒杀场景（N 人抢 M 名额）
├── targets/                 # 目标文件（generate_targets.sh 生成）
│   ├── trial.json           #   试算: 10000 个不同用户+商品
│   ├── lock.json            #   锁单: 5000 个不同用户各自新建团
│   ├── lock_idempotent.json #   幂等: 1000 请求同 out_trade_no
│   ├── lock_same_team.json  #   同团: 500 人加同一个团
│   └── settlement.json      #   结算: 5000 个不同 out_trade_no
└── results/                 # 压测结果（.bin 原始 + .txt 报告）
```

### lib.sh 做什么

`lib.sh` 是共享函数库，自动处理本地和服务器差异：

- **Docker 容器名**：本地 `group-buy-mysql/redis`，服务器 `gb-mysql/redis`
- **vegeta 路径**：优先 `$PATH` 里的，否则用 `~/vegeta`
- **平台差异**：macOS / Linux 自适应（CPU 信息获取等）

## 本地压测

```bash
# 1. 启动基础设施
make up
docker exec -i group-buy-mysql mysql -u dev -pdev123 group_buy_market < loadtest/seed.sql

# 2. 启动应用
make run                         # 或 make dev（热重载）

# 3. 生成目标文件（首次或种子数据变更后跑一次）
cd loadtest
bash generate_targets.sh

# 4. 冒烟测试
bash 01_smoke.sh

# 5. 锁单压测
bash 03_lock.sh                  # 多档位: 50/100/200/500/1000
bash 03_lock.sh 500 10s          # 单档位: 500/s × 10s

# 6. 试算压测
bash 02_trial.sh                 # 多档位: 100/500/1000/2000
bash 02_trial.sh 2000 5s         # 单档位

# 7. 秒杀
bash 05_seckill.sh

# 8. 查看结果
cat results/lock_500.txt
```

## 服务器压测

服务器容器名是 `gb-mysql` / `gb-redis`，`lib.sh` 自动检测，**脚本不用改**。

### 首次部署

```bash
# 1. 拷贝脚本到服务器
scp -r loadtest/ dev:~/

# 2. 安装 vegeta（如未安装）
# 本地下载 Linux x86_64 版，scp 上去：
curl -sL https://github.com/tsenart/vegeta/releases/download/v12.12.0/vegeta_12.12.0_linux_amd64.tar.gz -o /tmp/v.tar.gz
tar xzf /tmp/v.tar.gz -C /tmp
scp /tmp/vegeta dev:~/
ssh dev 'chmod +x ~/vegeta'

# 3. 生成目标文件
ssh dev 'cd ~/loadtest && bash generate_targets.sh'
```

### 日常使用

```bash
# 单档位锁单
ssh dev 'cd ~/loadtest && bash 03_lock.sh 500 10s'

# 多档位锁单（约 3 分钟）
ssh dev 'cd ~/loadtest && bash 03_lock.sh'

# 试算
ssh dev 'cd ~/loadtest && bash 02_trial.sh'

# 冒烟
ssh dev 'cd ~/loadtest && bash 01_smoke.sh'

# 查看结果
ssh dev 'cat ~/loadtest/results/lock_500.txt'
```

## 锁单各档位含义

| 速率 | 场景 |
|------|------|
| 50/s | 低频常规流量 |
| 100/s | 中等促销流量 |
| 200/s | 大促流量 |
| 500/s | 秒杀级流量 |
| 1000/s | 极限压测 |

## 本地 vs Dev 参考数据

| 场景 | 本地 M5 (10 核) | Dev (2 核 AMD) |
|------|:---:|:---:|
| 锁单 100/s | P50 5.8ms, 100 TPS | P50 35ms, 100 TPS |
| 锁单 500/s | P50 10.6ms, 500 TPS | P50 2.6s, 365 TPS |
| 锁单 1000/s | P50 7.9ms, 1000 TPS | P50 5.9s, 579 TPS |
| 试算 2000/s | P50 0.4ms, 2000 QPS | P50 0.58ms, 2000 QPS |
