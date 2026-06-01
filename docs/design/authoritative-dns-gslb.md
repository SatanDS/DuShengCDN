# 自建权威 DNS 与 GSLB 调度规划

你会学到：为什么需要自建权威 DNS、它与现有 Cloudflare 自动 DNS 的区别、权威 DNS 服务如何复用 GSLB 策略，以及后续应如何继续增强。

本文是自建权威 DNS 与实时 GSLB 的设计基线。它把“后台同步 DNS 记录”升级为“逐次 DNS 查询实时调度”的目标纳入产品边界，但不改变现有 OpenResty 配置发布、Agent 同步和回滚模型。

当前实现状态：Server 控制面已经具备 Zone、静态记录、DNS Worker Token、Worker 心跳/聚合上报、只读调度快照 API，以及 `proxy_routes.dns_provider_mode` / `dns_zone_id_ref` 和 `gslb_scheduling_states.scope_key` 数据基础。DNS Worker MVP 已提供独立 `cmd/dns-worker` 运行入口，可监听 UDP/TCP `53`、拉取并缓存只读快照、回答静态 DNS 记录，并对权威模式站点的 `A`/`AAAA` 查询实时执行 GSLB 选点。DNS Worker 查询面已经提供按来源 IP 的基础 QPS 限制和 UDP 响应大小保护，超限查询返回 `REFUSED`，超大 UDP 响应设置 TC 位让递归解析器回退 TCP。管理端已经接入 DNS 查询聚合观测、查询趋势、SERVFAIL/NXDOMAIN 趋势、Worker 快照一致性告警、Server 侧 Worker 公网 UDP/TCP 53 探测健康状态、复用在线 Agent 节点的 Worker 多点 UDP/TCP 53 可达性与 RTT 探测、Worker GeoIP 国家库加载状态、来源作用域分布、GSLB 调度状态、GSLB 调度模拟、Zone 委派检查和 Cloudflare 到自建权威 DNS 的迁移向导，可查看查询量、返回码、Worker/Zone/站点维度、返回目标分布、`cidr:203.0.113.0/24` / `country:HK` / `country:DE` / `global|bucket:42` 等来源作用域、当前实际目标、期望目标、防抖冷却状态、注册商 NS 匹配状态、Glue 提示、当前快照按来源模拟返回目标以及待迁移网站的 Zone/Worker/GSLB/公网探测准备状态。

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
* 支持 HK、EU 等多个节点池按来源 CIDR、国家代码、节点池权重、节点负载和健康状态调度；自建权威 DNS 查询面可对不同来源桶执行稳定加权分流。
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
| 生效方式 | Server 周期性重算并同步一组静态 A/AAAA 记录 | 每次 DNS 查询实时计算答案 |
| 来源识别 | 当前只能使用全局默认上下文 | 可按递归解析器 IP、EDNS Client Subnet、来源 CIDR 和 GeoIP 识别 |
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
   * 先按节点池策略中的来源 CIDR 匹配，再按国家代码匹配节点池；未命中时回退到全部启用池。
   * 按节点健康、OpenResty 状态、排空状态、调度开关和公网 IP 类型筛选候选。
   * 按池权重、来源分流桶、节点权重、连接数、CPU、内存和策略模式选择目标。
   * 按 `target_count` 返回一个或多个 IP。
4. 对 `CNAME`、`TXT`、`NS`、`SOA` 等查询，返回 Zone 静态记录或系统生成记录。
5. 将本次选择写入本地内存防抖状态；按聚合窗口通过 heartbeat 批量上报查询量、命中策略、返回目标、错误原因和当前可恢复的防抖状态。

防抖状态不应只按网站保存。自建权威 DNS 模式下应按调度作用域保存：

```text
route_id + record_type + source_scope
```

