-- take_limit_incr.lua
-- 原子检查用户限购次数 + 递增。
--
-- 场景：支付成功时，检查用户在该活动下是否已达参与上限，
--       未达上限则 +1。整个操作在 Lua 中原子完成。
--
-- 冷启动处理：
--   当 key 不存在时（Redis 重启、活动首次访问），用调用方
--   传入的 dbCount 初始化。整个 SET + INCR 在 Lua 内原子完成，
--   避免两个并发请求同时初始化导致少算。
--
-- 软限制说明：
--   两个不同 outTradeNo 的请求可能同时看到 count=4, limit=5，
--   都 INCR 到 6。这是营销约束不是库存约束，可接受。
--   DB 层有额外校验作为第二道防线。
--
-- KEYS[1]: take limit key  -- bgm:take:{activityId}:{userId} (int64)
-- ARGV[1]: dbCount         -- DB 中已有计数（仅 key 不存在时用于初始化）
-- ARGV[2]: limit           -- 参与上限（activities.take_limit）
-- ARGV[3]: ttlSeconds      -- 过期时间（秒），对齐活动 endTime
--
-- 返回值:
--   >0  = 允许参与，返回值为递增后的新计数
--    0  = 已达上限，拒绝

local key = KEYS[1]
local dbCount = tonumber(ARGV[1])
local limit = tonumber(ARGV[2])
local ttlSeconds = tonumber(ARGV[3])

-- 1. Key 不存在 → 从 DB 计数初始化
--    必须在 INCR 之前初始化，否则 Redis INCR 会从 0 开始（错误）
if redis.call('EXISTS', key) == 0 then
  redis.call('SET', key, dbCount, 'EX', ttlSeconds)
end

-- 2. 获取当前计数并检查是否已达上限
local current = tonumber(redis.call('GET', key))
if current >= limit then
  return 0
end

-- 3. 递增并返回新值
--    INCR 是原子操作，即使多个请求同时到达也不会丢计数
return redis.call('INCR', key)
