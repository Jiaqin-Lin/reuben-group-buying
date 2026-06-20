/** Shared TypeScript types mirroring Go models in internal/model/ */

// ===== Envelope =====
export interface ApiEnvelope<T = unknown> {
  code: string;
  info: string;
  data?: T;
}

// ===== Activity =====
export interface Activity {
  id: number;
  activity_id: number;
  name: string;
  discount_id: string;
  group_type: number; // 0=auto, 1=target
  target_count: number;
  take_limit: number;
  valid_minutes: number;
  status: number; // 0=created, 1=active, 2=expired, 3=abandoned
  start_time: string;
  end_time: string;
  tag_id: string | null;
  tag_scope: string | null;
}

// ===== Discount =====
export interface Discount {
  id: number;
  discount_id: string;
  name: string;
  description: string;
  plan_type: 'ZJ' | 'MJ' | 'ZK' | 'N';
  expression: string;
  discount_type: number; // 0=base, 1=tag
  tag_id: string | null;
}

// ===== Product =====
export interface Product {
  id: number;
  goods_id: string;
  goods_name: string;
  original_price: string;
}

// ===== ActivityProduct =====
export interface ActivityProduct {
  id: number;
  source: string;
  channel: string;
  goods_id: string;
  activity_id: number;
}

// ===== Order =====
export interface Order {
  id: number;
  user_id: string;
  team_id: string;
  order_id: string;
  activity_id: number;
  goods_id: string;
  source: string;
  channel: string;
  original_price: string;
  deduction_price: string;
  pay_price: string;
  status: number; // 0=locked, 1=paid, 2=refunded
  out_trade_no: string;
  out_trade_time: string | null;
  created_at: string;
  updated_at: string;
}

// ===== Team =====
export interface Team {
  id: number;
  team_id: string;
  activity_id: number;
  source: string;
  channel: string;
  original_price: string;
  deduction_price: string;
  pay_price: string;
  target_count: number;
  complete_count: number;
  lock_count: number;
  status: number; // 0=forming, 1=complete, 2=failed, 3=complete_with_refunds
  valid_start: string;
  valid_end: string;
  notify_type: string;
  notify_url: string | null;
  created_at: string;
}

// ===== Payment =====
export interface Payment {
  id: number;
  order_id: string;
  out_trade_no: string;
  user_id: string;
  team_id: string;
  amount: string;
  subject: string;
  trade_no: string | null;
  status: number;
  qr_code_url: string | null;
  pay_url: string | null;
  paid_at: string | null;
}

// ===== NotifyTask =====
export interface NotifyTask {
  id: number;
  activity_id: number;
  team_id: string;
  category: string | null;
  notify_type: string;
  notify_target: string | null;
  retry_count: number;
  status: number;
  payload: string;
  uuid: string;
}

// ===== CrowdTag =====
export interface CrowdTag {
  id: number;
  tag_id: string;
  tag_name: string;
  tag_desc: string;
  statistics: number;
}

export interface CrowdTagDetail {
  id: number;
  tag_id: string;
  user_id: string;
}

// ===== DynamicConfig =====
export interface DynamicConfig {
  config_key: string;
  config_value: string;
  version: number;
  updated_by: string;
  updated_at: string;
}

// ===== API Request/Response types =====

export interface TrialRequest {
  user_id: string;
  goods_id: string;
  source: string;
  channel: string;
}

export interface TrialResult {
  goods_id: string;
  goods_name: string;
  original_price: string;
  deduction_price: string;
  pay_price: string;
  activity_id: number;
  target_count: number;
  start_time: string;
  end_time: string;
  is_visible: boolean;
  is_enable: boolean;
}

export interface LockRequest {
  user_id: string;
  activity_id: number;
  goods_id: string;
  source: string;
  channel: string;
  out_trade_no: string;
  team_id?: string;
  notify_url?: string;
}

export interface LockResult {
  order_id: string;
  out_trade_no: string;
  user_id: string;
  team_id: string;
  original_price: string;
  deduction_price: string;
  pay_price: string;
  pay_url?: string;
  status: number;
}

export interface SettlementRequest {
  user_id: string;
  out_trade_no: string;
  out_trade_time: string;
  source: string;
  channel: string;
}

export interface SettlementResult {
  order_id: string;
  out_trade_no: string;
  team_id: string;
  activity_id: number;
  is_complete: boolean;
  take_count: number;
}

export interface RefundRequest {
  user_id: string;
  out_trade_no: string;
}

export interface RefundResult {
  order_id: string;
  out_trade_no: string;
  team_id: string;
  activity_id: number;
  refund_type: string;
  team_status: number;
}

// ===== Admin types =====

export interface DashboardStats {
  active_activities: number;
  forming_teams: number;
  today_orders: number;
  complete_teams: number;
  failed_teams: number;
  config_count: number;
  total_orders: number;
  recent_orders: Order[];
}

export interface PaginatedResponse<T> {
  items: T[];
  total: number;
  page: number;
  page_size: number;
}

export interface OrderFilter {
  status?: number;
  activity_id?: number;
  user_id?: string;
  start_date?: string;
  end_date?: string;
  page?: number;
  page_size?: number;
}
