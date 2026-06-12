# DuShengCDN 代码审查报告（代码优化专项）

> **修复进度（2026-06-11 第一批已完成，测试全部通过）**：
> - 已修复并验证：A1–A17 全部 18 项（发布计划终态守卫/互斥/池级阻断口径、regex location 引号、重复规则校验、回填保留用户规则、RouteRules 快照比较、稳定 upstream/CA 命名、规则批量加载接线、规则级 Basic Auth 密码保留、冗余 Sync 删除等），另修复 B9（上游 IP 排序）、`DedupeSupportFiles` 排序、以及全部 17 处 GBK 乱码文案（B 系列乱码项）。
> - 待处理：B 系列其余缺陷（B1–B8、B11–B23）、性能项 P1–P9、清理项（第四节）。

- 日期：2026-06-11
- 范围：`dushengcdn_server`（约 10.2 万行 Go）、`dushengcdn_agent`（约 1.6 万行 Go）、`web/` 前端（约 4.6 万行 TS/TSX），含工作区未提交改动（约 4400 行：配置发布计划 + 路径级路由规则 + v48 迁移）
- 方式：11 个并行审查代理分子系统通读代码并核实调用链，所有发现均经实际代码验证（非静态扫描）

## 摘要

整体工程质量较好：出站 HTTP 普遍带超时、批量查询意识强（`ByIDs` 系列、`runConcurrentQueries`）、鉴权中间件覆盖完整（未发现未授权管理端点）、Agent 侧安全细节到位（SSRF 防护、签名校验、GeoIP 原子落盘）、前端 react-query 用法规范。

主要问题集中在四类：

1. **未提交的发布计划/路径规则功能有多个可直接损坏生产的缺陷**（状态机无终态守卫可强推全网回滚；regex location 未加引号 / 重复 location 可让整节点 nginx 配置失效）。
2. **热路径 N+1 与重复计算**：节点心跳、DNS worker 心跳/快照、Cloudflare 每分钟对账、发布渲染管线、DNS worker 每查询路径，都存在按节点/路由/查询量线性放大的重复 DB 查询或 CPU 工作。
3. **约 2000+ 行可删除的死代码与多处三/四重复制实现**（旧渲染器整段 700 行、Basic Auth 哈希三处、GSLB 池匹配四处等）。
4. **系统性小问题**：十余处 GBK 乱码错误文案已固化进源码；Agent 侧状态/配置文件均为非原子写。

---

## 一、未提交改动中的问题（建议提交前修复）

### 高危

| # | 位置 | 问题 | 建议 |
|---|------|------|------|
| A1 | `dushengcdn_server/service/config_release_plan.go:215`（Evaluate）、`:294`（Complete）、`:305`（Fail） | 三个入口均不校验 `plan.Status` 终态。对已 completed 的计划再调 `/evaluate`，只要之后发过新配置且超过观察期，就会把它改判 failed、阻断其 checksum（完成时已更新为激活版本的 checksum），并经 `forceSyncConfigReleaseRollbackTargets` 把生产节点强推回旧版本，造成配置分裂；`GetNodeByNodeID` 的瞬时 DB 错误也被当作"节点缺失"直接 fail 整个计划；已 rolled_back 的目标还会在 `:244-248` 被翻回 succeeded（两个代理独立确认） | Evaluate/Advance/Complete/Fail 入口校验状态仅允许 running/observing；区分 DB 错误与节点缺失；终态目标不再更新 |
| A2 | `dushengcdn_server/service/openresty/route_renderer.go:493` + `dushengcdn_server/service/proxy_route.go:911` | regex 规则直接 `fmt.Sprintf("~ %s", path)` 渲染，校验只禁 `\r\n;`。合法正则如 `^/v[0-9]{2}/` 渲染出 `location ~ ^/v[0-9]{2}/ {`，nginx 解析失败 → 整节点配置发布报废（两个代理独立确认） | 渲染输出 `~ "..."` 带引号并转义/拒绝 `"`；`normalizeProxyRouteRuleMatch` 同步收紧字符集 |
| A3 | `dushengcdn_server/service/proxy_route.go:684` | 同路由内重复 `(match_type, path)` 规则不校验（`/static` 与 `/static/` 归一化后相同，库内索引非唯一），渲染出两个相同 location → nginx `duplicate location` 启动失败 | 循环内按 `(matchType, path)` 去重报错，或建唯一索引 |
| A17 | `dushengcdn_server/model/proxy_route_normalized.go:572` | （存量代码，但与新功能直接相关）`EnsureProxyRouteNormalizedTablesBackfilled` 按 `siteCount != routeCount` 触发后先 `clearProxyRouteNormalizedTables` 清空全部 9 张表、仅重建每路由一条默认规则——V48 之后用户创建的全部路径级规则/策略会被清掉，**数据丢失** | 改为按 route 增量修复缺失行，保留 `match_type <> 'default'` 的用户行 |

