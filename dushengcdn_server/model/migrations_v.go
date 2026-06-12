package model

import (
	"fmt"
	"gorm.io/gorm"
)

// migrateV2 upgrades the legacy schema to the first versioned schema by
// creating schema metadata, applying the current tables, and backfilling
// compatibility columns.
func migrateV2(db *gorm.DB, backend string) error {
	return migrateCurrentSchemaOnly(db, backend)
}

// migrateV3 upgrades observability shard tables from legacy ID layout to the
// current ID-sharded layout and migrates existing shard data into the new tables.
func migrateV3(db *gorm.DB, backend string) error {
	if db == nil {
		return fmt.Errorf("database handle is nil")
	}
	_ = backend
	if err := renameLegacyObservabilityShardTables(db); err != nil {
		return err
	}
	if err := autoMigrateObservabilityShardTables(db); err != nil {
		return err
	}
	if err := migrateLegacyNodeMetricSnapshots(db); err != nil {
		return err
	}
	if err := migrateLegacyNodeRequestReports(db); err != nil {
		return err
	}
	if err := migrateLegacyNodeAccessLogs(db); err != nil {
		return err
	}
	return dropLegacyObservabilityShardTables(db)
}

// migrateV4 introduces the origins schema and backfills proxy route origin
// references from existing origin_url values.
func migrateV4(db *gorm.DB, backend string) error {
	if err := applyCurrentSchema(db, backend); err != nil {
		return err
	}
	return backfillOriginsFromProxyRoutes(db)
}

// migrateV5 upgrades proxy_routes to website-level identity fields by
// backfilling site_name and domains while keeping domain as the primary-domain
// compatibility mirror.
func migrateV5(db *gorm.DB, backend string) error {
	if err := applyCurrentSchema(db, backend); err != nil {
		return err
	}
	if err := backfillOriginsFromProxyRoutes(db); err != nil {
		return err
	}
	if err := backfillProxyRouteSiteFields(db); err != nil {
		return err
	}
	return ensureProxyRouteSiteNameUniqueIndex(db)
}

// migrateV6 adds structured website-level rate limit fields to proxy_routes.
func migrateV6(db *gorm.DB, backend string) error {
	if err := applyCurrentSchema(db, backend); err != nil {
		return err
	}
	if err := backfillOriginsFromProxyRoutes(db); err != nil {
		return err
	}
	if err := backfillProxyRouteSiteFields(db); err != nil {
		return err
	}
	return ensureProxyRouteSiteNameUniqueIndex(db)
}

// migrateV7 adds structured website-level certificate lists to proxy_routes
// while keeping cert_id as the primary certificate compatibility mirror.
func migrateV7(db *gorm.DB, backend string) error {
	if err := applyCurrentSchema(db, backend); err != nil {
		return err
	}
	if err := backfillOriginsFromProxyRoutes(db); err != nil {
		return err
	}
	if err := backfillProxyRouteSiteFields(db); err != nil {
		return err
	}
	if err := ensureProxyRouteSiteNameUniqueIndex(db); err != nil {
		return err
	}
	return backfillProxyRouteCertificateFields(db)
}

// migrateV8 adds per-domain certificate assignments to proxy_routes while
// keeping cert_ids as the website-level compatibility mirror.
func migrateV8(db *gorm.DB, backend string) error {
	if err := applyCurrentSchema(db, backend); err != nil {
		return err
	}
	if err := backfillOriginsFromProxyRoutes(db); err != nil {
		return err
	}
	if err := backfillProxyRouteSiteFields(db); err != nil {
		return err
	}
	if err := ensureProxyRouteSiteNameUniqueIndex(db); err != nil {
		return err
	}
	if err := backfillProxyRouteCertificateFields(db); err != nil {
		return err
	}
	return backfillProxyRouteDomainCertificateFields(db)
}

