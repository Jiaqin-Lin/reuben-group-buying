import { api } from './client';
import type { TrialRequest, TrialResult } from './types';

export const trialApi = {
  trial: (req: TrialRequest) => api.post<TrialResult>('/api/v1/trial', req),
};
