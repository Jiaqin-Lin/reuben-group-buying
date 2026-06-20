import { Routes, Route, Navigate } from 'react-router-dom';
import { AppLayout } from './components/layout/AppLayout';
import { AdminLayout } from './components/layout/AdminLayout';
import { HomePage } from './pages/home/HomePage';
import { OrderPage } from './pages/order/OrderPage';
import { MyOrdersPage } from './pages/order/MyOrdersPage';
import { LoginPage } from './pages/auth/LoginPage';
import { DashboardPage } from './pages/admin/DashboardPage';
import { ConfigPage } from './pages/admin/ConfigPage';
import { ActivityListPage } from './pages/admin/ActivityListPage';
import { DiscountListPage } from './pages/admin/DiscountListPage';
import { ProductListPage } from './pages/admin/ProductListPage';
import { ActivityProductPage } from './pages/admin/ActivityProductPage';
import { CrowdTagPage } from './pages/admin/CrowdTagPage';
import { OrderMonitorPage } from './pages/admin/OrderMonitorPage';
import { TeamMonitorPage } from './pages/admin/TeamMonitorPage';

export function App() {
  return (
    <Routes>
      {/* User-facing */}
      <Route element={<AppLayout />}>
        <Route path="/" element={<HomePage />} />
        <Route path="/login" element={<LoginPage />} />
        <Route path="/orders" element={<MyOrdersPage />} />
        <Route path="/order/:outTradeNo" element={<OrderPage />} />
      </Route>

      {/* Admin */}
      <Route path="/admin" element={<AdminLayout />}>
        <Route index element={<DashboardPage />} />
        <Route path="configs" element={<ConfigPage />} />
        <Route path="activities" element={<ActivityListPage />} />
        <Route path="discounts" element={<DiscountListPage />} />
        <Route path="products" element={<ProductListPage />} />
        <Route path="activity-products" element={<ActivityProductPage />} />
        <Route path="crowd-tags" element={<CrowdTagPage />} />
        <Route path="orders-monitor" element={<OrderMonitorPage />} />
        <Route path="teams-monitor" element={<TeamMonitorPage />} />
        <Route path="*" element={<Navigate to="/admin" replace />} />
      </Route>

      {/* Catch-all */}
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}
