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

const fallingLeaves = [
  {
    left: '6%',
    size: '3.8rem',
    driftX: '11rem',
    midX: '-2rem',
    endX: '5rem',
    rotate: '-24deg',
    duration: '18s',
    delay: '-12s',
    opacity: '0.68',
  },
  {
    left: '16%',
    size: '2.5rem',
    driftX: '-7rem',
    midX: '4rem',
    endX: '-10rem',
    rotate: '36deg',
    duration: '22s',
    delay: '-5s',
    opacity: '0.46',
  },
  {
    left: '27%',
    size: '4.5rem',
    driftX: '8rem',
    midX: '-5rem',
    endX: '-3rem',
    rotate: '14deg',
    duration: '26s',
    delay: '-18s',
    opacity: '0.58',
  },
  {
    left: '42%',
    size: '2.9rem',
    driftX: '-10rem',
    midX: '5rem',
    endX: '-4rem',
    rotate: '-12deg',
    duration: '19s',
    delay: '-7s',
    opacity: '0.5',
  },
  {
    left: '53%',
    size: '5.2rem',
    driftX: '12rem',
    midX: '-3rem',
    endX: '7rem',
    rotate: '28deg',
    duration: '24s',
    delay: '-16s',
    opacity: '0.56',
  },
  {
    left: '67%',
    size: '3.2rem',
    driftX: '-9rem',
    midX: '6rem',
    endX: '-12rem',
    rotate: '-38deg',
    duration: '21s',
    delay: '-10s',
    opacity: '0.48',
  },
  {
    left: '78%',
    size: '4rem',
    driftX: '7rem',
    midX: '-6rem',
    endX: '12rem',
    rotate: '8deg',
    duration: '27s',
    delay: '-21s',
    opacity: '0.52',
  },
  {
    left: '91%',
    size: '2.7rem',
    driftX: '-12rem',
    midX: '3rem',
    endX: '-5rem',
    rotate: '44deg',
    duration: '20s',
    delay: '-3s',
    opacity: '0.44',
  },
  {
    left: '2%',
    size: '2.1rem',
    driftX: '8rem',
    midX: '-3rem',
    endX: '10rem',
    rotate: '18deg',
    duration: '17s',
    delay: '-2s',
    opacity: '0.4',
  },
  {
    left: '11%',
    size: '3rem',
    driftX: '-6rem',
    midX: '6rem',
    endX: '-8rem',
    rotate: '-46deg',
    duration: '25s',
    delay: '-22s',
    opacity: '0.5',
  },
  {
    left: '22%',
    size: '2.2rem',
    driftX: '10rem',
    midX: '-4rem',
    endX: '2rem',
    rotate: '52deg',
    duration: '16s',
    delay: '-9s',
    opacity: '0.42',
  },
  {
    left: '34%',
    size: '3.4rem',
    driftX: '-8rem',
    midX: '7rem',
    endX: '-11rem',
    rotate: '-18deg',
    duration: '23s',
    delay: '-14s',
    opacity: '0.48',
  },
  {
    left: '47%',
    size: '2.4rem',
    driftX: '6rem',
    midX: '-7rem',
    endX: '9rem',
    rotate: '34deg',
    duration: '18s',
    delay: '-1s',
    opacity: '0.38',
  },
  {
    left: '59%',
    size: '3.7rem',
    driftX: '-11rem',
    midX: '4rem',
    endX: '-6rem',
    rotate: '-52deg',
    duration: '28s',
    delay: '-24s',
    opacity: '0.5',
  },
  {
    left: '72%',
    size: '2.6rem',
    driftX: '9rem',
    midX: '-5rem',
    endX: '13rem',
    rotate: '22deg',
    duration: '19s',
    delay: '-6s',
    opacity: '0.42',
  },
  {
    left: '84%',
    size: '3.3rem',
    driftX: '-7rem',
    midX: '8rem',
    endX: '-9rem',
    rotate: '-8deg',
    duration: '24s',
    delay: '-17s',
    opacity: '0.47',
  },
  {
    left: '96%',
    size: '2.2rem',
    driftX: '-14rem',
    midX: '2rem',
    endX: '-16rem',
    rotate: '58deg',
    duration: '21s',
    delay: '-13s',
    opacity: '0.36',
  },
  {
    left: '64%',
    size: '2rem',
    driftX: '5rem',
    midX: '-8rem',
    endX: '3rem',
    rotate: '-30deg',
    duration: '15s',
    delay: '-11s',
    opacity: '0.34',
  },
];

