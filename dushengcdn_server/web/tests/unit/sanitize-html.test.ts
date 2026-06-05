import { describe, expect, it } from 'vitest';

import { sanitizeHtml } from '@/lib/utils/sanitize-html';

describe('sanitizeHtml', () => {
  it('removes scripts, event handlers, unsafe protocols and styles', () => {
    const html = sanitizeHtml(
      '<p onclick="alert(1)" style="color:red">Hello <strong>world</strong></p><script>alert(1)</script><a href="javascript:alert(1)" target="_blank">bad</a><img src="https://example.com/a.png" onerror="alert(1)" alt="a">',
    );

    expect(html).toContain('<strong>world</strong>');
    expect(html).toContain('<a target="_blank" rel="noreferrer noopener">bad</a>');
    expect(html).toContain('<img src="https://example.com/a.png" alt="a">');
    expect(html).not.toContain('script');
    expect(html).not.toContain('onclick');
    expect(html).not.toContain('onerror');
    expect(html).not.toContain('javascript:');
    expect(html).not.toContain('style=');
  });

  it('removes unsupported tags and protocol-relative media', () => {
    const html = sanitizeHtml(
      '<iframe src="https://example.com"></iframe><svg><animate onbegin="alert(1)"></animate></svg><img src="//example.com/a.png" alt="bad"><a href="/docs">docs</a><a href="mailto:admin@example.com">mail</a>',
    );

    expect(html).not.toContain('iframe');
    expect(html).not.toContain('svg');
    expect(html).not.toContain('animate');
    expect(html).toContain('<img alt="bad">');
    expect(html).toContain('<a href="/docs">docs</a>');
    expect(html).toContain('<a href="mailto:admin@example.com">mail</a>');
  });
});
