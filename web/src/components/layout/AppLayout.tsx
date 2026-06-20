import { Outlet } from 'react-router-dom';
import { Header } from './Header';

export function AppLayout() {
  return (
    <div className="min-h-screen flex flex-col bg-[var(--color-canvas)]">
      <div className="ambient-glow" />
      <Header variant="user" />
      <main className="flex-1 relative z-1">
        <Outlet />
      </main>
    </div>
  );
}
