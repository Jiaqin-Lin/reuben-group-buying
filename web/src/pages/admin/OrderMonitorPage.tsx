import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { adminApi } from '../../api/admin';
import { Card } from '../../components/ui/Card';
import { Button } from '../../components/ui/Button';
import { Select } from '../../components/ui/Select';
import { StatusBadge } from '../../components/ui/StatusBadge';
import { Loading } from '../../components/ui/Loading';
import { ErrorState } from '../../components/ui/ErrorState';
import { formatPrice, formatTime } from '../../utils/format';
import type { Order } from '../../api/types';

const STATUS_OPTS = [
  { value: '', label: '全部' },
  { value: '0', label: '待支付' },
  { value: '1', label: '已支付' },
  { value: '2', label: '已退款' },
];

export function OrderMonitorPage() {
  const [status, setStatus] = useState('');
  const [page, setPage] = useState(1);

  const { data, isLoading, isError, refetch } = useQuery({
    queryKey: ['admin', 'orders-monitor', status, page],
    queryFn: () => adminApi.listOrders({
      status: status ? Number(status) : undefined,
      page,
      page_size: 20,
    }),
  });

  if (isLoading) return <Loading lines={10} />;
  if (isError) return <ErrorState message="加载失败" onRetry={() => refetch()} />;

  const totalPages = Math.ceil((data?.total || 0) / 20);

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-xl font-medium text-[var(--color-text-primary)]">订单监控</h1>
        <Select
          options={STATUS_OPTS}
          value={status}
          onChange={e => { setStatus(e.target.value); setPage(1); }}
          className="w-32"
        />
      </div>

      <Card padding="none" className="overflow-x-auto">
        <table className="w-full">
          <thead>
            <tr className="bg-[var(--color-canvas)] border-b border-[#EAEAEA]">
              <th className="h-10 px-4 text-xs font-medium text-[var(--color-text-secondary)] text-left uppercase">订单号</th>
              <th className="h-10 px-4 text-xs font-medium text-[var(--color-text-secondary)] text-left uppercase">用户</th>
              <th className="h-10 px-4 text-xs font-medium text-[var(--color-text-secondary)] text-left uppercase">队伍</th>
              <th className="h-10 px-4 text-xs font-medium text-[var(--color-text-secondary)] text-left uppercase">金额</th>
              <th className="h-10 px-4 text-xs font-medium text-[var(--color-text-secondary)] text-left uppercase">状态</th>
              <th className="h-10 px-4 text-xs font-medium text-[var(--color-text-secondary)] text-left uppercase">时间</th>
            </tr>
          </thead>
          <tbody>
            {data?.items?.map((o: Order) => (
              <tr key={o.order_id} className="border-b border-[#EAEAEA] last:border-0">
                <td className="px-4 py-3 text-sm font-mono">{o.order_id}</td>
                <td className="px-4 py-3 text-sm">{o.user_id}</td>
                <td className="px-4 py-3 text-sm font-mono">{o.team_id}</td>
                <td className="px-4 py-3 text-sm font-mono text-[var(--color-accent)]">{formatPrice(o.pay_price)}</td>
                <td className="px-4 py-3"><StatusBadge type="order" status={o.status} /></td>
                <td className="px-4 py-3 text-xs text-[var(--color-text-muted)]">{formatTime(o.created_at)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </Card>

      {totalPages > 1 && (
        <div className="flex items-center justify-center gap-2 mt-4">
          <Button variant="secondary" size="sm" disabled={page <= 1} onClick={() => setPage(p => p - 1)}>上一页</Button>
          <span className="text-sm text-[var(--color-text-muted)]">{page} / {totalPages}</span>
          <Button variant="secondary" size="sm" disabled={page >= totalPages} onClick={() => setPage(p => p + 1)}>下一页</Button>
        </div>
      )}
    </div>
  );
}
