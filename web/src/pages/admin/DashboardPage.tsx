import { useQuery } from '@tanstack/react-query';
import { adminApi } from '../../api/admin';
import { Card } from '../../components/ui/Card';
import { StatusBadge } from '../../components/ui/StatusBadge';
import { Loading } from '../../components/ui/Loading';
import { ErrorState } from '../../components/ui/ErrorState';
import { formatPrice, formatTime } from '../../utils/format';
import type { Order } from '../../api/types';

function StatsCard({ label, value }: { label: string; value: string | number }) {
  return (
    <Card padding="lg" className="flex flex-col gap-1">
      <span className="text-xs text-[var(--color-text-muted)] tracking-wider uppercase">
        {label}
      </span>
      <span className="text-3xl font-medium text-[var(--color-text-primary)] font-mono tabular-nums">
        {value}
      </span>
    </Card>
  );
}

export function DashboardPage() {
  const { data, isLoading, isError, refetch } = useQuery({
    queryKey: ['admin', 'dashboard'],
    queryFn: adminApi.getDashboard,
  });

  if (isLoading) return <Loading lines={6} />;
  if (isError) return <ErrorState message="加载仪表盘失败" onRetry={() => refetch()} />;

  return (
    <div>
      <h1 className="text-xl font-medium text-[var(--color-text-primary)] mb-6">
        仪表盘
      </h1>

      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4 mb-8">
        <StatsCard label="活跃活动" value={data?.active_activities ?? 0} />
        <StatsCard label="成团中队伍" value={data?.forming_teams ?? 0} />
        <StatsCard label="已成团" value={data?.complete_teams ?? 0} />
        <StatsCard label="今日订单" value={data?.today_orders ?? 0} />
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-4 mb-8">
        <StatsCard label="已失败队伍" value={data?.failed_teams ?? 0} />
        <StatsCard label="配置加载项" value={data?.config_count ?? 0} />
      </div>

      {data?.recent_orders && data.recent_orders.length > 0 && (
        <Card padding="none" className="overflow-hidden">
          <div className="px-5 py-3 border-b border-[#EAEAEA]">
            <h2 className="text-sm font-medium text-[var(--color-text-primary)]">
              最近订单
            </h2>
          </div>
          <table className="w-full">
            <thead>
              <tr className="bg-[var(--color-canvas)] border-b border-[#EAEAEA]">
                <th className="h-10 px-4 text-xs font-medium text-[var(--color-text-secondary)] text-left uppercase tracking-wider">订单号</th>
                <th className="h-10 px-4 text-xs font-medium text-[var(--color-text-secondary)] text-left uppercase tracking-wider">用户</th>
                <th className="h-10 px-4 text-xs font-medium text-[var(--color-text-secondary)] text-left uppercase tracking-wider">金额</th>
                <th className="h-10 px-4 text-xs font-medium text-[var(--color-text-secondary)] text-left uppercase tracking-wider">状态</th>
                <th className="h-10 px-4 text-xs font-medium text-[var(--color-text-secondary)] text-left uppercase tracking-wider">时间</th>
              </tr>
            </thead>
            <tbody>
              {data.recent_orders.map((order: Order) => (
                <tr key={order.order_id} className="border-b border-[#EAEAEA] last:border-0">
                  <td className="px-4 py-3 text-sm font-mono text-[var(--color-text-primary)]">{order.order_id}</td>
                  <td className="px-4 py-3 text-sm text-[var(--color-text-secondary)]">{order.user_id}</td>
                  <td className="px-4 py-3 text-sm font-mono text-[var(--color-accent)]">{formatPrice(order.pay_price)}</td>
                  <td className="px-4 py-3"><StatusBadge type="order" status={order.status} /></td>
                  <td className="px-4 py-3 text-xs text-[var(--color-text-muted)]">{formatTime(order.created_at)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </Card>
      )}
    </div>
  );
}
