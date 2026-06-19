#!/bin/bash
# 锁单压测 — 完整写路径 (Redis + DB 事务)
# 用法: cd loadtest && bash 03_lock.sh [rate] [duration]
#   bash 03_lock.sh           # 默认: 多档位 (50/100/200/500)
#   bash 03_lock.sh 100 10s   # 单档: 100/s 跑 10 秒
set -eu
source "$(dirname "$0")/lib.sh"

TARGET="targets/lock.json"

check_server || exit 1
print_header "Step 3: Lock TPS — 库存充足, 全新建团"
print_env_info

echo "  数据流: Redis GET(幂等) → Trial(本地缓存) → Redis Lua(占名额)"
echo "         → DB TX (INSERT team + INSERT order) → Redis SET(缓存结果)"
echo "  特点: 5000个不同用户, team_id为空→各自新建团, 无锁竞争"
echo "  每个档位独立清理数据, 互不污染"
echo ""

run_lock() {
    local rate="$1" dur="${2:-10s}"
    clean_scenario "LOADTEST-%"
    vegeta_run "lock_${rate}" "$rate" "$dur" "$TARGET" \
        "锁单-完整写路径 rate=${rate}/s"
    echo "  Redis keys 残留: $(redis_exec DBSIZE)"
    clean_scenario "LOADTEST-%"
}

if [ $# -ge 1 ]; then
    run_lock "$1" "${2:-10s}"
else
    for rate in 50 100 200 500; do
        run_lock "$rate" "10s"
    done
fi

echo ""
echo "── Lock 汇总 ──"
for f in "$RESULTS_DIR"/lock_*.txt; do
    [ -f "$f" ] || continue
    name=$(basename "$f" .txt)
    echo -n "  $name: "
    grep "Requests" "$f" | head -1
done
