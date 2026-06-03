'use client';

import Image from 'next/image';
import Link from 'next/link';
import { usePathname } from 'next/navigation';
import { useEffect, useState } from 'react';
import type { CSSProperties, ReactNode } from 'react';

import { MapleMark } from '@/components/brand/maple-mark';
import { cn } from '@/lib/utils/cn';

interface PublicShellProps {
  children: ReactNode;
}

const mapleLeaves = [
  {
    size: '9rem',
    left: '5%',
    top: '12%',
    rotate: '-22deg',
    driftX: '18px',
    driftY: '34px',
    duration: '18s',
    delay: '-6s',
    opacity: '0.12',
  },
  {
    size: '5.5rem',
    left: '22%',
    top: '70%',
    rotate: '24deg',
    driftX: '-24px',
    driftY: '-30px',
    duration: '22s',
    delay: '-10s',
    opacity: '0.16',
  },
  {
    size: '7rem',
    left: '63%',
    top: '8%',
    rotate: '18deg',
    driftX: '28px',
    driftY: '24px',
    duration: '20s',
    delay: '-4s',
    opacity: '0.1',
  },
  {
    size: '11rem',
    left: '78%',
    top: '62%',
    rotate: '-14deg',
    driftX: '-34px',
    driftY: '20px',
    duration: '24s',
    delay: '-13s',
    opacity: '0.13',
  },
  {
    size: '4.5rem',
    left: '49%',
    top: '82%',
    rotate: '38deg',
    driftX: '18px',
    driftY: '-26px',
    duration: '16s',
    delay: '-8s',
    opacity: '0.18',
  },
];

function MapleBackdrop() {
  return (
    <div className="public-maple-field" aria-hidden="true">
      {mapleLeaves.map((leaf) => (
        <MapleMark
          key={`${leaf.left}-${leaf.top}`}
          className="public-maple-leaf"
          style={
            {
              '--leaf-size': leaf.size,
              '--leaf-left': leaf.left,
              '--leaf-top': leaf.top,
              '--leaf-rotate': leaf.rotate,
              '--leaf-drift-x': leaf.driftX,
              '--leaf-drift-y': leaf.driftY,
              '--leaf-duration': leaf.duration,
              '--leaf-delay': leaf.delay,
              '--leaf-opacity': leaf.opacity,
            } as CSSProperties
          }
        />
      ))}
    </div>
  );
}

export function PublicShell({ children }: PublicShellProps) {
  const pathname = usePathname();
  const isLoginPage = pathname === '/login';
  const [isLoginOpen, setIsLoginOpen] = useState(!isLoginPage);

  useEffect(() => {
    setIsLoginOpen(!isLoginPage);
  }, [isLoginPage]);

  return (
    <div className="public-portal relative min-h-screen overflow-hidden px-4 py-6 text-[var(--foreground-primary)] sm:px-6 lg:px-8">
      <MapleBackdrop />

      <header className="pointer-events-none fixed inset-0 z-20">
        <Link
          href="/"
          className="pointer-events-auto absolute left-5 top-5 flex min-w-0 items-center gap-3 sm:left-7 sm:top-7"
          aria-label="DuShengCDN 首页"
        >
          <span className="flex h-11 w-11 shrink-0 items-center justify-center rounded-[var(--radius-md)] border border-[var(--border-default)] bg-[var(--surface-panel)]/78 shadow-[var(--shadow-soft)] backdrop-blur">
            <Image
              src="/satan-du-leaf.png"
              alt=""
              width={36}
              height={36}
              priority
              className="h-8 w-8 object-contain"
            />
          </span>
        </Link>

        {isLoginPage ? (
          <button
            type="button"
            className={cn(
              'public-login-trigger pointer-events-auto absolute right-5 top-5 sm:right-7 sm:top-7',
              isLoginOpen && 'is-hidden',
            )}
            onClick={() => setIsLoginOpen(true)}
            aria-expanded={isLoginOpen}
            aria-controls="public-login-panel"
          >
            登录
          </button>
        ) : null}

        <Link
          href="/about"
          className="pointer-events-auto absolute bottom-5 right-5 rounded-full border border-[var(--border-default)] bg-[var(--surface-panel)]/70 px-3 py-1.5 text-sm text-[var(--brand-primary)] shadow-[var(--shadow-soft)] backdrop-blur transition hover:border-[var(--border-strong)] hover:bg-[var(--surface-panel)] sm:bottom-7 sm:right-7"
        >
          关于
        </Link>
      </header>

      <main
        className={cn(
          'relative z-10 mx-auto w-full max-w-6xl py-16 lg:min-h-screen lg:py-20',
          isLoginPage
            ? ['public-login-stage', isLoginOpen && 'is-open']
            : 'grid items-start gap-8 lg:grid-cols-[1.06fr_0.94fr] lg:items-center lg:gap-10',
        )}
      >
        <section
          className={cn(
            'flex flex-col items-center text-center',
            isLoginPage
              ? 'public-login-hero'
              : 'lg:items-start lg:text-left',
          )}
        >
          <div className="public-hero-mark flex h-28 w-48 items-center justify-center sm:h-44 sm:w-72 lg:h-48 lg:w-80">
            <Image
              src="/satan-du-logo.png"
              alt="SatanDu 渡生"
              width={360}
              height={215}
              priority
              className="h-auto w-full object-contain"
            />
          </div>
          <h1 className="mt-6 max-w-3xl text-3xl font-semibold tracking-normal text-balance text-[var(--foreground-primary)] sm:mt-8 sm:text-5xl lg:text-6xl">
            Welcome to Dusheng CDN
          </h1>
          <div className="brand-gradient-bar mt-6 h-1 w-32 rounded-full" />
        </section>

        <section
          id="public-login-panel"
          className={cn(
            isLoginPage
              ? 'public-login-panel w-full max-w-xl'
              : 'w-full max-w-xl justify-self-center lg:justify-self-end',
          )}
          aria-hidden={isLoginPage && !isLoginOpen ? true : undefined}
        >
          {children}
        </section>
      </main>
    </div>
  );
}
