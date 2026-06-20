import { useMutation } from '@tanstack/react-query';
import { trialApi } from '../api/trial';
import type { TrialRequest, TrialResult } from '../api/types';
import { ApiError } from '../api/client';

export function useTrial() {
  return useMutation<TrialResult, ApiError, TrialRequest>({
    mutationFn: trialApi.trial,
    gcTime: 10_000,
  });
}
