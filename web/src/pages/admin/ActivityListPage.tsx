import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { adminApi } from '../../api/admin';
import { Card } from '../../components/ui/Card';
import { Button } from '../../components/ui/Button';
import { Input } from '../../components/ui/Input';
import { Select } from '../../components/ui/Select';
import { Modal } from '../../components/ui/Modal';
import { StatusBadge } from '../../components/ui/StatusBadge';
import { Loading } from '../../components/ui/Loading';
import { ErrorState } from '../../components/ui/ErrorState';
import { useToast } from '../../context/ToastContext';
import { getErrorMessage } from '../../utils/constants';
import { required, isPositiveInt, minValue, dateRange, clearError, validateForm, type FieldErrors } from '../../utils/validate';
import type { Activity } from '../../api/types';

const STATUS_OPTS = [
  { value: 0, label: '已创建' },
  { value: 1, label: '进行中' },
  { value: 2, label: '已过期' },
  { value: 3, label: '已废弃' },
];

const emptyForm = {
  activity_id: 0,
  name: '',
  discount_id: '',
  group_type: 1,
  target_count: 3,
  take_limit: 5,
  valid_minutes: 30,
  status: 1,
  start_time: '',
  end_time: '',
  tag_id: '',
  tag_scope: '',
};

export function ActivityListPage() {
  const qc = useQueryClient();
  const { addToast } = useToast();
  const [showForm, setShowForm] = useState(false);
  const [editing, setEditing] = useState<Activity | null>(null);
  const [form, setForm] = useState(emptyForm);
  const [errors, setErrors] = useState<FieldErrors>({});

  const { data, isLoading, isError, refetch } = useQuery({
    queryKey: ['admin', 'activities'],
    queryFn: adminApi.listActivities,
  });

  const createMut = useMutation({
    mutationFn: adminApi.createActivity,
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['admin', 'activities'] }); setShowForm(false); addToast('创建成功', 'success'); },
    onError: (e: Error) => addToast(getErrorMessage((e as { code?: string }).code || ''), 'error'),
  });

  const updateMut = useMutation({
    mutationFn: ({ id, data }: { id: number; data: Partial<Activity> }) => adminApi.updateActivity(id, data),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['admin', 'activities'] }); setShowForm(false); addToast('更新成功', 'success'); },
    onError: (e: Error) => addToast(getErrorMessage((e as { code?: string }).code || ''), 'error'),
  });

  const deleteMut = useMutation({
    mutationFn: adminApi.deleteActivity,
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['admin', 'activities'] }); addToast('删除成功', 'success'); },
    onError: (e: Error) => addToast(getErrorMessage((e as { code?: string }).code || ''), 'error'),
  });

  const openCreate = () => { setEditing(null); setForm(emptyForm); setErrors({}); setShowForm(true); };
  const openEdit = (a: Activity) => {
    setEditing(a);
    setForm({
      activity_id: a.activity_id,
      name: a.name,
      discount_id: a.discount_id,
      group_type: a.group_type,
      target_count: a.target_count,
      take_limit: a.take_limit,
      valid_minutes: a.valid_minutes,
      status: a.status,
      start_time: a.start_time?.slice(0, 16) || '',
      end_time: a.end_time?.slice(0, 16) || '',
      tag_id: a.tag_id || '',
      tag_scope: a.tag_scope || '',
    });
    setErrors({});
    setShowForm(true);
  };

  const handleSave = () => {
    const errs = validateForm([
      { field: 'name', check: () => required(form.name, '名称') },
      { field: 'discount_id', check: () => required(form.discount_id, '折扣ID') },
      { field: 'target_count', check: () => isPositiveInt(form.target_count, '目标人数') || minValue(form.target_count, 2, '目标人数') },
      { field: 'take_limit', check: () => isPositiveInt(form.take_limit, '限购次数') || minValue(form.take_limit, 1, '限购次数') },
      { field: 'valid_minutes', check: () => isPositiveInt(form.valid_minutes, '有效分钟') || minValue(form.valid_minutes, 1, '有效分钟') },
      { field: 'start_time', check: () => required(form.start_time, '开始时间') },
      { field: 'end_time', check: () => required(form.end_time, '结束时间') || dateRange(form.start_time, form.end_time) },
    ]);
    if (Object.keys(errs).length > 0) { setErrors(errs); return; }

    // datetime-local → RFC3339: "2026-06-21T02:20" → "2026-06-21T02:20:00+08:00"
    const toRFC3339 = (v: string) => v ? v + ':00+08:00' : undefined;
    const payload = { ...form, target_count: Number(form.target_count), take_limit: Number(form.take_limit), valid_minutes: Number(form.valid_minutes), group_type: Number(form.group_type), status: Number(form.status), start_time: toRFC3339(form.start_time), end_time: toRFC3339(form.end_time), tag_id: form.tag_id || undefined, tag_scope: form.tag_scope || undefined };
    if (editing) {
      updateMut.mutate({ id: editing.activity_id, data: payload });
    } else {
      createMut.mutate(payload as Partial<Activity>);
    }
  };

  if (isLoading) return <Loading lines={8} />;
  if (isError) return <ErrorState message="加载失败" onRetry={() => refetch()} />;

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-xl font-medium text-[var(--color-text-primary)]">活动管理</h1>
        <Button size="sm" onClick={openCreate}>新建活动</Button>
      </div>

      <Card padding="none" className="overflow-x-auto">
        <table className="w-full">
          <thead>
            <tr className="bg-[var(--color-canvas)] border-b border-[#EAEAEA]">
              <th className="h-10 px-4 text-xs font-medium text-[var(--color-text-secondary)] text-left uppercase">ID</th>
              <th className="h-10 px-4 text-xs font-medium text-[var(--color-text-secondary)] text-left uppercase">名称</th>
              <th className="h-10 px-4 text-xs font-medium text-[var(--color-text-secondary)] text-left uppercase">折扣</th>
              <th className="h-10 px-4 text-xs font-medium text-[var(--color-text-secondary)] text-left uppercase">成团人数</th>
              <th className="h-10 px-4 text-xs font-medium text-[var(--color-text-secondary)] text-left uppercase">限购</th>
              <th className="h-10 px-4 text-xs font-medium text-[var(--color-text-secondary)] text-left uppercase">有效(分)</th>
              <th className="h-10 px-4 text-xs font-medium text-[var(--color-text-secondary)] text-left uppercase">开始时间</th>
              <th className="h-10 px-4 text-xs font-medium text-[var(--color-text-secondary)] text-left uppercase">结束时间</th>
              <th className="h-10 px-4 text-xs font-medium text-[var(--color-text-secondary)] text-left uppercase">状态</th>
              <th className="h-10 px-4 text-xs font-medium text-[var(--color-text-secondary)] text-left uppercase">操作</th>
            </tr>
          </thead>
          <tbody>
            {(!data || data.length === 0) ? (
						<tr><td colSpan={10} className="px-6 py-12 text-center text-[var(--color-text-secondary)]">暂无数据</td></tr>
					) : (data.map((a) => (
              <tr key={a.activity_id} className="border-b border-[#EAEAEA] last:border-0">
                <td className="px-4 py-3 text-sm font-mono">{a.activity_id}</td>
                <td className="px-4 py-3 text-sm">{a.name}</td>
                <td className="px-4 py-3 text-sm font-mono">{a.discount_id}</td>
                <td className="px-4 py-3 text-sm">{a.target_count}人</td>
                <td className="px-4 py-3 text-sm">{a.take_limit}次</td>
                <td className="px-4 py-3 text-sm">{a.valid_minutes}分</td>
                <td className="px-4 py-3 text-xs font-mono text-[var(--color-text-secondary)]">{a.start_time?.slice(0, 16) || '-'}</td>
                <td className="px-4 py-3 text-xs font-mono text-[var(--color-text-secondary)]">{a.end_time?.slice(0, 16) || '-'}</td>
                <td className="px-4 py-3"><StatusBadge type="activity" status={a.status} /></td>
                <td className="px-4 py-3">
                  <div className="flex gap-2">
                    <Button variant="ghost" size="sm" onClick={() => openEdit(a)}>编辑</Button>
                    <Button variant="ghost" size="sm" onClick={() => { if (confirm('确认删除?')) deleteMut.mutate(a.activity_id); }}>删除</Button>
                  </div>
                </td>
              </tr>
            )))}
          </tbody>
        </table>
      </Card>

      <Modal open={showForm} onClose={() => setShowForm(false)} title={editing ? '编辑活动' : '新建活动'} maxWidth="lg">
        <div className="grid grid-cols-2 gap-4">
          <Input label="活动 ID" type="number" value={String(form.activity_id)} onChange={e => setForm({ ...form, activity_id: Number(e.target.value) })} disabled={!!editing} />
          <Input label="名称" value={form.name} onChange={e => setForm({ ...form, name: e.target.value })} error={errors.name} />
          <Input label="折扣 ID" value={form.discount_id} onChange={e => { setForm({ ...form, discount_id: e.target.value }); setErrors(prev => clearError(prev, 'discount_id')); }} error={errors.discount_id} />
          <Select label="拼团类型" options={[{ value: 0, label: '自动成团' }, { value: 1, label: '目标拼团' }]} value={form.group_type} onChange={e => setForm({ ...form, group_type: Number(e.target.value) })} />
          <Input label="目标人数" type="number" value={String(form.target_count)} onChange={e => { setForm({ ...form, target_count: Number(e.target.value) }); setErrors(prev => clearError(prev, 'target_count')); }} error={errors.target_count} />
          <Input label="限购次数" type="number" value={String(form.take_limit)} onChange={e => { setForm({ ...form, take_limit: Number(e.target.value) }); setErrors(prev => clearError(prev, 'take_limit')); }} error={errors.take_limit} />
          <Input label="有效分钟" type="number" value={String(form.valid_minutes)} onChange={e => { setForm({ ...form, valid_minutes: Number(e.target.value) }); setErrors(prev => clearError(prev, 'valid_minutes')); }} error={errors.valid_minutes} />
          {editing && <Select label="状态" options={STATUS_OPTS} value={form.status} onChange={e => setForm({ ...form, status: Number(e.target.value) })} />}
          <Input label="开始时间" type="datetime-local" value={form.start_time} onChange={e => { setForm({ ...form, start_time: e.target.value }); setErrors(prev => clearError(clearError(prev, 'start_time'), 'end_time')); }} error={errors.start_time} />
          <Input label="结束时间" type="datetime-local" value={form.end_time} onChange={e => { setForm({ ...form, end_time: e.target.value }); setErrors(prev => clearError(prev, 'end_time')); }} error={errors.end_time} />
          <Input label="人群标签 ID" value={form.tag_id} onChange={e => setForm({ ...form, tag_id: e.target.value })} placeholder="可选" />
          <Input label="标签范围" value={form.tag_scope} onChange={e => setForm({ ...form, tag_scope: e.target.value })} placeholder="可选" />
        </div>
        <div className="flex justify-end gap-3 mt-6">
          <Button variant="secondary" onClick={() => setShowForm(false)}>取消</Button>
          <Button onClick={handleSave} loading={createMut.isPending || updateMut.isPending} disabled={Object.keys(errors).length > 0}>
            {editing ? '保存' : '创建'}
          </Button>
        </div>
      </Modal>
    </div>
  );
}
