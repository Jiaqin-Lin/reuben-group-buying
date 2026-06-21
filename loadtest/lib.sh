#!/bin/bash
# 拼团系统压测 — 共享函数库
# 用法: source "$(dirname "$0")/lib.sh"
#
# 支持 macOS 和 Linux；自动检测 docker 容器名和 vegeta 路径。

set -e
export no_proxy="localhost,127.0.0.1,::1"
export NO_PROXY="localhost,127.0.0.1,::1"

BASE_URL="${BASE_URL:-http://localhost:8080}"
RESULTS_DIR="$(dirname "$0")/results"
mkdir -p "$RESULTS_DIR"

# ── 颜色 ──
RED='\033[0;31m'; GREEN='\033[0;32m'; CYAN='\033[0;36m'; YELLOW='\033[0;33m'; NC='\033[0m'

# ── 自动检测 ──
# Docker 容器名：本地 macOS 用 group-buy-*，dev 服务器用 gb-*
if docker ps --format '{{.Names}}' 2>/dev/null | grep -q '^group-buy-mysql$'; then
    MYSQL_CONTAINER="group-buy-mysql"
    REDIS_CONTAINER="group-buy-redis"
else
    MYSQL_CONTAINER="gb-mysql"
    REDIS_CONTAINER="gb-redis"
fi

# vegeta 路径：优先用 PATH 里的，否则用 ~/vegeta
if command -v vegeta &>/dev/null; then
    VEGETA="vegeta"
elif [ -x "$HOME/vegeta" ]; then
    VEGETA="$HOME/vegeta"
else
    echo "ERROR: vegeta not found. Install: go install github.com/tsenart/vegeta/v12@latest"
    exit 1
fi

# ── 基础设施 ──

mysql_exec() {
    docker exec "$MYSQL_CONTAINER" mysql -u dev -pdev123 -N group_buy_market -e "$1" 2>/dev/null || true
}

redis_exec() {
    docker exec "$REDIS_CONTAINER" redis-cli "$@" 2>/dev/null || true
}

redis_quiet() {
    docker exec "$REDIS_CONTAINER" redis-cli "$@" > /dev/null 2>&1 || true
}

# ── 数据管理 ──

seed_db() {
    local seed_file="$(dirname "$0")/seed.sql"
    echo "  [Seed] 插入种子数据..."
    docker exec -i "$MYSQL_CONTAINER" mysql -u dev -pdev123 --default-character-set=utf8mb4 group_buy_market < "$seed_file" 2>/dev/null
    echo "  [Seed] 完成"
}

clean_scenario() {
    local pattern="${1:-LOADTEST-%}"
    echo "  [Clean] pattern=$pattern"
    mysql_exec "DELETE FROM payments WHERE order_id IN (SELECT order_id FROM orders WHERE out_trade_no LIKE '$pattern')" || true
    mysql_exec "DELETE FROM notify_tasks WHERE team_id IN (SELECT team_id FROM orders WHERE out_trade_no LIKE '$pattern')" || true
    mysql_exec "DELETE FROM orders WHERE out_trade_no LIKE '$pattern'" || true
    mysql_exec "DELETE t FROM teams t LEFT JOIN orders o ON t.team_id = o.team_id WHERE o.team_id IS NULL" || true
    # 清 Redis（Lua 批量删除，避免 docker exec --scan 逐 key 太慢）
    redis_quiet EVAL "local ks=redis.call('KEYS','bgm:*') for _,k in ipairs(ks) do redis.call('DEL',k) end return #ks" 0
    echo "  [Clean] done"
}

clean_all() {
    echo "  [Clean ALL]"
    mysql_exec "DELETE FROM payment_logs" || true
    mysql_exec "DELETE FROM payments" || true
    mysql_exec "DELETE FROM notify_tasks" || true
    mysql_exec "DELETE FROM orders" || true
    mysql_exec "DELETE FROM teams" || true
    redis_quiet FLUSHDB
    echo "  [Clean ALL] done"
}

# ── 压测 ──

vegeta_run() {
    local name="$1" rate="$2" duration="$3" target="$4" desc="${5:-}"
    local rate_num="${rate%/s}"

    echo ""
    echo -e "${CYAN}━━━ ${name} ━━━${NC}"
    echo "  场景: ${desc}"
    echo "  参数: rate=${rate_num}/s  duration=${duration}"

    local bin_out="$RESULTS_DIR/${name}.bin"
    local txt_out="$RESULTS_DIR/${name}.txt"

    $VEGETA attack -format=json -rate="${rate_num}/s" -duration="$duration" < "$target" > "$bin_out" 2>/dev/null

    echo ""
    $VEGETA report -type=text "$bin_out" | tee "$txt_out"

    local p99=$($VEGETA report -type=json "$bin_out" 2>/dev/null | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('latencies',{}).get('99th',0)/1000000)" 2>/dev/null || echo "?")
    local success=$($VEGETA report -type=json "$bin_out" 2>/dev/null | python3 -c "import sys,json; d=json.load(sys.stdin); print(round(d.get('success',0)*100,1))" 2>/dev/null || echo "?")
    local mean_lat=$($VEGETA report -type=json "$bin_out" 2>/dev/null | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('latencies',{}).get('mean',0)/1000000)" 2>/dev/null || echo "?")

    echo ""
    echo -e "  ${GREEN}结果: success=${success}%  latency_mean=${mean_lat}ms  p99=${p99}ms${NC}"
}

# ── 环境检查 ──

setup() {
    seed_db
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
    if [[ "$(uname -s)" == "Darwin" ]]; then
        echo "  CPU: $(sysctl -n machdep.cpu.brand_string 2>/dev/null) ($(sysctl -n hw.ncpu 2>/dev/null) cores)"
    else
        echo "  CPU: $(grep -m1 'model name' /proc/cpuinfo 2>/dev/null | cut -d: -f2 | xargs || echo '?') ($(nproc) cores)"
    fi
    echo "  Vegeta: $($VEGETA --version 2>&1 | head -1)"
    date '+  时间: %Y-%m-%d %H:%M:%S'
    echo ""
}
