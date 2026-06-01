# 后续完善清单

你会学到：当前项目接下来更适合补强哪些地方、为什么排这个优先级，以及每项完成时应该用什么证据验收。

本清单基于当前仓库状态和 `docs/reference/audit-report.md`，用于后续接手时避免在已完成能力里反复打转。每次完成或调整方向时，应同步更新本页。

## 高优先级

| 项目 | 当前状态 | 验收证据 |
| --- | --- | --- |
| Server Compose 本地参数隔离 | 已提供 `dushengcdn_server/.env.example`，`dushengcdn_server/docker-compose.yaml` 已改为读取 `.env` 变量；后续继续观察用户升级是否还会直接改仓库模板 | 文档说明可执行；源码 Compose 更新时不需要修改仓库内 `docker-compose.yaml` |
| 文档构建进入 CI 或本地一键验证 | 已新增 `.github/workflows/docs-ci.yml`，在 `docs/**` 或该 workflow 变化时安装依赖并执行 `pnpm build`；本地开发文档也补充了同样命令 | GitHub Actions `Docs CI` 通过；本地 `cd docs && pnpm install --frozen-lockfile && pnpm build` 通过 |
| 升级后旧 Agent 兼容窗口 | Server 已兼容旧全局 `AGENT_TOKEN` 的 HTTP 心跳、配置拉取和应用日志上报；旧 Token 只允许绑定已有 `node_id` 且不覆盖专属 Token 节点 | `go test ./...` 覆盖旧 Agent 兼容用例；排障文档说明旧 Token 迁移路径 |

## 中优先级

| 项目 | 当前状态 | 验收证据 |
| --- | --- | --- |
| 备份恢复脚本或管理端备份提示 | 已新增 `scripts/backup-server.sh`，支持 PostgreSQL Compose 与 SQLite 备份，并在部署、升级、完整教程和 CLI 参考中补充用法；恢复仍保持人工停 Server 后执行 | `bash -n scripts/backup-server.sh` 通过；SQLite 试跑生成数据库备份、数据目录归档和 `manifest.txt` |
| root 密码重置端到端 CLI 回归 | 已新增 `dushengcdn_server/reset_root_cli_test.go`，通过 `go run . --reset-root-password ...` 覆盖创建 root、拒绝旧密码、以及重置被禁用/降级 root 的完整命令入口 | `cd dushengcdn_server && go test . -run TestResetRootPasswordCLI -count=1` 和 `go test ./...` 通过 |
| 网站配置分区前端测试补强 | 已覆盖列表入口、分区展开、创建网站、域名、反向代理节点池、自动 DNS/GSLB、权威 DNS、WAF 和地区限制保存回归；后续可继续补 PoW、Basic Auth、缓存保存失败和加载态 | `cd dushengcdn_server/web && pnpm test -- tests/unit/proxy-routes-page.test.tsx` 通过 |
| 权威 DNS 调度评分增强 | 当前 Agent 多点探测默认只用于观测；启用门槛后参与候选过滤和同等候选排序 | 如扩展 RTT、丢包、区域覆盖评分，先更新 `docs/design/authoritative-dns-gslb.md`，再补 Worker/Server/前端测试 |

## 低优先级

| 项目 | 当前状态 | 验收证据 |
| --- | --- | --- |
| 英文文档同步中文新增内容 | 中文文档优先覆盖主要使用路径，英文页面存在滞后 | 英文 `docs/en/**` 与中文关键部署、升级、排障内容一致 |
| 示例 Compose 细化 | 已有 Server、Agent、DNS Worker Compose 模板 | 按不同部署场景补充最小生产模板、源码构建模板和 override 示例 |

## 维护规则

* 已完成项目应移动到对应审计报告或在本页标注完成证据，不继续作为开放待办。
* 新增超出当前产品边界的项目，应先更新设计文档。
* 每次修改部署、升级、配置或排障路径时，同步更新 README、指南和配置参考。
