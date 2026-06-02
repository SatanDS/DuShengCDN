import { z } from 'zod';

export const managedDomainSchema = z.object({
  domain: z
    .string()
    .trim()
    .min(1, '请输入域名')
    .max(255, '域名不能超过 255 个字符')
    .refine(
      (value) => !value.includes('://') && !value.includes('/'),
      '域名格式不合法',
    )
    .refine(
      (value) =>
        /^(?:\*\.)?(?=.{1,253}$)(?:[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,63}$/.test(
          value,
        ),
      '域名格式不合法',
    )
    .refine(
      (value) =>
        !value.includes('*') ||
        (value.startsWith('*.') && value.indexOf('*', 1) === -1),
      '通配符域名仅支持 *.example.com 格式',
    ),
  cert_id: z.string(),
  enabled: z.boolean(),
  remark: z.string().max(255, '备注不能超过 255 个字符'),
});

export const manualImportSchema = z.object({
  name: z
    .string()
    .trim()
    .min(1, '请输入证书名称')
    .max(255, '证书名称不能超过 255 个字符'),
  cert_pem: z.string().trim().min(1, '请输入证书内容'),
  key_pem: z.string().trim().min(1, '请输入私钥内容'),
  remark: z.string().max(255, '备注不能超过 255 个字符'),
});

export type ManagedDomainFormValues = z.infer<typeof managedDomainSchema>;
export type ManualImportFormValues = z.infer<typeof manualImportSchema>;

export type FileImportFormValues = {
  name: string;
  remark: string;
};

export const defaultManagedDomainValues: ManagedDomainFormValues = {
  domain: '',
  cert_id: '',
  enabled: true,
  remark: '',
};

export const defaultManualImportValues: ManualImportFormValues = {
  name: '',
  cert_pem: '',
  key_pem: '',
  remark: '',
};

export const defaultFileImportValues: FileImportFormValues = {
  name: '',
  remark: '',
};

export const acmeApplySchema = z
  .object({
    name: z.string().trim().min(1, '请输入证书名称').max(255),
    primary_domain: z.string().trim().min(1, '请输入主域名'),
    other_domains: z.string(),
    dns_provider_mode: z.enum(['cloudflare', 'authoritative']),
    dns_account_id: z.coerce.number(),
    dns_zone_id_ref: z.coerce.number().nullable(),
    acme_account_id: z.coerce.number(),
    key_algorithm: z.string(),
    auto_renew: z.boolean(),
    disable_cname: z.boolean().default(false),
    skip_dns: z.boolean().default(false),
    dns1: z.string().default(''),
    dns2: z.string().default(''),
    remark: z.string().max(255),
  })
  .superRefine((value, context) => {
    if (value.dns_provider_mode === 'cloudflare' && value.dns_account_id < 1) {
      context.addIssue({
        code: z.ZodIssueCode.custom,
        path: ['dns_account_id'],
        message: '请选择 Cloudflare 账号',
      });
    }
    if (
      value.dns_provider_mode === 'authoritative' &&
      (!value.dns_zone_id_ref || value.dns_zone_id_ref < 1)
    ) {
      context.addIssue({
        code: z.ZodIssueCode.custom,
        path: ['dns_zone_id_ref'],
        message: '请选择本地托管域名',
      });
    }
  });

export type AcmeApplyFormValues = z.infer<typeof acmeApplySchema>;

export const defaultAcmeApplyValues: AcmeApplyFormValues = {
  name: '',
  primary_domain: '',
  other_domains: '',
  dns_provider_mode: 'cloudflare',
  dns_account_id: 0,
  dns_zone_id_ref: null,
  acme_account_id: 0,
  key_algorithm: 'RSA2048',
  auto_renew: true,
  disable_cname: false,
  skip_dns: false,
  dns1: '',
  dns2: '',
  remark: '',
};