### 中危

| # | 位置 | 问题 | 建议 |
|---|------|------|------|
| A4 | `service/config_release_plan.go:96` | 阻断检查口径不一致：创建时查全局 `version.Checksum`，落库阻断的是池级 `artifact.Checksum`（`:130`、`:336-346`），多池场景阻断检查永不命中 | 统一用 artifact.Checksum（或两者都查） |
| A5 | `service/config_version.go:1413` | `snapshotRouteConfigEqual` 未比较新增的 `RouteRules` 字段：仅改路径规则时 diff 显示"无变化"，与 rendered checksum 自相矛盾（两个代理确认） | equal 函数加入 route_rules 逐字段比较（忽略 ID） |
| A6 | `service/proxy_route.go:695` + `service/openresty/route_renderer.go:680` | 规则行 delete+recreate 导致 ID 变化，而 upstream 名（`backend_..._rule_<id>`）与 CA 文件名内嵌 ID → 内容相同的重新保存也改变 checksum，`HasConfigChanges` 误报，发布后全网节点无意义 reload | 按 (match_type, path) upsert 保留行，或 upstream/CA 命名改用稳定标识 |
| A7 | `service/config_release_plan.go:75` | 同池并发两个计划无互斥：旧计划评估必超时失败 → 阻断 checksum + 对共享节点强推回滚 | 创建/启动时检查同池不存在 running 计划 |
| A9 | `service/config_release_plan.go:275` | `AdvanceConfigReleasePlan` 对 draft 计划的 `case 0: Start` 分支因健康度门槛永不可达（死分支）；draft 调 advance/evaluate 还可能因金丝雀节点离线直接 fail 并阻断候选 checksum | `CurrentStage == 0` 时跳过健康度门槛直接 Start |
| A10 | `service/proxy_route.go:925` | 规则级 Basic Auth 密码为空即报错，而 View 只回传 `*_configured` 不回传密码 → 前端读-改-写必失败；与路由级"密码留空保留旧 hash"语义不一致 | 对齐路由级 preserved-on-save 语义 |
| A11 | `service/config_version.go:1136` + `service/proxy_route.go:1330` | 发布管线 N+1：`loadProxyRouteRuleConfigs` 每路由 2-4 条查询，`buildCurrentConfigBundle` 三轮调用（快照/全局渲染/每池渲染）≈ `4R(2+P)` 条 SQL；`buildProxyRouteRuleViews` 让 ListProxyRoutes 每路由再跑一遍且与前者约 80 行重复；新写的批量函数 `model.ListProxyRouteRulesByRouteIDs`（`model/proxy_route_normalized.go:275`）从未被调用 | 批量函数接入 render/view context 一次加载分组复用；两处共用一个加载器 |
| A15 | `service/openresty/types.go:135` + `route_renderer.go:256` | `RouteRule` 嵌套与平铺两套覆盖字段并存，生产唯一构造方只赋平铺子集；WAF/CC/PoW/Region 等字段无人赋值且即便赋值也不生效（access.lua 按域名下发），约 70 行合并分支为死代码 | 删除嵌套结构与未实现字段，只保留实际支持的覆盖项 |

### 低危

- A8 `service/config_release_plan.go:556`：agent 上报空 checksum 的 success 直接标记目标 succeeded，且从不比对 `payload.Version` → checksum 为空时退而比对 Version。
- A12 `service/config_release_plan.go:96`：阻断 checksum 只增不删、无解除 API、`Force` 不绕过；且检查发生在版本创建之后，被拒时已产生孤儿 inactive 版本 → 提供解除接口，检查前移。
- A13 `service/config_release_plan.go:226`：Evaluate 循环逐目标 `GetNodeByNodeID`（应 IN 批量），逐条 UPDATE 错误被 `_ =` 吞掉；回滚元数据每目标重复计算 → 批量加载、错误记日志。
- A14 `service/proxy_route.go:691`：`replaceProxyRouteRuleInputsWithDB` 入口重复执行 `SyncProxyRouteNormalizedTablesWithDB`，同一事务里 Insert/Update 刚跑过同一份同步（10+ 条语句白跑）→ 删除冗余调用。
- A16 `service/config_release_plan.go:19`：`observing` 状态常量从未写入；`proxyRouteDefaultRulePriority`、`ListCachePoliciesByRouteID`/`ListSecurityPoliciesByRouteID` 未使用 → 实现或删除。
- A18 `controller/config_release_plan.go:188`：新文件又造了一个 `parseConfigReleasePlanID` 私有副本，`parseUintParam`（controller/authoritative_dns.go:550）早已存在 → 复用。

