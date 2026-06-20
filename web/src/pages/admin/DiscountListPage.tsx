import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { adminApi } from '../../api/admin';
import { Card } from '../../components/ui/Card';
import { Button } from '../../components/ui/Button';
import { Input } from '../../components/ui/Input';
import { Select } from '../../components/ui/Select';
import { Badge } from '../../components/ui/Badge';
import { Modal } from '../../components/ui/Modal';
import { Loading } from '../../components/ui/Loading';
import { ErrorState } from '../../components/ui/ErrorState';
import { useToast } from '../../context/ToastContext';
import { getErrorMessage, PlanTypeLabel } from '../../utils/constants';
import type { Discount } from '../../api/types';

const PLAN_OPTS = [
  { value: 'ZJ', label: '直减 (ZJ)' },
  { value: 'MJ', label: '满减 (MJ)' },
  { value: 'ZK', label: '折扣 (ZK)' },
  { value: 'N', label: '固定价 (N)' },
];

const emptyForm = { discount_id: '', name: '', description: '', plan_type: 'ZJ', expression: '', discount_type: 0, tag_id: '' };

export function DiscountListPage() {
  const qc = useQueryClient();
  const { addToast } = useToast();
  const [showForm, setShowForm] = useState(false);
  const [editing, setEditing] = useState<Discount | null>(null);
  const [form, setForm] = useState(emptyForm);

  const { data, isLoading, isError, refetch } = useQuery({
    queryKey: ['admin', 'discounts'],
    queryFn: adminApi.listDiscounts,
  });

  const createMut = useMutation({
    mutationFn: adminApi.createDiscount,
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['admin', 'discounts'] }); setShowForm(false); addToast('创建成功', 'success'); },
    onError: (e: Error) => addToast(getErrorMessage((e as { code?: string }).code || ''), 'error'),
  });

  const updateMut = useMutation({
    mutationFn: ({ id, data }: { id: string; data: Partial<Discount> }) => adminApi.updateDiscount(id, data),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['admin', 'discounts'] }); setShowForm(false); addToast('更新成功', 'success'); },
    onError: (e: Error) => addToast(getErrorMessage((e as { code?: string }).code || ''), 'error'),
  });

  const deleteMut = useMutation({
    mutationFn: adminApi.deleteDiscount,
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['admin', 'discounts'] }); addToast('删除成功', 'success'); },
    onError: (e: Error) => addToast(getErrorMessage((e as { code?: string }).code || ''), 'error'),
  });

  const openCreate = () => { setEditing(null); setForm(emptyForm); setShowForm(true); };
  const openEdit = (d: Discount) => {
    setEditing(d);
    setForm({ discount_id: d.discount_id, name: d.name, description: d.description, plan_type: d.plan_type, expression: d.expression, discount_type: d.discount_type, tag_id: d.tag_id || '' });
    setShowForm(true);
  };

  const handleSave = () => {
    const payload: Partial<Discount> = { ...form, discount_type: Number(form.discount_type), tag_id: form.tag_id || undefined, plan_type: form.plan_type as Discount['plan_type'] };
    if (editing) updateMut.mutate({ id: editing.discount_id, data: payload });
    else createMut.mutate(payload);
  };

  if (isLoading) return <Loading lines={8} />;
  if (isError) return <ErrorState message="加载失败" onRetry={() => refetch()} />;

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-xl font-medium text-[var(--color-text-primary)]">折扣管理</h1>
        <Button size="sm" onClick={openCreate}>新建折扣</Button>
      </div>

      <Card padding="none" className="overflow-x-auto">
        <table className="w-full">
          <thead>
            <tr className="bg-[var(--color-canvas)] border-b border-[#EAEAEA]">
              <th className="h-10 px-4 text-xs font-medium text-[var(--color-text-secondary)] text-left uppercase">ID</th>
              <th className="h-10 px-4 text-xs font-medium text-[var(--color-text-secondary)] text-left uppercase">名称</th>
              <th className="h-10 px-4 text-xs font-medium text-[var(--color-text-secondary)] text-left uppercase">类型</th>
              <th className="h-10 px-4 text-xs font-medium text-[var(--color-text-secondary)] text-left uppercase">表达式</th>
              <th className="h-10 px-4 text-xs font-medium text-[var(--color-text-secondary)] text-left uppercase">折扣分类</th>
              <th className="h-10 px-4 text-xs font-medium text-[var(--color-text-secondary)] text-left uppercase">操作</th>
            </tr>
          </thead>
          <tbody>
            {data?.map((d) => (
              <tr key={d.discount_id} className="border-b border-[#EAEAEA] last:border-0">
                <td className="px-4 py-3 text-sm font-mono">{d.discount_id}</td>
                <td className="px-4 py-3 text-sm">{d.name}</td>
                <td className="px-4 py-3"><Badge variant="info">{PlanTypeLabel[d.plan_type] || d.plan_type}</Badge></td>
                <td className="px-4 py-3 text-sm font-mono">{d.expression}</td>
                <td className="px-4 py-3"><Badge variant={d.discount_type === 0 ? 'neutral' : 'warning'}>{d.discount_type === 0 ? '基础' : '人群标签'}</Badge></td>
                <td className="px-4 py-3">
                  <div className="flex gap-2">
                    <Button variant="ghost" size="sm" onClick={() => openEdit(d)}>编辑</Button>
                    <Button variant="ghost" size="sm" onClick={() => { if (confirm('确认删除?')) deleteMut.mutate(d.discount_id); }}>删除</Button>
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </Card>

      <Modal open={showForm} onClose={() => setShowForm(false)} title={editing ? '编辑折扣' : '新建折扣'}>
        <div className="flex flex-col gap-4">
          <Input label="折扣 ID" value={form.discount_id} onChange={e => setForm({ ...form, discount_id: e.target.value })} disabled={!!editing} />
          <Input label="名称" value={form.name} onChange={e => setForm({ ...form, name: e.target.value })} />
          <Input label="描述" value={form.description} onChange={e => setForm({ ...form, description: e.target.value })} />
          <Select label="折扣类型" options={PLAN_OPTS} value={form.plan_type} onChange={e => setForm({ ...form, plan_type: e.target.value })} />
          <Input label="表达式" value={form.expression} onChange={e => setForm({ ...form, expression: e.target.value })} hint="ZJ=20, MJ=100-20, ZK=0.8, N=9.99" />
          <Select label="分类" options={[{ value: 0, label: '基础折扣' }, { value: 1, label: '人群标签折扣' }]} value={form.discount_type} onChange={e => setForm({ ...form, discount_type: Number(e.target.value) })} />
          <Input label="人群标签 ID" value={form.tag_id} onChange={e => setForm({ ...form, tag_id: e.target.value })} placeholder="仅人群标签折扣时需要" />
        </div>
        <div className="flex justify-end gap-3 mt-6">
          <Button variant="secondary" onClick={() => setShowForm(false)}>取消</Button>
          <Button onClick={handleSave} loading={createMut.isPending || updateMut.isPending}>{editing ? '保存' : '创建'}</Button>
        </div>
      </Modal>
    </div>
  );
}
