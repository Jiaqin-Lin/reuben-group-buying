import { api } from './client';
import type { DashboardStats, PaginatedResponse, Activity, Discount, Product, ActivityProduct, CrowdTag, CrowdTagDetail, Order, Team, OrderFilter } from './types';

// ===== Dashboard =====
export const adminApi = {
  // Configs (existing)
  listConfigs: () => api.get<Record<string, unknown>>('/api/v1/admin/configs'),
  updateConfig: (key: string, value: unknown, updatedBy?: string) =>
    api.put<{ key: string; updated: boolean }>(`/api/v1/admin/configs/${key}`, {
      value,
      updated_by: updatedBy || 'admin',
    }),

  // Dashboard
  getDashboard: () => api.get<DashboardStats>('/api/v1/admin/dashboard'),

  // Activities
  listActivities: () => api.get<Activity[]>('/api/v1/admin/activities'),
  getActivity: (id: number) => api.get<Activity>(`/api/v1/admin/activities/${id}`),
  createActivity: (data: Partial<Activity>) =>
    api.post<Activity>('/api/v1/admin/activities', data),
  updateActivity: (id: number, data: Partial<Activity>) =>
    api.put<Activity>(`/api/v1/admin/activities/${id}`, data),
  deleteActivity: (id: number) =>
    api.delete(`/api/v1/admin/activities/${id}`),

  // Discounts
  listDiscounts: () => api.get<Discount[]>('/api/v1/admin/discounts'),
  getDiscount: (id: string) => api.get<Discount>(`/api/v1/admin/discounts/${id}`),
  createDiscount: (data: Partial<Discount>) =>
    api.post<Discount>('/api/v1/admin/discounts', data),
  updateDiscount: (id: string, data: Partial<Discount>) =>
    api.put<Discount>(`/api/v1/admin/discounts/${id}`, data),
  deleteDiscount: (id: string) =>
    api.delete(`/api/v1/admin/discounts/${id}`),

  // Products
  listProducts: () => api.get<Product[]>('/api/v1/admin/products'),
  createProduct: (data: Partial<Product>) =>
    api.post<Product>('/api/v1/admin/products', data),
  updateProduct: (goodsId: string, data: Partial<Product>) =>
    api.put<Product>(`/api/v1/admin/products/${goodsId}`, data),
  deleteProduct: (goodsId: string) =>
    api.delete(`/api/v1/admin/products/${goodsId}`),

  // Activity-Product mappings
  listActivityProducts: () => api.get<ActivityProduct[]>('/api/v1/admin/activity-products'),
  createActivityProduct: (data: Partial<ActivityProduct>) =>
    api.post<ActivityProduct>('/api/v1/admin/activity-products', data),
  deleteActivityProduct: (source: string, channel: string, goodsId: string) =>
    api.delete(`/api/v1/admin/activity-products?source=${source}&channel=${channel}&goods_id=${goodsId}`),

  // Crowd tags
  listCrowdTags: () => api.get<CrowdTag[]>('/api/v1/admin/crowd-tags'),
  getCrowdTag: (tagId: string) => api.get<CrowdTag>(`/api/v1/admin/crowd-tags/${tagId}`),
  createCrowdTag: (data: Partial<CrowdTag>) =>
    api.post<CrowdTag>('/api/v1/admin/crowd-tags', data),
  updateCrowdTag: (tagId: string, data: Partial<CrowdTag>) =>
    api.put<CrowdTag>(`/api/v1/admin/crowd-tags/${tagId}`, data),
  deleteCrowdTag: (tagId: string) =>
    api.delete(`/api/v1/admin/crowd-tags/${tagId}`),
  listCrowdTagMembers: (tagId: string, page = 1, pageSize = 50) =>
    api.get<PaginatedResponse<CrowdTagDetail>>(`/api/v1/admin/crowd-tags/${tagId}/members?page=${page}&page_size=${pageSize}`),
  addCrowdTagMember: (tagId: string, userId: string) =>
    api.post<CrowdTagDetail>(`/api/v1/admin/crowd-tags/${tagId}/members`, { user_id: userId }),
  removeCrowdTagMember: (tagId: string, userId: string) =>
    api.delete(`/api/v1/admin/crowd-tags/${tagId}/members/${userId}`),

  // Orders (read-only)
  listOrders: (filter?: OrderFilter) => {
    const params = new URLSearchParams();
    if (filter?.status !== undefined) params.set('status', String(filter.status));
    if (filter?.activity_id) params.set('activity_id', String(filter.activity_id));
    if (filter?.user_id) params.set('user_id', filter.user_id);
    if (filter?.page) params.set('page', String(filter.page));
    if (filter?.page_size) params.set('page_size', String(filter.page_size));
    const qs = params.toString();
    return api.get<PaginatedResponse<Order>>(`/api/v1/admin/orders${qs ? '?' + qs : ''}`);
  },
  getOrder: (orderId: string) => api.get<Order>(`/api/v1/admin/orders/${orderId}`),

  // Teams (read-only)
  listTeams: (status?: number, page = 1, pageSize = 20) => {
    const params = new URLSearchParams({ page: String(page), page_size: String(pageSize) });
    if (status !== undefined) params.set('status', String(status));
    return api.get<PaginatedResponse<Team>>(`/api/v1/admin/teams?${params}`);
  },
  getTeam: (teamId: string) => api.get<Team>(`/api/v1/admin/teams/${teamId}`),
  getTeamOrders: (teamId: string) => api.get<Order[]>(`/api/v1/admin/teams/${teamId}/orders`),
};

// ===== User-facing order queries =====
export const orderApi = {
  listByUser: (userId: string) => api.get<Order[]>(`/api/v1/orders?user_id=${userId}`),
  getByOutTradeNo: (outTradeNo: string) => api.get<Order>(`/api/v1/orders/${outTradeNo}`),
};
