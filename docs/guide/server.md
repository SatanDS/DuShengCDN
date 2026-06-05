# 启动 Server

你会学到：如何从源码构建管理端前端、启动 DuShengCDN Server、选择 SQLite 或 PostgreSQL，并访问 Swagger。

DuShengCDN Server 是 Gin + GORM 单体控制面，负责管理端 UI、管理 API、Agent API、配置渲染、版本发布、数据存储与聚合查询。

## 前置条件

| 项目 | 要求 |
| --- | --- |
| Go | `1.25+` |
| Node.js | `18+` |
| pnpm | 推荐通过 `corepack enable` 使用项目声明的 pnpm |
| 数据库 | SQLite 文件目录可写，或可访问的 PostgreSQL 实例 |

生产环境建议显式配置 `SESSION_SECRET`，并优先使用 PostgreSQL。

## 构建管理端前端

Go Server 会托管 `dushengcdn_server/web/build` 中的静态产物。源码启动前先构建前端：

```bash
cd dushengcdn_server/web
corepack enable
pnpm install
pnpm build
```

常用前端检查：

```bash
pnpm lint
pnpm typecheck
pnpm test
```

## 使用 SQLite 启动

```bash
cd dushengcdn_server
export SESSION_SECRET="$(openssl rand -hex 32)"
export SQLITE_PATH='./dushengcdn.db'
export LOG_LEVEL='info'
go run .
```

默认监听 `3000` 端口，访问：

```text
http://localhost:3000
```

## 使用 PostgreSQL 启动

```bash
cd dushengcdn_server
export SESSION_SECRET="$(openssl rand -hex 32)"
export DSN='postgres://dushengcdn:secret@127.0.0.1:5432/dushengcdn?sslmode=disable'
export LOG_LEVEL='info'
go run .
```

`DSN` 设置后优先于 SQLite。`DSN` 与兼容旧命名的 `SQL_DSN` 同时存在时，优先使用 `DSN`。

如果目标 PostgreSQL 数据库为空且本地 `SQLITE_PATH` 文件存在，Server 启动阶段会尝试把 SQLite 数据迁移到 PostgreSQL，并在日志中输出迁移进度。

## 命令行参数

```bash
go run . --port 3000 --log-dir ./logs
```

| 参数 | 作用 | 默认值 |
| --- | --- | --- |
| `--port` | 指定 Server 监听端口 | `3000` |
| `--log-dir` | 指定日志目录 | 空，输出到标准输出 |
| `--reset-root-password` | 重置 `root` 用户密码后退出，不启动 HTTP 服务 | 空 |
| `--version` | 输出版本后退出 | `false` |
| `--help` | 输出帮助后退出 | `false` |

离线重置 root 密码：

```bash
cd dushengcdn_server
export DSN='postgres://dushengcdn:secret@127.0.0.1:5432/dushengcdn?sslmode=disable'
./dushengcdn-server --reset-root-password 'replace-with-new-password'
```

如果使用 SQLite，把 `DSN` 换成实际 `SQLITE_PATH`。执行前建议先停止正在运行的 Server。

## 首次登录

默认账号：

| 用户名 | 密码 |
| --- | --- |
| `root` | `123456` |

首次登录后请立即修改默认密码。

## Swagger

登录管理端后访问：

```text
http://localhost:3000/swagger/index.html
```

本地重新生成 Swagger：

```bash
go install github.com/swaggo/swag/cmd/swag@v1.16.4
cd dushengcdn_server
swag init -g main.go -o docs
```

Swagger 生成文件位于 `dushengcdn_server/docs`。