// migrateV9 adds PoW (Proof-of-Work) anti-bot protection fields to proxy_routes.
func migrateV9(db *gorm.DB, backend string) error {
	if err := applyCurrentSchema(db, backend); err != nil {
		return err
	}
	if err := backfillOriginsFromProxyRoutes(db); err != nil {
		return err
	}
	if err := backfillProxyRouteSiteFields(db); err != nil {
		return err
	}
	if err := ensureProxyRouteSiteNameUniqueIndex(db); err != nil {
		return err
	}
	if err := backfillProxyRouteCertificateFields(db); err != nil {
		return err
	}
	return backfillProxyRouteDomainCertificateFields(db)
}

// migrateV10 adds configurable auth sources and external account bindings.
func migrateV10(db *gorm.DB, backend string) error {
	if err := applyCurrentSchema(db, backend); err != nil {
		return err
	}
	return ensureDefaultGitHubAuthSource(db)
}

// migrateV11 adds acme and dns accounts and extends tls_certificates.
func migrateV11(db *gorm.DB, backend string) error {
	return migrateCurrentSchemaOnly(db, backend)
}

// migrateV12 adds basic authentication fields to proxy_routes.
func migrateV12(db *gorm.DB, backend string) error {
	return migrateCurrentSchemaOnly(db, backend)
}

// migrateV13 adds Cloudflare DNS automation fields to proxy_routes.
func migrateV13(db *gorm.DB, backend string) error {
	return migrateCurrentSchemaOnly(db, backend)
}

// migrateV14 adds country/region restriction fields to proxy_routes.
func migrateV14(db *gorm.DB, backend string) error {
	return migrateCurrentSchemaOnly(db, backend)
}

// migrateV15 adds optional local WAF fields to proxy_routes.
func migrateV15(db *gorm.DB, backend string) error {
	return migrateCurrentSchemaOnly(db, backend)
}

// migrateV16 adds basic CDN scheduling metadata and cache/upstream observability counters.
func migrateV16(db *gorm.DB, backend string) error {
	return migrateCurrentSchemaOnly(db, backend)
}

// migrateV17 adds byte-level access log fields used by observability metering.
func migrateV17(db *gorm.DB, backend string) error {
	return migrateCurrentSchemaOnly(db, backend)
}

// migrateV18 adds GSLB policy fields, DNS TTL, and scheduler debounce state.
func migrateV18(db *gorm.DB, backend string) error {
	return migrateCurrentSchemaOnly(db, backend)
}

// migrateV19 adds authoritative DNS control-plane tables and source-scoped GSLB state.
func migrateV19(db *gorm.DB, backend string) error {
	if err := applyCurrentSchema(db, backend); err != nil {
		return err
	}
	return ensureGSLBSchedulingStateScopeIndex(db)
}

// migrateV20 adds DNS Worker query duration rollup fields.
func migrateV20(db *gorm.DB, backend string) error {
	return migrateCurrentSchemaOnly(db, backend)
}

// migrateV21 adds persisted DNS Worker probe status fields.
func migrateV21(db *gorm.DB, backend string) error {
	return migrateCurrentSchemaOnly(db, backend)
}

// migrateV22 adds source-scoped DNS query rollups.
func migrateV22(db *gorm.DB, backend string) error {
	if err := applyCurrentSchema(db, backend); err != nil {
		return err
	}
	if db.Migrator().HasColumn(&DNSQueryRollup{}, "source_scope") {
		if err := db.Model(&DNSQueryRollup{}).
			Where("source_scope = '' OR source_scope IS NULL").
			Update("source_scope", "global").Error; err != nil {
			return fmt.Errorf("backfill dns_query_rollups.source_scope failed: %w", err)
		}
	}
	return nil
}

// migrateV23 adds DNS Worker GeoIP runtime status fields.
func migrateV23(db *gorm.DB, backend string) error {
	return migrateCurrentSchemaOnly(db, backend)
}

// migrateV24 adds node-side DNS Worker active probe status.
func migrateV24(db *gorm.DB, backend string) error {
	return migrateCurrentSchemaOnly(db, backend)
}

// migrateV25 adds DDoS protection provider and target fields to proxy_routes.
func migrateV25(db *gorm.DB, backend string) error {
	return migrateCurrentSchemaOnly(db, backend)
}

