const countryCodeNames: Record<string, string> = {
  AE: '阿联酋',
  AU: '澳大利亚',
  BR: '巴西',
  CA: '加拿大',
  CH: '瑞士',
  CN: '中国',
  DE: '德国',
  EU: '欧洲',
  FR: '法国',
  GB: '英国',
  HK: '香港',
  ID: '印度尼西亚',
  IN: '印度',
  JP: '日本',
  KR: '韩国',
  LU: '卢森堡',
  MO: '澳门',
  MY: '马来西亚',
  NL: '荷兰',
  PH: '菲律宾',
  RU: '俄罗斯',
  SC: '塞舌尔',
  SG: '新加坡',
  TH: '泰国',
  TW: '台湾',
  UK: '英国',
  US: '美国',
  VN: '越南',
};

const countryAliases: Record<string, string> = {
  australia: 'AU',
  brazil: 'BR',
  canada: 'CA',
  china: 'CN',
  france: 'FR',
  germany: 'DE',
  'hong kong': 'HK',
  india: 'IN',
  indonesia: 'ID',
  japan: 'JP',
  korea: 'KR',
  'macao sar': 'MO',
  macau: 'MO',
  malaysia: 'MY',
  netherlands: 'NL',
  philippines: 'PH',
  russia: 'RU',
  seychelles: 'SC',
  singapore: 'SG',
  switzerland: 'CH',
  taiwan: 'TW',
  thailand: 'TH',
  'the netherlands': 'NL',
  'united arab emirates': 'AE',
  'united kingdom': 'GB',
  'united states': 'US',
  vietnam: 'VN',
};

export function formatCountryName(
  value: string | null | undefined,
  fallback = '未识别地区',
) {
  const raw = (value ?? '').trim();
  if (!raw) {
    return fallback;
  }
  const text = raw.toLowerCase().startsWith('country:')
    ? raw.slice(raw.indexOf(':') + 1).trim()
    : raw;
  if (!text) {
    return fallback;
  }
  if (text.toLowerCase() === 'global') {
    return '全球';
  }
  const upper = text.toUpperCase();
  if (countryCodeNames[upper]) {
    return countryCodeNames[upper];
  }
  const aliasCode = countryAliases[text.toLowerCase()];
  if (aliasCode && countryCodeNames[aliasCode]) {
    return countryCodeNames[aliasCode];
  }
  return text;
}

export function formatRegionWithCode(value: string | null | undefined) {
  const raw = (value ?? '').trim();
  if (!raw) {
    return '未识别地区';
  }
  const normalized = raw.toLowerCase().startsWith('country:')
    ? raw.slice(raw.indexOf(':') + 1).trim()
    : raw;
  const label = formatCountryName(raw, raw);
  const upper = normalized.toUpperCase();
  if (upper.length === 2 && countryCodeNames[upper]) {
    return `${label} (${upper})`;
  }
  return label;
}