`source_scope` 支持 `global`、国家代码、来源 CIDR 和权重分流桶，例如 `country:HK`、`country:DE`、`cidr:203.0.113.0/24`、`global|bucket:42`。这样不同来源网段、地区和稳定分流桶可以各自保持冷却窗口，避免互相覆盖。

## 数据模型规划

当前已经新增或扩展以下对象；`dns_zone_assignments` 暂不落库，先按“全部 Worker 服务全部启用 Zone”处理。

| 对象 | 作用 |
| --- | --- |
| `dns_zones` | 托管权威 Zone，例如 `example.com`，保存 SOA、NS、默认 TTL、启用状态和序列号 |
| `dns_records` | Zone 内静态记录，至少支持 `A`、`AAAA`、`CNAME`、`TXT`、`MX`、`NS`、`SOA` |
| `dns_workers` | 权威 DNS Worker 身份、Token、公网地址、版本、心跳、最近快照状态、GeoIP 国家库加载状态和最近一次 UDP/TCP 探测结果 |
| `dns_zone_assignments` | 暂不落库，当前简化为全部 Worker 服务全部启用 Zone |
| `dns_query_rollups` | DNS 查询聚合指标，按时间窗口、Zone、站点、来源作用域、qtype、rcode 和 Worker 统计 |
| `dns_worker_node_probes` | 每个在线 Agent 节点对每个 DNS Worker 公网地址的最新 UDP/TCP `53` 探测结果、RTT、健康状态和错误样本 |
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
| `GET /api/dns-zones/{id}/delegation-check` | 查询公网 NS 并对比 Zone 期望 NS，返回委派状态、缺失/额外 NS 和 Glue 提示 |
| `POST /api/dns-zones/{id}/records` | 新增或更新静态 DNS 记录 |
| `POST /api/dns-records/{id}/update` | 更新静态 DNS 记录 |
| `POST /api/dns-records/{id}/delete` | 删除静态 DNS 记录 |
| `GET /api/dns-workers/` | 查看 DNS Worker 在线状态、监听地址、版本、快照时间和 GeoIP 国家库加载状态 |
| `GET /api/dns-workers/observability` | 查看 DNS Worker 查询聚合、查询趋势、SERVFAIL/NXDOMAIN 趋势、Worker 快照一致性、Worker 查询延迟、可用率、错误率、Agent 多点探测通过率/RTT、Worker/Zone/站点维度和返回目标分布 |
| `GET /api/dns-workers/scheduling-states` | 查看 `gslb_scheduling_states` 中每个站点、记录类型和来源作用域的当前实际目标、期望目标、最近评估时间和防抖状态 |
| `POST /api/dns-workers/simulate` | 按网站、记录类型、来源国家代码和来源 IP 预演当前权威 DNS 快照的 GSLB 返回目标，不写入真实防抖状态 |
| `POST /api/dns-workers/` | 创建 DNS Worker Token |
| `POST /api/dns-workers/{id}/probe` | 管理端按需从 Server 探测 DNS Worker 公网 UDP/TCP 53 可达性、RTT、RCODE 和应答数量，并保存最近一次探测结果 |
| `POST /api/dns-workers/{id}/delete` | 删除 DNS Worker |
| `GET /api/dns-snapshot` | DNS Worker 拉取只读调度快照，需 Worker Token |
| `POST /api/dns-worker-heartbeat` | DNS Worker 上报状态、快照版本、GeoIP 国家库状态和聚合指标 |

已落地的前端入口：