const cdnEntryPoints = [
  { label: '域名资产', detail: '证书 / 启用状态' },
  { label: '网站配置', detail: '域名 / 反代' },
  { label: '源站', detail: '回源地址复用' },
  { label: '节点和 IP 池', detail: '边缘节点编排' },
  { label: '本地自建解析', detail: 'DNS 响应端' },
  { label: '发布版本', detail: '配置快照' },
  { label: '观测计量', detail: '日志 / 带宽' },
];

const cdnSignals = [
  { value: 'TLS', label: '证书绑定状态', tone: 'pink' },
  { value: 'DNS', label: '解析与自动选 IP', tone: 'blue' },
  { value: 'P95', label: '带宽计量窗口', tone: 'violet' },
];

const cdnRails = [
  {
    title: '网站配置',
    meta: '域名 / HTTPS / 反向代理',
    copy: '集中维护域名、证书、HTTPS 跳转、源站、负载均衡、缓存、防护与认证策略。',
  },
  {
    title: '节点和 IP 池',
    meta: '边缘池 / 节点 / 公网 IP',
    copy: '用边缘池管理节点和公网 IP，为反向代理、智能解析和故障切换提供可选目标。',
  },
  {
    title: '发布与应用',
    meta: '配置版本 / 应用记录',
    copy: '发布前预览域名差异和渲染结果，发布后跟踪节点应用记录与配置校验。',
  },
];

const deliveryFlow = [
  {
    step: '01',
    title: '域名资产接入',
    copy: '先录入可复用域名资产，绑定默认证书，确认启用状态和后续站点归属。',
  },
  {
    step: '02',
    title: '站点策略编排',
    copy: '在网站配置里组织反代源站、缓存规则、流量限制、负载均衡和访问防护。',
  },
  {
    step: '03',
    title: '发布到边缘节点',
    copy: '生成配置版本并下发到 Agent 节点，通过应用记录确认每个节点是否生效。',
  },
  {
    step: '04',
    title: '观测计量回看',
    copy: '用访问明细、缓存命中、回源流量、状态码分布和带宽 P95 继续调优策略。',
  },
];

