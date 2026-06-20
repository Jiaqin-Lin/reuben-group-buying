import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { adminApi } from '../../api/admin';
import { Card } from '../../components/ui/Card';
import { Button } from '../../components/ui/Button';
import { Input } from '../../components/ui/Input';
import { Modal } from '../../components/ui/Modal';
import { Loading } from '../../components/ui/Loading';
import { ErrorState } from '../../components/ui/ErrorState';
import { useToast } from '../../context/ToastContext';
import { getErrorMessage } from '../../utils/constants';
import type { ActivityProduct } from '../../api/types';

const emptyForm = { source: '', channel: '', goods_id: '', activity_id: 0 };

export function ActivityProductPage() {
  const qc = useQueryClient();
  const { addToast } = useToast();
  const [showForm, setShowForm] = useState(false);
  const [form, setForm] = useState(emptyForm);

  const { data, isLoading, isError, refetch } = useQuery({
    queryKey: ['admin', 'activity-products'],
    queryFn: adminApi.listActivityProducts,
  });

  const createMut = useMutation({
    mutationFn: adminApi.createActivityProduct,
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['admin', 'activity-products'] }); setShowForm(false); addToast('添加成功', 'success'); },
    onError: (e: Error) => addToast(getErrorMessage((e as { code?: string }).code || ''), 'error'),
  });

  const deleteMut = useMutation({
    mutationFn: ({ source, channel, goodsId }: { source: string; channel: string; goodsId: string }) =>
      adminApi.deleteActivityProduct(source, channel, goodsId),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['admin', 'activity-products'] }); addToast('删除成功', 'success'); },
    onError: (e: Error) => addToast(getErrorMessage((e as { code?: string }).code || ''), 'error'),
  });

  if (isLoading) return <Loading lines={8} />;
  if (isError) return <ErrorState message="加载失败" onRetry={() => refetch()} />;

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-xl font-medium text-[var(--color-text-primary)]">活动商品映射</h1>
        <Button size="sm" onClick={() => { setForm(emptyForm); setShowForm(true); }}>添加映射</Button>
      </div>

      <Card padding="none" className="overflow-x-auto">
        <table className="w-full">
          <thead>
            <tr className="bg-[var(--color-canvas)] border-b border-[#EAEAEA]">
              <th className="h-10 px-4 text-xs font-medium text-[var(--color-text-secondary)] text-left uppercase">来源</th>
              <th className="h-10 px-4 text-xs font-medium text-[var(--color-text-secondary)] text-left uppercase">渠道</th>
              <th className="h-10 px-4 text-xs font-medium text-[var(--color-text-secondary)] text-left uppercase">商品 ID</th>
              <th className="h-10 px-4 text-xs font-medium text-[var(--color-text-secondary)] text-left uppercase">活动 ID</th>
              <th className="h-10 px-4 text-xs font-medium text-[var(--color-text-secondary)] text-left uppercase">操作</th>
            </tr>
          </thead>
          <tbody>
            {data?.map((m) => (
              <tr key={`${m.source}-${m.channel}-${m.goods_id}`} className="border-b border-[#EAEAEA] last:border-0">
                <td className="px-4 py-3 text-sm">{m.source}</td>
                <td className="px-4 py-3 text-sm">{m.channel}</td>
                <td className="px-4 py-3 text-sm font-mono">{m.goods_id}</td>
                <td className="px-4 py-3 text-sm font-mono">{m.activity_id}</td>
                <td className="px-4 py-3">
                  <Button variant="ghost" size="sm" onClick={() => { if (confirm('确认删除?')) deleteMut.mutate({ source: m.source, channel: m.channel, goodsId: m.goods_id }); }}>删除</Button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </Card>

      <Modal open={showForm} onClose={() => setShowForm(false)} title="添加映射">
        <div className="flex flex-col gap-4">
          <Input label="来源 (source)" value={form.source} onChange={e => setForm({ ...form, source: e.target.value })} placeholder="APP" />
          <Input label="渠道 (channel)" value={form.channel} onChange={e => setForm({ ...form, channel: e.target.value })} placeholder="WECHAT" />
          <Input label="商品 ID" value={form.goods_id} onChange={e => setForm({ ...form, goods_id: e.target.value })} />
          <Input label="活动 ID" type="number" value={String(form.activity_id)} onChange={e => setForm({ ...form, activity_id: Number(e.target.value) })} />
        </div>
        <div className="flex justify-end gap-3 mt-6">
          <Button variant="secondary" onClick={() => setShowForm(false)}>取消</Button>
          <Button onClick={() => createMut.mutate(form as Partial<ActivityProduct>)} loading={createMut.isPending}>添加</Button>
        </div>
      </Modal>
    </div>
  );
}
