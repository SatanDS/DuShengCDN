# Publishing Your First Configuration

You will learn: How to create your first site configuration, bind origins and certificates, publish a configuration version, and confirm that the Agent has applied it.

DuShengCDN's release link is centered on complete configuration versions. After modifying site configurations on the management console, you need to publish and activate the new version before the Agent pulls and applies it in subsequent heartbeats.

## Pre-release Check

Confirm that the following conditions are met:

| Project | Expectation |
| --- | --- |
| Server | Can log into the management console |
| Agent | At least one node is online |
| Origin | The Agent node can access the origin address |
| Domain | The domain has been resolved to the OpenResty node, or you are ready to verify via local hosts / curl Host header |
| HTTPS | If HTTPS is required, the certificate has been uploaded or hosted |

## Create Site Configuration

When adding a site configuration on the management console, you need at least:

| Field | Description |
| --- | --- |
| Site Name | Unique business identifier; defaults to the primary domain when omitted |
| Domains | At least one domain; the first item is treated as the primary domain |
| Origin URL | Valid `http://` or `https://` upstream address |
| Enabled | Only enabled site configurations participate in release rendering |

Example:

| Field | Example |
| --- | --- |
| Site Name | `app` |
| Domains | `app.example.com` |
| Origin URL | `http://10.0.0.20:8080` |

A domain can belong to only one site configuration. Site-level rate limits, reverse proxies, and cache configurations are shared by site.

## Bind Certificates

HTTPS certificates are bound per domain. Domains not bound to certificates will not be automatically placed in `443 ssl` server blocks.

If a site contains multiple domains, the publishing rendering will generate HTTPS configurations grouped by certificate and ensure all domains still belong to the same site snapshot.

## Publish and Activate

Standard link:

```text
Modify rules -> Preview / View diff -> Publish -> Generate complete configuration version -> Activate version -> Agent pulls -> Local application -> Report result
```

When publishing, the Server reads all enabled site configurations, OpenResty main configuration templates, performance parameters, and cache parameters, renders the complete OpenResty configuration, calculates the `checksum`, writes to `config_versions`, and then switches the activated version.

## Verify Results

After publishing, confirm on the management console:

| Location | Expected Result |
| --- | --- |
| Node List | Node is online |
| Node Details | The current version is consistent with the activated version |
| Apply Logs | The most recent application succeeded |
| Version Page | The new version is in the activated state |

Confirm the Agent logs on the node:

```bash
journalctl -u dushengcdn-agent -n 100 --no-pager
```

Access using the domain:

```bash
curl -I http://app.example.com
```

If the domain has not been officially resolved yet, you can temporarily specify the Host header to access the node IP:

```bash
curl -I -H 'Host: app.example.com' http://NODE_IP
```

HTTPS verification:

```bash
curl -I https://app.example.com
```

## Rollback

If the target version application fails and rolls back, the Agent will block repeated applications of the same `version + checksum` locally until the activated version or checksum on the control plane changes.

To roll back to an older version:

1. Open the configuration version page.
2. Find the previous confirmed working historical version.
3. Reactivate that version.
4. View the node application records to confirm that the Agent applied it successfully.