**未发现问题的部分**：路由组已正确挂 AdminAuth；v48 迁移的索引降级与回填顺序正确；新增测试质量不错。

---

## 二、存量代码中的重要缺陷

### 高危

| # | 位置 | 问题 | 建议 |
|---|------|------|------|
| B1 | `service/authoritative_dns.go:2163` | `ListDNSZoneWorkerAssignmentsByZoneIDs` 出错被吞，直接置空 zones/routes/nodes 并返回 → 一次瞬时 DB 错误会让 worker 应用**签名的空快照**，全部域名停止应答 | 错误向上传播，让 worker 保留上一份有效快照 |
| B2 | `service/authoritative_dns.go:1945`、`:4518` | 心跳/快照拉取用整行 `Save(worker)` 覆盖全部列，与管理端操作（请求更新、令牌轮换）并发时静默回滚对方写入 | 改 `Select(字段).Updates(...)` 只写各自负责的列 |
| B3 | `model/main.go:391` | SQLite→Postgres 迁移按基表名检查 `HasTable`，而分片表只建 `_00.._09` 从不建基表 → 观测数据**静默全部丢弃**；`:424-435` 分片写入分支为不可达死代码 | 遍历 `observabilityShardTables` 逐分片迁移，或明确文档化并删死分支 |
| B4 | `dushengcdn_agent/internal/state/state.go:97`、`config/config.go:553`、`nginx/manager.go:281` | 状态文件/agent.json（含 token）/nginx.conf 均为非原子 `os.WriteFile`，掉电截断 → Agent 崩溃循环或永久失联；状态文件每 10s 写 3 次，暴露窗口大（同仓库 geoip 已示范 temp+rename 正确做法） | 统一改"临时文件 + rename"；Load 失败降级重建而非退出 |

### 中危

| # | 位置 | 问题 | 建议 |
|---|------|------|------|
| B5 | `service/node.go:82`、`:130` | `normalizeNodeInputV2` 的 `err` 从未检查 → 所有校验错误（IP 非法、纬度越界等）一律报成"节点名不能为空" | 先判 err 再判空 |
| B6 | `service/update.go:193` | 自升级在持锁检查与置 `inProgress` 之间隔着数秒网络请求 → 并发请求可同时通过，重复下载并并发替换二进制/重启 | 首次持锁即置标志，失败回滚 |
| B7 | `service/lego_client.go:360` | ObtainSSL 仅按换行切附加域名，而入参校验接受逗号/空格/分号 → "a.com, b.com" 过校验但 ACME 下单失败 | 复用 `parseTLSCertificateDomains` |
| B8 | `service/openresty/utils.go:12` + `proxy_route.go:2372` | `QuoteNginxStringLiteral` 不处理 `$`：自定义头含 `$` 渲染出 `unknown "x" variable` → 全节点配置失效；命中已有变量名时还会发生插值 | 校验拒绝 `$`，或渲染层以字面量方式输出 |
| B9 | `service/openresty/upstream.go:206` | publish_resolve 解析出的上游 IP 按 DNS 应答顺序写入不排序 → 渲染文本/checksum 不确定，"无变更不发布"拦截失效 | 返回前 `sort.Strings(servers)` |
| B10 | `controller/option.go:450-491` | 单条 `UpdateOption` 复制批量路径校验链但漏掉 `validateCloudflareOption` → 两路校验漂移，可写入非法值 | 单条直接调 `updateOptions([]model.Option{option})` |
| B11 | `middleware/turnstile-check.go:104-111` | `session.Save()` 失败分支写完 JSON 仅 `return` 未 `c.Abort()` → 后续 handler 照常执行、响应体写两次 | 补 `c.Abort()` |
| B12 | `controller/proxy_route.go:211` | 清缓存请求体 JSON 解析失败被静默降级为 `Scope: "all"` 全量清除（`SwitchProxyRouteToAuthoritativeDNS:155` 同模式） | 用 `decodeOptionalJSONBody`：空体默认、非法 400 |
| B13 | `internal/dnsworker/server.go:92` | 运行期直接写 `s.udpServer.UDPSize`，与 serve goroutine 并发读构成数据竞争（dns.Server 启动后不可改） | 只更新有锁保护的 `DNSServer.UDPSize` |
| B14 | `service/authoritative_dns.go:4614` | 5000 条 rollup 上限只在非生产路径检查，真实心跳路径可塞数万行插入 | 检查移入 `RecordDNSWorkerHeartbeat` |
| B15 | `web/features/settings/components/settings-page.tsx:975` | 任一卡片保存后 invalidate 触发整组表单状态重建，清掉**其他卡片**未保存输入、密钥字段被置空 | dirty 跟踪或只重置当前卡片字段组 |

