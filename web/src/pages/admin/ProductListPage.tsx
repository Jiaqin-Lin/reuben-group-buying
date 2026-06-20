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
import { formatPrice } from '../../utils/format';
import { getErrorMessage } from '../../utils/constants';
import type { Product } from '../../api/types';

const emptyForm = { goods_id: '', goods_name: '', original_price: '' };

export function ProductListPage() {
  const qc = useQueryClient();
  const { addToast } = useToast();
  const [showForm, setShowForm] = useState(false);
  const [editing, setEditing] = useState<Product | null>(null);
  const [form, setForm] = useState(emptyForm);

  const { data, isLoading, isError, refetch } = useQuery({
    queryKey: ['admin', 'products'],
    queryFn: adminApi.listProducts,
  });

  const createMut = useMutation({
    mutationFn: adminApi.createProduct,
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['admin', 'products'] }); setShowForm(false); addToast('创建成功', 'success'); },
    onError: (e: Error) => addToast(getErrorMessage((e as { code?: string }).code || ''), 'error'),
  });

  const updateMut = useMutation({
    mutationFn: ({ id, data }: { id: string; data: Partial<Product> }) => adminApi.updateProduct(id, data),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['admin', 'products'] }); setShowForm(false); addToast('更新成功', 'success'); },
    onError: (e: Error) => addToast(getErrorMessage((e as { code?: string }).code || ''), 'error'),
  });

  const deleteMut = useMutation({
    mutationFn: adminApi.deleteProduct,
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['admin', 'products'] }); addToast('删除成功', 'success'); },
    onError: (e: Error) => addToast(getErrorMessage((e as { code?: string }).code || ''), 'error'),
  });

  const openCreate = () => { setEditing(null); setForm(emptyForm); setShowForm(true); };
  const openEdit = (p: Product) => { setEditing(p); setForm({ goods_id: p.goods_id, goods_name: p.goods_name, original_price: p.original_price }); setShowForm(true); };

  const handleSave = () => {
    if (!/^\d+(\.\d{1,2})?$/.test(form.original_price)) {
      addToast('请输入合法的价格（如 100 或 100.00）', 'error');
      return;
    }
    if (editing) updateMut.mutate({ id: editing.goods_id, data: form });
    else createMut.mutate(form as Partial<Product>);
  };

  if (isLoading) return <Loading lines={5} />;
  if (isError) return <ErrorState message="加载失败" onRetry={() => refetch()} />;

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-xl font-medium text-[var(--color-text-primary)]">商品管理</h1>
        <Button size="sm" onClick={openCreate}>新建商品</Button>
      </div>

      <Card padding="none" className="overflow-x-auto">
        <table className="w-full">
          <thead>
            <tr className="bg-[var(--color-canvas)] border-b border-[#EAEAEA]">
              <th className="h-10 px-4 text-xs font-medium text-[var(--color-text-secondary)] text-left uppercase">商品 ID</th>
              <th className="h-10 px-4 text-xs font-medium text-[var(--color-text-secondary)] text-left uppercase">名称</th>
              <th className="h-10 px-4 text-xs font-medium text-[var(--color-text-secondary)] text-left uppercase">原价</th>
              <th className="h-10 px-4 text-xs font-medium text-[var(--color-text-secondary)] text-left uppercase">操作</th>
            </tr>
          </thead>
          <tbody>
            {data?.map((p) => (
              <tr key={p.goods_id} className="border-b border-[#EAEAEA] last:border-0">
                <td className="px-4 py-3 text-sm font-mono">{p.goods_id}</td>
                <td className="px-4 py-3 text-sm">{p.goods_name}</td>
                <td className="px-4 py-3 text-sm font-mono text-[var(--color-accent)]">{formatPrice(p.original_price)}</td>
                <td className="px-4 py-3">
                  <div className="flex gap-2">
                    <Button variant="ghost" size="sm" onClick={() => openEdit(p)}>编辑</Button>
                    <Button variant="ghost" size="sm" onClick={() => { if (confirm('确认删除?')) deleteMut.mutate(p.goods_id); }}>删除</Button>
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </Card>

      <Modal open={showForm} onClose={() => setShowForm(false)} title={editing ? '编辑商品' : '新建商品'}>
        <div className="flex flex-col gap-4">
          <Input label="商品 ID" value={form.goods_id} onChange={e => setForm({ ...form, goods_id: e.target.value })} disabled={!!editing} />
          <Input label="名称" value={form.goods_name} onChange={e => setForm({ ...form, goods_name: e.target.value })} />
          <Input label="原价" value={form.original_price} onChange={e => setForm({ ...form, original_price: e.target.value })} placeholder="100.00" />
        </div>
        <div className="flex justify-end gap-3 mt-6">
          <Button variant="secondary" onClick={() => setShowForm(false)}>取消</Button>
          <Button onClick={handleSave} loading={createMut.isPending || updateMut.isPending}>{editing ? '保存' : '创建'}</Button>
        </div>
      </Modal>
    </div>
  );
}
