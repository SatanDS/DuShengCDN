'use client';

import {
  ResourceInput,
  ResourceSelect,
} from '@/features/shared/components/resource-primitives';

export function buildNodePoolOptions(
  nodes: Array<{ pool_name?: string | null }>,
) {
  const options = new Set<string>(['default']);
  for (const node of nodes) {
    const poolName = node.pool_name?.trim();
    if (poolName) {
      options.add(poolName);
    }
  }

  return Array.from(options).sort((a, b) => {
    if (a === 'default') {
      return -1;
    }
    if (b === 'default') {
      return 1;
    }
    return a.localeCompare(b);
  });
}

export function getNodesForPool<T extends { pool_name?: string | null }>(
  nodes: T[],
  poolName: string,
) {
  const normalizedPoolName = poolName.trim() || 'default';
  return nodes.filter(
    (node) => (node.pool_name?.trim() || 'default') === normalizedPoolName,
  );
}

export function formatNodeName(
  node: { name?: string | null; node_id?: string | null; ip?: string | null },
) {
  return node.name?.trim() || node.node_id?.trim() || node.ip?.trim() || '未命名节点';
}

export function NodePoolSelect({
  value,
  options,
  disabled,
  compact = false,
  onChange,
  onBlur,
  name,
  selectAriaLabel = '节点池选择',
  inputAriaLabel = '节点池名称',
}: {
  value: string;
  options: string[];
  disabled?: boolean;
  compact?: boolean;
  name?: string;
  selectAriaLabel?: string;
  inputAriaLabel?: string;
  onChange: (value: string) => void;
  onBlur?: () => void;
}) {
  const normalizedValue = value.trim();
  const hasUnknownValue =
    normalizedValue !== '' && !options.includes(normalizedValue);
  const selectValue = hasUnknownValue ? '' : value;

  return (
    <div
      className={
        compact
          ? 'grid gap-2'
          : 'grid gap-3 md:grid-cols-[minmax(0,280px)_minmax(0,1fr)]'
      }
    >
      <ResourceSelect
        value={selectValue}
        disabled={disabled}
        onChange={(event) => onChange(event.target.value)}
        onBlur={onBlur}
        aria-label={selectAriaLabel}
      >
        {selectValue === '' ? (
          <option value="" disabled>
            请选择现有节点池
          </option>
        ) : null}
        {options.map((option) => (
          <option key={option} value={option}>
            {option}
          </option>
        ))}
      </ResourceSelect>
      <ResourceInput
        name={name}
        value={value}
        disabled={disabled}
        onChange={(event) => onChange(event.target.value)}
        onBlur={onBlur}
        placeholder="default"
        aria-label={inputAriaLabel}
      />
      {hasUnknownValue ? (
        <p className="text-xs text-[var(--status-warning-foreground)] md:col-span-2">
          当前填写的“{normalizedValue}”不在现有节点池里，请从下拉选择或先到边缘节点 / IP 池创建对应节点池。
        </p>
      ) : null}
    </div>
  );
}