### 低危

- B16 `service/configversion/publisher.go:20`：版本号按字符串排序 + `%03d` 补零，单日第 1000 次发布起唯一索引反复冲突、当天无法恢复 → 解析数字取 MAX 或按 `id desc`。
- B17 `service/config_version.go:603`：事务提交且 WS 已广播之后构建视图失败会 `return nil, err`，调用方误判发布失败（实际已激活）→ 降级返回成功。
- B18 `model/main.go:106`：root 账户 check-then-insert，多实例并发首启第二个实例直接启动失败 → 识别 unique 冲突按已存在处理。
- B19 `service/authoritative_dns.go:952`：删 Zone 先删记录再删 zone 且非事务 → 用事务包裹。
- B20 `service/tls_certificate.go:414`：`cert.Update()` 错误被忽略且协程先于落库启动；`lego_client.go:313/401` 的 `Save` 同样未检查 → 先落库查错再起协程。
- B21 `dushengcdn_agent/internal/agent/runner.go:248`：WS 读 goroutine 在通道满 + 连接退出时永久阻塞，反复重连累积泄漏 → 每连接 done channel。
- B22 `controller/user.go:349`：`/api/user/search` 无分页上限；非数字关键字在 PostgreSQL 下 `id = ?` 直接报类型错误 → 加 Limit、纯数字才拼 id 条件。
- B23 `model/migrations.go:2738-2750`：path 级规则迁移的整批覆写若被复用于已有多规则的库会损坏数据（当前 V48 时刻安全）→ 加注释或收紧条件。

### 系统性：GBK 乱码错误文案（已固化进源码，用户可见）

`controller/response.go:13`（`respondBadRequest` 默认 400 文案）、`controller/option.go:319/323/327/357/363/521`、`controller/wechat.go:52/265`、`service/observability.go:182`、`service/config_version.go:1696`（及附近 3 处）、`service/openresty/validation.go:14`、`tls_renderer.go:42`、`main_template.go:52/56`。
建议全局 grep 乱码片段（如 `鏃犳`、`涓嶈兘`、`璺敱`）统一修复。

---

## 三、性能优化机会（按热路径分组，影响 = 频率 × 成本）

### P1 节点心跳路径（每节点每 ~10s）

- `service/observability.go:566`：每批日志摄入新建 GeoIP resolver = 每次心跳重新 `maxminddb.Open`；mmdb 缺失时在同步心跳处理器内触发最长 2 分钟、128MB 的网络下载；批内 IP 缓存随 resolver 丢弃 → 改进程级单例 + 启动期预热。
- `service/observability.go:600`：每次摄入在插入事务内跑跨全部分片的保留期 DELETE（已有每日 3 点定时清理）→ 从摄入路径移除或按节点每小时节流。
- `service/observability.go:451`：Agent 补传 N 条缓冲记录开 N+1 个事务、N+1 个 resolver → 合并单事务单批。
- `service/agent.go:475`：心跳元数据查询拉整行（含数 MB `RenderedConfig` 等大字段）只用 Version/Checksum 两列；`ListNodeViews` 同模式按节点 N+1 → `Select` 轻量列 + ristretto 短 TTL 缓存。
- `service/agent.go:230` + `service/authoritative_dns.go:1688`：同一次心跳 `ListDNSWorkers()` 全表查两遍（probe targets + pending updates），而待下发更新极少存在 → 条件查询/内存标志短路 + 心跳内复用。

### P2 DNS worker 心跳与快照（控制面）

- `service/authoritative_dns.go:4996`：心跳写事务内对每条路由全表扫描 nodes（N+1，SQLite 下阻塞全部写入）→ 循环外加载一次传入。
- `service/authoritative_dns.go:4088`：每个 worker 每次拉取都全量重建快照（两次大 JSON marshal + SHA256），GSLB 调度状态逐路由单查 → 调度状态并入批量预载；未过滤快照按数据版本缓存。
- `service/authoritative_dns.go:4703`：每次心跳 ACL 构建两次（各 3 条查询）、rollups 过滤两次 → 传参复用。
- `service/authoritative_dns.go:6361/6500/5452`：可观测性汇总同一请求 `ListDNSWorkers()` 执行 3 次、`ListNodes()` 重复 → 顶层加载一次下传。
- `service/authoritative_dns.go:6800`：probe 统计 O(P×N) 线性查找 → nodeID→index map。

