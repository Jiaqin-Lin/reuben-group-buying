#!/bin/bash
# 试算压测 — 纯读性能（本地缓存命中，0 DB 查询）
# 用法: cd loadtest && bash 02_trial.sh [rate] [duration]
#   bash 02_trial.sh          # 默认: 多档位跑 (100/500/1000/2000)
#   bash 02_trial.sh 500 5s   # 单档: 500/s 跑 5 秒
set -eu
source "$(dirname "$0")/lib.sh"

TARGET="targets/trial.json"

check_server || exit 1
print_header "Step 2: Trial — 纯读性能 (本地缓存)"
print_env_info

echo "  数据流: 请求 → Handler → Service → LocalCache(内存) → 返回"
echo "  DB 查询: 0 次"
echo ""

if [ $# -ge 2 ]; then
    # 单档手动模式
    vegeta_run "trial_${1}" "$1" "$2" "$TARGET" "试算-纯读-本地缓存"
else
    # 多档自动模式
    for rate in 100 500 1000 2000; do
        vegeta_run "trial_${rate}" "$rate" "10s" "$TARGET" "试算-纯读-本地缓存 rate=${rate}/s"
    done
fi

echo ""
echo "── Trial 汇总 ──"
for f in "$RESULTS_DIR"/trial_*.txt; do
    [ -f "$f" ] || continue
    name=$(basename "$f" .txt)
    echo -n "  $name: "
    grep "Requests" "$f" | head -1
done
