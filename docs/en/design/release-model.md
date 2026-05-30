# Release Model

You will learn: Why DuShengCDN uses a complete configuration version as the release unit, and how publishing, activation, Agent application, and rollback work.

DuShengCDN's release model is centered on complete configuration versions rather than modifying node configurations online.

Standard link:

```text
Modify rules -> Preview / View diff -> Publish -> Generate complete configuration version -> Activate version -> Agent pulls -> Local application -> Report result
```

## Publishing Rules

When publishing, the Server must:

1. Read all enabled `proxy_routes`.
2. Read the OpenResty main configuration template, performance parameters, cache parameters, and necessary Lua resources on the Server side.
3. Read domain and certificate binding relationships.
4. Render the complete OpenResty configuration.
5. Calculate the `checksum`.
6. Write to `config_versions`.
7. Switch the activated version.
8. Let the Agent discover and apply it in subsequent heartbeats.

The version number format is fixed as `YYYYMMDD-NNN`.

## Preview and Publishing

Preview and diff are read-only capabilities and do not generate release records.

Publishing generates a new complete configuration version. The version must contain sufficient information so that future rollbacks can be re-applied based on historical snapshots, without relying on current mutable configurations.

## Activating Version

There can only be one activated version globally at a time.Differentiated versions grouped by nodes are currently not supported.

The Agent obtains the activated version summary through the heartbeat; only when the remote version or checksum is inconsistent with the local state does the Agent enter the synchronization flow.

## Immutable History

Historical versions are immutable. Rollback is not achieved by modifying older versions, but by reactivating older versions.

The result of doing this is:

* Every version can be traced back.
* The rollback link is consistent with the ordinary release application link.
* The Agent does not need to understand "reverse patch", but only needs to apply a target version.

## Agent Application Policy

When discovering a new version, the Agent will:

1. Pull the details of the target version.
2. Back up old files.
3. Write the main configuration, route configurations, certificates, and necessary Lua resources.
4. Execute OpenResty configuration verification.
5. reload; if it is not started during runtime, try to start OpenResty with the current configuration.
6. Report success, warning, or failure.

If the activation of the new configuration fails, the Agent must try to restore execution; report a warning when the rollback succeeds, and report a failure when it still cannot recover after rollback.

Once a target `version + checksum` application fails and rolls back, the Agent will block repeated applications of this target in its local state. Only when the remote activated version or checksum changes is it allowed to try again.

## Design Constraints

* Publishing must read all enabled site configurations, rather than only rendering the modified object this time.
* Rollback is achieved by reactivating older versions, without modifying historical versions.
* The Agent API is fixed to use the node-exclusive `agent_token`; the first access can use the `discovery_token`.
* The Server does not provide remote shell or arbitrary command execution entries.
* The configuration version must save complete snapshots, rendering results, and `checksum`.
