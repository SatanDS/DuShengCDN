# DuShengCDN 项目续点记录 2026-06-02

## 本次目标

对当前项目做一次面向上线的审核和收口：修复本地自建解析、DNS 响应端、自动 DNS、证书申请、攻击防护等流程中的明显断点；把面板里偏专业的词尽量换成更直观的中文，并用 `!` 说明图标承载白话解释；补充关键测试，最后提交并推送。

## 已完成重点

- 证书申请支持选择 `本地自建解析（权威 DNS）`，可选择 `本地托管域名`，后端 ACME DNS-01 会临时写入 `_acme-challenge` TXT 并清理。
- 证书申请弹窗在网站详情打开时会自动带入当前网站域名和证书名称，并按域名自动匹配托管域名。
- 如果网站配置已经切到 `本地自建解析` 并绑定托管域名，从网站详情点 `申请证书` 会默认选中本地自建解析和同一个托管域名；托管域名列表异步加载完成后也会回填下拉框，避免显示为空。
- 如果从证书页直接点 `申请证书`，输入的证书域名能匹配已启用的本地托管域名时，弹窗会自动切换到 `本地自建解析（权威 DNS）` 并选中最匹配的托管域名，避免用户只能看到 Cloudflare 账号。
- 网站详情将“申请证书”和“导入证书”拆成两个入口，避免用户把上传证书和 ACME 申请混在一起。
- 证书页文案继续统一：空状态明确提示可 `导入证书` 或 `申请证书`，证书来源把 `ACME 申请` 显示为更直观的 `自动申请`，导入弹窗标题统一为 `导入证书`。
- 证书申请弹窗继续新手化：`权威 DNS 托管域名` 改为 `本地托管域名`；验证方式保留 `本地自建解析（权威 DNS）`，让用户能明确找到权威 DNS 申请入口；`ACME`、`DNS-01`、`CNAME` 等术语从主文案中移出，改由 `!` 说明解释。
- 证书导入、编辑和详情继续统一：主文案使用 `证书内容`、`私钥内容`，`PEM`、文件后缀和私钥匹配等细节放到 `!` 说明里；校验错误也改为 `请输入证书内容`、`请输入私钥内容`、`请选择本地托管域名`。
- DNS 记录 A/AAAA 创建改成一个输入框一个 IP，可点 `+` 增加，MX 优先级加了白话说明。
- 左侧入口与多数用户可见文案统一为 `本地自建解析`、`DNS 响应端`、`解析配置`、`按压力优先`、`攻击防护模式`、`API 密钥`。
- 通用 `!` 说明图标保留悬停/聚焦说明，但不再抢占表单控件 label，避免辅助标签和测试误判。
- 左侧入口继续简化：`DNS 账号` 改为 `Cloudflare 账号`，`节点/IP池` 改为 `节点和IP池`，`OpenResty配置` 改为 `代理服务配置`；Cloudflare 账号页同步替换成功提示、删除确认、空状态和 API 密钥说明。
- 网站配置的自动解析域名创建/编辑文案继续新手化：主界面使用 `解析模式`、`Cloudflare 账号`、`解析缓存时间`、`多节点智能解析`、`最大处理器压力`、`清洗池` 等直观中文，`DNS`、`TTL`、`GSLB`、`CNAME`、`CPU` 等术语放进 `!` 说明里解释。
- 本地自建解析页继续白话化：`网站 DNS 模式` 改成 `网站解析模式`，迁移提示中的 `自动 DNS` 改成 `自动解析域名`，模拟结果的 `TTL` 显示为 `缓存时间`；后端返回的 `GSLB 阈值`、`OpenResty 健康`、`DNS Worker 多点探测` 等诊断原因在面板上会转换成 `压力上限`、`代理服务是否正常`、`响应端多地探测` 等说明。
- 设置页本地解析运行参数补充解释：`节点数据有效期` 改成 `节点状态数据有效时间`，明确它不是健康检查间隔，而是节点上报的连接数、处理器和内存数据多久还可用于按压力选 IP；`Agent 探测调度门槛` 改成 `按响应端探测结果筛选节点`。
- 复制部署脚本/命令的剪贴板工具继续增强：当浏览器没有提供 `Clipboard API` 或自动复制失败时，会明确提示当前页面不是 HTTPS/安全上下文不足或浏览器未提供剪贴板写入接口，不再暴露 `writeText` undefined 这类技术报错。
- 自动 DNS 的攻击防护三段式配置已落地：关闭/自动，提供方 Cloudflare/自定义，目标随提供方变化。
- 面板部署脚本默认可自动创建并安装同机 DNS Worker，且安装前检查本机是否已有 Worker，避免覆盖。
- DNS Worker 诊断脚本、部署文档、配置文档已补充同机部署、端口、快照、排障路径。

