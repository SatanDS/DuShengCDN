# 自建权威 DNS 与 GSLB 调度规划

你会学到：为什么需要自建权威 DNS、它与现有 Cloudflare 自动 DNS 的区别、权威 DNS 服务如何复用 GSLB 策略，以及后续应如何继续增强。

本文是自建权威 DNS 与实时 GSLB 的设计基线。它把“后台同步 DNS 记录”升级为“逐次 DNS 查询实时调度”的目标纳入产品边界，但不改变现有 OpenResty 配置发布、Agent 同步和回滚模型。

当前实现状态：Server 控制面已经具备 Zone、静态记录、DNS Worker Token、Worker 心跳/聚合上报、只读调度快照 API，以及 `proxy_routes.dns_provider_mode` / `dns_zone_id_ref` 和 `gslb_scheduling_states.scope_key` 数据基础。DNS Worker MVP 已提供独立 `cmd/dns-worker` 运行入口，可监听 UDP/TCP `53`、拉取并缓存只读快照、回答静态 DNS 记录，并对权威模式站点的 `A`/`AAAA` 查询实时执行 GSLB 选点。管理端已经接入 DNS 查询聚合观测，可查看查询量、返回码、Worker/Zone/站点维度和返回目标分布。

## 目标能力

目标链路：

```text
用户访问域名
  ↓
递归 DNS 查询 DuShengCDN 权威 DNS
  ↓
权威 DNS 根据来源、地区、节点健康、节点负载和节点池权重返回边缘 IP
  ↓
用户连接选中的 OpenResty 边缘节点
  ↓
边缘节点回源到网站配置里的源站
```

需要解决的能力：

* 每次 DNS 查询都能按来源上下文实时选点，而不是等待后台巡检改 Cloudflare 记录。
* 支持 HK、EU 等多个节点池按权重、国家代码、节点负载和健康状态调度。
* 复用现有 `proxy_routes.gslb_policy`、`nodes.public_ips`、节点权重、排空状态和观测快照。
* Cloudflare 自动 DNS 继续保留，作为托管 DNS 同步模式；自建权威 DNS 作为新的实时调度模式。
* DNS 查询服务必须能独立部署多实例，避免管理端故障直接导致权威 DNS 不可用。

非目标：

* 不把 DuShengCDN 变成通用 DNS 托管平台。
* 不在第一阶段实现 DNSSEC、复杂 Zone 模板、全量记录编辑器或 Anycast 网络。
* 不把 DNS 查询日志扩展为长期原始日志平台，只保留受控聚合指标。
* 不通过 DNS 层拆分 OpenResty 配置版本，所有反代节点仍默认消费同一份激活版本。

## 与现有 Cloudflare 模式的区别

| 项目 | Cloudflare 自动 DNS | 自建权威 DNS |
| --- | --- | --- |
| 生效方式 | Server 周期性重算并同步 A/AAAA 记录 | 每次 DNS 查询实时计算答案 |
| 来源识别 | 当前只能使用全局默认上下文 | 可按递归解析器 IP、EDNS Client Subnet 和 GeoIP 识别 |
| 调度粒度 | 受巡检周期、TTL 和递归缓存影响 | 受权威 DNS TTL 和递归缓存影响，响应更直接 |
| Cloudflare 橙云 | 支持 | 不支持，除非域名仍托管在 Cloudflare |
| 部署要求 | 需要 Cloudflare API Token | 需要将域名 NS 委派到 DuShengCDN DNS 节点 |
| 高可用要求 | Cloudflare 承担权威 DNS 可用性 | 用户至少部署 2 个 DuShengCDN DNS 节点 |

## 组件形态

新增一个轻量权威 DNS 运行角色，建议命名为 `dushengcdn_dns`。它可以先作为 `dushengcdn_server` 的内置子进程启动，后续拆成独立二进制；无论部署形态如何，职责边界保持一致。

