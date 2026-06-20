import type { ApiEnvelope } from './types';

const API_BASE = '';

export class ApiError extends Error {
  code: string;
  info: string;

  constructor(code: string, info: string) {
    super(info);
    this.code = code;
    this.info = info;
    this.name = 'ApiError';
  }
}

function getAdminToken(): string | null {
  try {
    return sessionStorage.getItem('gbm_admin_token');
  } catch {
    return null;
  }
}

async function request<T>(
  path: string,
  options: RequestInit = {},
): Promise<T> {
  const url = `${API_BASE}${path}`;

  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
  };

  // Inject admin token for admin API paths
  if (path.startsWith('/api/v1/admin')) {
    const token = getAdminToken();
    if (token) {
      headers['X-Admin-Token'] = token;
    }
  }

  const res = await fetch(url, {
    headers: {
      ...headers,
      ...(options.headers as Record<string, string> | undefined),
    },
    ...options,
  });

  // Handle non-JSON responses (e.g., QR HTML page, pay notify plain text)
  const contentType = res.headers.get('content-type') || '';
  if (!contentType.includes('application/json')) {
    const text = await res.text();
    if (!res.ok) {
      throw new ApiError('HTTP_ERROR', `HTTP ${res.status}: ${res.statusText}`);
    }
    return text as unknown as T;
  }

  const envelope: ApiEnvelope<T> = await res.json();

  // 优先检查业务错误码（后端 HTTP 4xx/5xx 也可能携带业务 code+info）
  if (envelope.code && envelope.code !== '0000') {
    throw new ApiError(envelope.code, envelope.info);
  }

  if (!res.ok) {
    throw new ApiError(
      'HTTP_ERROR',
      `HTTP ${res.status}: ${res.statusText}`,
    );
  }

  return envelope.data as T;
}

export const api = {
  get: <T>(path: string, signal?: AbortSignal) =>
    request<T>(path, { method: 'GET', signal }),
  post: <T>(path: string, body?: unknown) =>
    request<T>(path, { method: 'POST', body: JSON.stringify(body) }),
  put: <T>(path: string, body?: unknown) =>
    request<T>(path, { method: 'PUT', body: JSON.stringify(body) }),
  delete: <T>(path: string) =>
    request<T>(path, { method: 'DELETE' }),
};
