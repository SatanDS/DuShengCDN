# Repository Layout

You will learn: What the Server, Agent, frontend, scripts, and documentation directories in the DuShengCDN repository are responsible for, and which layer to place your logic when contributing code.

| Path | Responsibility |
| --- | --- |
| `dushengcdn_server` | Gin + GORM + SQLite/PostgreSQL monolithic control plane |
| `dushengcdn_server/web` | Next.js 15 App Router management console frontend, statically exported and hosted by the Go Server |
| `dushengcdn_agent` | Go monolithic Agent, running on the node side |
| `scripts` | Helper scripts such as Agent installation, uninstallation, etc. |
| `docs` | VitePress documentation site, design baseline, development constraints, deployment, and configuration documents |

## Server Layering

| Directory | Responsibility |
| --- | --- |
| `controller/` | Parameter parsing, calling services, returning responses |
| `service/` | Business logic, verification, transaction orchestration, configuration rendering |
| `model/` | Model definition, database version, and migration |
| `router/` | Route registration |
| `middleware/` | Auth, authorization, rate limiting, and other cross-cutting logic |
| `common/` | Configuration, global state, and initialization entry points |
| `utils/` | Pure utility functions and general helpers |

## Agent Modules

| Module | Responsibility |
| --- | --- |
| `config` | Configuration reading and default values |
| `heartbeat` | Heartbeat and version summary judgment |
| `sync` | Configuration pulling and application orchestration |
| `nginx` / `openresty` | OpenResty file writing, verification, reload, startup, and rollback |
| `state` | Local state and observability supplementary reporting buffer |
| `httpclient` | Server communication |
| `protocol` | Agent API protocol types |
| `internal/updater` | Agent self-updating |

## Frontend Layering

| Directory | Responsibility |
| --- | --- |
| `app/` | Routes, layouts, page assembly |
| `features/` | Organize modules by business domains |
| `components/` | Reuse components across features |
| `lib/` | Request client, environment variables, utility functions, constants |
| `store/` | A small amount of cross-page UI state |
| `types/` | Shared type definitions |
