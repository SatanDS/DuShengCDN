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

const showPublicShowcase = false;

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

const cdnEntryPoints = [
  { label: '授权开通', detail: '站点 / 面板 / Agent' },
  { label: '域名接入', detail: '证书 / HTTPS / 回源' },
  { label: '节点编排', detail: '边缘池 / 公网 IP' },
  { label: '智能解析', detail: 'DNS / 自动选路' },
  { label: '缓存策略', detail: '规则 / 压缩 / 限流' },
  { label: '安全防护', detail: '访问控制 / 证书' },
  { label: '运营计量', detail: '日志 / 带宽 / P95' },
];

const cdnSignals = [
  { value: 'License', label: '授权后可独立部署运营', tone: 'pink' },
  { value: 'Edge', label: '节点池与域名策略统一下发', tone: 'blue' },
  { value: 'Metering', label: '带宽、日志、客户用量可核算', tone: 'violet' },
];

const cdnRails = [
  {
    title: '授权即交付完整 CDN 控制台',
    meta: 'License / Console / Branding',
    copy: '不是单个模板页面，而是一套可运营的 CDN 管理系统：控制台、节点 Agent、域名接入、证书和发布链路一起交付。',
  },
  {
    title: '面向客户售卖 CDN 功能',
    meta: 'Tenant / Plan / Metering',
    copy: '授权方可以用自己的品牌对外提供域名加速、反向代理、HTTPS、智能解析、缓存和流量计量能力。',
  },
  {
    title: '从接入到发布有完整闭环',
    meta: 'Deploy / Observe / Iterate',
    copy: '配置版本、节点应用记录、访问日志、回源流量和证书状态串起来，方便后续运维、核算和扩容。',
  },
];

const deliveryFlow = [
  {
    step: '01',
    title: '授权与环境初始化',
    copy: '确认授权范围、部署方式和品牌信息，初始化控制台、后端服务、数据库与节点通信基础配置。',
  },
  {
    step: '02',
    title: '节点池与域名接入',
    copy: '接入边缘节点、公网 IP、源站和业务域名，把 TLS 证书、反代策略与智能解析绑定到同一套资产模型。',
  },
  {
    step: '03',
    title: '策略发布与客户开通',
    copy: '按客户或业务创建站点配置，生成版本后下发到 Agent 节点，让域名、缓存、安全和回源策略同时生效。',
  },
  {
    step: '04',
    title: '运营计量与续费',
    copy: '用带宽、访问日志、缓存命中、节点流量和证书状态做运营看板，为套餐、续费和扩容提供依据。',
  },
];

const cdnCapabilities = [
  {
    title: '反向代理与缓存策略',
    tag: 'Proxy',
    copy: '按站点维护源站、缓存规则、压缩、超时、代理缓冲、回源行为和访问限制，适配不同客户域名。',
  },
  {
    title: '智能解析与节点选路',
    tag: 'DNS',
    copy: '支持 Cloudflare 同步和本地自建解析，结合节点池权重、在线状态和 IP 池做自动选路。',
  },
  {
    title: 'HTTPS 证书生命周期',
    tag: 'TLS',
    copy: '统一管理证书导入、申请、绑定、默认配置和到期状态，让授权客户的 HTTPS 接入更可控。',
  },
  {
    title: '运营观测与计量',
    tag: 'Ops',
    copy: '聚合访问日志、节点流量、回源流量、缓存命中、TOP URL/IP、地区分布和 P95 带宽窗口。',
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
          <h2 id="public-portal-showcase-title">可授权运营的 CDN 站点能力</h2>
          <p>
            面向需要自有品牌 CDN
            面板、节点调度、域名加速和运营计量的团队，提供可部署、可授权、可继续扩展的完整站点能力。
          </p>
        </div>
      </div>

      <div className="public-cdn-console">
        <div className="public-console-topline">
          <div>
            <span className="public-console-label">License Console</span>
            <h3>授权、开通、计量，放在同一条运营链路里</h3>
          </div>
          <div className="public-search-pill">
            授权码 / 域名 / 节点 / 客户套餐
          </div>
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
            <div className="public-edge-map" aria-hidden="true">
              <span className="public-edge-route is-primary" />
              <span className="public-edge-route is-secondary" />
              <span className="public-edge-route is-tertiary" />
              <span className="public-edge-node is-origin" />
              <span className="public-edge-node is-node-a" />
              <span className="public-edge-node is-node-b" />
              <span className="public-edge-node is-node-c" />
              <span className="public-edge-node is-client" />
              <span className="public-edge-packet is-one" />
              <span className="public-edge-packet is-two" />
              <span className="public-edge-packet is-three" />
            </div>
            <div>
              <span>授权运行中</span>
              <h4>域名、证书、缓存和节点策略同步到边缘网络</h4>
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
          <span>License Delivery</span>
          <h3>从授权部署到客户续费</h3>
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
          <h3>卖的是能落地运营的 CDN 系统能力</h3>
          <p>
            页面展示不再只像功能清单，而是把授权方真正关心的“能不能部署、能不能开客户、能不能计量、能不能持续运维”放到前台。
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
          <h1 className="public-hero-title mt-6 max-w-3xl text-3xl font-semibold tracking-normal text-balance text-[var(--foreground-primary)] sm:mt-8 sm:text-5xl lg:text-6xl">
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

      {isLoginPage && showPublicShowcase ? <PublicCdnShowcase /> : null}
    </div>
  );
}
