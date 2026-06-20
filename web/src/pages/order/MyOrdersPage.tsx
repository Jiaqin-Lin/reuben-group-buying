import { useNavigate } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { useAuth } from '../../context/AuthContext';
import { useToast } from '../../context/ToastContext';
import { useRefund } from '../../hooks/useRefund';
import { orderApi } from '../../api/admin';

import { Button } from '../../components/ui/Button';
import { Table } from '../../components/ui/Table';
import { StatusBadge } from '../../components/ui/StatusBadge';
import { EmptyState } from '../../components/ui/EmptyState';
import { ErrorState } from '../../components/ui/ErrorState';
import { PageLoading } from '../../components/ui/Loading';
import { formatPrice, formatTime } from '../../utils/format';
import { getErrorMessage } from '../../utils/constants';
import type { Order } from '../../api/types';

export function MyOrdersPage() {
  const { userId, isLoggedIn } = useAuth();
  const navigate = useNavigate();
  const { addToast } = useToast();
  const refundMutation = useRefund();

  const { data: orders, isLoading, error, refetch } = useQuery({
    queryKey: ['my-orders', userId],
    queryFn: () => orderApi.listByUser(userId!),
    enabled: isLoggedIn && !!userId,
  });

  const handleRefund = (order: Order) => {
    if (!userId) return;
    refundMutation.mutate(
      {
        user_id: userId,
        out_trade_no: order.out_trade_no,
      },
      {
        onSuccess: (data) => {
          addToast(`退款成功 (${data.refund_type})`, 'success');
          refetch();
        },
        onError: (err) => {
          addToast(getErrorMessage(err.code), 'error');
        },
      },
    );
  };

  if (!isLoggedIn) {
    navigate('/login', { replace: true });
    return null;
  }

  if (isLoading) return <PageLoading />;
  if (error) {
    return (
      <ErrorState
        message="加载失败"
        onRetry={() => refetch()}
      />
    );
  }

  return (
    <div className="max-w-3xl mx-auto px-4 py-8">
      <h1 className="text-xl font-medium text-[var(--color-text-primary)] mb-6">
        我的订单
      </h1>

      {!orders || orders.length === 0 ? (
        <EmptyState
          title="暂无订单"
          description="去首页参与拼团吧"
          action={
            <Button variant="secondary" size="sm" onClick={() => navigate('/')}>
              去首页
            </Button>
          }
        />
      ) : (
        <Table<Order>
          columns={[
            {
              key: 'order_id',
              header: '订单号',
              render: (row) => (
                <span className="font-mono text-xs">{row.order_id}</span>
              ),
            },
            {
              key: 'team_id',
              header: '拼团ID',
              render: (row) => (
                <span className="font-mono text-xs">{row.team_id}</span>
              ),
            },
            {
              key: 'pay_price',
              header: '金额',
              render: (row) => (
                <span className="font-mono font-medium text-[var(--color-accent)]">
                  {formatPrice(row.pay_price)}
                </span>
              ),
            },
            {
              key: 'status',
              header: '状态',
              render: (row) => <StatusBadge type="order" status={row.status} />,
            },
            {
              key: 'created_at',
              header: '时间',
              render: (row) => (
                <span className="text-xs text-[var(--color-text-muted)]">
                  {formatTime(row.created_at)}
                </span>
              ),
            },
            {
              key: 'actions',
              header: '操作',
              align: 'right',
              render: (row) => (
                <div className="flex items-center justify-end gap-2">
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => navigate(`/order/${row.out_trade_no}`)}
                  >
                    详情
                  </Button>
                  {(row.status === 0 || row.status === 1) && (
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => handleRefund(row)}
                      loading={
                        refundMutation.isPending &&
                        refundMutation.variables?.out_trade_no === row.out_trade_no
                      }
                    >
                      {row.status === 0 ? '取消' : '退款'}
                    </Button>
                  )}
                </div>
              ),
            },
          ]}
          data={orders}
          keyExtractor={(row) => row.order_id}
        />
      )}
    </div>
  );
}
