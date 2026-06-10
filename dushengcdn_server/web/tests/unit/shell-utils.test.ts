import { describe, expect, it } from 'vitest';

import { shellQuote } from '@/lib/utils/shell';

describe('shellQuote', () => {
  it('single-quotes shell arguments and escapes embedded quotes', () => {
    expect(shellQuote("https://cdn.example.com/a b?x=';echo bad")).toBe(
      "'https://cdn.example.com/a b?x='\\'';echo bad'",
    );
  });
});
