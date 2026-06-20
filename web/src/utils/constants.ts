/** Order status enum (matches Go model Order.Status) */
export const OrderStatus = {
  Locked: 0,
  Paid: 1,
  Refunded: 2,
} as const;

export const OrderStatusLabel: Record<number, string> = {
  0: '待支付',
  1: '已支付',
  2: '已退款',
};

/** Team status enum (matches Go model Team.Status) */
export const TeamStatus = {
  Forming: 0,
  Complete: 1,
  Failed: 2,
  CompleteWithRefunds: 3,
} as const;

export const TeamStatusLabel: Record<number, string> = {
  0: '成团中',
  1: '已成团',
  2: '已失败',
  3: '已成团(部分退款)',
};

/** Payment status enum */
export const PaymentStatus = {
  Pending: 0,
  Paid: 1,
  Closed: 2,
} as const;

export const PaymentStatusLabel: Record<number, string> = {
  0: '待支付',
  1: '已支付',
  2: '已关闭',
};

/** Discount plan types */
export const PlanTypeLabel: Record<string, string> = {
  ZJ: '直减',
  MJ: '满减',
  ZK: '折扣',
  N: '固定价',
};

/** Activity status */
export const ActivityStatusLabel: Record<number, string> = {
  0: '已创建',
  1: '进行中',
  2: '已过期',
  3: '已废弃',
};

/** Notify task status */
export const NotifyStatusLabel: Record<number, string> = {
  0: '待发送',
  1: '已成功',
  2: '重试中',
  3: '已失败',
};

/** Error code to Chinese message mapping */
export const ErrorMessages: Record<string, string> = {
  '0000': '成功',
  '0001': '未知错误',
  '0002': '参数无效',
  '0003': '重复请求',
  '0004': '更新失败',
  '0005': 'HTTP 错误',
  E0001: '无可用折扣',
  E0002: '试算失败（无配置）',
  E0003: '活动已降级',
  E0004: '活动已切换',
  E0006: '拼团已满',
  E0007: '人群标签受限',
  E0008: '名额不足',
  E0101: '活动未开启',
  E0102: '活动时间无效',
  E0103: '参与次数已达上限',
  E0104: '订单不存在',
  E0105: '渠道已黑名单',
  E0106: '订单时间无效',
  E0107: '退款状态无效',
  E0200: '请求过于频繁',
  P0001: '支付创建失败',
  P0002: '支付通知验签失败',
  P0003: '支付通知订单不存在',
  P0004: '退款失败',
};

export function getErrorMessage(code: string): string {
  return ErrorMessages[code] || `未知错误 (${code})`;
}
