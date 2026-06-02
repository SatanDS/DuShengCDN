import { afterEach, describe, expect, it, vi } from 'vitest';

import { ClipboardCopyError, copyToClipboard } from '@/lib/utils/clipboard';

function setClipboard(clipboard: Pick<Clipboard, 'writeText'> | undefined) {
  Object.defineProperty(navigator, 'clipboard', {
    configurable: true,
    value: clipboard,
  });
}

function setExecCommand(handler: ((command: string) => boolean) | undefined) {
  Object.defineProperty(document, 'execCommand', {
    configurable: true,
    value: handler ? vi.fn(handler) : undefined,
  });
}

afterEach(() => {
  Reflect.deleteProperty(navigator, 'clipboard');
  Reflect.deleteProperty(document, 'execCommand');
  document.body.innerHTML = '';
  vi.restoreAllMocks();
});

describe('copyToClipboard', () => {
  it('uses the browser Clipboard API when it is available', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    const execCommand = vi.fn(() => true);

    setClipboard({ writeText });
    setExecCommand(execCommand);

    await copyToClipboard('curl install command');

    expect(writeText).toHaveBeenCalledWith('curl install command');
    expect(execCommand).not.toHaveBeenCalled();
  });

  it('falls back to selection copy when Clipboard API is unavailable', async () => {
    const execCommand = vi.fn(() => true);

    setClipboard(undefined);
    setExecCommand(execCommand);

    await copyToClipboard('docker run command');

    expect(execCommand).toHaveBeenCalledWith('copy');
    expect(document.querySelector('textarea')).toBeNull();
  });

  it('explains HTTP clipboard restrictions when automatic copy fails', async () => {
    setClipboard(undefined);
    setExecCommand(() => false);

    await expect(copyToClipboard('install command')).rejects.toMatchObject({
      name: 'ClipboardCopyError',
      message: expect.stringContaining('当前面板通过 HTTP 访问'),
    } satisfies Partial<ClipboardCopyError>);
  });

  it('explains when the browser does not expose Clipboard API', async () => {
    setClipboard(undefined);
    setExecCommand(() => false);

    await expect(copyToClipboard('agent install command')).rejects.toMatchObject({
      name: 'ClipboardCopyError',
      message: expect.stringContaining('浏览器没有提供剪贴板写入接口'),
    } satisfies Partial<ClipboardCopyError>);
  });
});
