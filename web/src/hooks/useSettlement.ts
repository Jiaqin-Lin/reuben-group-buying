import { useMutation } from '@tanstack/react-query';
import { tradeApi } from '../api/trade';
import type { SettlementRequest, SettlementResult } from '../api/types';
import { ApiError } from '../api/client';

export function useSettlement() {
  return useMutation<SettlementResult, ApiError, SettlementRequest>({
    mutationFn: tradeApi.settlement,
  });
}
