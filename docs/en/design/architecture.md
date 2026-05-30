# Architecture

DuShengCDN consists of Server, Agent, and local OpenResty on each node.

```text
DuShengCDN Server (Gin + SQLite/PostgreSQL + Web UI)
        |
        | HTTP API / Config Pull
        v
DuShengCDN Agent (register / heartbeat / sync / apply / update)
        |
        v
OpenResty binary
        |
        v
Origin
```

## Server

`dushengcdn_server` is a monolithic control plane based on Gin, GORM, SQLite/PostgreSQL, the existing login/session system, and the static frontend build.

It owns the admin UI and API, Agent API, configuration rendering, version publishing, storage, and aggregate queries.

## Agent

`dushengcdn_agent` is a single Go binary that runs on each node. It controls OpenResty through `openresty_path`, or `openresty` by default. Docker deployments use an Agent image that already includes OpenResty and follows the same binary-control flow.

It handles registration, heartbeat, sync, file writes, `openresty -t`, reload, rollback, self-update, and lightweight collection.

## Frontend

`dushengcdn_server/web` is the production frontend baseline: Next.js App Router, React 19, TypeScript, and Tailwind CSS.
