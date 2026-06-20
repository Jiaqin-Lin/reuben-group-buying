import { Badge } from './Badge';

interface StatusConfig {
  variant: 'success' | 'warning' | 'error' | 'info' | 'neutral';
  label: string;
}

const STATUS_MAP: Record<string, Record<number, StatusConfig>> = {
  order: {
    0: { variant: 'warning', label: '待支付' },
    1: { variant: 'success', label: '已支付' },
    2: { variant: 'error', label: '已退款' },
  },
  team: {
    0: { variant: 'warning', label: '成团中' },
    1: { variant: 'success', label: '已成团' },
    2: { variant: 'error', label: '已失败' },
    3: { variant: 'info', label: '已成团(含退款)' },
  },
  payment: {
    0: { variant: 'warning', label: '待支付' },
    1: { variant: 'success', label: '已支付' },
    2: { variant: 'error', label: '已关闭' },
  },
  activity: {
    0: { variant: 'neutral', label: '已创建' },
    1: { variant: 'success', label: '进行中' },
    2: { variant: 'error', label: '已过期' },
    3: { variant: 'warning', label: '已废弃' },
  },
  notify: {
    0: { variant: 'warning', label: '待发送' },
    1: { variant: 'success', label: '已成功' },
    2: { variant: 'info', label: '重试中' },
    3: { variant: 'error', label: '已失败' },
  },
};

interface StatusBadgeProps {
  type: keyof typeof STATUS_MAP;
  status: number;
}

export function StatusBadge({ type, status }: StatusBadgeProps) {
  const config = STATUS_MAP[type]?.[status];
  if (!config) {
    return <Badge variant="neutral">{status}</Badge>;
  }
  return <Badge variant={config.variant}>{config.label}</Badge>;
}
