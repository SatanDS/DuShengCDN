export interface AuthUser {
  id: number;
  username: string;
  display_name: string;
  role: number;
  status: number;
  email?: string;
  github_id?: string;
  wechat_id?: string;
  csrf_token?: string;
}

export interface LoginPayload {
  username: string;
  password: string;
}

export interface RegisterPayload {
  username: string;
  password: string;
  email?: string;
  verification_code?: string;
}

export interface PasswordResetRequestPayload {
  email: string;
  token: string;
  new_password: string;
}
