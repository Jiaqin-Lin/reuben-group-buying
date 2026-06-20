import { api } from './client';
import type {
  LockRequest,
  LockResult,
  SettlementRequest,
  SettlementResult,
  RefundRequest,
  RefundResult,
  Payment,
} from './types';

export const tradeApi = {
  lock: (req: LockRequest) => api.post<LockResult>('/api/v1/trade/lock', req),
  settlement: (req: SettlementRequest) =>
    api.post<SettlementResult>('/api/v1/trade/settlement', req),
  refund: (req: RefundRequest) =>
    api.post<RefundResult>('/api/v1/trade/refund', req),
  getPayment: (outTradeNo: string) =>
    api.get<{ payment: Payment }>(`/api/v1/payments/${outTradeNo}`),
};