### P3 Cloudflare 对账 cron（每分钟）

- `service/proxy_route_dns.go:282` + `gslb_scheduler.go:95`：每条路由先 `selectProxyRouteDNSTargets` 再在 sync 内部完整重选一遍；每次选择触发 `ListNodes()` + 跨分片指标 UNION + 调度状态单查 ≈ 每路由每分钟 2×3 个全量查询；已有的 `loadGSLBDNSSchedulingData` 批量预载机制（权威 DNS 路径在用）此处完全未用 → 循环前预载一次贯穿传入，删除循环内预选择。
- `service/gslb_scheduler.go:184`：调度状态未进 `gslbDNSSchedulingData`，权威快照路径同样形成按路由 N+1 → 批量预载进 Data。
- `service/gslb_policy.go:509`、`gslb_scheduler.go:160`：CIDR 每次匹配双重解析、策略 decode 后重复 normalize → 预编译 `[]*net.IPNet`、只 normalize 一次。

### P4 发布/预览/Diff 渲染管线

- A11（见上）规则加载 N+1 ×(2+P)。
- `service/openresty/upstream.go:37` + `config_version.go:518`：渲染路径内嵌实时 DNS 解析，`context.Background()` 无超时、无记忆化；Preview/Diff/HasConfigChanges/发布都全量触发，全局渲染 + 每池渲染重复解析同一域名；单个慢 DNS 拖死接口 → 带超时 context + bundle 内按 host 记忆化。
- `service/config_version.go:2355`：只读路径（HasConfigChanges）隐式触发 `ensureConfigVersionArtifactsForPools` 写事务 → 读写分离。
- `service/openresty/tls_renderer.go:28`：同一证书被按"域名×证书"重复 PEM+x509 解析 → 按证书 ID 缓存解析结果。
- `service/config_version.go:465`：Diff 对同一快照重复 normalize 3-4 遍（每遍含逐路由 json.Marshal）→ 入口规范化一次复用。
- `service/agent_ws.go:288` + `config_release_plan.go:362`：版本广播对每客户端逐个查 node + 整行 artifact（2N 条查询）→ 一次 ListNodes 建映射、按池去重取轻量列。
- `service/openresty/route_renderer.go:148`：多证书时每个 HTTPS server 重复渲染相同 location 正文 → 渲染一次复用。
- `service/openresty/utils.go:35`：`DedupeSupportFiles` map 迭代输出顺序随机 → 产物字节不确定，按 Path 排序。

### P5 DNS worker 每查询热路径（internal/dnsworker，按 QPS 放大）

- `dnssec.go:65`：每个 DO 位查询对每把密钥做 AES-GCM 解密（含读环境变量）+ 私钥解析 → 快照加载时解析一次缓存。
- `scheduler.go:183`：每查询 `normalizePolicy` 深拷贝全部 pools 重建去重 map（快照加载已做过同样的事）；`:684` 每 CIDR 每查询重复 `ParseCIDR` → 信任快照归一化结果；CIDR 预解析为 `netip.Prefix`。
- `runner.go:96-110`：每 10s 无条件全量拉取 + 签名校验 + 重建索引 + 落盘，无版本短路；Save 内部双重归一化双重序列化 → 版本相同跳过；checksum 接收已归一化对象。
- `rate_limiter.go:101`：桶数 >4096 后每包 O(n) prune 且洪泛时删不掉任何条目、map 无上界、全局单锁 → prune 加最小间隔 + 硬上限 + 分片锁。
- `dnssec.go:309`：NSEC3 否定应答每次全 zone 重算哈希并排序（NXDOMAIN 攻击放大）→ 快照预计算排序哈希链 + 二分。
- `operator_cidr.go:254`：每查询 big.Int 堆分配 → `netip.Addr`/uint32 值类型。
- `server.go:404`：findBestZone 线性后缀匹配 → 按 label 剥离精确查找。

### P6 访问日志查询

