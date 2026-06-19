// Package errcode 统一业务错误码。
// 简化自 Java ResponseCode.java，保留业务语义但去掉了枚举的过度设计。
package errcode

// 错误码常量
const (
	CodeSuccess = "0000"

	CodeUnknownErr     = "0001" // 未知失败
	CodeInvalidParam   = "0002" // 非法参数
	CodeDuplicateEntry = "0003" // 唯一索引冲突
	CodeUpdateZero     = "0004" // 更新记录为0
	CodeHTTPError      = "0005" // HTTP接口调用异常

	// 拼团业务错误码
	CodeNoDiscountService   = "E0001" // 不存在对应的折扣计算服务
	CodeTrialFailed         = "E0002" // 无拼团营销配置或试算结果异常
	CodeActivityDegrade     = "E0003" // 拼团活动降级拦截
	CodeActivityCutOver     = "E0004" // 拼团活动切量拦截
	CodeTeamFull            = "E0006" // 拼团组队完结，锁单量已达成
	CodeCrowdBlocked        = "E0007" // 拼团人群限定，不可参与
	CodeStockInsufficient   = "E0008" // 拼团组队失败，缓存库存不足

	CodeActivityInactive    = "E0101" // 拼团活动未生效
	CodeActivityTimeInvalid = "E0102" // 不在拼团活动有效时间内
	CodeTakeLimitReached    = "E0103" // 当前用户参与此拼团次数已达上限
	CodeOrderNotFound       = "E0104" // 不存在的外部交易单号或用户已退单
	CodeChannelBlacklisted  = "E0105" // SC渠道黑名单拦截
	CodeOrderTimeInvalid    = "E0106" // 订单交易时间不在拼团有效时间范围内
	CodeRefundStateInvalid  = "E0107" // 订单状态不允许退单

	CodeRateLimit = "E0200" // 请求过于频繁
)

// 错误码对应的默认消息
var codeMessages = map[string]string{
	CodeSuccess:             "成功",
	CodeUnknownErr:          "未知失败",
	CodeInvalidParam:        "非法参数",
	CodeDuplicateEntry:      "唯一索引冲突",
	CodeUpdateZero:          "更新记录为0",
	CodeHTTPError:           "HTTP接口调用异常",
	CodeNoDiscountService:   "不存在对应的折扣计算服务",
	CodeTrialFailed:         "无拼团营销配置或试算结果异常",
	CodeActivityDegrade:     "拼团活动降级拦截",
	CodeActivityCutOver:     "拼团活动切量拦截",
	CodeTeamFull:            "拼团组队完结，锁单量已达成",
	CodeCrowdBlocked:        "拼团人群限定，不可参与",
	CodeStockInsufficient:   "拼团组队失败，缓存库存不足",
	CodeActivityInactive:    "拼团活动未生效",
	CodeActivityTimeInvalid: "不在拼团活动有效时间内",
	CodeTakeLimitReached:    "当前用户参与此拼团次数已达上限",
	CodeOrderNotFound:       "不存在的外部交易单号或用户已退单",
	CodeChannelBlacklisted:  "SC渠道黑名单拦截",
	CodeOrderTimeInvalid:    "订单交易时间不在拼团有效时间范围内",
	CodeRefundStateInvalid:  "订单状态不允许退单",
	CodeRateLimit:           "请求过于频繁",
}

// Message 返回错误码对应的默认消息。
func Message(code string) string {
	if msg, ok := codeMessages[code]; ok {
		return msg
	}
	return "未知错误"
}
