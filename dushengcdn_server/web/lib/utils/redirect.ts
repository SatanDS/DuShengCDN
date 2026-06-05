export function normalizeInternalRedirect(value: string | null | undefined) {
  const redirect = value?.trim();

  if (!redirect) {
    return '/';
  }

  if (
    !redirect.startsWith('/') ||
    redirect.startsWith('//') ||
    redirect.includes('\\') ||
    /^[a-z][a-z0-9+.-]*:/i.test(redirect)
  ) {
    return '/';
  }

  return redirect;
}

export function normalizeTrustedExternalUrl(value: string) {
  const url = value.trim();

  if (!url) {
    throw new Error('跳转地址为空。');
  }

  try {
    const parsed = new URL(url);
    if (parsed.protocol === 'https:' || parsed.protocol === 'http:') {
      return parsed.toString();
    }
  } catch {
    throw new Error('跳转地址格式无效。');
  }

  throw new Error('跳转地址协议不受支持。');
}
