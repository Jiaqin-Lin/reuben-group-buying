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
import { required, clearError, validateForm, type FieldErrors } from '../../utils/validate';
import type { ActivityProduct } from '../../api/types';

const emptyForm = { source: '', channel: '', goods_id: '', activity_id: 0 };

export function ActivityProductPage() {
  const qc = useQueryClient();
  const { addToast } = useToast();
  const [showForm, setShowForm] = useState(false);
  const [form, setForm] = useState(emptyForm);
  const [errors, setErrors] = useState<FieldErrors>({});

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

  const handleCreate = () => {
    const errs = validateForm([
      { field: 'source', check: () => required(form.source, '来源') },
      { field: 'channel', check: () => required(form.channel, '渠道') },
      { field: 'goods_id', check: () => required(form.goods_id, '商品ID') },
      { field: 'activity_id', check: () => required(form.activity_id, '活动ID') },
    ]);
    if (Object.keys(errs).length > 0) { setErrors(errs); return; }
    createMut.mutate(form as Partial<ActivityProduct>);
  };

  if (isLoading) return <Loading lines={8} />;
  if (isError) return <ErrorState message="加载失败" onRetry={() => refetch()} />;

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-xl font-medium text-[var(--color-text-primary)]">活动商品映射</h1>
        <Button size="sm" onClick={() => { setForm(emptyForm); setErrors({}); setShowForm(true); }}>添加映射</Button>
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
            {(!data || data.length === 0) ? (
						<tr><td colSpan={10} className="px-6 py-12 text-center text-[var(--color-text-secondary)]">暂无数据</td></tr>
					) : (data.map((m) => (
              <tr key={`${m.source}-${m.channel}-${m.goods_id}`} className="border-b border-[#EAEAEA] last:border-0">
                <td className="px-4 py-3 text-sm">{m.source}</td>
                <td className="px-4 py-3 text-sm">{m.channel}</td>
                <td className="px-4 py-3 text-sm font-mono">{m.goods_id}</td>
                <td className="px-4 py-3 text-sm font-mono">{m.activity_id}</td>
                <td className="px-4 py-3">
                  <Button variant="ghost" size="sm" onClick={() => { if (confirm('确认删除?')) deleteMut.mutate({ source: m.source, channel: m.channel, goodsId: m.goods_id }); }}>删除</Button>
                </td>
              </tr>
            )))}
          </tbody>
        </table>
      </Card>

      <Modal open={showForm} onClose={() => setShowForm(false)} title="添加映射">
        <div className="flex flex-col gap-4">
          <Input label="来源 (source)" value={form.source} onChange={e => { setForm({ ...form, source: e.target.value }); setErrors(prev => clearError(prev, 'source')); }} placeholder="APP" error={errors.source} />
          <Input label="渠道 (channel)" value={form.channel} onChange={e => { setForm({ ...form, channel: e.target.value }); setErrors(prev => clearError(prev, 'channel')); }} placeholder="WECHAT" error={errors.channel} />
          <Input label="商品 ID" value={form.goods_id} onChange={e => { setForm({ ...form, goods_id: e.target.value }); setErrors(prev => clearError(prev, 'goods_id')); }} error={errors.goods_id} />
          <Input label="活动 ID" type="number" value={String(form.activity_id)} onChange={e => { setForm({ ...form, activity_id: Number(e.target.value) }); setErrors(prev => clearError(prev, 'activity_id')); }} error={errors.activity_id} />
        </div>
        <div className="flex justify-end gap-3 mt-6">
          <Button variant="secondary" onClick={() => setShowForm(false)}>取消</Button>
          <Button onClick={handleCreate} loading={createMut.isPending} disabled={Object.keys(errors).length > 0}>添加</Button>
        </div>
      </Modal>
    </div>
  );
}
