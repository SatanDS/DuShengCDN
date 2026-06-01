# 系统架构

你会学到：DuShengCDN 的整体架构、Server、Agent、OpenResty 与管理端前端的职责边界，以及一次配置发布从管理端到节点生效的请求流。

DuShengCDN 由 Server、Agent、节点本地 OpenResty、DNS Worker 和管理端前端组成。Server 是控制面，Agent 是节点侧唯一受控落地入口，OpenResty 是实际数据面。自建权威 DNS 使用独立 DNS Worker 运行角色，用于逐次 DNS 查询实时执行 GSLB 调度；它不替代 Agent/OpenResty 数据面。

```text
Browser
  |
  | Management UI / API
  v
DuShengCDN Server (Gin + GORM + SQLite/PostgreSQL)
  |
  | Agent API / heartbeat / config pull
  v
DuShengCDN Agent
  |
  | write config / openresty -t / reload / rollback
  v
OpenResty binary
  |
  | reverse proxy
  v
Origin
```

自建权威 DNS 阶段增加入口调度链路：

```text
Recursive Resolver
  |
  | UDP/TCP 53
  v
DuShengCDN DNS Worker
  |
  | snapshot pull / heartbeat
  v
DuShengCDN Server
  |
  | metrics / node state
  v
Agent + OpenResty edge nodes
```

## 组件职责

| 组件 | 职责 |
| --- | --- |
| Server | 管理端 UI、管理 API、Agent API、配置渲染、版本发布、数据存储、聚合查询和 DNS Worker 探测目标下发 |
| Agent | 注册、心跳、同步、写入文件、校验、reload、失败回滚、自更新、轻量采集，以及受控的 DNS Worker UDP/TCP 53 可达性探测 |
| OpenResty | 接收真实流量，按 DuShengCDN 渲染的配置执行反向代理 |
| Frontend | 管理网站配置、源站、证书、节点、版本、用户、设置与观测页面 |
| DNS Worker | 权威 DNS 查询服务运行角色；从 Server 拉取只读调度快照，监听 UDP/TCP `53`，并按来源、地区、节点健康和负载实时返回 A/AAAA 答案 |

## Server

`dushengcdn_server` 是单体控制面：

* Gin 提供 HTTP 服务。
* GORM 访问 SQLite 或 PostgreSQL。
* 现有登录体系提供管理端 Session。
* 认证源与外部账号绑定支持 GitHub OAuth 和标准 OIDC。
* Go Server 托管 `dushengcdn_server/web` 静态构建产物。

Server 不直接 SSH 到节点，也不在线修改节点文件。它只保存控制面状态、生成完整配置版本，并通过 Agent API 让节点主动拉取。

## Agent

`dushengcdn_agent` 是 Go 单体程序：

* 单二进制运行在节点侧。
* 启动后读取或生成本地节点信息。
* 周期性 heartbeat，上报状态并获取激活版本摘要。
* 发现新版本后拉取配置、备份旧文件、写入新文件、校验并 reload。
* 应用失败时尝试恢复运行并回滚。

Agent 通过 `openresty_path` 指向的 OpenResty 二进制统一执行校验、reload、启动与重启；未配置时默认调用 `openresty`。Docker 部署时，Agent 镜像内置 OpenResty 二进制，仍走同一套二进制控制逻辑。

## Frontend

`dushengcdn_server/web` 是正式管理端前端：

* Next.js App Router。
* React 19。
* TypeScript。
* Tailwind CSS。
* TanStack Query 管理服务端状态。

前端静态导出后由 Go Server 托管。所有 API 请求应统一经过 `lib/api/`，并处理 `success/message/data` 响应结构。

## 数据与请求流

### 管理端请求流

```text
Browser -> Frontend -> /api/* -> controller -> service -> model -> database
```

管理端变更类接口使用 `POST`，只读接口使用 `GET`。成功与失败都返回清晰的 `message`。

### Agent 同步流

```text
Agent heartbeat -> Server 返回激活版本摘要
Agent 发现新版本 -> 拉取配置详情
Agent 写入主配置 / 路由配置 / 证书 / Lua 资源
Agent 执行 OpenResty 校验与 reload
Agent 上报应用结果
```

默认启用 WS 连接升级时，Agent 会先通过 HTTP heartbeat 获取设置，随后尝试连接 Agent WebSocket。WS 成功后，周期性状态上报改由 WS 承载；Server 发布或激活版本后会向已连接 Agent 广播激活版本摘要，使 Agent 立即进入既有同步流程。WS 断开或建立失败时，Agent 自动退回 HTTP heartbeat。

权威 DNS 启用后，Server 会在 Agent settings 中下发少量在线 DNS Worker 的公网探测目标。Agent 在节点本地对这些目标执行 UDP/TCP `53` DNS 查询探测，并在下一次 heartbeat 或 WebSocket status 中上报结果；Server 将最新结果保存到 `dns_worker_node_probes`，用于「权威 DNS」可用性面板展示多节点探测通过率和 RTT。

### 运行时指令流

```text
Browser -> Server 管理 API -> Agent WebSocket -> Agent -> OpenResty 本地缓存目录 / HTTP 预热请求
```