| 组件 | 职责 |
| --- | --- |
| Server 控制面 | 保存网站、节点、GSLB 策略、DNS Zone、DNS Worker、静态记录和权限配置 |
| DNS Worker | 监听 UDP/TCP 53，加载 Server 下发的只读调度快照，实时回答权威 DNS 查询 |
| Agent/OpenResty | 继续负责反向代理配置应用、运行时健康和观测上报 |
| Frontend | 提供 Zone、NS、调度模式、GSLB 策略、DNS Worker 状态和查询聚合视图 |

推荐生产拓扑：

```text
Registrar
  |
  | NS: ns1.example.net, ns2.example.net
  v
DuShengCDN DNS Worker x 2+
  |
  | snapshot pull / heartbeat
  v
DuShengCDN Server
  |
  | agent heartbeat / metrics
  v
DuShengCDN Agent + OpenResty edge nodes
```

DNS Worker 不应该在查询路径里访问数据库，也不应该在每个查询里调用外部 HTTP GeoIP API。查询路径只使用本地内存快照、本地 GeoIP 数据库和短 TTL 缓存。Server 不可用时，DNS Worker 继续使用最后一次有效快照服务，直到快照超过最大有效期。

## 查询调度流程

权威 DNS 收到查询后：

1. 解析 `qname`、`qtype`、查询来源 IP 和 EDNS Client Subnet。
2. 匹配托管 Zone 与网站配置域名。
3. 对 `A` / `AAAA` 查询：
   * 若站点启用 GSLB，读取 `proxy_routes.gslb_policy`。
   * 根据来源 IP 或 ECS 解析国家代码，生成 `GSLBSourceContext`。
   * 按国家代码匹配节点池；未命中时回退到全部启用池。
   * 按节点健康、OpenResty 状态、排空状态、调度开关和公网 IP 类型筛选候选。
   * 按池权重、节点权重、连接数、CPU、内存和策略模式排序。
   * 按 `target_count` 返回一个或多个 IP。
4. 对 `CNAME`、`TXT`、`NS`、`SOA` 等查询，返回 Zone 静态记录或系统生成记录。
5. 将本次选择写入本地内存防抖状态；按聚合窗口上报查询量、命中策略、返回目标和错误原因。

防抖状态不应只按网站保存。自建权威 DNS 模式下应按调度作用域保存：

```text
route_id + record_type + source_scope
```

`source_scope` 第一阶段建议使用国家代码，例如 `country:HK`、`country:DE`、`global`。这样 HK 来源和 EU 来源可以各自保持冷却窗口，避免互相覆盖。

## 数据模型规划

当前已经新增或扩展以下对象；`dns_zone_assignments` 暂不落库，先按“全部 Worker 服务全部启用 Zone”处理。

| 对象 | 作用 |
| --- | --- |
| `dns_zones` | 托管权威 Zone，例如 `example.com`，保存 SOA、NS、默认 TTL、启用状态和序列号 |
| `dns_records` | Zone 内静态记录，至少支持 `A`、`AAAA`、`CNAME`、`TXT`、`MX`、`NS`、`SOA` |
| `dns_workers` | 权威 DNS Worker 身份、Token、公网地址、版本、心跳和最近快照状态 |
| `dns_zone_assignments` | 暂不落库，当前简化为全部 Worker 服务全部启用 Zone |
| `dns_query_rollups` | DNS 查询聚合指标，按时间窗口、Zone、站点、qtype、rcode 和 Worker 统计 |
| `gslb_scheduling_states` | 扩展 `record_type` 与 `scope_key` 维度，保存按来源作用域的最近选择和防抖状态 |

`proxy_routes` 已补充 DNS 调度模式字段：

| 字段 | 作用 |
| --- | --- |
| `dns_provider_mode` | `cloudflare` 或 `authoritative`，决定是后台同步托管 DNS 还是自建权威 DNS 实时回答 |
| `dns_zone_id_ref` | 关联 `dns_zones`，用于把网站域名纳入自建权威 Zone |
最终以 `dns_provider_mode` 为准，避免长期维护两套开关。

## API 与前端规划

已实现的控制面 API：

