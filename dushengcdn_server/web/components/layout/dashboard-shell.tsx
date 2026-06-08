import type { ReactNode } from 'react';

import { DashboardSidebar } from '@/components/layout/dashboard-sidebar';
import { DashboardTopbar } from '@/components/layout/dashboard-topbar';

interface DashboardShellProps {
  children: ReactNode;
}

export function DashboardShell({ children }: DashboardShellProps) {
  return (
    <div className="dashboard-portal relative flex min-h-screen overflow-x-hidden bg-transparent text-[var(--foreground-primary)]">
      <div
        className="dashboard-maple-haze pointer-events-none fixed inset-0 z-0"
        aria-hidden="true"
      />
      <DashboardSidebar />
      <div className="dashboard-shell-content relative z-10 flex min-w-0 flex-1 flex-col min-[1000px]:pl-4">
        <DashboardTopbar />
        <main className="dashboard-main flex-1 px-4 py-6 md:px-8 md:py-8">
          {children}
        </main>
      </div>
    </div>
  );
}
