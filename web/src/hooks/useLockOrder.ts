import { useMutation } from '@tanstack/react-query';
import { tradeApi } from '../api/trade';
import type { LockRequest, LockResult } from '../api/types';
import { ApiError } from '../api/client';

export function useLockOrder() {
  return useMutation<LockResult, ApiError, LockRequest>({
    mutationFn: tradeApi.lock,
  });
}