| API | 作用 |
| --- | --- |
| `GET /api/dns-zones/` | 列出托管 Zone、NS、状态和最近发布序列号 |
| `POST /api/dns-zones/` | 创建 Zone，校验域名和 SOA 参数 |
| `POST /api/dns-zones/{id}/update` | 更新 Zone、默认 TTL、SOA、NS 与启用状态 |
| `POST /api/dns-zones/{id}/records` | 新增或更新静态 DNS 记录 |
| `POST /api/dns-records/{id}/update` | 更新静态 DNS 记录 |
| `POST /api/dns-records/{id}/delete` | 删除静态 DNS 记录 |
| `GET /api/dns-workers/` | 查看 DNS Worker 在线状态、监听地址、版本和快照时间 |
| `GET /api/dns-workers/observability` | 查看 DNS Worker 查询聚合、返回码、Worker/Zone/站点维度和返回目标分布 |
| `POST /api/dns-workers/` | 创建 DNS Worker Token |
| `POST /api/dns-workers/{id}/delete` | 删除 DNS Worker |
| `GET /api/dns-snapshot` | DNS Worker 拉取只读调度快照，需 Worker Token |
| `POST /api/dns-worker-heartbeat` | DNS Worker 上报状态、快照版本和聚合指标 |

已落地的前端入口：

* 左侧「权威 DNS」主菜单作为独立基础设施资源入口。
* 「权威 DNS」页面支持 Zone、NS、SOA、静态记录和 DNS Worker Token 管理，并展示 Worker 在线状态、版本、最近心跳、快照时间、查询量、返回码、返回目标和动态站点分布。
* 网站配置的「自动 DNS」分区支持 `Cloudflare 同步` 和 `自建权威 DNS` 两种模式。
* GSLB 节点池策略继续放在网站配置里，权威 DNS 只负责实时执行策略。

仍待增强的前端能力：

* Zone 委派检查、Glue 提示和迁移向导。
* DNS Worker 查询趋势图、SERVFAIL/NXDOMAIN 比例趋势和多 Worker 快照一致性告警。

## DNS 协议行为

第一阶段支持：

* UDP 53 和 TCP 53。
* `A`、`AAAA`、`CNAME`、`TXT`、`MX`、`NS`、`SOA`。
* `NOERROR`、`NXDOMAIN`、`NODATA`、`SERVFAIL`。
* EDNS Client Subnet 可选开启，默认优先使用 ECS，未提供时使用递归解析器 IP。
* `AXFR` 默认拒绝，后续如需支持必须配置来源 IP 白名单和 TSIG。

以上 DNS 协议行为已在 DNS Worker MVP 中落地。当前实现默认拒绝 `AXFR`/`IXFR`，支持 `A`、`AAAA`、`CNAME`、`TXT`、`MX`、`NS`、`SOA`，并通过心跳批量上报聚合查询指标。

TTL 规则：

* Cloudflare 模式继续沿用当前 `0/1` 自动 TTL、`2-29` 提升到 `30`、最高 `86400` 的规则。
* 权威 DNS 模式中，`0/1` 应映射为 `AuthoritativeDNSDefaultTTL`，默认建议 `30` 秒。
* GSLB 记录建议默认 TTL `30` 到 `60` 秒；静态记录可使用 Zone 默认 TTL。

## 高可用与失效策略

权威 DNS 是入口能力，必须按高可用设计：

* 生产至少部署两个 DNS Worker，并在注册商配置两个 NS。
* DNS Worker 保存最后一次有效快照和快照签名；Server 暂时不可用时继续服务。
* 快照超过 `AuthoritativeDNSSnapshotMaxAge` 后，动态 GSLB 记录返回 `SERVFAIL`，静态 SOA/NS 可继续返回。
* DNS Worker 不直接修改数据库，不在查询路径里写入状态。
* 防抖状态保存在 Worker 内存，Worker 重启后可从快照和上报状态恢复最近一次全局选择，但不保证逐来源状态完全恢复。
* 查询聚合按窗口批量上报，失败时本地缓冲，避免每次查询写库。

## 安全约束

