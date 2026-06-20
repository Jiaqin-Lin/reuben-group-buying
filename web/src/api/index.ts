import { api } from './client';
import type { PaginatedResponse, Team, Order } from './types';

export const teamApi = {
  listByActivity: (activityId: number, page = 1, pageSize = 20) => {
    const params = new URLSearchParams({
      activity_id: String(activityId),
      page: String(page),
      page_size: String(pageSize),
    });
    return api.get<PaginatedResponse<Team>>(`/api/v1/teams?${params}`);
  },
  getDetail: (teamId: string) =>
    api.get<{ team: Team; members: Order[] }>(`/api/v1/teams/${teamId}`),
};
