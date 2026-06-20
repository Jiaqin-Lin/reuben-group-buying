import { useState } from 'react';
import { useAdminConfigs, useUpdateConfig } from '../../hooks/useAdminConfigs';
import { useToast } from '../../context/ToastContext';
import { Card } from '../../components/ui/Card';
import { Badge } from '../../components/ui/Badge';
import { Button } from '../../components/ui/Button';
import { ErrorState } from '../../components/ui/ErrorState';
import { Loading } from '../../components/ui/Loading';
import {
  getConfigDef,
  getConfigTypeLabel,
} from '../../utils/configRegistry';
import { getErrorMessage } from '../../utils/constants';

/** Extract raw value from backend's { value: ... } wrapper. */
function unwrap(val: unknown): unknown {
  if (val && typeof val === 'object' && 'value' in (val as Record<string, unknown>)) {
    return (val as Record<string, unknown>).value;
  }
  return val;
}

export function ConfigPage() {
  const { data: configs, isLoading, isError, refetch } = useAdminConfigs();
  const updateMutation = useUpdateConfig();
  const { addToast } = useToast();

  const [editingKey, setEditingKey] = useState<string | null>(null);
  const [editValue, setEditValue] = useState<unknown>(null);

  if (isLoading) return <Loading lines={8} />;
  if (isError) {
    return (
      <ErrorState
        message="加载配置失败"
        onRetry={() => refetch()}
      />
    );
  }

  const startEdit = (key: string, rawValue: unknown) => {
    const currentValue = unwrap(rawValue);
    setEditingKey(key);
    const def = getConfigDef(key);
    if (def?.type === 'bool') {
      setEditValue(!currentValue);
    } else {
      setEditValue(currentValue);
    }
  };

  const cancelEdit = () => {
    setEditingKey(null);
    setEditValue(null);
  };

  const saveEdit = (key: string) => {
    updateMutation.mutate(
      { key, value: editValue },
      {
        onSuccess: () => {
          addToast(`配置 ${key} 已更新`, 'success');
          setEditingKey(null);
          setEditValue(null);
        },
        onError: (err) => {
          addToast(getErrorMessage(err.code), 'error');
        },
      },
    );
  };

  const formatDisplayValue = (key: string, rawValue: unknown) => {
    const val = unwrap(rawValue);
    const def = getConfigDef(key);
    if (def?.type === 'bool') {
      return (
        <Badge variant={val ? 'success' : 'neutral'}>
          {val ? 'true' : 'false'}
        </Badge>
      );
    }
    return (
      <span className="font-mono text-sm text-[var(--color-text-primary)]">
        {String(val)}
      </span>
    );
  };

  const renderEditor = (key: string) => {
    const def = getConfigDef(key);
    const isEditing = editingKey === key;
    if (!isEditing) return null;

    if (def?.type === 'bool') {
      return (
        <div className="flex items-center gap-2">
          <button
            onClick={() => setEditValue(!editValue)}
            className={`relative inline-flex h-6 w-11 items-center rounded-full transition-colors cursor-pointer ${
              editValue ? 'bg-[#111]' : 'bg-[#EAEAEA]'
            }`}
          >
            <span
              className={`inline-block h-4 w-4 rounded-full bg-white transition-transform ${
                editValue ? 'translate-x-6' : 'translate-x-1'
              }`}
            />
          </button>
          <span className="text-xs text-[var(--color-text-muted)]">
            {editValue ? 'true' : 'false'}
          </span>
        </div>
      );
    }

    return (
      <input
        type={def?.type === 'int' ? 'number' : 'text'}
        value={String(editValue ?? '')}
        onChange={(e) => {
          const v = def?.type === 'int' ? Number(e.target.value) : e.target.value;
          setEditValue(v);
        }}
        className="h-8 w-40 px-2 rounded-md border border-[#EAEAEA] text-sm font-mono focus:outline-none focus:border-[var(--color-accent)] focus:ring-1 focus:ring-[var(--color-accent-border)]"
        autoFocus
        onKeyDown={(e) => {
          if (e.key === 'Enter') saveEdit(key);
          if (e.key === 'Escape') cancelEdit();
        }}
      />
    );
  };

  const entries = configs ? Object.entries(configs) : [];

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-xl font-medium text-[var(--color-text-primary)]">
          动态配置
        </h1>
        <Badge variant="neutral">
          {entries.length} 项配置
        </Badge>
      </div>

      <Card padding="none">
        <div className="divide-y divide-[#EAEAEA]">
          {entries.map(([key, value]) => {
            const def = getConfigDef(key);
            const isEditing = editingKey === key;
            return (
              <div
                key={key}
                className="flex items-center justify-between px-5 py-4 gap-4"
              >
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2 mb-0.5">
                    <span className="font-mono text-sm font-medium text-[var(--color-text-primary)]">
                      {key}
                    </span>
                    {def && (
                      <Badge variant="neutral">
                        {getConfigTypeLabel(def.type)}
                      </Badge>
                    )}
                  </div>
                  <p className="text-xs text-[var(--color-text-muted)]">
                    {def?.description || '无描述'}
                  </p>
                </div>

                <div className="flex items-center gap-2 flex-shrink-0">
                  {/* Edit mode: show editor + save/cancel */}
                  {isEditing ? (
                    <>
                      {renderEditor(key)}
                      <Button
                        size="sm"
                        onClick={() => saveEdit(key)}
                        loading={updateMutation.isPending}
                      >
                        保存
                      </Button>
                      <Button variant="ghost" size="sm" onClick={cancelEdit}>
                        取消
                      </Button>
                    </>
                  ) : (
                    <>
                      {/* View mode: show value + edit button */}
                      {formatDisplayValue(key, value)}
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => startEdit(key, value)}
                      >
                        编辑
                      </Button>
                    </>
                  )}
                </div>
              </div>
            );
          })}

          {entries.length === 0 && (
            <div className="px-5 py-12 text-center text-sm text-[var(--color-text-muted)]">
              暂无配置数据
            </div>
          )}
        </div>
      </Card>
    </div>
  );
}
