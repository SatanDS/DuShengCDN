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
  return apiRequest<void>('/user/logout');
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
  const searchParams = new URLSearchParams({ email });
  if (turnstileToken) {
    searchParams.set('turnstile', turnstileToken);
  }

  return apiRequest<void>(`/verification?${searchParams.toString()}`);
}

export function sendPasswordResetEmail(email: string, turnstileToken?: string) {
  const searchParams = new URLSearchParams({ email });
  if (turnstileToken) {
    searchParams.set('turnstile', turnstileToken);
  }

  return apiRequest<void>(`/reset_password?${searchParams.toString()}`);
}

export function resetPassword(payload: PasswordResetRequestPayload) {
  return apiRequest<string>('/user/reset', {
    method: 'POST',
    body: JSON.stringify(payload),
  });
}

export function exchangeGitHubCode(code: string) {
  return apiRequest<AuthUser>(`/oauth/github?code=${encodeURIComponent(code)}`);
}

export interface OAuthAuthorizeResult {
  authorize_url: string;
}

export interface OAuthCallbackResult {
  status: 'logged_in' | 'registered' | 'linked' | 'link_required';
  user?: AuthUser;
}

export interface LinkExistingOAuthPayload {
  username: string;
  password: string;
}

export function getOAuthAuthorizeUrl(source: number | string) {
  return apiRequest<OAuthAuthorizeResult>(
    `/oauth/${encodeURIComponent(String(source))}/authorize`,
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
