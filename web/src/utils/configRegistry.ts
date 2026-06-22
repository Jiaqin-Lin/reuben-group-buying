/**
 * Dynamic config metadata registry.
 * Mirrors Go internal/config/dynamic/defs.go
 * Used by the admin ConfigPage to show type, defaults, and descriptions.
 */

export type ConfigType = 'int' | 'bool' | 'string';

export interface ConfigDef {
  key: string;
  type: ConfigType;
  default: unknown;
  description: string;
}

export const CONFIG_REGISTRY: ConfigDef[] = [
  {
    key: 'trial.cache_ttl',
    type: 'int',
    default: 0,
    description: '试算结果缓存 TTL（秒），0 表示禁用缓存',
  },
  {
    key: 'lock.result_ttl',
    type: 'int',
    default: 600,
    description: '锁单结果缓存 TTL（秒），用于幂等返回',
  },
  {
    key: 'order.lock_ttl',
    type: 'int',
    default: 3,
    description: '分布式锁 TTL（秒），防止同一单号并发',
  },
  {
    key: 'notify.max_retry',
    type: 'int',
    default: 5,
    description: '回调通知最大重试次数',
  },
  {
    key: 'notify.worker_count',
    type: 'int',
    default: 10,
    description: '回调通知并发 worker 数量',
  },
  {
    key: 'timeout.scan_batch',
    type: 'int',
    default: 100,
    description: '超时扫描每批处理数量',
  },
  {
    key: 'feature.skip_crowd',
    type: 'bool',
    default: false,
    description: '跳过人群标签检查（应急开关）',
  },
  {
    key: 'feature.use_mock_payment',
    type: 'bool',
    default: true,
    description: 'Mock支付：true=Mock自动成功 false=真实支付宝',
  },
];

export function getConfigDef(key: string): ConfigDef | undefined {
  return CONFIG_REGISTRY.find((c) => c.key === key);
}

export function getConfigTypeLabel(type: ConfigType): string {
  switch (type) {
    case 'int':
      return '整数';
    case 'bool':
      return '布尔';
    case 'string':
      return '字符串';
  }
}
