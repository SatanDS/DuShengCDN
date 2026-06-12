'use client';

export function InfoTile({
  label,
  value,
  helper,
}: {
  label: string;
  value: string | number;
  helper?: string;
}) {
  return (
    <div className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-panel)] px-4 py-3">
      <p className="text-xs tracking-[0.18em] text-[var(--foreground-muted)] uppercase">
        {label}
      </p>
      <p className="mt-2 text-sm break-all text-[var(--foreground-primary)]">
        {value}
      </p>
      {helper ? (
        <p className="mt-1 text-xs text-[var(--foreground-secondary)]">
          {helper}
        </p>
      ) : null}
    </div>
  );
}
