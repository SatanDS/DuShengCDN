import { describe, expect, it } from 'vitest';

import {
  parseCustomHeadersText,
  parseOriginUrl,
  parseOriginUrls,
  validateOriginHost,
} from '@/features/proxy-routes/helpers';

describe('proxy route helpers', () => {
  it('rejects origin URL userinfo', () => {
    expect(parseOriginUrls('https://user:pass@origin.example.net:8443')).toEqual({
      urls: [],
      error:
        '源站地址不能包含用户名或密码：https://user:pass@origin.example.net:8443',
    });
    expect(() =>
      parseOriginUrl('https://user:pass@origin.example.net:8443'),
    ).toThrow('源站地址不能包含用户名或密码');
  });

  it('rejects unsafe origin path characters', () => {
    expect(parseOriginUrls('https://origin.example.net/app;internal;')).toEqual({
      urls: [],
      error:
        '源站地址路径或查询参数包含不安全字符：https://origin.example.net/app;internal;',
    });
    expect(() => parseOriginUrl('https://origin.example.net/app;internal;')).toThrow(
      '源站地址路径或查询参数包含不安全字符',
    );
  });

  it('rejects dynamic origin host and custom Host header overrides', () => {
    expect(validateOriginHost('$http_x_origin')).toBe('回源 Host 格式不合法');
    expect(validateOriginHost('origin.example.net:65536')).toBe('回源 Host 格式不合法');
    expect(parseCustomHeadersText('Host: attacker.example.com')).toEqual({
      headers: [],
      error: 'Host 请求头请使用回源 Host 配置',
    });
  });
});
