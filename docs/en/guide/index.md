# Guide

You will learn how the DuShengCDN documentation is organized, which pages to read for a first run, and where to find deployment, usage, troubleshooting, and development information.

DuShengCDN is a self-hosted OpenResty control plane. It brings reverse proxy site configuration, immutable releases, Agent-based node sync, TLS certificates, and basic observability into one management UI for a single team or organization.

## Recommended Path

If you are new to DuShengCDN, read these pages in order:

1. [Quick Start](./quick-start.md): start the Server with Docker Compose, sign in, and connect the first Agent.
2. [Usage](./usage.md): learn common operations for sites, origins, certificates, releases, rollbacks, and observability.
3. [Deployment](./deployment.md): run the Server and Agent in an environment closer to production.
4. [Configuration](../reference/configuration.md): look up Server environment variables, runtime options, and Agent configuration fields.
5. [Troubleshooting](./troubleshooting.md): debug login, database, node sync, OpenResty apply, and frontend build issues.

## Find by Role

| Goal | Start Here |
| --- | --- |
| Run the management UI in a few minutes | [Quick Start](./quick-start.md) |
| Publish the first reverse proxy site | [Publish First Site](./first-site.md) |
| Connect or reinstall a node Agent | [Connect Agent](./agent.md) |
| Start the Server from source | [Run Server](./server.md) |
| Configure GitHub or OIDC login | [SSO Login](./sso.md) |
| Upgrade the Server or Agent | [Upgrade and Maintenance](./upgrade.md) |
| Contribute code or fix issues | [Local Development](./development.md) and [Development Constraints](../design/development.md) |
| Understand architecture and releases | [Architecture](../design/architecture.md) and [Release Model](../design/release-model.md) |

## Documentation Areas

`guide/` is for users and operators. It provides executable steps from installation to daily operations.

`reference/` collects stable facts, such as configuration fields, commands, API conventions, and repository layout.

`design/` is for maintainers and contributors. It describes product boundaries, architecture, release model, and engineering constraints. Update the related design page before implementing changes that alter those boundaries.