- `model/node_access_log.go:1028`、`:1117`：过滤用前置通配 `LIKE '%x%'`、列上套 `LOWER(TRIM())` → 迁移专门建的复合索引全部失效，每次查询 10 分片全表扫描 → node_id 精确匹配、host/remote_addr 前缀匹配或存储时归一化。
- `model/node_access_log.go:1077` + `service/access_log.go:244`：UNION ALL 不下推排序/限量 + 深分页 OFFSET + 每次翻页全分片 `COUNT(*)+COUNT(DISTINCT)` 覆盖 90 天 → 下推 LIMIT、keyset 翻页、count 缓存或仅首页计算。
- `model/node_access_log.go:171`：每个查询维护 SQL/内存双实现（~900 行近半为 fallback）且 SQL 错误被静默吞掉；fallback 内还有每行扫 10 分片的真 N+1 → 删内存回退或仅保留并 `slog.Warn`。

### P7 Agent 模块（每边缘节点每 10s）

- `internal/sync/service.go:66`：每次心跳先全量重读配置包 + Walk 证书目录 + 整包 SHA-256，发生在所有短路判断之前 → Manager 缓存上次 checksum（Apply 后失效）。
- `internal/observability/traffic.go:200`、`:111`：访问日志无上限进心跳负载；首次运行 offset=0 整文件历史日志全量上报 → 每窗口设上限；首次从文件末尾开始。
- `internal/agent/runner.go:1215`：observability-buffer.json 每周期 3 轮全量读+写（MarshalIndent）→ 常驻内存、失败才落盘、紧凑序列化。
- `internal/observability/traffic.go:137` 等：agent-state.json 每周期 Load ~6 次、Save 最多 3 次（无变化也写）→ Store 内存缓存、变更才落盘。
- `internal/agent/runner.go:1159`：DNS 探测同步嵌在心跳构造中（单目标 UDP/TCP 各 3s 超时），可拖慢心跳逼近超时 → 独立 goroutine 自节奏，心跳附带缓存结果。
- `internal/observability/collector.go:31`：每心跳全量采集系统画像 + 指纹计算后常态丢弃 → 内存记住指纹低频重采。
- `internal/nginx/manager.go:1839`：每次 Apply 无条件重写全部受管文件（含未变的内置 Lua/证书）+ 两次全目录 Walk → 写前比较内容哈希。

### P8 其他

- `middleware/auth.go:29`：每请求全量回表查用户（token 路径 hash 未命中还二次查询）→ 短 TTL 缓存（key 含密码指纹）或 Select 必要列。
- `model/migrations.go:3104`：版本已是 48 仍每次启动重放 V2→V48 全链校验 + 全模型 AutoMigrate，其中 3 次全量加载 proxy_routes（`:653/:720/:756`）→ 版本相等只做轻量校验。
- `model/proxy_route_normalized.go:228`、`:590`：运行时查询每次 `Migrator().HasTable` 自省（共 10 处，一次 9 连查；域名冲突检查在写路径热点）→ 启动后 sync.Once 缓存。
- `model/proxy_route.go:143`：身份冲突检查对 JSON 文本列拼 `LIKE '%"x"%'` OR 链全表扫描，而带索引的 `proxy_site_domains` 规范表已存在 → 改查规范表。
- `model/main.go:302`、`node_access_log.go:815`：`Limit(1).Count` 不会提前终止（GORM 仍全量聚合）→ `SELECT 1 ... LIMIT 1` 存在性探测。
- `model/main.go:413`：整表迁移 OFFSET 翻页 O(n²)，无主键表无 ORDER BY 可能丢行/重行 → keyset 翻页（`migrations.go:918` 已有现成写法）。
- `model/option.go:150`：UpdateOptions 每键 FirstOrCreate+Save 两次往返 → `clause.OnConflict` 批量 upsert（origin_health_status.go:29 已有同款）。
- `common/logger.go:56`：每条日志顺序跑 8 个正则脱敏 → 廉价子串预检短路；`sensitiveFragments` 提升包级。
- `utils/geoip/geoip.go:101`：每次写缓存后 `items.Wait()`（ristretto 测试用 API，阻塞写缓冲落盘）→ 删除。
- `controller/misc.go:29`：公开 `/api/status` 每次查 auth_sources 表 → 短 TTL/变更失效快照。
- `controller/option.go:237`：`OpenRestyResolvers` 校验每次编译正则（同文件其余 5 个均为包级）→ 提升包级。
- `service/authoritative_dns.go:1457`：`net.LookupNS` 无 context 超时，委派检查可被拖数十秒 → `net.Resolver.LookupNS(ctx)`。
- `service/dnssec.go:277`、`:5672`：启用 DNSSEC 同一行读 3 次；单次探测重复解析地址 3 次 → 传参复用。

### P9 前端