* 左侧「权威 DNS」主菜单作为独立基础设施资源入口。
* 「权威 DNS」页面支持 Zone、NS、SOA、静态记录和 DNS Worker Token 管理，并展示 Worker 在线状态、版本、最近心跳、快照时间、GeoIP 国家库加载状态、查询量、查询趋势、SERVFAIL/NXDOMAIN 趋势、快照一致性、Worker 查询延迟、可用率、错误率、Agent 多点探测通过率/RTT、返回码、返回目标、来源作用域、动态站点分布和当前 GSLB 调度状态。
* DNS Worker 列表支持按需探测单个 Worker 的公网 UDP/TCP 53，返回 RTT、RCODE、应答数量和错误信息，并在刷新后继续展示最近一次结果，用于验证 Server 到该 NS 的解析可达性。
* Worker 可用性面板会把最近一次公网探测归类为 `healthy`、`partial`、`failed`、`stale` 或 `unknown`，并展示各 Agent 节点对 DNS Worker 的 UDP/TCP `53` 探测结果、平均 RTT、最大 RTT、过期数量和失败原因；Agent 多点探测超过新鲜度窗口后仍保留明细但不计入健康通过率，避免旧成功结果误导排障；迁移向导会要求至少一个在线 Worker 通过最新 UDP/TCP 53 探测后再标记站点为可切换。
* 「GSLB 调度模拟」可选择权威 DNS 模式网站、记录类型、来源国家代码和来源 IP，基于 Server 当前生成的只读快照复用 DNS Worker 调度器预演返回目标、TTL、来源作用域和快照版本，并展示匹配节点池、候选节点、被跳过节点与原因，不改变真实调度状态。
* Zone 详情支持按需执行委派检查，对比注册商当前公网 NS 与 Zone 期望 NS，并在 NS 位于当前 Zone 内时提示需要配置注册商 Glue/主机记录。
* 网站配置的「自动 DNS」分区支持 `Cloudflare 同步` 和 `自建权威 DNS` 两种模式。
* 「权威 DNS」页面提供迁移向导，可列出 Cloudflare 模式网站候选，检查域名是否完整落在某个已启用 Zone 下、是否存在在线 Worker、是否至少一个在线 Worker 通过公网 UDP/TCP 53 探测、是否已启用站点 GSLB，并可对满足条件的站点一键切换到自建权威 DNS；切换成功后会自动刷新网站 DNS 模式、执行 Zone 委派检查、探测在线 Worker 公网 UDP/TCP 53，并按当前快照执行 global 与来源国家 GSLB 模拟，切换后仍需在注册商确认 NS 委派。
* GSLB 节点池策略继续放在网站配置里，权威 DNS 只负责实时执行策略。

迁移体验约束：

* 当前版本不直接修改注册商 NS，只负责把站点 DNS 模式切换到自建权威 DNS，并在切换后自动复测模式、委派、Worker 公网可达性和 GSLB 模拟结果；委派不匹配、Glue 缺失或注册商侧 NS 未生效时，仍需要用户到注册商完成调整。

## DNS 协议行为

第一阶段支持：

* UDP 53 和 TCP 53。
* `A`、`AAAA`、`CNAME`、`TXT`、`MX`、`NS`、`SOA`。
* `NOERROR`、`NXDOMAIN`、`NODATA`、`SERVFAIL`。
* EDNS Client Subnet 可选开启，默认优先使用 ECS，未提供时使用递归解析器 IP。
* `AXFR` 默认拒绝，后续如需支持必须配置来源 IP 白名单和 TSIG。

以上 DNS 协议行为已在 DNS Worker MVP 中落地。当前实现默认拒绝 `AXFR`/`IXFR`，支持 `A`、`AAAA`、`CNAME`、`TXT`、`MX`、`NS`、`SOA`，会按 EDNS UDP buffer 和 Worker 侧上限截断超大 UDP 响应并设置 TC 位，并通过心跳批量上报聚合查询指标。

TTL 规则：

* Cloudflare 模式继续沿用当前 `0/1` 自动 TTL、`2-29` 提升到 `30`、最高 `86400` 的规则。
* 权威 DNS 模式中，`0/1` 应映射为 `AuthoritativeDNSDefaultTTL`，默认建议 `30` 秒。
* GSLB 记录建议默认 TTL `30` 到 `60` 秒；静态记录可使用 Zone 默认 TTL。

## 高可用与失效策略

