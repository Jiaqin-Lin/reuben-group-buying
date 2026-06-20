import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useAuth } from '../../context/AuthContext';
import { Button } from '../../components/ui/Button';
import { Input } from '../../components/ui/Input';
import { Card } from '../../components/ui/Card';

export function LoginPage() {
  const { setUserId, setAdminToken } = useAuth();
  const navigate = useNavigate();
  const [userId, setUserIdLocal] = useState('');
  const [adminToken, setAdminTokenLocal] = useState('');
  const [showAdmin, setShowAdmin] = useState(false);
  const [error, setError] = useState('');

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    const trimmed = userId.trim();
    if (!trimmed) {
      setError('请输入用户 ID');
      return;
    }
    setUserId(trimmed);
    if (adminToken.trim()) {
      setAdminToken(adminToken.trim());
    }
    navigate('/', { replace: true });
  };

  return (
    <div className="flex items-center justify-center min-h-[80vh] px-4">
      <Card className="w-full max-w-sm" padding="lg">
        <div className="text-center mb-6">
          <h1 className="text-xl font-medium text-[var(--color-text-primary)] mb-1">
            拼团
          </h1>
          <p className="text-sm text-[var(--color-text-muted)]">
            输入用户 ID 开始拼团
          </p>
        </div>
        <form onSubmit={handleSubmit} className="flex flex-col gap-4">
          <Input
            label="用户 ID"
            placeholder="例如: U1"
            value={userId}
            onChange={(e) => {
              setUserIdLocal(e.target.value);
              setError('');
            }}
            error={error}
            autoFocus
          />

          <button
            type="button"
            className="text-xs text-[var(--color-text-muted)] hover:text-[var(--color-text-secondary)] transition-colors text-left"
            onClick={() => setShowAdmin(!showAdmin)}
          >
            {showAdmin ? '收起' : '管理员模式 ▸'}
          </button>

          {showAdmin && (
            <Input
              label="Admin Token"
              placeholder="默认: admin-dev-token"
              value={adminToken}
              onChange={(e) => setAdminTokenLocal(e.target.value)}
            />
          )}

          <Button type="submit" className="w-full">
            进入
          </Button>
        </form>
        <p className="text-xs text-[var(--color-text-muted)] text-center mt-4">
          演示环境，无需密码
        </p>
      </Card>
    </div>
  );
}
