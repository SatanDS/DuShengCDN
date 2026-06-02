export class ClipboardCopyError extends Error {
  readonly originalError: unknown;

  constructor(message: string, originalError?: unknown) {
    super(message);
    this.name = 'ClipboardCopyError';
    this.originalError = originalError;
  }
}

function getClipboardUnavailableReason(hasClipboardWrite: boolean) {
  if (hasClipboardWrite) {
    return '';
  }

  return '浏览器没有提供剪贴板写入接口，通常是因为当前页面不是 HTTPS、安全上下文不足，或浏览器策略禁用了复制能力。';
}

function buildClipboardFailureMessage(hasClipboardWrite: boolean) {
  const unavailableReason = getClipboardUnavailableReason(hasClipboardWrite);
  const suffix = unavailableReason ? `具体原因：${unavailableReason}` : '';

  if (typeof window === 'undefined') {
    return `复制失败：当前运行环境不支持浏览器剪贴板写入，请手动复制内容。${suffix}`;
  }

  if (window.location.protocol === 'http:') {
    return `复制失败：当前面板通过 HTTP 访问；在公网 IP 或域名下这不是浏览器认可的安全上下文，浏览器可能不会提供 Clipboard API，兼容复制也未成功。请手动复制内容，或改用 HTTPS 后重试。${suffix}`;
  }

  if (window.isSecureContext === false) {
    return `复制失败：当前页面不是浏览器认可的安全上下文，剪贴板写入被禁用。请手动复制内容，或改用 HTTPS 后重试。${suffix}`;
  }

  return `复制失败：浏览器拒绝写入剪贴板，可能是站点权限被拒绝、页面未获得焦点，或复制操作没有由点击触发。请手动复制内容后重试。${suffix}`;
}

function copyWithSelection(value: string) {
  if (
    typeof document === 'undefined' ||
    typeof document.execCommand !== 'function'
  ) {
    return false;
  }

  const container = document.body ?? document.documentElement;
  if (!container) {
    return false;
  }

  const textarea = document.createElement('textarea');
  const activeElement =
    document.activeElement instanceof HTMLElement
      ? document.activeElement
      : null;

  textarea.value = value;
  textarea.setAttribute('readonly', '');
  textarea.setAttribute('aria-hidden', 'true');
  textarea.style.position = 'fixed';
  textarea.style.top = '0';
  textarea.style.left = '-9999px';
  textarea.style.width = '1px';
  textarea.style.height = '1px';
  textarea.style.opacity = '0';

  container.appendChild(textarea);

  try {
    textarea.focus();
    textarea.select();
    textarea.setSelectionRange(0, value.length);

    return document.execCommand('copy');
  } catch {
    return false;
  } finally {
    textarea.remove();
    activeElement?.focus();
  }
}

export async function copyToClipboard(value: string) {
  let clipboardError: unknown;
  const hasClipboardWrite =
    typeof navigator !== 'undefined' &&
    typeof navigator.clipboard?.writeText === 'function';

  if (hasClipboardWrite) {
    try {
      await navigator.clipboard.writeText(value);
      return;
    } catch (error) {
      clipboardError = error;
    }
  }

  if (copyWithSelection(value)) {
    return;
  }

  throw new ClipboardCopyError(
    buildClipboardFailureMessage(hasClipboardWrite),
    clipboardError,
  );
}