* DNS Worker 使用专属 Token，不复用 Agent Token 或管理员 Session。
* Worker 拉取快照和上报 heartbeat 必须走 HTTPS。
* 快照不包含管理端敏感信息、Cloudflare Token、Session Secret 或用户密码。
* Zone Transfer 默认禁用。
* 查询日志默认聚合，不保存完整来源 IP；如未来提供原始日志，必须有采样、脱敏和保留天数。
* DNS Worker 应具备基础 QPS 限制和响应大小控制，避免被放大攻击滥用。

## 实施阶段

### 阶段 1：设计与数据基础（已落地 Server 控制面）

* 已新增 `dns_zones`、`dns_records`、`dns_workers`、`dns_query_rollups` 迁移。
* 已扩展 `gslb_scheduling_states` 的 `scope_key` 维度。
* 已为 `proxy_routes` 增加 `dns_provider_mode` 和 `dns_zone_id_ref`。
* 已提供 Zone、Worker、静态记录、Worker 心跳/聚合上报和只读快照 API。
* 已提供前端「权威 DNS」入口和网站自动 DNS 模式选择。

### 阶段 2：权威 DNS Worker MVP（已落地）

* 使用 `github.com/miekg/dns` 实现 UDP/TCP 53 监听。
* DNS Worker 从 Server 拉取只读调度快照，写入本地缓存并在内存中构建 Zone、记录和站点索引。
* 支持 `SOA`、`NS`、静态记录和网站 `A`/`AAAA` 动态 GSLB 回答。
* 在 Worker 内复用同等 GSLB 策略语义，按节点池、池权重、节点权重、OpenResty 健康、排空、调度开关和负载阈值选点。
* 支持 EDNS Client Subnet 来源识别；配置本地 MaxMind Country MMDB 后可按国家代码命中节点池，否则回退到 `global` 作用域。
* 防抖状态按 `route_id + record_type + source_scope` 保存在 Worker 内存中。
* 聚合查询指标按窗口批量 heartbeat 上报；上报失败时保留在内存中等待下次重试。
* 已增加单元测试覆盖 DNS 响应、NXDOMAIN、NODATA、GSLB 选点、ECS 作用域、TTL、快照降级、AXFR 拒绝和聚合上报。

### 阶段 3：观测、联调与迁移体验

* DNS Worker 已上报查询聚合、返回目标分布和错误码分布。
* 管理端已展示 DNS Worker 状态、查询量、返回码、Worker/Zone/站点维度和返回目标分布。
* 管理端仍需补 Zone 委派检查、查询趋势图和多 Worker 快照一致性告警。
* 提供从 Cloudflare 同步模式切换到自建权威 DNS 模式的向导。
* 文档补充注册商 NS、Glue、端口、防火墙和回滚步骤。

### 阶段 4：增强能力

* 可选 TSIG/AXFR 白名单。
* 可选 DNSSEC 设计。
* 更细粒度来源作用域，例如 ASN、省份或自定义 CIDR。
* DNS Worker 多地部署的健康探测和快照版本一致性告警。

## 验收标准

第一阶段完成时：

* 设计文档、部署文档、配置参考和使用文档说明一致。
* 数据模型迁移具备升级校验和回归测试。
* 管理端可以创建 Zone、Worker Token，并把网站绑定到权威 DNS 模式。

第二阶段完成时：

* `dig @worker-ip example.com A` 能按 GSLB 策略返回在线节点公网 IP。
* HK/EU 等国家来源能命中不同节点池；无来源信息时走全局策略。
* 节点离线、排空、OpenResty 不健康或超过负载阈值时不会被返回。
* Server 不可用时，DNS Worker 可使用最后一次有效快照继续回答。
* 单元测试覆盖 DNS 协议响应、GSLB 调度、TTL、防抖和快照失效。

第三阶段完成时：

* 管理端能展示 DNS Worker 在线状态、查询量、错误码、返回目标和委派检查。
* 文档能指导用户从 Cloudflare 模式迁移到自建权威 DNS，并说明回滚方式。
* 生产部署建议明确要求至少两个 DNS Worker 和 UDP/TCP 53 防火墙放行。