// migrateV26 adds ACME certificate DNS provider selection fields.
func migrateV26(db *gorm.DB, backend string) error {
	return migrateCurrentSchemaOnly(db, backend)
}

// migrateV27 adds access log reason fields for protection hit explanations.
func migrateV27(db *gorm.DB, backend string) error {
	return migrateCurrentSchemaOnly(db, backend)
}

// migrateV28 adds node-side CC protection fields to proxy_routes.
func migrateV28(db *gorm.DB, backend string) error {
	return migrateCurrentSchemaOnly(db, backend)
}

// migrateV29 adds per-request cache status to access logs for cache diagnostics.
func migrateV29(db *gorm.DB, backend string) error {
	return migrateCurrentSchemaOnly(db, backend)
}

// migrateV30 adds the commercial license singleton used by private deployments.
func migrateV30(db *gorm.DB, backend string) error {
	return migrateCurrentSchemaOnly(db, backend)
}

// migrateV31 adds lookup indexes for DNS Worker node probe health summaries.
func migrateV31(db *gorm.DB, backend string) error {
	return migrateCurrentSchemaOnly(db, backend)
}

// migrateV32 adds online activation lease state to commercial licenses.
func migrateV32(db *gorm.DB, backend string) error {
	return migrateCurrentSchemaOnly(db, backend)
}

// migrateV33 adds license-level commercial activation revocations.
func migrateV33(db *gorm.DB, backend string) error {
	return migrateCurrentSchemaOnly(db, backend)
}

// migrateV34 adds DNS Worker heartbeat and query rollup health markers.
func migrateV34(db *gorm.DB, backend string) error {
	return migrateCurrentSchemaOnly(db, backend)
}

// migrateV35 adds DNS Worker source database capability fields.
func migrateV35(db *gorm.DB, backend string) error {
	if backend == "postgres" {
		return addPostgresColumnsIfMissing(db, "dns_workers", []postgresColumnDefinition{
			{name: "asn_database_path", definition: "varchar(512) NOT NULL DEFAULT ''"},
			{name: "asn_last_error", definition: "text NOT NULL DEFAULT ''"},
			{name: "geo_ip_database_type", definition: "varchar(128) NOT NULL DEFAULT ''"},
			{name: "asn_database_type", definition: "varchar(128) NOT NULL DEFAULT ''"},
			{name: "geo_ip_country_enabled", definition: "boolean NOT NULL DEFAULT false"},
			{name: "geo_ip_asn_enabled", definition: "boolean NOT NULL DEFAULT false"},
			{name: "geo_ip_operator_enabled", definition: "boolean NOT NULL DEFAULT false"},
			{name: "operator_cidr_database_path", definition: "varchar(512) NOT NULL DEFAULT ''"},
			{name: "operator_cidr_last_error", definition: "text NOT NULL DEFAULT ''"},
		})
	}
	return applyCurrentSchema(db, backend)
}

// migrateV36 adds manual DNS Worker update request fields.
func migrateV36(db *gorm.DB, backend string) error {
	if backend == "postgres" {
		return addPostgresColumnsIfMissing(db, "dns_workers", []postgresColumnDefinition{
			{name: "update_requested", definition: "boolean NOT NULL DEFAULT false"},
			{name: "update_channel", definition: "varchar(32) NOT NULL DEFAULT 'stable'"},
			{name: "update_tag", definition: "varchar(128) NOT NULL DEFAULT ''"},
		})
	}
	return applyCurrentSchema(db, backend)
}

// migrateV37 adds DNS Worker self-update capability state.
func migrateV37(db *gorm.DB, backend string) error {
	if backend == "postgres" {
		return addPostgresColumnsIfMissing(db, "dns_workers", []postgresColumnDefinition{
			{name: "update_supported", definition: "boolean NOT NULL DEFAULT false"},
			{name: "last_update_supported_at", definition: "timestamptz"},
		})
	}
	return applyCurrentSchema(db, backend)
}

