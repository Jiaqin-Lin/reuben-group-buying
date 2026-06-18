-- release_stock.lua
-- 原子释放团名额，退单/超时取消时调用。
--
-- 操作是幂等的：permitId 不在 SET 中也不报错。
-- 同时删除 full key，让新人能重新加入。
--
-- KEYS[1]: holders key  -- bgm:stock:{activityId}:{teamId}:holders (SET)
-- KEYS[2]: full key     -- bgm:stock:{activityId}:{teamId}:full (String)
-- ARGV[1]: permitId     -- outTradeNo，要释放的占位标识

local holdersKey = KEYS[1]
local fullKey = KEYS[2]
local permitId = ARGV[1]

-- 1. 从 holders SET 中移除 permitId
--    SREM 是幂等的：成员不存在也不报错，返回 0
redis.call('SREM', holdersKey, permitId)

-- 2. 删除满标，允许新人重新加入
--    注意：即使团不是因为满员而释放（例如只有 2/3 人时退单），
--    删 full key 也是安全的——它本来就不存在，DEL 返回 0
redis.call('DEL', fullKey)

return 1
