#!/bin/bash
# ==============================================================================
# 秒杀压测 — N 人抢 M 个名额 (库存不足, Redis 满标快速拒绝)
# ==============================================================================
# 为什么这个脚本有 3 个参数？因为秒杀场景的变量比其他场景多：
#   参数1: 多少人同时抢 (默认 500)
#   参数2: 团一共几个名额 (默认 3，含创始人占的1个)
#   参数3: 每秒发多少请求 (默认 200)
# 其他场景(02_trial, 03_lock)只需要调 rate，因为目标数固定。
# ==============================================================================
# 用法:
#   bash 05_seckill.sh              # 默认: 500人抢3名额, 200/s
#   bash 05_seckill.sh 1000 5 500   # 1000人抢5名额, 500/s
#   bash 05_seckill.sh 100 2 100    # 100人抢2名额(即1人成功), 100/s
set -eu
source "$(dirname "$0")/lib.sh"

# ── 参数 ──
N="${1:-500}"          # 抢团人数
M="${2:-3}"            # 总名额（含创始人）
RATE="${3:-200}"       # 请求速率/s
ACTIVITY_ID=200001
GOODS_ID="G_ZJ"
FOUNDER_UID="SECKILL-FOUNDER"
PREFIX="SECKILL"

check_server || exit 1
print_header "Step 5: Seckill — ${N}人抢${M}名额"

echo "  数据流: 第1个请求(创始人) → 完整 Lock → 占第1个名额"
echo "         后续N个请求 → Redis Lua(满标检查) → 仅${M}-1人成功 → 其余被Redis拦截"
echo "         拦截路径: 0 DB查询, 纯Redis拒绝"
echo "  关键Redis Key: bgm:stock:${ACTIVITY_ID}:{teamId}:full"
echo ""

# ═══════════════════════════════════════════════════
# 1. 清理
# ═══════════════════════════════════════════════════
clean_scenario "${PREFIX}-%"
# 额外清理可能残留的 stock key
redis_exec --scan --pattern "bgm:stock:${ACTIVITY_ID}:SECKILL*" | while read -r key; do
    [ -n "$key" ] && redis_quiet DEL "$key"
done

# ═══════════════════════════════════════════════════
# 2. 创始人创建团
# ═══════════════════════════════════════════════════
echo "  [Setup] 创始人创建目标团..."
CREATE_RESP=$(curl -s --noproxy '*' -X POST "$BASE_URL/api/v1/trade/lock" \
    -H 'Content-Type: application/json' \
    -d "{\"user_id\":\"$FOUNDER_UID\",\"activity_id\":$ACTIVITY_ID,\"goods_id\":\"$GOODS_ID\",\"source\":\"APP\",\"channel\":\"WECHAT\",\"out_trade_no\":\"${PREFIX}-CREATE-TEAM\"}")

TEAM_ID=$(echo "$CREATE_RESP" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('data',{}).get('team_id',''))" 2>/dev/null)

if [ -z "$TEAM_ID" ] || [ "$TEAM_ID" = "null" ]; then
    echo -e "  ${RED}✗ 创始人创建团失败!${NC}"
    echo "  Response: $CREATE_RESP"
    exit 1
fi

echo -e "  ${GREEN}✓${NC} Team: $TEAM_ID (已占 1/${M} 名额)"

# ═══════════════════════════════════════════════════
# 3. 生成抢团 targets
# ═══════════════════════════════════════════════════
echo "  [Setup] 生成 ${N} 个抢团请求..."
TMP_TARGET=$(mktemp /tmp/seckill_targets.XXXXXX.json)
for i in $(seq 0 $((N - 1))); do
    u_id=$(printf "SK%06d" $i)
    BODY="{\"user_id\":\"$u_id\",\"activity_id\":$ACTIVITY_ID,\"goods_id\":\"$GOODS_ID\",\"source\":\"APP\",\"channel\":\"WECHAT\",\"out_trade_no\":\"${PREFIX}-$u_id\",\"team_id\":\"$TEAM_ID\"}"
    BODY_B64=$(echo -n "$BODY" | base64)
    echo "{\"method\":\"POST\",\"url\":\"http://localhost:8080/api/v1/trade/lock\",\"header\":{\"Content-Type\":[\"application/json\"]},\"body\":\"$BODY_B64\"}"
done > "$TMP_TARGET"

# ═══════════════════════════════════════════════════
# 4. 压测
# ═══════════════════════════════════════════════════
# 持续时间 = N / rate (确保发完所有请求)
DURATION=$(python3 -c "import math; print(math.ceil($N / $RATE) + 0.5)")
vegeta_run "seckill_${N}_${M}" "$RATE" "${DURATION}s" "$TMP_TARGET" \
    "${N}人抢${M}名额(已占1), 仅$((M-1))人成功→其余被Redis拦截"

# ═══════════════════════════════════════════════════
# 5. 验证
# ═══════════════════════════════════════════════════
echo ""
echo "  ── 验证 ──"

# 团状态
TEAM_INFO=$(mysql_exec "SELECT CONCAT('lock_count=', lock_count, ' target=', target_count, ' status=', status) FROM teams WHERE team_id='$TEAM_ID'")
echo "  Team $TEAM_ID: $TEAM_INFO"

# 团内订单数
JOIN_COUNT=$(mysql_exec "SELECT COUNT(*) FROM orders WHERE team_id='$TEAM_ID'")
echo "  团内订单数: $JOIN_COUNT (期望 $M = 1创始人 + $((M-1))抢到)"

# Redis full key
FULL_KEY=$(redis_exec EXISTS "bgm:stock:${ACTIVITY_ID}:${TEAM_ID}:full")
echo "  Redis bgm:stock:${ACTIVITY_ID}:${TEAM_ID}:full = $FULL_KEY (期望 1, 满标标记)"

# 成功/拦截统计（从 vegeta JSON 结果分析）
echo ""
echo "  ── 请求分布 ──"
	echo "  期望: ${M} 人成功加入团, $((N + 1 - M)) 个请求被拦截"

rm -f "$TMP_TARGET"