权威 DNS 是入口能力，必须按高可用设计：

* 生产至少部署两个 DNS Worker，并在注册商配置两个 NS。
* DNS Worker 保存最后一次有效快照，并在本地缓存文件中写入 SHA-256 checksum 完整性元数据；启动加载缓存时会先校验 checksum，并从快照中的 GSLB 防抖状态恢复最近可用选择，Server 暂时不可用时继续使用最后一次校验通过的快照服务。
* 快照超过 `AuthoritativeDNSSnapshotMaxAge` 后，动态 GSLB 记录返回 `SERVFAIL`，静态 SOA/NS 可继续返回。
* 管理端会按最近心跳检测在线 Worker 的快照版本和快照年龄，并在多 Worker 版本不一致或快照过期时告警。
* 管理端会基于 Worker 心跳聚合展示在线率、查询错误率和本地查询处理耗时，并可按需从 Server 探测某个 Worker 的 UDP/TCP 53 可达性；最近探测会参与迁移准备状态。在线 Agent 还会接收 Server 下发的少量 Worker 探测目标，主动探测公网 UDP/TCP `53` 可达性并回传 RTT，用于补充多节点视角；过期的 Agent 探测结果不会继续计入健康通过率，但当前结果整体仍仅进入观测面板，尚不参与 GSLB 调度评分。
* DNS Worker 不直接修改数据库，不在查询路径里写入状态。
* 快照携带 Server 侧最近一次 GSLB 防抖状态，Worker 启动或拉取新快照后会恢复可用的 `route_id + record_type + source_scope` 选择状态；逐查询产生的新状态先保存在 Worker 内存中，再通过 heartbeat 批量回传 Server。
* 查询聚合按窗口批量上报，失败时本地缓冲，避免每次查询写库。

## 安全约束

* DNS Worker 使用专属 Token，不复用 Agent Token 或管理员 Session。
* Worker 拉取快照和上报 heartbeat 必须走 HTTPS。
* 快照不包含管理端敏感信息、Cloudflare Token、Session Secret 或用户密码。
* Zone Transfer 默认禁用。
* 查询日志默认聚合，不保存完整来源 IP；如未来提供原始日志，必须有采样、脱敏和保留天数。
* DNS Worker 已具备按来源 IP 的基础 QPS 限制和 UDP 响应大小控制；超限查询返回 `REFUSED` 并进入聚合指标，超大 UDP 响应会截断并设置 TC 位，避免被放大攻击滥用。

## 实施阶段

### 阶段 1：设计与数据基础（已落地 Server 控制面）

* 已新增 `dns_zones`、`dns_records`、`dns_workers`、`dns_query_rollups` 迁移。
* 已新增 `dns_worker_node_probes` 迁移，用于保存 Agent 节点对 DNS Worker 的最新主动探测结果。
* 已扩展 `gslb_scheduling_states` 的 `scope_key` 维度。
* 已为 `proxy_routes` 增加 `dns_provider_mode` 和 `dns_zone_id_ref`。
* 已提供 Zone、Worker、静态记录、Worker 心跳/聚合上报和只读快照 API。
* 已提供前端「权威 DNS」入口和网站自动 DNS 模式选择。

### 阶段 2：权威 DNS Worker MVP（已落地）