缓存清理与预热是运行时操作，不生成配置版本，也不修改历史快照。Server 只向网站绑定节点池内、在线且 OpenResty 健康的 Agent 下发结构化指令；Agent 仅执行受限的缓存目录清理和 HTTP/HTTPS 预热请求，不暴露远程 shell。

### 自动 DNS 调度流

```text
ProxyRoute DNS 设置 -> Server 选择默认节点池、GSLB 多节点池或 DDoS 临时防护目标 -> Cloudflare DNS A/AAAA 多记录同步
```

节点池、公网 IP 池、权重、调度开关和排空状态保存在 `nodes`。网站配置通过 `proxy_routes.node_pool` 绑定默认目标节点池；当启用 Cloudflare 自动 DNS 且记录内容交给系统自动选择时，Server 会按记录类型、节点健康状态、OpenResty 状态、节点池和调度模式选择一个或多个公网 IP，并同步到 Cloudflare。

`proxy_routes.node_pool` 与 `proxy_routes.gslb_policy` 不承担同一层职责。前者是网站默认承载池，用于未启用 GSLB 时的自动 DNS 选点、缓存清理/预热下发范围和 Cloudflare DDoS 防护期回退池；后者只在 DNS A/AAAA 选点时引用多个节点池形成 GSLB 策略，不生成按节点池拆分的 OpenResty 配置。

启用 `proxy_routes.gslb_enabled` 后，Server 改用 `proxy_routes.gslb_policy` 里的多节点池策略：先按来源 CIDR 匹配可用池，再按来源国家代码匹配可用池（Cloudflare DNS 同步模式默认使用全局上下文；实时来源主要由权威 DNS Worker 提供），最后按池权重、节点权重、最新 `node_metric_snapshots` 负载评分和阈值筛选目标。最近一次选择会写入 `gslb_scheduling_states`，在冷却期内旧目标仍健康时保持不变，避免 DNS 记录来回抖动。

当 DDoS 自动防护进入攻击期时，Cloudflare DNS 同步流会暂停 GSLB：选择 Cloudflare 提供方时临时回到默认节点池并强制橙云，选择自定义提供方时只返回指定清洗节点/IP 池。攻击期临时目标不会覆盖用户保存的正常记录内容；指标恢复后下一轮巡检重新按固定记录、默认节点池或 GSLB 策略同步。

### 权威 DNS 实时调度流

```text
DNS Query -> DNS Worker 匹配 Zone/站点 -> 读取本地调度快照 -> 复用 GSLB 选点 -> 返回 A/AAAA/静态记录
```

自建权威 DNS 模式下，DNS Worker 不在查询路径访问数据库，也不向节点发命令。它从 Server 拉取只读快照，快照包含启用 Zone、静态记录、网站域名、GSLB 策略、节点公网 IP、节点健康摘要和负载摘要。查询时优先使用 EDNS Client Subnet 识别用户来源，未提供时使用递归解析器 IP；来源会被解析成 `GSLBSourceContext`，用于先按来源 CIDR、再按国家代码匹配节点池。

DNS Worker 的防抖状态按 `route_id + record_type + source_scope` 保存，避免不同来源地区互相覆盖选择结果。查询聚合按窗口批量上报 Server，用于展示 QPS、rcode、返回目标和错误原因；默认不保存完整原始查询日志。

### 反向代理流

```text
Client -> OpenResty server block -> named upstream -> Origin
```

网站配置是反向代理聚合边界。一条网站配置可绑定多个域名，并共享站点级流量限制、反向代理和缓存配置。

## 核心对象

当前有效实体包括：

* `proxy_routes`
* `gslb_scheduling_states`
* `dns_zones`
* `dns_records`
* `dns_workers`
* `dns_query_rollups`
* `dns_worker_node_probes`
* `origins`
* `config_versions`
* `nodes`
* `auth_sources`
* `external_accounts`
* `node_system_profiles`
* `apply_logs`
* `tls_certificates`
* `managed_domains`
* `node_request_reports`
* `node_access_logs`
* `node_metric_snapshots`
* `traffic_analytics_rollups`
* `node_health_events`

## 关键设计决策

| 决策 | 原因 |
| --- | --- |
| 完整配置版本，而不是在线 patch | 让预览、激活、历史和回滚有稳定边界 |
| Agent 主动拉取 | Server 不需要 SSH 权限，也不暴露远程命令入口 |
| 全局单激活版本 | 降低 MVP 复杂度，保证所有节点默认一致 |
| 网站配置聚合多域名 | 支持一个业务站点共享站点级策略，同时允许按域名绑定证书 |
| 节点池只影响调度与运行时操作 | 保持配置发布仍是全局完整版本，避免引入按节点分组版本模型 |
| 权威 DNS 使用只读快照 | 查询路径不能依赖数据库或外部 API，Server 短暂不可用时仍可使用最后一次有效快照回答 |
| 观测数据服务端聚合 | 避免前端临时统计造成口径不一致 |

## 贡献者阅读建议

如果要修改架构相关代码，先阅读：

1. [产品边界](./index.md)
2. [发布模型](./release-model.md)
3. [开发约束](./development.md)
4. [仓库结构](../reference/repository.md)
