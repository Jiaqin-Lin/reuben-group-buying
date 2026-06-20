import { useState } from 'react';
import { Link } from 'react-router-dom';
import { useQueryClient } from '@tanstack/react-query';
import { useAuth } from '../../context/AuthContext';
import { Button } from '../ui/Button';

interface HeaderProps {
  variant?: 'user' | 'admin';
  onToggleMenu?: () => void;
}

export function Header({ variant = 'user', onToggleMenu }: HeaderProps) {
  const { userId, setUserId, logout, isLoggedIn, isAdmin } = useAuth();
  const queryClient = useQueryClient();
  const [switching, setSwitching] = useState(false);
  const [newId, setNewId] = useState('');

  const handleSwitch = () => {
    const trimmed = newId.trim();
    if (trimmed && trimmed !== userId) {
      setUserId(trimmed);
      setNewId('');
      setSwitching(false);
      queryClient.invalidateQueries();
    }
  };

  return (
    <header className="sticky top-0 z-40 bg-[var(--color-surface)]/80 backdrop-blur-sm border-b border-[#EAEAEA]">
      <div className="max-w-6xl mx-auto px-6 h-14 flex items-center justify-between">
        <div className="flex items-center gap-3">
          {variant === 'admin' && onToggleMenu && (
            <button
              onClick={onToggleMenu}
              className="md:hidden flex items-center justify-center w-8 h-8 rounded-md hover:bg-[var(--color-canvas)] transition-colors cursor-pointer"
              aria-label="切换菜单"
            >
              <svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2} strokeLinecap="round">
                <path d="M3 6h18M3 12h18M3 18h18" />
              </svg>
            </button>
          )}
          <Link
          to={variant === 'admin' ? '/admin' : '/'}
          className="text-base font-medium text-[var(--color-text-primary)] hover:text-[var(--color-accent)] transition-colors no-underline"
        >
          <span className="font-mono tracking-tight">拼团</span>
          {variant === 'admin' && (
            <span className="text-[var(--color-text-muted)] font-normal ml-1 text-sm">
              管理
            </span>
          )}
        </Link>
        </div>

        <div className="flex items-center gap-3">
          {variant === 'user' && isLoggedIn && (
            <Link
              to="/orders"
              className="text-sm text-[var(--color-text-secondary)] hover:text-[var(--color-text-primary)] transition-colors no-underline"
            >
              我的订单
            </Link>
          )}
          {variant === 'user' && isAdmin && (
            <Link
              to="/admin"
              className="text-sm text-[var(--color-text-muted)] hover:text-[var(--color-text-primary)] transition-colors no-underline"
            >
              管理
            </Link>
          )}
          {variant === 'admin' && (
            <Link
              to="/"
              className="text-sm text-[var(--color-text-muted)] hover:text-[var(--color-text-primary)] transition-colors no-underline"
            >
              返回拼团
            </Link>
          )}
          {isLoggedIn ? (
            <div className="flex items-center gap-2">
              {switching ? (
                <div className="flex items-center gap-1">
                  <input
                    type="text"
                    value={newId}
                    onChange={(e) => setNewId(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter') handleSwitch();
                      if (e.key === 'Escape') setSwitching(false);
                    }}
                    placeholder="新用户 ID"
                    className="w-20 h-7 px-2 text-xs rounded border border-[#EAEAEA] focus:outline-none focus:border-[var(--color-accent)] font-mono"
                    autoFocus
                  />
                  <Button variant="ghost" size="sm" onClick={handleSwitch}>
                    确定
                  </Button>
                  <Button variant="ghost" size="sm" onClick={() => setSwitching(false)}>
                    取消
                  </Button>
                </div>
              ) : (
                <>
                  <button
                    onClick={() => { setNewId(userId || ''); setSwitching(true); }}
                    className="text-xs text-[var(--color-text-muted)] font-mono hover:text-[var(--color-accent)] transition-colors cursor-pointer border-b border-dotted border-[var(--color-text-muted)]"
                    title="点击切换用户"
                  >
                    {userId}
                  </button>
                  <Button variant="ghost" size="sm" onClick={logout}>
                    退出
                  </Button>
                </>
              )}
            </div>
          ) : variant === 'user' ? (
            <Link to="/login">
              <Button variant="secondary" size="sm">
                登录
              </Button>
            </Link>
          ) : null}
        </div>
      </div>
    </header>
  );
}
