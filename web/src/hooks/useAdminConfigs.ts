import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { adminApi } from '../api/admin';
import { ApiError } from '../api/client';

export function useAdminConfigs() {
  return useQuery({
    queryKey: ['admin', 'configs'],
    queryFn: adminApi.listConfigs,
    staleTime: 30_000,
  });
}

export function useUpdateConfig() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({
      key,
      value,
      updatedBy,
    }: {
      key: string;
      value: unknown;
      updatedBy?: string;
    }) => adminApi.updateConfig(key, value, updatedBy),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin', 'configs'] });
    },
    onError: (err: ApiError) => {
      throw err;
    },
  });
}
