const contentSecurityPolicyDirectives = [
  "default-src 'self'",
  "base-uri 'self'",
  "object-src 'none'",
  "frame-ancestors 'none'",
  "form-action 'self'",
  "script-src 'self' 'unsafe-inline' https://challenges.cloudflare.com",
  "style-src 'self' 'unsafe-inline'",
  "img-src 'self' data: blob: https:",
  "font-src 'self' data:",
  "connect-src 'self' http: https: ws: wss:",
  "frame-src https://challenges.cloudflare.com",
  "worker-src 'self' blob:",
  'upgrade-insecure-requests',
];

export const contentSecurityPolicy = contentSecurityPolicyDirectives.join('; ');
