import type { HTMLAttributes, ReactNode } from 'react';

import { cn } from '@/lib/utils/cn';

interface AppCardProps extends HTMLAttributes<HTMLDivElement> {
  title?: string;
  description?: string;
  action?: ReactNode;
}

export function AppCard({
  title,
  description,
  action,
  className,
  children,
  ...props
}: AppCardProps) {
  return (
    <section
      className={cn(
        'dashboard-liquid-card rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[var(--surface-card)]/90 shadow-[var(--shadow-soft)] backdrop-blur',
        className,
      )}
      {...props}
    >
      {(title || description || action) && (
        <header className='flex flex-col gap-3 border-b border-[var(--border-default)] px-4 py-4 md:flex-row md:items-start md:justify-between md:px-6 md:py-5'>
          <div className='min-w-0 space-y-1'>
            {title ? <h2 className='text-lg font-semibold text-[var(--foreground-primary)]'>{title}</h2> : null}
            {description ? (
              <p className='text-sm leading-6 text-[var(--foreground-secondary)]'>{description}</p>
            ) : null}
          </div>
          {action ? <div className='shrink-0 max-sm:w-full'>{action}</div> : null}
        </header>
      )}
      <div className='px-4 py-4 md:px-6 md:py-5'>{children}</div>
    </section>
  );
}
