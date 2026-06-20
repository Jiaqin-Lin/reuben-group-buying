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
import type { CrowdTag, CrowdTagDetail } from '../../api/types';

const emptyForm = { tag_id: '', tag_name: '', tag_desc: '' };

export function CrowdTagPage() {
  const qc = useQueryClient();
  const { addToast } = useToast();
  const [showForm, setShowForm] = useState(false);
  const [editing, setEditing] = useState<CrowdTag | null>(null);
  const [form, setForm] = useState(emptyForm);
  const [errors, setErrors] = useState<FieldErrors>({});
  const [selectedTag, setSelectedTag] = useState<string | null>(null);
  const [newMemberId, setNewMemberId] = useState('');

  const { data, isLoading, isError, refetch } = useQuery({
    queryKey: ['admin', 'crowd-tags'],
    queryFn: adminApi.listCrowdTags,
  });

  const membersQuery = useQuery({
    queryKey: ['admin', 'crowd-tags', selectedTag, 'members'],
    queryFn: () => adminApi.listCrowdTagMembers(selectedTag!),
    enabled: !!selectedTag,
  });

  const createMut = useMutation({
    mutationFn: adminApi.createCrowdTag,
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['admin', 'crowd-tags'] }); setShowForm(false); addToast('创建成功', 'success'); },
    onError: (e: Error) => addToast(getErrorMessage((e as { code?: string }).code || ''), 'error'),
  });
  const updateMut = useMutation({
    mutationFn: ({ id, data }: { id: string; data: Partial<CrowdTag> }) => adminApi.updateCrowdTag(id, data),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['admin', 'crowd-tags'] }); setShowForm(false); addToast('更新成功', 'success'); },
    onError: (e: Error) => addToast(getErrorMessage((e as { code?: string }).code || ''), 'error'),
  });
  const deleteMut = useMutation({
    mutationFn: adminApi.deleteCrowdTag,
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['admin', 'crowd-tags'] }); addToast('删除成功', 'success'); },
    onError: (e: Error) => addToast(getErrorMessage((e as { code?: string }).code || ''), 'error'),
  });
  const addMemberMut = useMutation({
    mutationFn: ({ tagId, userId }: { tagId: string; userId: string }) => adminApi.addCrowdTagMember(tagId, userId),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['admin', 'crowd-tags', selectedTag, 'members'] }); setNewMemberId(''); addToast('添加成功', 'success'); },
    onError: (e: Error) => addToast(getErrorMessage((e as { code?: string }).code || ''), 'error'),
  });
  const removeMemberMut = useMutation({
    mutationFn: ({ tagId, userId }: { tagId: string; userId: string }) => adminApi.removeCrowdTagMember(tagId, userId),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['admin', 'crowd-tags', selectedTag, 'members'] }); addToast('移除成功', 'success'); },
    onError: (e: Error) => addToast(getErrorMessage((e as { code?: string }).code || ''), 'error'),
  });

  const openCreate = () => { setEditing(null); setForm(emptyForm); setErrors({}); setShowForm(true); };
  const openEdit = (t: CrowdTag) => { setEditing(t); setForm({ tag_id: t.tag_id, tag_name: t.tag_name, tag_desc: t.tag_desc }); setErrors({}); setShowForm(true); };
  const handleSave = () => {
    const errs = validateForm([
      { field: 'tag_id', check: () => editing ? null : required(form.tag_id, '标签ID') },
      { field: 'tag_name', check: () => required(form.tag_name, '名称') },
    ]);
    if (Object.keys(errs).length > 0) { setErrors(errs); return; }
    if (editing) updateMut.mutate({ id: editing.tag_id, data: form });
    else createMut.mutate(form as Partial<CrowdTag>);
  };

  if (isLoading) return <Loading lines={5} />;
  if (isError) return <ErrorState message="加载失败" onRetry={() => refetch()} />;

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-xl font-medium text-[var(--color-text-primary)]">人群标签</h1>
        <Button size="sm" onClick={openCreate}>新建标签</Button>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Tag list */}
        <Card padding="none" className="overflow-hidden">
          <table className="w-full">
            <thead>
              <tr className="bg-[var(--color-canvas)] border-b border-[#EAEAEA]">
                <th className="h-10 px-4 text-xs font-medium text-[var(--color-text-secondary)] text-left uppercase">标签 ID</th>
                <th className="h-10 px-4 text-xs font-medium text-[var(--color-text-secondary)] text-left uppercase">名称</th>
                <th className="h-10 px-4 text-xs font-medium text-[var(--color-text-secondary)] text-left uppercase">描述</th>
                <th className="h-10 px-4 text-xs font-medium text-[var(--color-text-secondary)] text-left uppercase">人数</th>
                <th className="h-10 px-4 text-xs font-medium text-[var(--color-text-secondary)] text-left uppercase">操作</th>
              </tr>
            </thead>
            <tbody>
              {data?.map((t) => (
                <tr key={t.tag_id} className={`border-b border-[#EAEAEA] last:border-0 cursor-pointer ${selectedTag === t.tag_id ? 'bg-[var(--color-canvas)]' : ''}`} onClick={() => setSelectedTag(t.tag_id)}>
                  <td className="px-4 py-3 text-sm font-mono">{t.tag_id}</td>
                  <td className="px-4 py-3 text-sm">{t.tag_name}</td>
                  <td className="px-4 py-3 text-xs text-[var(--color-text-secondary)] max-w-[160px] truncate" title={t.tag_desc}>{t.tag_desc || '-'}</td>
                  <td className="px-4 py-3 text-sm">{t.statistics}</td>
                  <td className="px-4 py-3" onClick={e => e.stopPropagation()}>
                    <div className="flex gap-2">
                      <Button variant="ghost" size="sm" onClick={() => openEdit(t)}>编辑</Button>
                      <Button variant="ghost" size="sm" onClick={() => { if (confirm('确认删除?')) deleteMut.mutate(t.tag_id); }}>删除</Button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </Card>

        {/* Members panel */}
        <Card padding="md">
          <h2 className="text-sm font-medium text-[var(--color-text-primary)] mb-3">
            {selectedTag ? `标签 ${selectedTag} 成员` : '选择标签查看成员'}
          </h2>
          {selectedTag && (
            <>
              <div className="flex gap-2 mb-3">
                <Input
                  placeholder="输入用户 ID"
                  value={newMemberId}
                  onChange={e => setNewMemberId(e.target.value)}
                  className="flex-1"
                />
                <Button size="sm" onClick={() => addMemberMut.mutate({ tagId: selectedTag, userId: newMemberId })} loading={addMemberMut.isPending} disabled={!newMemberId}>
                  添加
                </Button>
              </div>
              {membersQuery.isLoading ? <Loading lines={3} /> : (
                <div className="max-h-80 overflow-y-auto">
                  {membersQuery.data?.items?.map((m: CrowdTagDetail) => (
                    <div key={m.id} className="flex items-center justify-between py-2 border-b border-[#EAEAEA] last:border-0">
                      <span className="text-sm font-mono">{m.user_id}</span>
                      <Button variant="ghost" size="sm" onClick={() => removeMemberMut.mutate({ tagId: selectedTag, userId: m.user_id })}>移除</Button>
                    </div>
                  ))}
                  {(!membersQuery.data?.items || membersQuery.data.items.length === 0) && (
                    <p className="text-sm text-[var(--color-text-muted)] py-4 text-center">暂无成员</p>
                  )}
                </div>
              )}
            </>
          )}
        </Card>
      </div>

      <Modal open={showForm} onClose={() => setShowForm(false)} title={editing ? '编辑标签' : '新建标签'}>
        <div className="flex flex-col gap-4">
          <Input label="标签 ID" value={form.tag_id} onChange={e => { setForm({ ...form, tag_id: e.target.value }); setErrors(prev => clearError(prev, 'tag_id')); }} disabled={!!editing} error={errors.tag_id} />
          <Input label="名称" value={form.tag_name} onChange={e => setForm({ ...form, tag_name: e.target.value })} error={errors.tag_name} />
          <Input label="描述" value={form.tag_desc} onChange={e => setForm({ ...form, tag_desc: e.target.value })} />
        </div>
        <div className="flex justify-end gap-3 mt-6">
          <Button variant="secondary" onClick={() => setShowForm(false)}>取消</Button>
          <Button onClick={handleSave} loading={createMut.isPending || updateMut.isPending} disabled={Object.keys(errors).length > 0}>{editing ? '保存' : '创建'}</Button>
        </div>
      </Modal>
    </div>
  );
}