- `web/components/data/trend-chart.tsx:35`（rank-chart 同）：引入完整版 ECharts（~1MB），同项目 world-stage-map 已示范 `echarts/core` 按需注册 → 改按需引入。
- `web/features/dashboard/components/world-stage-map.tsx:13`：602KB GeoJSON 打进 JS chunk → 移至 public/ 运行时 fetch + registerMap。
- `web/features/nodes/components/node-detail-page.tsx:318`：单节点详情页 4 路轮询（含全量节点列表 5s 只为找一个节点）≈ 40+ 请求/分钟 → 放宽间隔/按 Tab 门控/提供单节点接口。
- `web/features/access-logs/components/access-logs-page.tsx:299`、`:340`：默认 Tab 是 metering，但明细与 IP 汇总两类日志扫描查询未设 `enabled` 即并发发起 → 按 activeTab 门控。

---

## 四、重复与死代码清理（合计可删 2000+ 行）

### 死代码（已 grep 验证零调用）

| 位置 | 内容 | 规模 |
|------|------|------|
| `service/config_version.go:1918` 起 | 旧渲染器整条链（renderRouteConfigWithContextAndQueries 及十余个专属辅助函数），已被 service/openresty 取代，含一份会漂移的重复实现 | ~700 行 |
| `service/authoritative_dns.go` | `snapshotAuthoritativeRoutes`(3989)、`snapshotNodes`(4100)、`loadGSLBSchedulingStatesForUpdates`(5092)、`queryDNSObservabilityRecentRows`(5796) 等 ~10 个死函数（其中 snapshotAuthoritativeRoutes 一旦被调用即触发 N+1） | ~数百行 |
| `service/proxy_route.go:1705/1668/2223/46` | `normalizeProxyRouteDNSSettingsV2`/V1、`resolveProxyRouteDomainCertIDs`、`proxyRouteDefaultRulePriority` | ~100 行 |
| `service/access_log.go:538`、`observability_trends.go:53/96/163/216` | 预聚合重构前的内存聚合实现（生产已用 *FromBuckets 版本） | ~350 行 |
| `service/gslb_source.go:22` | 整个 GSLBSourceResolver 接口 + HTTP 实现（策略层已封死该路径） | 整文件大部 |
| `service/cloudflare_dns.go:403/60/116` | `UpsertDNSRecord` + 入参类型 + `parseCloudflareCredentials` | ~80 行 |
| `service/proxy_route_dns.go:566/600/386/28` 等 | 死函数群 + 只写不读的字段 | ~100 行 |
| `service/update.go:1441`、`commercial_license.go:1282`、`access_log_region.go:44` | 三个无调用函数；`update.go:816` 还有一次完全重复的签名校验 | |
| `controller/github.go:226`、`user.go:853`、`wechat.go:250`、`github.go:300` | 未注册路由的死 handler（EmailBind 还带"跳过邮箱占用检查"隐患） | |
| `middleware/rate-limit.go:149`、`auth.go:162` | Download/UploadRateLimit 未挂载（但设置页仍暴露 4 个无效配置项）、TokenOnlyAuth | |
| `utils/` | `math.go`(IntMax/Max 互为重复且等价内建 max)、browser.go、format.go、html.go、network.go、uuid.go、value.go(Interface2String) | 6 文件 8 函数 |
| `internal/dnsworker` | `source.go:312-389` ECS 包装群、`server.go:310-322`、`scheduler.go:449`、`utils/security/verification.go:90` | |
| `dushengcdn_agent/internal/config/config.go:603` | `firstNonEmpty`；`ExecutorOptions` 13 字段仅用 2 个且 main.go 两次完整构造 | |
| `web/features/shared/components/module-page-skeleton.tsx` 等 | ModulePageSkeleton / PublicRoutePlaceholder / FeaturePlaceholder 死链三文件 | 3 文件 |
| `model/main.go:424-435` | 不可达的分片迁移分支（见 B3） | |

### 重复实现（改一处漏一处的风险点）

