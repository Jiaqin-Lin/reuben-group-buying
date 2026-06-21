#!/bin/bash
# 生成 vegeta JSON 格式的压测目标文件。
# 用法: bash generate_targets.sh
# 输出: loadtest/targets/*.json
set -eu

BASE="${BASE_URL:-http://localhost:8080}"
OUT="$(dirname "$0")/targets"
mkdir -p "$OUT"

python3 -c "
import json, base64, os, random

OUT = '$OUT'
BASE = '$BASE'

def write_targets(path, entries):
    with open(path, 'w') as f:
        for e in entries:
            body = json.dumps(e['body'], ensure_ascii=False)
            t = {
                'method': e.get('method', 'POST'),
                'url': BASE + e['url'],
                'header': {'Content-Type': ['application/json']},
                'body': base64.b64encode(body.encode()).decode(),
            }
            json.dump(t, f)
            f.write('\n')
    print(f'  {path}: {len(entries)} targets')

# ── 试算: 10000 个不同用户+商品 ──
trial_goods = ['G_ZJ', 'G_MJ', 'G_ZK', 'G_N', 'G_LOCK1']
entries = []
for i in range(10000):
    uid = f'U{random.randint(1, 999999):06d}'
    gid = random.choice(trial_goods)
    entries.append({
        'url': '/api/v1/trial',
        'body': {'user_id': uid, 'goods_id': gid, 'source': 'APP', 'channel': 'WECHAT'},
    })
write_targets(OUT + '/trial.json', entries)

# ── 锁单-新建团: 5000 个不同用户，各自新建团 ──
entries = []
for i in range(5000):
    uid = f'U{i+100000:06d}'
    otn = f'LOADTEST-{uid}-{i}'
    entries.append({
        'url': '/api/v1/trade/lock',
        'body': {
            'user_id': uid,
            'activity_id': 300001,
            'goods_id': 'G_LOCK1',
            'source': 'APP', 'channel': 'WECHAT',
            'out_trade_no': otn,
        },
    })
write_targets(OUT + '/lock.json', entries)

# ── 锁单-幂等: 1000 个请求，同一个 out_trade_no ──
entries = []
for i in range(1000):
    entries.append({
        'url': '/api/v1/trade/lock',
        'body': {
            'user_id': 'U_IDEMPOTENT',
            'activity_id': 300001,
            'goods_id': 'G_LOCK1',
            'source': 'APP', 'channel': 'WECHAT',
            'out_trade_no': 'LOADTEST-IDEMPOTENT-001',
        },
    })
write_targets(OUT + '/lock_idempotent.json', entries)

# ── 锁单-同团: 500 个不同用户，加入同一个团 ──
entries = []
for i in range(500):
    uid = f'U_TEAM{i:04d}'
    otn = f'TEAMTEST-{uid}-{i}'
    entries.append({
        'url': '/api/v1/trade/lock',
        'body': {
            'user_id': uid,
            'activity_id': 300003,  # 2人团活动
            'goods_id': 'G_LOCK3',
            'source': 'APP', 'channel': 'WECHAT',
            'out_trade_no': otn,
            'team_id': 'TEAM-SHARED-001',
        },
    })
write_targets(OUT + '/lock_same_team.json', entries)

# ── 结算: 5000 个不同 out_trade_no (需先跑锁单产生订单) ──
entries = []
for i in range(5000):
    uid = f'U{i+100000:06d}'
    otn = f'LOADTEST-{uid}-{i}'
    entries.append({
        'url': '/api/v1/trade/settlement',
        'body': {
            'user_id': uid,
            'out_trade_no': otn,
            'out_trade_time': '2026-06-21T16:00:00+08:00',
            'source': 'APP',
            'channel': 'WECHAT',
        },
    })
write_targets(OUT + '/settlement.json', entries)

print('Done: all target files generated')
"