* 使用 `github.com/miekg/dns` 实现 UDP/TCP 53 监听。
* DNS Worker 从 Server 拉取只读调度快照，写入本地缓存并在内存中构建 Zone、记录和站点索引。
* 支持 `SOA`、`NS`、静态记录和网站 `A`/`AAAA` 动态 GSLB 回答。
* 在 Worker 内复用同等 GSLB 策略语义，按节点池、池权重、来源分流桶、节点权重、OpenResty 健康、排空、调度开关、最大连接数、最大 CPU 使用率和最大内存使用率选点。
* 支持 EDNS Client Subnet 来源识别；节点池策略可按来源 CIDR 优先命中，也可在配置本地 MaxMind Country MMDB 后按国家代码命中节点池，否则回退到 `global` 作用域。
* 防抖状态按 `route_id + record_type + source_scope` 保存在 Worker 内存中。
* 支持按来源 IP 的基础 QPS 限制和 UDP 响应大小保护，避免异常递归解析器或放大流量压垮查询面。
* 聚合查询指标按窗口批量 heartbeat 上报；上报失败时保留在内存中等待下次重试。
* 已增加单元测试覆盖 DNS 响应、NXDOMAIN、NODATA、GSLB 选点、ECS 作用域、TTL、快照降级、AXFR 拒绝和聚合上报。

### 阶段 3：观测、联调与迁移体验

* DNS Worker 已上报查询聚合、返回目标分布、来源作用域分布、错误码分布和本地查询处理耗时。
* 管理端已展示 DNS Worker 状态、GeoIP 国家库加载状态、查询量、查询趋势、SERVFAIL/NXDOMAIN 趋势、快照一致性、Worker 可用率、查询错误率、平均/最大查询延迟、Agent 多点探测通过率/RTT、返回码、来源作用域、Worker/Zone/站点维度、返回目标分布、当前 GSLB 调度状态、按需 Worker UDP/TCP 53 探测、最近公网探测健康状态、GSLB 调度模拟诊断、Zone 委派检查和 Glue 提示。
* 已提供从 Cloudflare 同步模式切换到自建权威 DNS 模式的迁移检查向导和一键切换动作。
* 文档补充注册商 NS、Glue、端口、防火墙和回滚步骤。

### 阶段 4：增强能力

* 可选 TSIG/AXFR 白名单。
* 可选 DNSSEC 设计。
* 后续更细粒度来源作用域，例如 ASN、省份等。
* 已提供基于心跳和真实 DNS 查询聚合的 Worker 查询延迟、错误率和可用性看板。
* 已提供 Server 侧按需 Worker UDP/TCP 53 探测，验证单个 NS 的解析可达性。
* 已提供复用在线 Agent 节点的主动多点 DNS Worker UDP/TCP 53 探测，验证不同边缘节点到 Worker NS 的公网 RTT 和解析可达性。
* 后续可将 Agent 多点探测纳入 GSLB 调度评分，或扩展独立探测点网络、丢包统计和更细区域覆盖。

## 验收标准

第一阶段完成时：

* 设计文档、部署文档、配置参考和使用文档说明一致。
* 数据模型迁移具备升级校验和回归测试。
* 管理端可以创建 Zone、Worker Token，并把网站绑定到权威 DNS 模式。

第二阶段完成时：

* `dig @worker-ip example.com A` 能按 GSLB 策略返回在线节点公网 IP。
* HK/EU 等国家来源能命中不同节点池；无来源信息时走全局策略。
* 节点离线、排空、OpenResty 不健康或超过负载阈值时不会被返回。
* Server 不可用时，DNS Worker 可使用最后一次校验通过的有效快照继续回答。
* 单元测试覆盖 DNS 协议响应、GSLB 调度、TTL、防抖和快照失效。

第三阶段完成时：

* 管理端能展示 DNS Worker 在线状态、查询量、查询趋势、SERVFAIL/NXDOMAIN 趋势、快照一致性、查询延迟、可用率、错误率、错误码、来源作用域、返回目标、GSLB 模拟诊断、按需 Worker 探测和委派检查。
* 管理端能在「权威 DNS」迁移向导里列出 Cloudflare 模式站点候选，检查 Zone、在线 Worker、公网 UDP/TCP 53 探测、GSLB 与回滚提示，并对满足条件的站点执行一键切换。
* 文档能指导用户从 Cloudflare 模式迁移到自建权威 DNS，并说明回滚方式。
* 生产部署建议明确要求至少两个 DNS Worker 和 UDP/TCP 53 防火墙放行。
