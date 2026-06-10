import { apiRequest } from '@/lib/api/client';
import type {
  AuthUser,
  LoginPayload,
  PasswordResetRequestPayload,
  RegisterPayload,
} from '@/types/auth';

export function getCurrentUser() {
  return apiRequest<AuthUser>('/user/self');
}

export function login(payload: LoginPayload) {
  return apiRequest<AuthUser>('/user/login', {
    method: 'POST',
    body: JSON.stringify(payload),
  });
}

export function logout() {
  return apiRequest<void>('/user/logout', {
    method: 'POST',
  });
}

export function register(payload: RegisterPayload, turnstileToken?: string) {
  const query = turnstileToken
    ? `?turnstile=${encodeURIComponent(turnstileToken)}`
    : '';

  return apiRequest<void>(`/user/register${query}`, {
    method: 'POST',
    body: JSON.stringify(payload),
  });
}

export function sendEmailVerification(email: string, turnstileToken?: string) {
  const searchParams = new URLSearchParams();
  if (turnstileToken) {
    searchParams.set('turnstile', turnstileToken);
  }
  const query = searchParams.size > 0 ? `?${searchParams.toString()}` : '';

  return apiRequest<void>(`/verification${query}`, {
    method: 'POST',
    body: JSON.stringify({ email }),
  });
}

export function sendPasswordResetEmail(email: string, turnstileToken?: string) {
  const searchParams = new URLSearchParams();
  if (turnstileToken) {
    searchParams.set('turnstile', turnstileToken);
  }
  const query = searchParams.size > 0 ? `?${searchParams.toString()}` : '';

  return apiRequest<void>(`/reset_password${query}`, {
    method: 'POST',
    body: JSON.stringify({ email }),
  });
}

export function resetPassword(payload: PasswordResetRequestPayload) {
  return apiRequest<void>('/user/reset', {
    method: 'POST',
    body: JSON.stringify(payload),
  });
}

export function getLegacyGitHubAuthorizeUrl() {
  return apiRequest<OAuthAuthorizeResult>('/oauth/github/authorize');
}

export function exchangeGitHubCode(code: string, state: string) {
  const searchParams = new URLSearchParams({ code, state });
  return apiRequest<AuthUser>(`/oauth/github?${searchParams.toString()}`);
}

export interface OAuthAuthorizeResult {
  authorize_url: string;
}

export interface OAuthCallbackResult {
  status: 'logged_in' | 'registered' | 'linked' | 'link_required';
  user?: AuthUser;
  csrf_token?: string;
}

export interface LinkExistingOAuthPayload {
  username: string;
  password: string;
}

export function getOAuthAuthorizeUrl(source: number | string) {
  const sourcePath = String(source).startsWith('/oauth/')
    ? String(source)
    : `/oauth/${encodeURIComponent(String(source))}/authorize`;
  return apiRequest<OAuthAuthorizeResult>(
    sourcePath,
  );
}

export function exchangeOAuthCode(
  source: number | string,
  code: string,
  state: string,
) {
  const searchParams = new URLSearchParams({ code, state });
  return apiRequest<OAuthCallbackResult>(
    `/oauth/${encodeURIComponent(String(source))}/callback?${searchParams.toString()}`,
  );
}

export function linkExistingOAuthAccount(payload: LinkExistingOAuthPayload) {
  return apiRequest<OAuthCallbackResult>('/oauth/link-existing', {
    method: 'POST',
    body: JSON.stringify(payload),
  });
}
