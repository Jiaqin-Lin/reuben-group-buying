#!/bin/bash
# 拼团系统压测 — 共享函数库
# 用法: source "$(dirname "$0")/lib.sh"

set -e
export no_proxy="localhost,127.0.0.1,::1"
export NO_PROXY="localhost,127.0.0.1,::1"

BASE_URL="${BASE_URL:-http://localhost:8080}"
RESULTS_DIR="$(dirname "$0")/results"
mkdir -p "$RESULTS_DIR"

# ── 颜色 ──
RED='\033[0;31m'; GREEN='\033[0;32m'; CYAN='\033[0;36m'; YELLOW='\033[0;33m'; NC='\033[0m'

# ── 基础设施 ──

mysql_exec() {
    docker exec group-buy-mysql mysql -u dev -pdev123 -N group_buy_market -e "$1" 2>/dev/null || true
}

redis_exec() {
    docker exec group-buy-redis redis-cli "$@" 2>/dev/null || true
}

redis_quiet() {
    docker exec group-buy-redis redis-cli "$@" > /dev/null 2>&1 || true
}

# ── 数据管理 ──

# seed_db 幂等插入种子数据（activities, discounts, products, activity_products）
seed_db() {
    local seed_file="$(dirname "$0")/seed.sql"
    echo "  [Seed] 插入种子数据..."
    docker exec -i group-buy-mysql mysql -u dev -pdev123 --default-character-set=utf8mb4 group_buy_market < "$seed_file" 2>/dev/null
    echo "  [Seed] 完成"
}

# clean_scenario 删除压测产生的订单/团/Redis key，保留种子数据
# 用法: clean_scenario "PATTERN"   # pattern 匹配 out_trade_no LIKE
clean_scenario() {
    local pattern="${1:-LOADTEST-%}"
    echo "  [Clean] pattern=$pattern"
    echo -n "    MySQL: "

    # 删关联表
    mysql_exec "DELETE FROM payments WHERE order_id IN (SELECT order_id FROM orders WHERE out_trade_no LIKE '$pattern')" || true
    mysql_exec "DELETE FROM notify_tasks WHERE team_id IN (SELECT team_id FROM orders WHERE out_trade_no LIKE '$pattern')" || true
    mysql_exec "DELETE FROM orders WHERE out_trade_no LIKE '$pattern'" || true
    # 清理无订单引用的孤儿团
    mysql_exec "DELETE t FROM teams t LEFT JOIN orders o ON t.team_id = o.team_id WHERE o.team_id IS NULL" || true
    echo -n "ok, "

    # 清 Redis 业务 key
    echo -n "Redis: "
    local cleaned=0
    for prefix in "bgm:stock:" "bgm:lock:result:" "bgm:lock:order:" "bgm:take:" "bgm:trial:"; do
        redis_exec --scan --pattern "${prefix}*" | while read -r key; do
            [ -n "$key" ] && redis_quiet DEL "$key" && cleaned=$((cleaned+1))
        done
    done
    echo "ok"
}

# clean_all 清空所有业务表 + Redis（用于完全重置）
clean_all() {
    echo "  [Clean ALL]"
    echo -n "    MySQL: "
    mysql_exec "DELETE FROM payment_logs" || true
    mysql_exec "DELETE FROM payments" || true
    mysql_exec "DELETE FROM notify_tasks" || true
    mysql_exec "DELETE FROM orders" || true
    mysql_exec "DELETE FROM teams" || true
    echo -n "ok, "
    echo -n "Redis: "
    redis_quiet FLUSHDB
    echo "ok"
}

# ── 压测 ──

# vegeta_run 运行 vegeta 压测，输出文本报告到 stdout 并保存 .txt 文件
# 用法: vegeta_run <name> <rate> <duration> <target_file> [description]
#   name       - 场景名，同时用于结果文件名 (如 "trial_100")
#   rate       - 请求速率，如 "100/s" 或 "100"
#   duration   - 持续时间，如 "10s"
#   target_file- vegeta target JSON 文件路径
#   description- 场景描述 (可选)
vegeta_run() {
    local name="$1" rate="$2" duration="$3" target="$4" desc="${5:-}"
    local total_req=$(wc -l < "$target" | tr -d ' ')

    # 统一 rate 格式（允许传 "100/s" 或 "100"）
    local rate_num="${rate%/s}"

    echo ""
    echo -e "${CYAN}━━━ ${name} ━━━${NC}"
    echo "  场景: ${desc}"
    echo "  参数: rate=${rate_num}/s  duration=${duration}  requests=${total_req}"

    local bin_out="$RESULTS_DIR/${name}.bin"
    local txt_out="$RESULTS_DIR/${name}.txt"

    # 执行压测
    vegeta attack -format=json -rate="${rate_num}/s" -duration="$duration" < "$target" > "$bin_out" 2>/dev/null

    # 输出文本报告（stdout + 文件）
    echo ""
    vegeta report -type=text "$bin_out" | tee "$txt_out"

    # 提取关键指标一行总结
    local p99=$(vegeta report -type=json "$bin_out" 2>/dev/null | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('latencies',{}).get('99th',0)/1000000)" 2>/dev/null || echo "?")
    local success=$(vegeta report -type=json "$bin_out" 2>/dev/null | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('success',0))" 2>/dev/null || echo "?")
    local mean_lat=$(vegeta report -type=json "$bin_out" 2>/dev/null | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('latencies',{}).get('mean',0)/1000000)" 2>/dev/null || echo "?")

    echo ""
    echo -e "  ${GREEN}结果: success=${success}%  latency_mean=${mean_lat}ms  p99=${p99}ms${NC}"
    echo "  报告: $txt_out"
}

# ── 环境检查 ──

# setup 环境准备：种子数据 + 服务检查
# 每个压测脚本开头调用一次，确保种子数据存在、服务可达
setup() {
    # 1. 种子数据（INSERT IGNORE，重复执行无害）
    seed_db

    # 2. 检查服务
    echo -n "  Server: "
    local resp=$(curl -s --noproxy '*' "$BASE_URL/health" 2>/dev/null || echo '{"status":"DOWN"}')
    local status=$(echo "$resp" | python3 -c "import sys,json; print(json.load(sys.stdin).get('status','?'))" 2>/dev/null || echo '?')
    if [ "$status" = "ok" ]; then
        echo -e "${GREEN}UP${NC}"
    else
        echo -e "${RED}DOWN${NC}"
        return 1
    fi
}

# check_server 确认应用健康（别名，兼容旧脚本）
check_server() { setup; }

print_header() {
    echo ""
    echo -e "${CYAN}╔══════════════════════════════════════════════════════╗${NC}"
    echo -e "${CYAN}║${NC}  $1"
    echo -e "${CYAN}╚══════════════════════════════════════════════════════╝${NC}"
    echo ""
}

print_env_info() {
    echo "── 环境 ──"
    echo "  CPU: $(sysctl -n machdep.cpu.brand_string 2>/dev/null || echo '?') ($(sysctl -n hw.ncpu 2>/dev/null || echo '?') cores)"
    echo "  Vegeta: $(vegeta --version 2>&1 | head -1)"
    date '+  时间: %Y-%m-%d %H:%M:%S'
    echo ""
}
