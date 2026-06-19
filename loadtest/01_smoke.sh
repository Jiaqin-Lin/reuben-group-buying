#!/bin/bash
# 冒烟测试 — 快速验证 4 个核心接口功能正常
# 用法: cd loadtest && bash 01_smoke.sh
set -eu
source "$(dirname "$0")/lib.sh"

print_header "Step 1: Smoke Test (功能验证)"

check_server || { echo -e "${RED}Server not running!${NC}"; exit 1; }

B="$BASE_URL"
PASS=0
FAIL=0

check() {
    local label="$1" resp
    resp=$(curl -s --noproxy '*' "${@:2}")
    if echo "$resp" | grep -q '"code":"0000"'; then
        echo -e "  ${GREEN}✓${NC} $label"
        PASS=$((PASS+1))
    else
        echo -e "  ${RED}✗${NC} $label FAIL"
        echo "    response: $resp"
        FAIL=$((FAIL+1))
    fi
}

echo ""

# 1. 试算
check "Trial"  -X POST "$B/api/v1/trial" \
    -H 'Content-Type: application/json' \
    -d '{"user_id":"SMOKE","goods_id":"G_ZJ","source":"APP","channel":"WECHAT"}'

# 2. 锁单
check "Lock"   -X POST "$B/api/v1/trade/lock" \
    -H 'Content-Type: application/json' \
    -d '{"user_id":"SMOKE","activity_id":200001,"goods_id":"G_ZJ","source":"APP","channel":"WECHAT","out_trade_no":"SMOKE-001"}'

# 3. 幂等（同 out_trade_no 重复锁单 → 返回缓存）
check "Idempotent" -X POST "$B/api/v1/trade/lock" \
    -H 'Content-Type: application/json' \
    -d '{"user_id":"SMOKE","activity_id":200001,"goods_id":"G_ZJ","source":"APP","channel":"WECHAT","out_trade_no":"SMOKE-001"}'

# 4. 结算
check "Settlement" -X POST "$B/api/v1/trade/settlement" \
    -H 'Content-Type: application/json' \
    -d '{"user_id":"SMOKE","out_trade_no":"SMOKE-001","out_trade_time":"2026-06-19T16:00:00+08:00","source":"APP","channel":"WECHAT"}'

# 5. 退单
check "Refund"  -X POST "$B/api/v1/trade/refund" \
    -H 'Content-Type: application/json' \
    -d '{"user_id":"SMOKE","out_trade_no":"SMOKE-001"}'

# 清理冒烟数据
clean_scenario "SMOKE-%"

echo ""
echo -e "  ${GREEN}通过: $PASS${NC}  ${RED}失败: $FAIL${NC}"
[ "$FAIL" -eq 0 ] || exit 1
