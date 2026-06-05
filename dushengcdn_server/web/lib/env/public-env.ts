import { z } from 'zod';

const publicEnvSchema = z.object({
  NEXT_PUBLIC_API_BASE_URL: z.string().default('/api'),
  NEXT_PUBLIC_APP_VERSION: z.string().default('dev'),
});

const rawEnv = {
  NEXT_PUBLIC_API_BASE_URL: process.env.NEXT_PUBLIC_API_BASE_URL,
  NEXT_PUBLIC_APP_VERSION: process.env.NEXT_PUBLIC_APP_VERSION,
};

const parsedEnv = publicEnvSchema.parse(rawEnv);

export function normalizeApiBaseUrl(value: string) {
  const trimmed = value.trim();

  if (!trimmed) {
    return '/api';
  }

  if (trimmed === '/') {
    return '';
  }

  if (trimmed.startsWith('/')) {
    if (trimmed.startsWith('//') || trimmed.includes('\\')) {
      throw new Error(
        'NEXT_PUBLIC_API_BASE_URL must be a same-origin path or an http(s) URL.',
      );
    }
    return trimmed.replace(/\/+$/, '');
  }

  let parsed: URL;
  try {
    parsed = new URL(trimmed);
  } catch {
    throw new Error(
      'NEXT_PUBLIC_API_BASE_URL must be a same-origin path or an http(s) URL.',
    );
  }

  if (
    (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') ||
    parsed.username ||
    parsed.password ||
    parsed.search ||
    parsed.hash
  ) {
    throw new Error(
      'NEXT_PUBLIC_API_BASE_URL must be a same-origin path or an http(s) URL.',
    );
  }

  return parsed.toString().replace(/\/+$/, '');
}

export const publicEnv = {
  apiBaseUrl: normalizeApiBaseUrl(parsedEnv.NEXT_PUBLIC_API_BASE_URL),
  appVersion: parsedEnv.NEXT_PUBLIC_APP_VERSION,
};
