'use client';

import {
  createContext,
  type ReactNode,
  useCallback,
  useContext,
  useMemo,
  useRef,
  useState,
} from 'react';

import { AppModal } from '@/components/ui/app-modal';
import {
  DangerButton,
  PrimaryButton,
  SecondaryButton,
} from '@/features/shared/components/resource-primitives';

type ConfirmTone = 'default' | 'danger';

export interface ConfirmDialogOptions {
  title: string;
  message?: ReactNode;
  confirmLabel?: string;
  cancelLabel?: string;
  tone?: ConfirmTone;
}

interface ConfirmDialogState extends ConfirmDialogOptions {
  id: number;
}

type ConfirmDialogContextValue = (
  options: ConfirmDialogOptions,
) => Promise<boolean>;

const ConfirmDialogContext = createContext<ConfirmDialogContextValue | null>(
  null,
);

function ConfirmDialogMessage({ message }: { message?: ReactNode }) {
  if (!message) {
    return null;
  }

  if (typeof message === 'string') {
    return (
      <p className="break-words whitespace-pre-line text-sm leading-6 text-[var(--foreground-secondary)]">
        {message}
      </p>
    );
  }

  return <>{message}</>;
}

export function ConfirmDialogProvider({ children }: { children: ReactNode }) {
  const [dialog, setDialog] = useState<ConfirmDialogState | null>(null);
  const resolverRef = useRef<((confirmed: boolean) => void) | null>(null);
  const nextIdRef = useRef(1);

  const closeDialog = useCallback((confirmed: boolean) => {
    resolverRef.current?.(confirmed);
    resolverRef.current = null;
    setDialog(null);
  }, []);

  const confirm = useCallback<ConfirmDialogContextValue>((options) => {
    resolverRef.current?.(false);

    return new Promise<boolean>((resolve) => {
      resolverRef.current = resolve;
      setDialog({
        ...options,
        id: nextIdRef.current,
      });
      nextIdRef.current += 1;
    });
  }, []);

  const value = useMemo(() => confirm, [confirm]);
  const ConfirmButton = dialog?.tone === 'danger' ? DangerButton : PrimaryButton;

  return (
    <ConfirmDialogContext.Provider value={value}>
      {children}
      {dialog ? (
        <AppModal
          key={dialog.id}
          isOpen={true}
          title={dialog.title}
          size="sm"
          onClose={() => closeDialog(false)}
          footer={
            <div className="flex flex-wrap justify-end gap-3">
              <SecondaryButton type="button" onClick={() => closeDialog(false)}>
                {dialog.cancelLabel ?? '取消'}
              </SecondaryButton>
              <ConfirmButton
                type="button"
                autoFocus
                onClick={() => closeDialog(true)}
              >
                {dialog.confirmLabel ?? '确认'}
              </ConfirmButton>
            </div>
          }
        >
          <ConfirmDialogMessage message={dialog.message} />
        </AppModal>
      ) : null}
    </ConfirmDialogContext.Provider>
  );
}

export function useConfirmDialog() {
  const context = useContext(ConfirmDialogContext);

  if (!context) {
    throw new Error('useConfirmDialog must be used within ConfirmDialogProvider');
  }

  return context;
}
