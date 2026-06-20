import { useMutation } from '@tanstack/react-query';
import { tradeApi } from '../api/trade';
import type { RefundRequest, RefundResult } from '../api/types';
import { ApiError } from '../api/client';

export function useRefund() {
  return useMutation<RefundResult, ApiError, RefundRequest>({
    mutationFn: tradeApi.refund,
  });
}