| 内容 | 位置 | 建议 |
|------|------|------|
| Basic Auth 哈希 + 盐常量 ×3 | `service/openresty/access_renderer.go:57+12`、`model/proxy_route_normalized.go:931+16`、`service/proxy_route.go:955` | 收敛单一来源（任一处不同步 = Basic Auth 静默全失效） |
| 路由规则加载 ×2 | `service/proxy_route.go:1330` vs `config_version.go:1136`（~80 行逐行相同） | 共享加载器（与 A11 一并解决） |
| GSLB 池匹配循环 ×4 | `gslb_scheduler.go:267/485/554/584` | 抽 matchPoolSource 返回匹配维度 |
| 节点 IP 按记录类型过滤 ×3 | `proxy_route_dns.go:514`、`gslb_scheduler.go:360`、`gslb_policy.go:303` | 抽 nodeDNSContents(node, recordType) |
| DDoS/默认池目标选择 ×2 | `proxy_route_dns.go:333` vs `:356`（仅池名与 Reason 不同） | 合并传参 |
| 分片迁移器 ×3 + dedup-insert 成对复制 | `model/migrations.go:909/948/987` | 泛型 `migrateLegacyShardRows[T]`（已有 `queryAcrossShards[T]` 先例） |
| postgres ALTER 块 ×7 | `model/migrations.go:2146/2190/2227/2256/2288/2329/2369` | 抽 addPostgresColumnsIfMissing |
| controller 内联 parseUint + 手写响应 ×30 | proxy_route.go ×6、tls_certificate.go ×7、node.go ×9 等 | 统一 `parseUintParam` + `respondX`（均已存在） |
| agent runtime 配置读/写/恢复 ×4 套 | `dushengcdn_agent/internal/nginx/manager.go:1320-1404/1485-1555/1593-1647`（~190 行只差文件名）+ 文件名清单第 4 处重复 | 泛化为 3 个助手 + 单一常量切片 |
| sync 服务"已最新"/"blocked"块 ×2 | `internal/sync/service.go:94-124` vs `:193-226` | 提取助手 |
| tryRegister 手工重演心跳序列 | `internal/agent/runner.go:1063-1083` | 直接调 performHeartbeatCycle(ctx, id, true) |
| isPublicIP / minInt 重复内建或现有实现 | `internal/dnsworker/scheduler.go:709`、`source.go:401` | 复用 iputil.IsPublic / 内建 min |
| 前端 getErrorMessage ×14、FeedbackState ×14 | about-page.tsx:18 等 | lib/utils 共享 + 复用 ToastTone |
| `service/origin_helpers.go:114` | formatOriginHost if/else 两分支完全相同 | 删除无效条件 |

### 结构性

- 前端三个巨型单文件组件合计 14,895 行（authoritative-dns-page.tsx 6086 / proxy-route-config-page.tsx 4621 / settings-page.tsx 4188），约占前端代码 1/3；文件内已有子组件划分，按面板拆文件即可，纯重组无逻辑改动。
- `service/authoritative_dns.go` 7667 行"上帝文件"，建议按 Zone/Record CRUD、Worker 心跳、快照、可观测性拆分。
- `common/init.go:36-192`：import 期执行 `flag.Parse()`、`os.Exit`、建目录等重副作用，任何传递依赖 common 的二进制都被波及 → 移入显式 Initialize() 由 server main 调用。

---

## 五、做得好的地方（保持）

- 出站 HTTP 统一走带超时的 `security.NewPublicHTTPClient`；Cloudflare 客户端有 15s 超时与响应体限读。
- 批量查询意识：`ByIDs` 系列、`runConcurrentQueries`、`GetLatestApplyLogsByNodeIDs`、证书 render context 预加载。
- 鉴权覆盖完整：全部管理组挂 AdminAuth/RootAuth + CSRF；agent/DNS-worker 各自 token 认证 + ristretto 正/负缓存；未发现越权端点。
- Agent 安全细节：SSRF 防护、发布签名校验、日志脱敏、GeoIP temp+rename 原子落盘、WS 重连退避。
- DNS 镜像下载有大小校验与原子换目录；DNSSEC 私钥落库前加密。
- 前端统一 apiRequest + react-query（共享 queryKey、staleTime、focus 不重拉），事件清理规范，图表懒加载。

---

## 六、建议处理顺序

1. **提交前**：修 A1-A3、A17（可炸生产/丢数据），顺手 A4-A11（均在本次改动文件内，改动成本低）。
2. **第一批存量修复**：B1-B4（DNS 空快照、丢失更新、迁移丢数据、Agent 非原子写）+ 乱码文案全局修复。
3. **第一批性能**：P1 心跳路径（GeoIP 单例、保留期清理移出、轻量列查询）+ P3 Cloudflare 对账预载 + P2 worker 心跳 N+1——这三组是随节点/路由规模线性恶化的。
4. **第二批性能**：P4 渲染管线（DNS 解析超时+记忆化、规则批量加载）+ P5 DNS worker 每查询路径 + P6 日志查询索引化。
5. **清理**：死代码删除（低风险大收益）→ 重复实现收敛（Basic Auth 哈希优先，属正确性风险）→ 巨型文件拆分。
