import React, { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { adminApi } from '../../api/admin';
import { Card } from '../../components/ui/Card';
import { Button } from '../../components/ui/Button';
import { Select } from '../../components/ui/Select';
import { StatusBadge } from '../../components/ui/StatusBadge';
import { Loading } from '../../components/ui/Loading';
import { ErrorState } from '../../components/ui/ErrorState';
import { formatTime } from '../../utils/format';
import type { Order, Team } from '../../api/types';

const STATUS_OPTS = [
  { value: '', label: '全部' },
  { value: '0', label: '成团中' },
  { value: '1', label: '已成团' },
  { value: '2', label: '已失败' },
  { value: '3', label: '部分退款' },
];

export function TeamMonitorPage() {
  const [status, setStatus] = useState('');
  const [page, setPage] = useState(1);
  const [expandedTeam, setExpandedTeam] = useState<string | null>(null);

  const { data, isLoading, isError, refetch } = useQuery({
    queryKey: ['admin', 'teams-monitor', status, page],
    queryFn: () => adminApi.listTeams(status ? Number(status) : undefined, page, 20),
  });

  const membersQuery = useQuery({
    queryKey: ['admin', 'teams', expandedTeam, 'members'],
    queryFn: () => adminApi.getTeamOrders(expandedTeam!),
    enabled: !!expandedTeam,
  });

  if (isLoading) return <Loading lines={10} />;
  if (isError) return <ErrorState message="加载失败" onRetry={() => refetch()} />;

  const totalPages = Math.ceil((data?.total || 0) / 20);

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-xl font-medium text-[var(--color-text-primary)]">队伍监控</h1>
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
              <th className="h-10 px-4 text-xs font-medium text-[var(--color-text-secondary)] text-left uppercase">队伍 ID</th>
              <th className="h-10 px-4 text-xs font-medium text-[var(--color-text-secondary)] text-left uppercase">活动 ID</th>
              <th className="h-10 px-4 text-xs font-medium text-[var(--color-text-secondary)] text-left uppercase">进度</th>
              <th className="h-10 px-4 text-xs font-medium text-[var(--color-text-secondary)] text-left uppercase">状态</th>
              <th className="h-10 px-4 text-xs font-medium text-[var(--color-text-secondary)] text-left uppercase">有效期</th>
              <th className="h-10 px-4 text-xs font-medium text-[var(--color-text-secondary)] text-left uppercase">操作</th>
            </tr>
          </thead>
          <tbody>
            {data?.items?.map((t: Team) => (
              <React.Fragment key={t.team_id}>
                <tr className="border-b border-[#EAEAEA] last:border-0">
                  <td className="px-4 py-3 text-sm font-mono">{t.team_id}</td>
                  <td className="px-4 py-3 text-sm">{t.activity_id}</td>
                  <td className="px-4 py-3 text-sm">{t.complete_count}/{t.target_count}</td>
                  <td className="px-4 py-3"><StatusBadge type="team" status={t.status} /></td>
                  <td className="px-4 py-3 text-xs text-[var(--color-text-muted)]">{formatTime(t.valid_end)}</td>
                  <td className="px-4 py-3">
                    <Button variant="ghost" size="sm" onClick={() => setExpandedTeam(expandedTeam === t.team_id ? null : t.team_id)}>
                      {expandedTeam === t.team_id ? '收起' : '成员'}
                    </Button>
                  </td>
                </tr>
                {expandedTeam === t.team_id && (
                  <tr key={`${t.team_id}-members`}>
                    <td colSpan={6} className="px-4 py-3 bg-[var(--color-canvas)]">
                      {membersQuery.isLoading ? <Loading lines={1} /> : (
                        <div className="flex flex-wrap gap-2">
                          {membersQuery.data?.map((o: Order) => (
                            <span key={o.order_id} className="inline-flex items-center gap-1 px-2 py-1 rounded-md bg-white border border-[#EAEAEA] text-xs">
                              <span className="font-mono">{o.user_id}</span>
                              <StatusBadge type="order" status={o.status} />
                            </span>
                          ))}
                        </div>
                      )}
                    </td>
                  </tr>
                )}
              </React.Fragment>
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
