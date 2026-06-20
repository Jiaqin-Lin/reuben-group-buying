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

export function Sidebar() {
  return (
    <aside className="w-[220px] min-h-[calc(100vh-3.5rem)] border-r border-[#EAEAEA] bg-[var(--color-surface)] flex-shrink-0 hidden md:block">
      <nav className="p-4 flex flex-col gap-0.5">
        {NAV_ITEMS.map((item) => (
          <NavLink
            key={item.to}
            to={item.to}
            end={item.to === '/admin'}
            className={({ isActive }) =>
              cn(
                'flex items-center px-3 h-9 rounded-md text-sm transition-colors no-underline',
                isActive
                  ? 'bg-[#111] text-white font-medium'
                  : 'text-[var(--color-text-secondary)] hover:bg-[var(--color-canvas)] hover:text-[var(--color-text-primary)]',
              )
            }
          >
            {item.label}
          </NavLink>
        ))}
      </nav>
    </aside>
  );
}