const cdnCapabilities = [
  {
    title: '反向代理与缓存',
    tag: 'Proxy',
    copy: '按站点维护源站、缓存规则、压缩、超时和代理缓冲参数，适配不同业务域名。',
  },
  {
    title: '智能解析',
    tag: 'DNS',
    copy: '支持 Cloudflare 同步和本地自建解析，结合节点池权重与在线状态自动选择 IP。',
  },
  {
    title: 'HTTPS 证书',
    tag: 'TLS',
    copy: '统一管理证书导入、申请、绑定和到期状态，让站点 HTTPS 配置和域名资产联动。',
  },
  {
    title: '访问观测',
    tag: 'Logs',
    copy: '聚合访问日志、节点流量、回源流量、缓存命中、TOP URL/IP 与地区分布。',
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

function RealMapleRain() {
  return (
    <div className="public-real-maple-rain" aria-hidden="true">
      {fallingLeaves.map((leaf) => (
        <span
          className="public-real-maple-leaf"
          key={`${leaf.left}-${leaf.delay}`}
          style={
            {
              '--fall-left': leaf.left,
              '--fall-size': leaf.size,
              '--fall-drift-x': leaf.driftX,
              '--fall-mid-x': leaf.midX,
              '--fall-end-x': leaf.endX,
              '--fall-rotate': leaf.rotate,
              '--fall-duration': leaf.duration,
              '--fall-delay': leaf.delay,
              '--fall-opacity': leaf.opacity,
            } as CSSProperties
          }
        />
      ))}
    </div>
  );
}

function PublicCdnShowcase() {
  return (
    <section
      className="public-scroll-showcase relative z-10 mx-auto w-full max-w-6xl"
      aria-labelledby="public-portal-showcase-title"
    >
      <div className="public-showcase-intro">
        <div className="public-showcase-kicker">
          <MapleMark className="h-6 w-6" aria-hidden="true" />
          <span>DuSheng CDN</span>
        </div>
        <div className="public-showcase-heading">
          <h2 id="public-portal-showcase-title">把 CDN 管理链路放到首屏之后</h2>
          <p>
            基于项目后台已经具备的域名资产、网站配置、源站、节点和 IP
            池、DNS、证书、发布版本与观测计量，向下滚动后直接展示 DuSheng CDN
            的核心工作流。
          </p>
        </div>
      </div>

      <div className="public-cdn-console">
        <div className="public-console-topline">
          <div>
            <span className="public-console-label">Console Preview</span>
            <h3>DuSheng CDN 管理视图</h3>
          </div>
          <div className="public-search-pill">搜索域名 / 节点 / 证书</div>
        </div>

        <div className="public-category-strip" aria-label="DuShengCDN 功能导航">
          {cdnEntryPoints.map((category) => (
            <div className="public-category-chip" key={category.label}>
              <strong>{category.label}</strong>
              <span>{category.detail}</span>
            </div>
          ))}
        </div>

        <div className="public-content-rack">
          <div className="public-feature-screen">
            <div className="public-feature-screen-leaf">
              <MapleMark className="h-16 w-16" aria-hidden="true" />
            </div>
            <div>
              <span>正在下发</span>
              <h4>站点配置同步到边缘节点</h4>
            </div>
          </div>

          <div className="public-rail-list">
            {cdnRails.map((rail) => (
              <article className="public-rail-item" key={rail.title}>
                <div>
                  <span>{rail.meta}</span>
                  <h4>{rail.title}</h4>
                </div>
                <p>{rail.copy}</p>
              </article>
            ))}
          </div>
        </div>

        <div className="public-signal-grid">
          {cdnSignals.map((signal) => (
            <div
              className={cn('public-signal-card', `is-${signal.tone}`)}
              key={signal.label}
            >
              <strong>{signal.value}</strong>
              <span>{signal.label}</span>
            </div>
          ))}
        </div>
      </div>

      <div className="public-scroll-band">
        <div className="public-scroll-heading">
          <span>Delivery Flow</span>
          <h3>从域名接入到边缘发布</h3>
        </div>
        <div className="public-flow-grid">
          {deliveryFlow.map((item) => (
            <article className="public-flow-card" key={item.step}>
              <span>{item.step}</span>
              <h4>{item.title}</h4>
              <p>{item.copy}</p>
            </article>
          ))}
        </div>
      </div>

      <div className="public-capability-layout">
        <div className="public-capability-copy">
          <span>DuSheng CDN</span>
          <h3>后台能力围绕 CDN 运维链路排布</h3>
          <p>
            这一屏不做纯宣传，而是把项目实际页面里的管理对象，落到 CDN
            后台每天需要处理的接入、发布、解析、证书、缓存和观测上。
          </p>
        </div>
        <div className="public-capability-grid">
          {cdnCapabilities.map((capability) => (
            <article className="public-capability-card" key={capability.title}>
              <span>{capability.tag}</span>
              <h4>{capability.title}</h4>
              <p>{capability.copy}</p>
            </article>
          ))}
        </div>
      </div>
    </section>
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
      <RealMapleRain />

      <header className="pointer-events-none fixed inset-0 z-20">
        {isLoginPage ? (
          <button
            type="button"
            className={cn(
              'public-login-trigger pointer-events-auto absolute top-5 right-5 sm:top-7 sm:right-7',
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
          className="pointer-events-auto absolute right-5 bottom-5 rounded-full border border-[var(--border-default)] bg-[var(--surface-panel)]/70 px-3 py-1.5 text-sm text-[var(--brand-primary)] shadow-[var(--shadow-soft)] backdrop-blur transition hover:border-[var(--border-strong)] hover:bg-[var(--surface-panel)] sm:right-7 sm:bottom-7"
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
            isLoginPage ? 'public-login-hero' : 'lg:items-start lg:text-left',
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

      {isLoginPage ? <PublicCdnShowcase /> : null}
    </div>
  );
}
