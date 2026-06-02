'use client';

import { useEffect, useMemo, useRef } from 'react';
import { useQueryClient } from '@tanstack/react-query';

import type { TlsCertificateItem } from '@/features/tls-certificates/types';

export function useCertificateApplicationRefresh(
  certificates: TlsCertificateItem[],
) {
  const queryClient = useQueryClient();
  const hadApplyingCertificateRef = useRef(false);
  const hasApplyingCertificate = useMemo(
    () =>
      certificates.some(
        (certificate) => certificate.apply_status === 'applying',
      ),
    [certificates],
  );

  useEffect(() => {
    if (!hasApplyingCertificate) {
      if (hadApplyingCertificateRef.current) {
        hadApplyingCertificateRef.current = false;
        void queryClient.invalidateQueries({ queryKey: ['managed-domains'] });
      }
      return;
    }

    hadApplyingCertificateRef.current = true;
    const timer = window.setInterval(() => {
      void Promise.all([
        queryClient.invalidateQueries({ queryKey: ['tls-certificates'] }),
        queryClient.invalidateQueries({ queryKey: ['managed-domains'] }),
      ]);
    }, 5000);

    return () => window.clearInterval(timer);
  }, [hasApplyingCertificate, queryClient]);

  return hasApplyingCertificate;
}