## 已验证

在 `D:\DushengCDN` 当前工作树执行通过：

- `git diff --check`
- `cd dushengcdn_server/web; pnpm lint`
- `cd dushengcdn_server/web; pnpm tsc --noEmit --pretty false`
- `cd dushengcdn_server/web; pnpm vitest run tests/unit/navigation-utils.test.ts tests/unit/proxy-routes-page.test.tsx tests/unit/authoritative-dns-page.test.tsx tests/unit/certificate-apply-modal.test.tsx tests/unit/website-detail-page.test.tsx`
- `cd dushengcdn_server/web; pnpm vitest run tests/unit/certificate-apply-modal.test.tsx tests/unit/website-detail-page.test.tsx`
- `cd dushengcdn_server/web; pnpm vitest run tests/unit/certificate-apply-modal.test.tsx tests/unit/website-detail-page.test.tsx --reporter=dot`
- `cd dushengcdn_server/web; pnpm vitest run tests/unit/proxy-routes-page.test.tsx --reporter=dot`
- `cd dushengcdn_server/web; pnpm vitest run tests/unit/navigation-utils.test.ts --reporter=dot`
- `cd dushengcdn_server/web; pnpm vitest run tests/unit/authoritative-dns-page.test.tsx --reporter=dot`
- `cd dushengcdn_server/web; pnpm vitest run tests/unit/clipboard-utils.test.ts --reporter=dot`
- `cd dushengcdn_server/web; pnpm tsc --noEmit --pretty false`
- `cd dushengcdn_server/web; pnpm lint`
- `cd dushengcdn_server; go test ./model`
- `cd dushengcdn_server; go test ./internal/dnsworker`
- `cd dushengcdn_server; go test ./service`
- `cd dushengcdn_server; go test ./...`

Go 测试在 Windows 下使用 `GOCACHE=D:\DushengCDN\.go-cache`。

## 线上升级提醒

源码 Compose 面板建议使用：

```bash
cd /opt/dushengcdn
git fetch origin main
git pull --ff-only origin main
cd dushengcdn_server
cp -n .env.example .env
DUSHENGCDN_VERSION="$(git describe --tags --always --dirty)" docker compose --env-file .env up -d --build
docker compose ps
panel_port="$(grep -E '^DUSHENGCDN_HTTP_PORT=' .env | tail -n1 | cut -d= -f2-)"
curl -I "http://127.0.0.1:${panel_port:-3010}/api/status"
```

如果只想用一体化脚本并允许它默认检查/安装同机 DNS 响应端：

```bash
cd /opt/dushengcdn
git pull --ff-only origin main
bash scripts/install-server.sh
```

若服务器已经手动部署过 DNS 响应端，脚本会先检查并跳过；确认要覆盖时才加 `--force-dns-worker-reinstall`。

## 继续时优先检查

1. 线上面板升级后，确认浏览器里 `申请证书 -> 验证方式 -> 本地自建解析（权威 DNS）` 可见，并能列出已启用托管域名。
   - 如果从某个网站详情进入申请证书，且该网站配置已使用本地自建解析，弹窗应自动选中本地自建解析和对应托管域名。
2. 确认左侧 `本地自建解析` 中至少存在匹配域名的启用 Zone，例如申请 `www.example.com` 时需要 `example.com`。
3. 如果权威 DNS 仍不生效，先在 Worker 主机跑 `scripts/diagnose-dns-worker.sh --public-ip PUBLIC_IP --zone example.com`，重点看 UDP/TCP 53、公网地址、快照和日志。
4. 如果网站自动 DNS 不能解析到本地自建解析，检查网站详情的 `DNS 模式` 是否为 `本地自建解析`，托管域名是否匹配，在线 DNS 响应端是否有未过期且一致的解析配置。
5. 文档里仍保留部分 `DNS Worker`、`GSLB`、`DDoS` 作为技术名词和配置字段说明；后续如果继续做新手化文案，可优先改 README/usage 中的用户路径段落。

## 注意

- 不要回退当前工作树里的用户/前序改动。
- 数据库结构已提升版本并带迁移，继续改表结构时必须同步 `model/database_schema_version.go` 和显式迁移测试。
- DNS Worker 查询路径不能访问数据库或外部 GeoIP HTTP API，只能使用本地只读快照。
- Agent 仍不能暴露远程 shell 或任意端口扫描；DNS Worker 多点探测只能使用 Server 下发的结构化 UDP/TCP 53 目标。
