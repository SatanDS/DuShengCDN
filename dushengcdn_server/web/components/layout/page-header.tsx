import type {ReactNode} from 'react';

interface PageHeaderProps {
    title: string;
    description?: string;
    action?: ReactNode;
}

export function PageHeader({title, description, action}: PageHeaderProps) {
    return (
        <div className='flex min-w-0 flex-col gap-4 lg:flex-row lg:items-end lg:justify-between'>
            <div className='min-w-0 space-y-3'>
                <p className='text-sm font-medium uppercase tracking-[0.24em] text-[var(--brand-primary)]'>DuShengCDN</p>
                <div className='min-w-0 space-y-2'>
                    <h1 className='break-words text-2xl font-semibold tracking-tight text-[var(--foreground-primary)] sm:text-3xl'>{title}</h1>
                    {description ? (
                        <p className='max-w-3xl break-words text-sm leading-7 text-[var(--foreground-secondary)]'>
                            {description}
                        </p>
                    ) : null}
                </div>
            </div>
            {action ? <div className='flex flex-wrap gap-3 max-sm:w-full'>{action}</div> : null}
        </div>
    );
}
