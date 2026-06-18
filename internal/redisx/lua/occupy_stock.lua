-- occupy_stock.lua
-- 原子占用团名额，基于 Redis SET 的信号量模式。
--
-- 为什么用 SET 而不是 INCR/DECR 计数器？
--   1. 幂等：SET 用 SISMEMBER 天然支持同 permitId 重复加入
--   2. 可审计：SMEMBERS 能查出团里有哪些人，排查问题方便
--   3. 释放精准：SREM 只移除指定 permitId，不会多删少删
--
-- KEYS[1]: holders key  -- bgm:stock:{activityId}:{teamId}:holders (SET)
-- KEYS[2]: full key     -- bgm:stock:{activityId}:{teamId}:full (String, "1" = 已满)
-- ARGV[1]: permitId     -- outTradeNo, 防同一外部单号重复占位
-- ARGV[2]: targetCount  -- 团目标人数（成团需要的人数上限）
-- ARGV[3]: ttlSeconds   -- 过期时间（秒），仅在首次加入时设置
--
-- 返回值:
--   1  = 占用成功
--   2  = 已占用（幂等，permitId 已在 SET 中，重复请求安全）
--  -1  = 团已满（无法加入）

local holdersKey = KEYS[1]
local fullKey = KEYS[2]
local permitId = ARGV[1]
local targetCount = tonumber(ARGV[2])
local ttlSeconds = tonumber(ARGV[3])

-- 1. 快速失败：团已标记满 → 直接拒绝，避免不必要的 SCARD
if redis.call('EXISTS', fullKey) == 1 then
  return -1
end

-- 2. 幂等检查：同 permitId 重复请求，直接返回成功
--    这是防重核心：即使 Redis 操作成功但 DB 写入失败，
--    用户用同 outTradeNo 重试时不会多占名额
if redis.call('SISMEMBER', holdersKey, permitId) == 1 then
  return 2
end

-- 3. 容量检查：当前已占用人数是否已达上限
local current = redis.call('SCARD', holdersKey)
if current >= targetCount then
  -- 标记满，防止后续请求继续 SCARD（性能优化）
  -- TTL 与团有效期一致，到期自动清理
  redis.call('SET', fullKey, '1', 'EX', ttlSeconds)
  return -1
end

-- 4. 占用名额
redis.call('SADD', holdersKey, permitId)

-- 5. TTL 管理：仅在首次加入时设置过期时间
--    设计理由：团队有效期从创建（第一个成员加入）起算，
--    不应因后续加入而延长。例如 30 分钟拼团，第一个用户
--    10:00 加入，团队 10:30 到期；第 10 分钟加入的用户
--    不应该把到期时间延到 10:40。
--    注意：SADD 在 key 不存在时会创建 key（TTL = -1）
local currentTTL = redis.call('TTL', holdersKey)
if currentTTL == -1 then
  redis.call('EXPIRE', holdersKey, ttlSeconds)
end

-- 6. 检查本次加入后是否满员，满则标记
--    放在 SADD 之后检查，用 current+1 避免再次 SCARD
if current + 1 >= targetCount then
  redis.call('SET', fullKey, '1', 'EX', ttlSeconds)
end

return 1
