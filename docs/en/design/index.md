# Product Boundary

You will learn: What DuShengCDN is, what problems it solves, who the target users are, what the current stable capabilities are, and which design boundaries cannot be bypassed during implementation.

DuShengCDN is a self-hosted OpenResty control plane oriented toward single-team or single-organization internal operation and maintenance (O&M) scenarios. It resolves the issues of scattered management in reverse proxy configuration, node synchronization, certificate hosting, configuration release/rollback, and basic observability.

## Project Positioning

DuShengCDN is suitable for teams that need to centrally manage multiple OpenResty proxy nodes:

* Want to maintain reverse proxy site configurations using a management console.
* Want every configuration change to have a complete version, preview, activation, and rollback.
* Want nodes to actively sync configuration, rather than having the control plane SSH into nodes to execute commands.
* Want to manage TLS certificates, domain assets, node statuses, and basic access analytics within the same system.

DuShengCDN is currently not positioned as a general-purpose log platform, service mesh, Kubernetes Ingress Controller, or multi-tenant cloud platform.

## Target Users

| User | Needs |
| --- | --- |
| Self-hosted users | Quickly deploy a visual OpenResty control plane |
| Internal O&M teams | Manage multiple reverse proxy nodes, certificates, and configuration versions |
| Development teams | Provide a unified entry point and basic access analytics for internal services |
| Contributors | Fix defects, strengthen tests, and improve documentation within clear boundaries |

## Current Stable Capabilities

| Capability | Description |
| --- | --- |
| Reverse Proxy Rule Management | Uses site configuration as the aggregation boundary, supporting multi-domain and origin configuration |
| Site-level Configuration | One rule corresponds to one site, which can bind one or more domains and share site-level configuration |
| Origin Management | Maintains a lightweight origin directory and allows sites to save renderable origin snapshots |
| Configuration Versioning | Supports preview, publishing, activation, immutable history, and rollback |
| Agent Synchronization | Supports registration, heartbeat, synchronization, application result reporting, and self-updating |
| OpenResty Hosting | Manages main configuration templates, performance parameters, cache parameters, and Lua resources |
| HTTPS/TLS | Hosts certificates and domain assets, and binds certificates on a per-domain basis |
| Basic Observability | Aggregates node requests, resource snapshots, health events, and access analytics |
| Node Management | Node status, token systems, deployment, and update links |
| Console Frontend | Next.js-based official management console |
| Auth Source Login | Supports configuring GitHub and standard OIDC login entries as authentication sources, allowing third-party accounts to bind to existing local users |

Default working method:

* All nodes consume the same globally activated version.
* The Server saves configuration and status, and does not directly manage nodes via SSH.
* The Agent is the only controlled landing entry point on the node side.

## Typical Use Cases

| Scenario | Description |
| --- | --- |
| Unified Entry for Internal Services | Expose multiple internal HTTP services through a unified domain and certificate |
| Config Sync for Multi-node Reverse Proxy | Multiple OpenResty nodes consume the same activated configuration |
| Config Change Review | View preview or diff before publishing, and retain immutable history after publishing |
| Quick Rollback | Reactivate an older version, letting the Agent pull and apply it |
| Certificate Hosting | Bind TLS certificates for different domains |
| Basic Observability | View node status, request aggregation, access analytics, and health events |

## Core Objects

Currently active entities:

* `proxy_routes`
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

## Site Configuration Constraints

`proxy_routes` is upgraded from a "single-domain rule" to a "site configuration" aggregate object. One record corresponds to one website, which can bind one or more domains and share a set of site-level configurations.

Constraints:

* `proxy_routes.site_name` is the unique business identifier of the website.
* `proxy_routes.domains` contains at least one domain, and `domains[0]` is used as the primary domain.
* Any domain can globally belong to only one `proxy_routes`.
* During the migration period, `proxy_routes.domain` can be kept as a mirror field of `domains[0]`, but business read/write and subsequent extensions must be based on `site_name` + `domains`.
* Site-level rate limits, reverse proxies, and cache configurations are currently shared by site and are not configured differently on a per-domain basis within the same website.
* HTTPS allows binding certificates per domain within the same site.

## Origin Constraints

`origins` only saves the origin address, display name, and remarks, and does not carry protocols, ports, paths, weights, or health check policies.

`proxy_routes` can optionally associate an `origins` record to reuse the origin address; the rule still saves a complete `origin_url` snapshot to participate in rendering and version snapshots.

Upstream constraints:

* `proxy_routes` must contain at least one upstream address.
* To maintain compatibility with historical data, the `origin_url` main upstream field is retained, and multiple upstreams are allowed to be added within the same rule for load balancing.
* Upstreams are rendered uniformly as a named `upstream` with keepalive.
* A single upstream can carry a base path or query and append it in `proxy_pass`.
* Multiple upstreams are restricted to pure `scheme://host[:port]`.
* `proxy_routes.origin_host` is an optional field, used to override the `Host` request header when back-origin.
* All upstream addresses must be legal `http://` or `https://`.

## HTTPS Constraints

`proxy_routes.domain_cert_ids` is used to record domain-certificate bindings parallel to `domains`; a value of `0` indicates that HTTPS is not enabled for the domain, retaining only HTTP.

During publishing rendering:

* Domains with certificates are output as separate `443 ssl` `server` blocks grouped by certificate.
* Domains not bound to a certificate must not be automatically brought into HTTPS.
* All domains in `proxy_routes.domains` must be included in the same site configuration to avoid the same site being split in version snapshots.

## Authentication Source Constraints

`auth_sources` is the configuration object for third-party login entries on the management console, currently supporting only two types: `github` and `oidc`. Enabled authentication sources will be displayed on the login page.

`external_accounts` saves the binding relationship between external accounts of authentication sources and local users. When a third-party account logs in for the first time:

* If it is bound to a local user, it logs in directly.
* If there is an existing local login session, it binds to the current user.
* If it is not bound and registration is allowed, a normal user is automatically created and bound.
* If it is not bound and registration is closed, the user is only allowed to enter an existing local account and password to complete the binding.

The old `users.github_id` only serves as a source for upgrade migration; new third-party account login and binding relationships must be based on `external_accounts`.

## Version and Observability Constraints

* `config_versions` must save complete snapshots, rendering results, and `checksum`.
* There can only be one activated version globally at a time.
* Rollback is achieved by reactivating older versions.
* `nodes` only carries control plane status and low-frequency summaries, not high-frequency observability facts.
* Metrics, trends, and access analytics prioritize server-side aggregation results, rather than temporary frontend statistics.
* Access details are only retained for controlled time windows, not evolving into a general-purpose log platform.

## Documentation Maintenance Principles

* Update this document when the product scope or system boundary changes.
* Update [System Architecture](./architecture.md) when the system structure or module responsibilities change.
* Update [Release Model](./release-model.md) when the release, synchronization, or rollback model changes.
* Update [Development Constraints](./development.md) when development constraints, code specifications, or interface conventions change.
* Update [Deployment Guide](../guide/deployment.md) and README when deployment methods change.
* Update [Configuration Reference](../reference/configuration.md) when configuration items change.
* Completed phases will no longer be backfilled in the form of "version plans".
* Before starting a new phase, complete the design first, then enter implementation.
