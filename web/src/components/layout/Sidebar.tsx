import { NavLink } from 'react-router-dom';
import { cn } from '../../utils/cn';

interface NavItem {
  to: string;
  label: string;
}

const NAV_ITEMS: NavItem[] = [
  { to: '/admin', label: '仪表盘' },
  { to: '/admin/configs', label: '动态配置' },
  { to: '/admin/activities', label: '活动管理' },
  { to: '/admin/discounts', label: '折扣管理' },
  { to: '/admin/products', label: '商品管理' },
  { to: '/admin/activity-products', label: '活动商品映射' },
  { to: '/admin/crowd-tags', label: '人群标签' },
  { to: '/admin/orders-monitor', label: '订单监控' },
  { to: '/admin/teams-monitor', label: '队伍监控' },
];

interface SidebarProps {
  open?: boolean;
  onClose?: () => void;
}

const navLinkClass = ({ isActive }: { isActive: boolean }) =>
  cn(
    'flex items-center px-3 h-9 rounded-md text-sm transition-colors no-underline',
    isActive
      ? 'bg-[#111] text-white font-medium'
      : 'text-[var(--color-text-secondary)] hover:bg-[var(--color-canvas)] hover:text-[var(--color-text-primary)]',
  );

export function Sidebar({ open, onClose }: SidebarProps) {
  const handleNavClick = () => {
    onClose?.();
  };

  return (
    <>
      {/* Desktop sidebar — unchanged */}
      <aside className="w-[220px] min-h-[calc(100vh-3.5rem)] border-r border-[#EAEAEA] bg-[var(--color-surface)] flex-shrink-0 hidden md:block">
        <nav className="p-4 flex flex-col gap-0.5">
          {NAV_ITEMS.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.to === '/admin'}
              className={navLinkClass}
            >
              {item.label}
            </NavLink>
          ))}
        </nav>
      </aside>

      {/* Mobile overlay */}
      {open && (
        <div
          className="fixed inset-0 z-30 bg-black/20 md:hidden"
          onClick={onClose}
        />
      )}

      {/* Mobile drawer */}
      <aside
        className={cn(
          'fixed top-0 left-0 bottom-0 z-30 w-[260px] bg-[var(--color-surface)] border-r border-[#EAEAEA] md:hidden',
          'transform transition-transform duration-200 ease-in-out',
          open ? 'translate-x-0' : '-translate-x-full',
        )}
      >
        {/* Close button */}
        <div className="flex items-center justify-between px-4 h-14 border-b border-[#EAEAEA]">
          <span className="text-sm font-medium text-[var(--color-text-primary)] font-mono tracking-tight">
            拼团<span className="text-[var(--color-text-muted)] font-normal ml-1 text-xs">管理</span>
          </span>
          <button
            onClick={onClose}
            className="flex items-center justify-center w-8 h-8 rounded-md hover:bg-[var(--color-canvas)] transition-colors cursor-pointer"
            aria-label="关闭菜单"
          >
            <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2} strokeLinecap="round">
              <path d="M18 6L6 18M6 6l12 12" />
            </svg>
          </button>
        </div>

        <nav className="p-4 flex flex-col gap-0.5">
          {NAV_ITEMS.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.to === '/admin'}
              className={navLinkClass}
              onClick={handleNavClick}
            >
              {item.label}
            </NavLink>
          ))}
        </nav>
      </aside>
    </>
  );
}
