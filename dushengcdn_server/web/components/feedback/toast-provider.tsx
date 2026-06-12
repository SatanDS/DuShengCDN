'use client';

import {
  AlertCircle,
  CheckCircle2,
  Info,
  X,
  type LucideIcon,
} from 'lucide-react';
import {
  createContext,
  type ReactNode,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
} from 'react';

import { cn } from '@/lib/utils/cn';

export type ToastTone = 'info' | 'success' | 'danger';

export type FeedbackState = {
  tone: ToastTone;
  message: string;
};

export interface ToastInput {
  tone?: ToastTone;
  message: string;
  detail?: string;
  durationMs?: number;
}

interface ToastState extends Required<Pick<ToastInput, 'tone' | 'message'>> {
  id: number;
  detail?: string;
  durationMs: number;
}

interface ToastContextValue {
  showToast: (toast: ToastInput) => void;
  dismissToast: () => void;
}

const ToastContext = createContext<ToastContextValue | null>(null);

const toastToneClasses = {
  info: 'border-[var(--status-info-border)] bg-[var(--status-info-soft)] text-[var(--status-info-foreground)]',
  success:
    'border-[var(--status-success-border)] bg-[var(--status-success-soft)] text-[var(--status-success-foreground)]',
  danger:
    'border-[var(--status-danger-border)] bg-[var(--status-danger-soft)] text-[var(--status-danger-foreground)]',
} satisfies Record<ToastTone, string>;

const toastToneIcons = {
  info: Info,
  success: CheckCircle2,
  danger: AlertCircle,
} satisfies Record<ToastTone, LucideIcon>;

function FloatingToast({
  toast,
  onClose,
}: {
  toast: ToastState;
  onClose: () => void;
}) {
  const Icon = toastToneIcons[toast.tone];

  return (
    <div className="pointer-events-none fixed top-24 right-4 z-[100] flex w-[min(calc(100vw-2rem),34rem)] justify-end md:right-8">
      <div
        className={cn(
          'pointer-events-auto w-fit min-w-[14rem] max-w-full rounded-2xl border px-4 py-3 text-sm leading-6 shadow-2xl shadow-black/30 backdrop-blur',
          toastToneClasses[toast.tone],
        )}
        role="status"
        aria-live={toast.tone === 'danger' ? 'assertive' : 'polite'}
      >
        <div className="flex items-start gap-3">
          <Icon className="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
          <div className="min-w-0 flex-1">
            <p className="break-words font-medium">{toast.message}</p>
            {toast.detail ? (
              <p className="mt-1 break-words text-xs leading-5 opacity-85">
                {toast.detail}
              </p>
            ) : null}
          </div>
          <button
            type="button"
            onClick={onClose}
            className="shrink-0 rounded-full p-1 opacity-70 transition hover:bg-white/10 hover:opacity-100"
            aria-label="关闭提示"
          >
            <X className="h-4 w-4" aria-hidden="true" />
          </button>
        </div>
      </div>
    </div>
  );
}

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toast, setToast] = useState<ToastState | null>(null);

  const showToast = useCallback((nextToast: ToastInput) => {
    setToast({
      id: Date.now(),
      tone: nextToast.tone ?? 'info',
      message: nextToast.message,
      detail: nextToast.detail,
      durationMs: nextToast.durationMs ?? 8000,
    });
  }, []);

  const dismissToast = useCallback(() => {
    setToast(null);
  }, []);

  useEffect(() => {
    if (!toast || toast.durationMs <= 0) {
      return;
    }

    const timer = window.setTimeout(() => {
      setToast((current) => (current?.id === toast.id ? null : current));
    }, toast.durationMs);

    return () => window.clearTimeout(timer);
  }, [toast]);

  const value = useMemo(
    () => ({ showToast, dismissToast }),
    [dismissToast, showToast],
  );

  return (
    <ToastContext.Provider value={value}>
      {children}
      {toast ? <FloatingToast toast={toast} onClose={dismissToast} /> : null}
    </ToastContext.Provider>
  );
}

export function useToast() {
  const context = useContext(ToastContext);

  if (!context) {
    throw new Error('useToast must be used within ToastProvider');
  }

  return context;
}

export function useToastFeedback<TFeedback extends ToastInput = ToastInput>() {
  const { showToast, dismissToast } = useToast();

  const setFeedback = useCallback(
    (feedback: TFeedback | null) => {
      if (feedback) {
        showToast(feedback);
      } else {
        dismissToast();
      }
    },
    [dismissToast, showToast],
  );

  return useMemo(
    () => ({
      feedback: null as TFeedback | null,
      setFeedback,
    }),
    [setFeedback],
  );
}
