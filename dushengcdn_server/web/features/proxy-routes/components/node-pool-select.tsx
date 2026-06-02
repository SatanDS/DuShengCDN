'use client';

import {
  ResourceInput,
  ResourceSelect,
} from '@/features/shared/components/resource-primitives';

export function buildNodePoolOptions(
  nodes: Array<{ pool_name?: string | null }>,
  currentValue?: string | null,
) {
  const options = new Set<string>(['default']);
  for (const node of nodes) {
    const poolName = node.pool_name?.trim();
    if (poolName) {
      options.add(poolName);
    }
  }

  const normalizedCurrentValue = currentValue?.trim();
  if (normalizedCurrentValue) {
    options.add(normalizedCurrentValue);
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
  const selectOptions =
    value.trim() && !options.includes(value.trim())
      ? [...options, value.trim()]
      : options;

  return (
    <div
      className={
        compact
          ? 'grid gap-2'
          : 'grid gap-3 md:grid-cols-[minmax(0,280px)_minmax(0,1fr)]'
      }
    >
      <ResourceSelect
        value={value}
        disabled={disabled}
        onChange={(event) => onChange(event.target.value)}
        onBlur={onBlur}
        aria-label={selectAriaLabel}
      >
        {selectOptions.map((option) => (
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
    </div>
  );
}
