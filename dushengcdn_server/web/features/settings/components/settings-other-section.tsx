import { AppCard } from '@/components/ui/app-card';
import {
  PrimaryButton,
  ResourceField,
  ResourceInput,
  ResourceTextarea,
} from '@/features/shared/components/resource-primitives';

export type OtherSettingsFields = {
  Notice: string;
  SystemName: string;
  HomePageLink: string;
  About: string;
  Footer: string;
};

type OtherSettingsFieldKey = keyof OtherSettingsFields;

type SettingsOtherSectionProps = {
  busyKey: string | null;
  otherFields: OtherSettingsFields;
  onFieldChange: (key: OtherSettingsFieldKey, value: string) => void;
  onSaveAbout: () => void;
  onSaveBrand: () => void;
};

export function SettingsOtherSection({
  busyKey,
  otherFields,
  onFieldChange,
  onSaveAbout,
  onSaveBrand,
}: SettingsOtherSectionProps) {
  return (
    <div className="space-y-6">
      <div className="grid gap-6 xl:grid-cols-[1fr_1fr]">
        <AppCard
          title="公告与品牌信息"
          description="用于控制首页公告、系统名称、默认首页链接和页脚展示。"
          action={
            <PrimaryButton
              type="button"
              onClick={onSaveBrand}
              disabled={busyKey === 'other-brand'}
            >
              {busyKey === 'other-brand' ? '保存中...' : '保存基础信息'}
            </PrimaryButton>
          }
        >
          <div className="space-y-5">
            <ResourceField label="系统名称">
              <ResourceInput
                value={otherFields.SystemName}
                onChange={(event) =>
                  onFieldChange('SystemName', event.target.value)
                }
                placeholder="DuShengCDN"
              />
            </ResourceField>
            <ResourceField label="首页链接">
              <ResourceInput
                value={otherFields.HomePageLink}
                onChange={(event) =>
                  onFieldChange('HomePageLink', event.target.value)
                }
                placeholder="https://example.com"
              />
            </ResourceField>
            <ResourceField label="公告">
              <ResourceTextarea
                value={otherFields.Notice}
                onChange={(event) =>
                  onFieldChange('Notice', event.target.value)
                }
                placeholder="可在此编辑首页公告内容"
              />
            </ResourceField>
            <ResourceField label="页脚 HTML">
              <ResourceTextarea
                value={otherFields.Footer}
                onChange={(event) =>
                  onFieldChange('Footer', event.target.value)
                }
                placeholder="留空则使用默认页脚"
              />
            </ResourceField>
          </div>
        </AppCard>

        <AppCard
          title="关于页内容"
          description="支持 Markdown / HTML 内容编辑，保存后会同步到公开关于页。"
          action={
            <PrimaryButton
              type="button"
              onClick={onSaveAbout}
              disabled={busyKey === 'other-about'}
            >
              {busyKey === 'other-about' ? '保存中...' : '保存关于内容'}
            </PrimaryButton>
          }
        >
          <div className="space-y-5">
            <ResourceField
              label="关于内容"
              hint="支持 Markdown 和 HTML，保存后会同步到公开关于页。"
            >
              <ResourceTextarea
                value={otherFields.About}
                onChange={(event) => onFieldChange('About', event.target.value)}
                placeholder="在这里编辑关于 DuShengCDN 的介绍内容"
                className="min-h-48"
              />
            </ResourceField>
          </div>
        </AppCard>
      </div>
    </div>
  );
}