// migrateV38 adds DNS Worker operator-facing remarks.
func migrateV38(db *gorm.DB, backend string) error {
	if backend == "postgres" {
		return addPostgresColumnsIfMissing(db, "dns_workers", []postgresColumnDefinition{
			{name: "remark", definition: "varchar(255) NOT NULL DEFAULT ''"},
		})
	}
	return applyCurrentSchema(db, backend)
}

// migrateV39 adds DNS Worker remote uninstall request state.
func migrateV39(db *gorm.DB, backend string) error {
	if backend == "postgres" {
		return addPostgresColumnsIfMissing(db, "dns_workers", []postgresColumnDefinition{
			{name: "uninstall_supported", definition: "boolean NOT NULL DEFAULT false"},
			{name: "last_uninstall_supported_at", definition: "timestamptz"},
			{name: "uninstall_requested", definition: "boolean NOT NULL DEFAULT false"},
			{name: "uninstall_requested_at", definition: "timestamptz"},
		})
	}
	return applyCurrentSchema(db, backend)
}

// migrateV40 adds DNS Worker update dispatch diagnostics.
func migrateV40(db *gorm.DB, backend string) error {
	if backend == "postgres" {
		return addPostgresColumnsIfMissing(db, "dns_workers", []postgresColumnDefinition{
			{name: "update_dispatch_mode", definition: "varchar(32) NOT NULL DEFAULT ''"},
			{name: "update_dispatch_message", definition: "text NOT NULL DEFAULT ''"},
			{name: "update_dispatched_at", definition: "timestamptz"},
			{name: "update_dispatched_node_id", definition: "varchar(64) NOT NULL DEFAULT ''"},
			{name: "last_remote_ip", definition: "varchar(64) NOT NULL DEFAULT ''"},
		})
	}
	return applyCurrentSchema(db, backend)
}

// migrateV41 adds independent DNS Worker source dimensions for observability.
func migrateV41(db *gorm.DB, backend string) error {
	if backend == "postgres" {
		return addPostgresColumnsIfMissing(db, "dns_query_rollups", []postgresColumnDefinition{
			{name: "source_country", definition: "varchar(8) NOT NULL DEFAULT ''"},
			{name: "source_asn", definition: "bigint NOT NULL DEFAULT 0"},
			{name: "source_operator", definition: "varchar(64) NOT NULL DEFAULT ''"},
		})
	}
	return applyCurrentSchema(db, backend)
}

func migrateV42(db *gorm.DB, backend string) error {
	db = migrationSession(db)
	if err := applyCurrentSchema(db, backend); err != nil {
		return err
	}
	return backfillLegacyConfigVersionArtifacts(db)
}

func migrateV43(db *gorm.DB, backend string) error {
	db = migrationSession(db)
	if err := applyCurrentSchema(db, backend); err != nil {
		return err
	}
	return backfillDNSWorkerTokenHashes(db)
}

func migrateV44(db *gorm.DB, backend string) error {
	db = migrationSession(db)
	return migrateCurrentSchemaOnly(db, backend)
}

func migrateV45(db *gorm.DB, backend string) error {
	db = migrationSession(db)
	if err := applyCurrentSchema(db, backend); err != nil {
		return err
	}
	return BackfillProxyRouteNormalizedTables(db)
}

func migrateV46(db *gorm.DB, backend string) error {
	db = migrationSession(db)
	if err := applyCurrentSchema(db, backend); err != nil {
		return err
	}
	if err := migrateProxyRouteBasicAuthPasswordHashes(db); err != nil {
		return err
	}
	return BackfillProxyRouteNormalizedTables(db)
}

func migrateV47(db *gorm.DB, backend string) error {
	db = migrationSession(db)
	if err := applyCurrentSchema(db, backend); err != nil {
		return err
	}
	return backfillProxyRouteOriginConnectionFields(db)
}

func migrateV48(db *gorm.DB, backend string) error {
	db = migrationSession(db)
	if err := applyCurrentSchema(db, backend); err != nil {
		return err
	}
	if err := migrateProxyRouteRulePathLevelSchema(db); err != nil {
		return err
	}
	return BackfillProxyRouteNormalizedTables(db)
}
