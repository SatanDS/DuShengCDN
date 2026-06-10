import { publicEnv } from '@/lib/env/public-env';
import type { ApiEnvelope } from '@/types/api';

const csrfTokenStore = {
  value: '',
};

export function setCSRFToken(token: string | undefined | null) {
  csrfTokenStore.value = token ?? '';
}

export function getCSRFToken() {
  return csrfTokenStore.value;
}

export class ApiError extends Error {
  constructor(
    message: string,
    public readonly status: number,
  ) {
    super(message);
    this.name = 'ApiError';
  }
}

export function getApiUrl(path: string) {
  const normalizedPath = path.startsWith('/') ? path : `/${path}`;
  return `${publicEnv.apiBaseUrl}${normalizedPath}`;
}

export async function apiRequest<T>(path: string, init?: RequestInit) {
  const headers = new Headers(init?.headers ?? {});
  const method = init?.method?.toUpperCase() ?? 'GET';

  if (!(init?.body instanceof FormData) && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json');
  }
  if (method !== 'GET' && method !== 'HEAD' && !headers.has('X-CSRF-Token')) {
    const csrfToken = getCSRFToken();
    if (csrfToken) {
      headers.set('X-CSRF-Token', csrfToken);
    }
  }

  const response = await fetch(getApiUrl(path), {
    ...init,
    credentials: 'include',
    headers,
    cache: method === 'GET' ? 'no-store' : init?.cache,
  });

  let payload: ApiEnvelope<T> | null = null;

  try {
    payload = (await response.json()) as ApiEnvelope<T>;
  } catch {
    payload = null;
  }

  if (!response.ok) {
    throw new ApiError(
      payload?.message || `请求失败（${response.status}）`,
      response.status,
    );
  }

  if (!payload) {
    throw new ApiError('响应格式无效', response.status);
  }

  if (!payload.success) {
    throw new ApiError(payload.message || '请求失败', response.status);
  }

  return payload.data;
}
