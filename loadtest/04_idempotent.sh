#!/bin/bash
# 幂等性压测 — 同 outTradeNo 重复请求 (缓存命中)
# 用法: cd loadtest && bash 04_idempotent.sh
set -eu
source "$(dirname "$0")/lib.sh"

TARGET="targets/lock_idempotent.json"
OUT_TRADE_NO="IDEM-OUT-TRADE-001"
PATTERN="IDEM-%"

check_server || exit 1
print_header "Step 4: Idempotency — 同 outTradeNo 并发"
print_env_info

echo "  数据流: 请求1 → 完整 Lock 路径 → 缓存结果(10min TTL)"
echo "         请求2~N → Redis GET 命中 → 直接返回缓存"
echo "  验证: 1000 个请求同一个 out_trade_no, DB 里只有 1 条订单"
echo ""

# 清理
clean_scenario "$PATTERN"

vegeta_run "idempotent" "200/s" "5s" "$TARGET" \
    "幂等-1000个请求同一outTradeNo, 仅1条DB写入"

# 验证
echo ""
echo "  ── 验证 ──"
ORDER_COUNT=$(mysql_exec "SELECT COUNT(*) FROM orders WHERE out_trade_no='$OUT_TRADE_NO'")
echo "  DB 订单数 (out_trade_no=$OUT_TRADE_NO): $ORDER_COUNT (期望 1)"
if [ "$ORDER_COUNT" -eq 1 ]; then
    echo -e "  ${GREEN}幂等正确: 1000次请求仅创建1条订单${NC}"
else
    echo -e "  ${RED}幂等异常: 期望1条, 实际${ORDER_COUNT}条${NC}"
fi

# 清理
clean_scenario "$PATTERN"
