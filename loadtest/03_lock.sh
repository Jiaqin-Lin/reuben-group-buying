#!/bin/bash
# 锁单压测 — 完整写路径 (Redis + DB 事务)
# 用法: cd loadtest && bash 03_lock.sh [rate] [duration]
#   bash 03_lock.sh           # 默认: 多档位 (50/100/200/500/1000)
#   bash 03_lock.sh 500 10s   # 单档: 500/s 跑 10 秒
set -eu
DIR="$(dirname "$0")"
source "$DIR/lib.sh"

TARGET="$DIR/targets/lock.json"

[ -f "$TARGET" ] || { echo "Run: bash generate_targets.sh first"; exit 1; }

setup || exit 1
print_header "Step 3: Lock TPS — 库存充足, 全新建团"
print_env_info

echo "  数据流: Redis GET(幂等) → Trial(本地缓存) → Redis Lua(占名额)"
echo "         → DB TX (INSERT team + INSERT order) → Redis SET(缓存结果)"
echo "  特点: 5000个不同用户, team_id为空→各自新建团, 无锁竞争"
echo ""

run_lock() {
    local rate="$1" dur="${2:-10s}"
    clean_scenario "LOADTEST-%"
    vegeta_run "lock_${rate}" "$rate" "$dur" "$TARGET" "锁单-完整写路径 rate=${rate}/s"
    echo "  Redis keys 残留: $(redis_exec DBSIZE)"
    clean_scenario "LOADTEST-%"
}

if [ $# -ge 1 ]; then
    run_lock "$1" "${2:-10s}"
else
    for rate in 50 100 200 500 1000; do
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
