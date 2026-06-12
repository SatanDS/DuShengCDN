package model

import (
	"errors"
	"fmt"
	"gorm.io/gorm"
)

func validateDatabaseSchemaV2(db *gorm.DB, backend string) error {
	if db == nil {
		return fmt.Errorf("database handle is nil")
	}
	if !db.Migrator().HasTable(&DatabaseSchemaVersion{}) {
		return fmt.Errorf("table %s is missing", (&DatabaseSchemaVersion{}).TableName())
	}
	models, err := buildDBModels()
	if err != nil {
		return err
	}
	for _, item := range models {
		if isShardedObservabilityTable(item.tableName) {
			for _, table := range observabilityShardTables(item.tableName) {
				if !db.Migrator().HasTable(table) {
					return fmt.Errorf("sharded table %s is missing", table)
				}
			}
			continue
		}
		if !db.Migrator().HasTable(item.value) {
			return fmt.Errorf("table %s is missing", item.tableName)
		}
	}
	if !db.Migrator().HasColumn(&NodeHealthEvent{}, "metadata_json") {
		return fmt.Errorf("column node_health_events.metadata_json is missing")
	}
	_ = backend
	return nil
}

func validateDatabaseSchemaV3(db *gorm.DB, backend string) error {
	if err := validateDatabaseSchemaV2(db, backend); err != nil {
		return err
	}
	for _, baseTable := range shardedObservabilityBaseTables() {
		for _, table := range observabilityShardTables(baseTable) {
			legacyTable := legacyObservabilityShardTableName(table)
			if db.Migrator().HasTable(legacyTable) {
				return fmt.Errorf("legacy sharded table %s still exists", legacyTable)
			}
		}
	}
	return nil
}

func validateDatabaseSchemaV4(db *gorm.DB, backend string) error {
	if err := validateDatabaseSchemaV3(db, backend); err != nil {
		return err
	}
	if !db.Migrator().HasTable(&Origin{}) {
		return fmt.Errorf("table origins is missing")
	}
	if !db.Migrator().HasColumn(&ProxyRoute{}, "origin_id") {
		return fmt.Errorf("column proxy_routes.origin_id is missing")
	}
	return nil
}

func validateDatabaseSchemaV5(db *gorm.DB, backend string) error {
	if err := validateDatabaseSchemaV4(db, backend); err != nil {
		return err
	}
	if !db.Migrator().HasColumn(&ProxyRoute{}, "site_name") {
		return fmt.Errorf("column proxy_routes.site_name is missing")
	}
	if !db.Migrator().HasColumn(&ProxyRoute{}, "domains") {
		return fmt.Errorf("column proxy_routes.domains is missing")
	}

	var routes []ProxyRoute
	if err := db.Order("id asc").Find(&routes).Error; err != nil {
		return fmt.Errorf("list proxy routes for validation failed: %w", err)
	}

	siteNames := make(map[string]uint, len(routes))
	domainOwners := make(map[string]uint, len(routes))
	for _, route := range routes {
		domains, err := decodeProxyRouteDomainsForMigration(route.Domains, route.Domain)
		if err != nil {
			return fmt.Errorf("proxy route %d domains are invalid: %w", route.ID, err)
		}
		if len(domains) == 0 {
			return fmt.Errorf("proxy route %d domains are empty", route.ID)
		}
		if route.Domain != domains[0] {
			return fmt.Errorf("proxy route %d primary domain mirror is invalid", route.ID)
		}

		siteName := normalizeProxyRouteSiteNameForMigration(route.SiteName, domains[0])
		if siteName == "" {
			return fmt.Errorf("proxy route %d site_name is empty", route.ID)
		}
		if existingID, ok := siteNames[siteName]; ok && existingID != route.ID {
			return fmt.Errorf("proxy route site_name %s is duplicated", siteName)
		}
		siteNames[siteName] = route.ID

		localSeen := make(map[string]struct{}, len(domains))
		for _, domain := range domains {
			if _, ok := localSeen[domain]; ok {
				return fmt.Errorf("proxy route %d contains duplicated domain %s", route.ID, domain)
			}
			localSeen[domain] = struct{}{}
			if existingID, ok := domainOwners[domain]; ok && existingID != route.ID {
				return fmt.Errorf("proxy route domain %s is duplicated", domain)
			}
			domainOwners[domain] = route.ID
		}
	}
	return nil
}

func validateDatabaseSchemaV6(db *gorm.DB, backend string) error {
	if err := validateDatabaseSchemaV5(db, backend); err != nil {
		return err
	}
	if !db.Migrator().HasColumn(&ProxyRoute{}, "limit_conn_per_server") {
		return fmt.Errorf("column proxy_routes.limit_conn_per_server is missing")
	}
	if !db.Migrator().HasColumn(&ProxyRoute{}, "limit_conn_per_ip") {
		return fmt.Errorf("column proxy_routes.limit_conn_per_ip is missing")
	}
	if !db.Migrator().HasColumn(&ProxyRoute{}, "limit_rate") {
		return fmt.Errorf("column proxy_routes.limit_rate is missing")
	}
	return nil
}

func validateDatabaseSchemaV7(db *gorm.DB, backend string) error {
	if err := validateDatabaseSchemaV6(db, backend); err != nil {
		return err
	}
	if !db.Migrator().HasColumn(&ProxyRoute{}, "cert_ids") {
		return fmt.Errorf("column proxy_routes.cert_ids is missing")
	}

	var routes []ProxyRoute
	if err := db.Order("id asc").Find(&routes).Error; err != nil {
		return fmt.Errorf("list proxy routes for certificate validation failed: %w", err)
	}
	for _, route := range routes {
		certIDs, err := decodeProxyRouteCertIDsForMigration(route.CertIDs, route.CertID)
		if err != nil {
			return fmt.Errorf("proxy route %d cert_ids are invalid: %w", route.ID, err)
		}
		if route.EnableHTTPS && len(certIDs) == 0 {
			return fmt.Errorf("proxy route %d has https enabled without cert_ids", route.ID)
		}
		if !route.EnableHTTPS && route.RedirectHTTP {
			return fmt.Errorf("proxy route %d enables redirect_http without https", route.ID)
		}
		if len(certIDs) == 0 {
			if route.CertID != nil {
				return fmt.Errorf("proxy route %d primary cert_id mirror is invalid", route.ID)
			}
			continue
		}
		if route.CertID == nil || *route.CertID != certIDs[0] {
			return fmt.Errorf("proxy route %d primary cert_id mirror is invalid", route.ID)
		}
	}
	return nil
}

func validateDatabaseSchemaV8(db *gorm.DB, backend string) error {
	if err := validateDatabaseSchemaV7(db, backend); err != nil {
		return err
	}
	if !db.Migrator().HasColumn(&ProxyRoute{}, "domain_cert_ids") {
		return fmt.Errorf("column proxy_routes.domain_cert_ids is missing")
	}

	var routes []ProxyRoute
	if err := db.Order("id asc").Find(&routes).Error; err != nil {
		return fmt.Errorf("list proxy routes for domain certificate validation failed: %w", err)
	}
	for _, route := range routes {
		domains, err := decodeProxyRouteDomainsForMigration(route.Domains, route.Domain)
		if err != nil {
			return fmt.Errorf("proxy route %d domains are invalid: %w", route.ID, err)
		}
		domainCertIDs, err := decodeProxyRouteDomainCertIDsForMigration(route.DomainCertIDs, len(domains))
		if err != nil {
			return fmt.Errorf("proxy route %d domain_cert_ids are invalid: %w", route.ID, err)
		}
		certIDs, err := decodeProxyRouteCertIDsForMigration(route.CertIDs, route.CertID)
		if err != nil {
			return fmt.Errorf("proxy route %d cert_ids are invalid: %w", route.ID, err)
		}
		if !route.EnableHTTPS {
			if len(domainCertIDs) != 0 {
				return fmt.Errorf("proxy route %d has domain_cert_ids while https is disabled", route.ID)
			}
			continue
		}
		if len(domainCertIDs) != len(domains) {
			return fmt.Errorf("proxy route %d domain_cert_ids length is invalid", route.ID)
		}
		normalizedCertIDs := uniqueProxyRouteCertIDsFromDomainAssignments(domainCertIDs)
		if len(normalizedCertIDs) == 0 {
			return fmt.Errorf("proxy route %d has https enabled without domain certificate assignments", route.ID)
		}
		if !uintSlicesEqualForMigration(certIDs, normalizedCertIDs) {
			return fmt.Errorf("proxy route %d cert_ids mirror is invalid", route.ID)
		}
		if route.CertID == nil || *route.CertID != normalizedCertIDs[0] {
			return fmt.Errorf("proxy route %d primary cert_id mirror is invalid", route.ID)
		}
	}
	return nil
}

func validateDatabaseSchemaV9(db *gorm.DB, backend string) error {
	if err := validateDatabaseSchemaV8(db, backend); err != nil {
		return err
	}
	if !db.Migrator().HasColumn(&ProxyRoute{}, "pow_enabled") {
		return fmt.Errorf("column proxy_routes.pow_enabled is missing")
	}
	if !db.Migrator().HasColumn(&ProxyRoute{}, "pow_config") {
		return fmt.Errorf("column proxy_routes.pow_config is missing")
	}
	return nil
}

func validateDatabaseSchemaV10(db *gorm.DB, backend string) error {
	if err := validateDatabaseSchemaV9(db, backend); err != nil {
		return err
	}
	if !db.Migrator().HasTable(&AuthSource{}) {
		return fmt.Errorf("table auth_sources is missing")
	}
	if !db.Migrator().HasTable(&ExternalAccount{}) {
		return fmt.Errorf("table external_accounts is missing")
	}
	return nil
}

func validateDatabaseSchemaV11(db *gorm.DB, backend string) error {
	if err := validateDatabaseSchemaV10(db, backend); err != nil {
		return err
	}
	if !db.Migrator().HasTable(&AcmeAccount{}) {
		return fmt.Errorf("table acme_accounts is missing")
	}
	if !db.Migrator().HasTable(&DnsAccount{}) {
		return fmt.Errorf("table dns_accounts is missing")
	}
	if !db.Migrator().HasColumn(&TLSCertificate{}, "provider") {
		return fmt.Errorf("column tls_certificates.provider is missing")
	}
	return nil
}

func validateDatabaseSchemaV12(db *gorm.DB, backend string) error {
	if err := validateDatabaseSchemaV11(db, backend); err != nil {
		return err
	}
	if !db.Migrator().HasColumn(&ProxyRoute{}, "basic_auth_enabled") {
		return fmt.Errorf("column proxy_routes.basic_auth_enabled is missing")
	}
	return nil
}

func validateDatabaseSchemaV13(db *gorm.DB, backend string) error {
	if err := validateDatabaseSchemaV12(db, backend); err != nil {
		return err
	}
	if !db.Migrator().HasColumn(&ProxyRoute{}, "dns_auto_sync") {
		return fmt.Errorf("column proxy_routes.dns_auto_sync is missing")
	}
	if !db.Migrator().HasColumn(&ProxyRoute{}, "dns_account_id") {
		return fmt.Errorf("column proxy_routes.dns_account_id is missing")
	}
	if !db.Migrator().HasColumn(&ProxyRoute{}, "dns_auto_target") {
		return fmt.Errorf("column proxy_routes.dns_auto_target is missing")
	}
	if !db.Migrator().HasColumn(&ProxyRoute{}, "cloudflare_proxied") {
		return fmt.Errorf("column proxy_routes.cloudflare_proxied is missing")
	}
	if !db.Migrator().HasColumn(&ProxyRoute{}, "ddos_protection_mode") {
		return fmt.Errorf("column proxy_routes.ddos_protection_mode is missing")
	}
	return nil
}

func validateDatabaseSchemaV14(db *gorm.DB, backend string) error {
	if err := validateDatabaseSchemaV13(db, backend); err != nil {
		return err
	}
	if !db.Migrator().HasColumn(&ProxyRoute{}, "region_restriction_enabled") {
		return fmt.Errorf("column proxy_routes.region_restriction_enabled is missing")
	}
	if !db.Migrator().HasColumn(&ProxyRoute{}, "region_restriction_mode") {
		return fmt.Errorf("column proxy_routes.region_restriction_mode is missing")
	}
	if !db.Migrator().HasColumn(&ProxyRoute{}, "region_restriction_countries") {
		return fmt.Errorf("column proxy_routes.region_restriction_countries is missing")
	}
	return nil
}

func validateDatabaseSchemaV15(db *gorm.DB, backend string) error {
	if err := validateDatabaseSchemaV14(db, backend); err != nil {
		return err
	}
	if !db.Migrator().HasColumn(&ProxyRoute{}, "waf_enabled") {
		return fmt.Errorf("column proxy_routes.waf_enabled is missing")
	}
	if !db.Migrator().HasColumn(&ProxyRoute{}, "waf_mode") {
		return fmt.Errorf("column proxy_routes.waf_mode is missing")
	}
	if !db.Migrator().HasColumn(&ProxyRoute{}, "waf_config") {
		return fmt.Errorf("column proxy_routes.waf_config is missing")
	}
	return nil
}

func validateDatabaseSchemaV16(db *gorm.DB, backend string) error {
	if err := validateDatabaseSchemaV15(db, backend); err != nil {
		return err
	}
	nodeColumns := []string{
		"pool_name",
		"tags",
		"weight",
		"public_ips",
		"scheduling_enabled",
		"drain_mode",
	}
	for _, column := range nodeColumns {
		if !db.Migrator().HasColumn(&Node{}, column) {
			return fmt.Errorf("column nodes.%s is missing", column)
		}
	}
	routeColumns := []string{
		"node_pool",
		"dns_target_count",
		"dns_schedule_mode",
	}
	for _, column := range routeColumns {
		if !db.Migrator().HasColumn(&ProxyRoute{}, column) {
			return fmt.Errorf("column proxy_routes.%s is missing", column)
		}
	}
	reportColumns := []string{
		"cache_hit_count",
		"cache_miss_count",
		"cache_bypass_count",
		"cache_expired_count",
		"cache_stale_count",
		"upstream_error_count",
		"upstream_response_ms",
	}
	for _, table := range observabilityShardTables("node_request_reports") {
		for _, column := range reportColumns {
			if !db.Migrator().HasColumn(table, column) {
				return fmt.Errorf("column %s.%s is missing", table, column)
			}
		}
	}
	_ = backend
	return nil
}

func validateDatabaseSchemaV17(db *gorm.DB, backend string) error {
	if err := validateDatabaseSchemaV16(db, backend); err != nil {
		return err
	}
	accessLogColumns := []string{
		"request_bytes",
		"response_bytes",
		"upstream_bytes",
		"operator",
	}
	for _, table := range observabilityShardTables("node_access_logs") {
		for _, column := range accessLogColumns {
			if !db.Migrator().HasColumn(table, column) {
				return fmt.Errorf("column %s.%s is missing", table, column)
			}
		}
	}
	_ = backend
	return nil
}

func validateDatabaseSchemaV18(db *gorm.DB, backend string) error {
	if err := validateDatabaseSchemaV17(db, backend); err != nil {
		return err
	}
	routeColumns := []string{
		"dns_ttl",
		"gslb_enabled",
		"gslb_policy",
	}
	for _, column := range routeColumns {
		if !db.Migrator().HasColumn(&ProxyRoute{}, column) {
			return fmt.Errorf("column proxy_routes.%s is missing", column)
		}
	}
	if !db.Migrator().HasTable(&GSLBSchedulingState{}) {
		return fmt.Errorf("table gslb_scheduling_states is missing")
	}
	_ = backend
	return nil
}

func validateDatabaseSchemaV19(db *gorm.DB, backend string) error {
	if err := validateDatabaseSchemaV18(db, backend); err != nil {
		return err
	}
	for _, item := range []any{
		&DNSZone{},
		&DNSRecord{},
		&DNSWorker{},
		&DNSQueryRollup{},
	} {
		if !db.Migrator().HasTable(item) {
			return fmt.Errorf("authoritative dns table is missing: %T", item)
		}
	}
	routeColumns := []string{
		"dns_provider_mode",
		"dns_zone_id_ref",
	}
	for _, column := range routeColumns {
		if !db.Migrator().HasColumn(&ProxyRoute{}, column) {
			return fmt.Errorf("column proxy_routes.%s is missing", column)
		}
	}
	if !db.Migrator().HasColumn(&GSLBSchedulingState{}, "scope_key") {
		return fmt.Errorf("column gslb_scheduling_states.scope_key is missing")
	}
	_ = backend
	return nil
}

func validateDatabaseSchemaV20(db *gorm.DB, backend string) error {
	if err := validateDatabaseSchemaV19(db, backend); err != nil {
		return err
	}
	for _, column := range []string{"total_duration_ms", "max_duration_ms"} {
		if !db.Migrator().HasColumn(&DNSQueryRollup{}, column) {
			return fmt.Errorf("column dns_query_rollups.%s is missing", column)
		}
	}
	_ = backend
	return nil
}

func validateDatabaseSchemaV21(db *gorm.DB, backend string) error {
	if err := validateDatabaseSchemaV20(db, backend); err != nil {
		return err
	}
	for _, column := range []string{"last_probe_at", "last_probe_query", "last_probe_result"} {
		if !db.Migrator().HasColumn(&DNSWorker{}, column) {
			return fmt.Errorf("column dns_workers.%s is missing", column)
		}
	}
	_ = backend
	return nil
}

func validateDatabaseSchemaV22(db *gorm.DB, backend string) error {
	if err := validateDatabaseSchemaV21(db, backend); err != nil {
		return err
	}
	if !db.Migrator().HasColumn(&DNSQueryRollup{}, "source_scope") {
		return fmt.Errorf("column dns_query_rollups.source_scope is missing")
	}
	_ = backend
	return nil
}

func validateDatabaseSchemaV23(db *gorm.DB, backend string) error {
	if err := validateDatabaseSchemaV22(db, backend); err != nil {
		return err
	}
	for _, column := range []string{"geo_ip_enabled", "geo_ip_database_path", "geo_ip_last_error"} {
		if !db.Migrator().HasColumn(&DNSWorker{}, column) {
			return fmt.Errorf("column dns_workers.%s is missing", column)
		}
	}
	_ = backend
	return nil
}

func validateDatabaseSchemaV24(db *gorm.DB, backend string) error {
	if err := validateDatabaseSchemaV23(db, backend); err != nil {
		return err
	}
	if !db.Migrator().HasTable(&DNSWorkerNodeProbe{}) {
		return fmt.Errorf("table dns_worker_node_probes is missing")
	}
	for _, column := range []string{
		"worker_id",
		"node_id",
		"public_address",
		"query_name",
		"query_type",
		"checked_at",
		"results_json",
		"healthy",
		"average_rtt_ms",
		"max_rtt_ms",
		"last_error",
		"failure_samples",
	} {
		if !db.Migrator().HasColumn(&DNSWorkerNodeProbe{}, column) {
			return fmt.Errorf("column dns_worker_node_probes.%s is missing", column)
		}
	}
	_ = backend
	return nil
}

func validateDatabaseSchemaV25(db *gorm.DB, backend string) error {
	if err := validateDatabaseSchemaV24(db, backend); err != nil {
		return err
	}
	for _, column := range []string{"ddos_protection_provider", "ddos_protection_target"} {
		if !db.Migrator().HasColumn(&ProxyRoute{}, column) {
			return fmt.Errorf("column proxy_routes.%s is missing", column)
		}
	}
	_ = backend
	return nil
}

func validateDatabaseSchemaV26(db *gorm.DB, backend string) error {
	if err := validateDatabaseSchemaV25(db, backend); err != nil {
		return err
	}
	for _, column := range []string{"dns_provider_mode", "dns_zone_id_ref"} {
		if !db.Migrator().HasColumn(&TLSCertificate{}, column) {
			return fmt.Errorf("column tls_certificates.%s is missing", column)
		}
	}
	_ = backend
	return nil
}

func validateDatabaseSchemaV27(db *gorm.DB, backend string) error {
	if err := validateDatabaseSchemaV26(db, backend); err != nil {
		return err
	}
	for _, table := range observabilityShardTables("node_access_logs") {
		for _, column := range []string{"reason", "operator"} {
			if !db.Migrator().HasColumn(table, column) {
				return fmt.Errorf("column %s.%s is missing", table, column)
			}
		}
	}
	_ = backend
	return nil
}

func validateDatabaseSchemaV28(db *gorm.DB, backend string) error {
	if err := validateDatabaseSchemaV27(db, backend); err != nil {
		return err
	}
	if !db.Migrator().HasColumn(&ProxyRoute{}, "cc_enabled") {
		return fmt.Errorf("column proxy_routes.cc_enabled is missing")
	}
	if !db.Migrator().HasColumn(&ProxyRoute{}, "cc_mode") {
		return fmt.Errorf("column proxy_routes.cc_mode is missing")
	}
	if !db.Migrator().HasColumn(&ProxyRoute{}, "cc_config") {
		return fmt.Errorf("column proxy_routes.cc_config is missing")
	}
	_ = backend
	return nil
}

func validateDatabaseSchemaV29(db *gorm.DB, backend string) error {
	if err := validateDatabaseSchemaV28(db, backend); err != nil {
		return err
	}
	for _, table := range observabilityShardTables("node_access_logs") {
		if !db.Migrator().HasColumn(table, "cache_status") {
			return fmt.Errorf("column %s.cache_status is missing", table)
		}
	}
	_ = backend
	return nil
}

func validateDatabaseSchemaV30(db *gorm.DB, backend string) error {
	if err := validateDatabaseSchemaV29(db, backend); err != nil {
		return err
	}
	if !db.Migrator().HasTable(&CommercialLicense{}) {
		return fmt.Errorf("table commercial_licenses is missing")
	}
	for _, column := range []string{
		"license_id",
		"customer_id",
		"customer_name",
		"plan",
		"status",
		"token",
		"token_hash",
		"fingerprint",
		"features_json",
		"max_nodes",
		"max_sites",
		"issued_at",
		"expires_at",
		"last_validated_at",
		"last_validation_error",
	} {
		if !db.Migrator().HasColumn(&CommercialLicense{}, column) {
			return fmt.Errorf("column commercial_licenses.%s is missing", column)
		}
	}
	_ = backend
	return nil
}

func validateDatabaseSchemaV31(db *gorm.DB, backend string) error {
	if err := validateDatabaseSchemaV30(db, backend); err != nil {
		return err
	}
	if !db.Migrator().HasIndex(&DNSWorkerNodeProbe{}, "idx_dns_worker_node_probe_node") {
		return fmt.Errorf("index dns_worker_node_probes.idx_dns_worker_node_probe_node is missing")
	}
	_ = backend
	return nil
}

func validateDatabaseSchemaV32(db *gorm.DB, backend string) error {
	if err := validateDatabaseSchemaV31(db, backend); err != nil {
		return err
	}
	if !db.Migrator().HasTable(&CommercialLicenseActivation{}) {
		return fmt.Errorf("table commercial_license_activations is missing")
	}
	for _, column := range []string{
		"activation_id",
		"machine_fingerprint",
		"lease_token",
		"lease_expires_at",
		"last_lease_renewed_at",
	} {
		if !db.Migrator().HasColumn(&CommercialLicense{}, column) {
			return fmt.Errorf("column commercial_licenses.%s is missing", column)
		}
	}
	for _, column := range []string{
		"activation_id",
		"license_id",
		"customer_id",
		"machine_fingerprint",
		"server_version",
		"build_watermark",
		"instance_hostname",
		"revoked_at",
		"last_lease_issued_at",
		"last_lease_expires_at",
	} {
		if !db.Migrator().HasColumn(&CommercialLicenseActivation{}, column) {
			return fmt.Errorf("column commercial_license_activations.%s is missing", column)
		}
	}
	_ = backend
	return nil
}

func validateDatabaseSchemaV33(db *gorm.DB, backend string) error {
	if err := validateDatabaseSchemaV32(db, backend); err != nil {
		return err
	}
	if !db.Migrator().HasTable(&CommercialLicenseRevocation{}) {
		return fmt.Errorf("table commercial_license_revocations is missing")
	}
	for _, column := range []string{
		"license_id",
		"customer_id",
		"reason",
		"revoked_at",
	} {
		if !db.Migrator().HasColumn(&CommercialLicenseRevocation{}, column) {
			return fmt.Errorf("column commercial_license_revocations.%s is missing", column)
		}
	}
	_ = backend
	return nil
}

func validateDatabaseSchemaV34(db *gorm.DB, backend string) error {
	if err := validateDatabaseSchemaV33(db, backend); err != nil {
		return err
	}
	for _, column := range []string{
		"last_heartbeat_at",
		"last_rollup_at",
		"last_rollup_count",
	} {
		if !db.Migrator().HasColumn(&DNSWorker{}, column) {
			return fmt.Errorf("column dns_workers.%s is missing", column)
		}
	}
	_ = backend
	return nil
}

func validateDatabaseSchemaV35(db *gorm.DB, backend string) error {
	if err := validateDatabaseSchemaV34(db, backend); err != nil {
		return err
	}
	for _, column := range []string{
		"asn_database_path",
		"asn_last_error",
		"geo_ip_database_type",
		"asn_database_type",
		"geo_ip_country_enabled",
		"geo_ip_asn_enabled",
		"geo_ip_operator_enabled",
		"operator_cidr_database_path",
		"operator_cidr_last_error",
	} {
		if !db.Migrator().HasColumn(&DNSWorker{}, column) {
			return fmt.Errorf("column dns_workers.%s is missing", column)
		}
	}
	_ = backend
	return nil
}

func validateDatabaseSchemaV36(db *gorm.DB, backend string) error {
	if err := validateDatabaseSchemaV35(db, backend); err != nil {
		return err
	}
	for _, column := range []string{
		"update_requested",
		"update_channel",
		"update_tag",
	} {
		if !db.Migrator().HasColumn(&DNSWorker{}, column) {
			return fmt.Errorf("column dns_workers.%s is missing", column)
		}
	}
	_ = backend
	return nil
}

func validateDatabaseSchemaV37(db *gorm.DB, backend string) error {
	if err := validateDatabaseSchemaV36(db, backend); err != nil {
		return err
	}
	for _, column := range []string{
		"update_supported",
		"last_update_supported_at",
	} {
		if !db.Migrator().HasColumn(&DNSWorker{}, column) {
			return fmt.Errorf("column dns_workers.%s is missing", column)
		}
	}
	_ = backend
	return nil
}

func validateDatabaseSchemaV38(db *gorm.DB, backend string) error {
	if err := validateDatabaseSchemaV37(db, backend); err != nil {
		return err
	}
	if !db.Migrator().HasColumn(&DNSWorker{}, "remark") {
		return fmt.Errorf("column dns_workers.remark is missing")
	}
	_ = backend
	return nil
}

func validateDatabaseSchemaV39(db *gorm.DB, backend string) error {
	if err := validateDatabaseSchemaV38(db, backend); err != nil {
		return err
	}
	for _, column := range []string{
		"uninstall_supported",
		"last_uninstall_supported_at",
		"uninstall_requested",
		"uninstall_requested_at",
	} {
		if !db.Migrator().HasColumn(&DNSWorker{}, column) {
			return fmt.Errorf("column dns_workers.%s is missing", column)
		}
	}
	_ = backend
	return nil
}

func validateDatabaseSchemaV40(db *gorm.DB, backend string) error {
	if err := validateDatabaseSchemaV39(db, backend); err != nil {
		return err
	}
	for _, column := range []string{
		"update_dispatch_mode",
		"update_dispatch_message",
		"update_dispatched_at",
		"update_dispatched_node_id",
		"last_remote_ip",
	} {
		if !db.Migrator().HasColumn(&DNSWorker{}, column) {
			return fmt.Errorf("column dns_workers.%s is missing", column)
		}
	}
	_ = backend
	return nil
}

func validateDatabaseSchemaV41(db *gorm.DB, backend string) error {
	if err := validateDatabaseSchemaV40(db, backend); err != nil {
		return err
	}
	for _, column := range []string{
		"source_country",
		"source_asn",
		"source_operator",
	} {
		if !db.Migrator().HasColumn(&DNSQueryRollup{}, column) {
			return fmt.Errorf("column dns_query_rollups.%s is missing", column)
		}
	}
	_ = backend
	return nil
}

func validateDatabaseSchemaV42(db *gorm.DB, backend string) error {
	if err := validateDatabaseSchemaV41(db, backend); err != nil {
		return err
	}
	if !db.Migrator().HasTable(&ConfigVersionArtifact{}) {
		return errors.New("table config_version_artifacts is missing")
	}
	for _, column := range []string{
		"config_version_id",
		"pool_name",
		"checksum",
		"main_config_checksum",
		"route_config_checksum",
		"rendered_config",
		"support_files_json",
		"route_count",
	} {
		if !db.Migrator().HasColumn(&ConfigVersionArtifact{}, column) {
			return fmt.Errorf("column config_version_artifacts.%s is missing", column)
		}
	}
	if !db.Migrator().HasColumn(&Node{}, "current_checksum") {
		return errors.New("column nodes.current_checksum is missing")
	}
	_ = backend
	return nil
}

func validateDatabaseSchemaV43(db *gorm.DB, backend string) error {
	if err := validateDatabaseSchemaV42(db, backend); err != nil {
		return err
	}
	for _, column := range []string{"token_hash", "token_prefix", "token_revoked_at"} {
		if !db.Migrator().HasColumn(&DNSWorker{}, column) {
			return fmt.Errorf("column dns_workers.%s is missing", column)
		}
	}
	for _, column := range []string{"dnssec_enabled", "dnssec_denial_mode", "dnssec_nsec3_salt", "dnssec_nsec3_iterations", "dnssec_signature_validity"} {
		if !db.Migrator().HasColumn(&DNSZone{}, column) {
			return fmt.Errorf("column dns_zones.%s is missing", column)
		}
	}
	if !db.Migrator().HasTable(&DNSSECKey{}) {
		return errors.New("table dnssec_keys is missing")
	}
	for _, column := range []string{"unhealthy_count", "recovery_count"} {
		if !db.Migrator().HasColumn(&GSLBSchedulingState{}, column) {
			return fmt.Errorf("column gslb_scheduling_states.%s is missing", column)
		}
	}
	if !db.Migrator().HasTable(&DNSZoneWorkerAssignment{}) {
		return errors.New("table dns_zone_worker_assignments is missing")
	}
	_ = backend
	return nil
}

func validateDatabaseSchemaV44(db *gorm.DB, backend string) error {
	if err := validateDatabaseSchemaV43(db, backend); err != nil {
		return err
	}
	if !db.Migrator().HasColumn(&ProxyRoute{}, "proxy_buffering_mode") {
		return errors.New("column proxy_routes.proxy_buffering_mode is missing")
	}
	_ = backend
	return nil
}

func validateDatabaseSchemaV45(db *gorm.DB, backend string) error {
	if err := validateDatabaseSchemaV44(db, backend); err != nil {
		return err
	}
	if err := validateProxyRouteNormalizedSchema(db, true); err != nil {
		return err
	}
	_ = backend
	return nil
}

func validateProxyRouteNormalizedSchema(db *gorm.DB, validateBackfill bool) error {
	return validateProxyRouteNormalizedSchemaWithCache(db, newSchemaIntrospectionCache(db), validateBackfill)
}

func validateProxyRouteNormalizedSchemaWithCache(db *gorm.DB, schemaCache *schemaIntrospectionCache, validateBackfill bool) error {
	normalizedTables := []struct {
		model   any
		table   string
		columns []string
	}{
		{model: &ProxySite{}, table: "proxy_sites", columns: []string{"id", "proxy_route_id", "name", "node_pool", "enabled", "remark"}},
		{model: &ProxySiteDomain{}, table: "proxy_site_domains", columns: []string{"id", "proxy_site_id", "proxy_route_id", "domain", "is_primary", "sort_order"}},
		{model: &OriginGroup{}, table: "origin_groups", columns: []string{"id", "proxy_route_id", "origin_id", "name", "resolve_mode", "health_check_path", "connect_timeout", "read_timeout"}},
		{model: &OriginServer{}, table: "origin_servers", columns: []string{"id", "origin_group_id", "proxy_route_id", "origin_id", "url", "scheme", "host", "port", "weight", "backup", "sni", "host_header", "enabled", "uri", "sort_order"}},
		{model: &ProxyRouteRule{}, table: "proxy_route_rules", columns: []string{"id", "proxy_route_id", "proxy_site_id", "origin_group_id", "limit_conn_per_server", "limit_conn_per_ip", "limit_rate", "proxy_buffering_mode", "custom_headers_json"}},
		{model: &CachePolicy{}, table: "cache_policies", columns: []string{"id", "proxy_route_id", "enabled", "default_ttl", "status_ttls", "cache_key", "bypass_cookies", "bypass_headers", "include_query", "ignore_query_params", "cache_methods", "policy", "rules_json"}},
		{model: &TLSBinding{}, table: "tls_bindings", columns: []string{"id", "proxy_route_id", "proxy_site_id", "domain", "cert_id", "enable_https", "redirect_http", "is_primary", "sort_order"}},
		{model: &DNSBinding{}, table: "dns_bindings", columns: []string{"id", "proxy_route_id", "dns_auto_sync", "dns_account_id", "dns_zone_id", "dns_record_type", "dns_record_name", "dns_record_content", "dns_auto_target", "dns_target_count", "dns_schedule_mode", "dns_ttl", "dns_provider_mode", "dns_zone_id_ref", "gslb_enabled", "gslb_policy_json", "dns_record_ids_json", "cloudflare_proxied", "last_sync_status", "last_sync_message", "last_synced_at"}},
		{model: &SecurityPolicy{}, table: "security_policies", columns: []string{"id", "proxy_route_id", "pow_enabled", "pow_config", "waf_enabled", "waf_mode", "waf_config", "cc_enabled", "cc_mode", "cc_config", "basic_auth_enabled", "basic_auth_username", "basic_auth_password_hash", "region_restriction_enabled", "region_restriction_mode", "region_restriction_countries_json", "ddos_protection_mode", "ddos_protection_provider", "ddos_protection_target"}},
	}
	for _, item := range normalizedTables {
		if !schemaCache.HasTable(item.model, item.table) {
			return fmt.Errorf("table %s is missing", item.table)
		}
		for _, column := range item.columns {
			if !schemaCache.HasColumn(item.model, item.table, column) {
				return fmt.Errorf("column %s.%s is missing", item.table, column)
			}
		}
	}
	if !validateBackfill {
		return nil
	}

	var routeCount int64
	if err := db.Model(&ProxyRoute{}).Count(&routeCount).Error; err != nil {
		return err
	}
	if routeCount > 0 {
		var siteCount int64
		if err := db.Model(&ProxySite{}).Count(&siteCount).Error; err != nil {
			return err
		}
		if siteCount != routeCount {
			return fmt.Errorf("proxy route normalized table backfill incomplete: proxy_sites=%d proxy_routes=%d", siteCount, routeCount)
		}
	}
	return nil
}

func validateDatabaseSchemaV46(db *gorm.DB, backend string) error {
	if err := validateDatabaseSchemaV45(db, backend); err != nil {
		return err
	}
	for _, column := range []string{"basic_auth_password_hash", "basic_auth_password_updated_at"} {
		if !db.Migrator().HasColumn(&ProxyRoute{}, column) {
			return fmt.Errorf("column proxy_routes.%s is missing", column)
		}
	}
	if !db.Migrator().HasColumn(&SecurityPolicy{}, "basic_auth_password_updated_at") {
		return errors.New("column security_policies.basic_auth_password_updated_at is missing")
	}
	hasLegacyPasswordColumn, err := databaseTableHasColumn(db, "proxy_routes", "basic_auth_password")
	if err != nil {
		return err
	}
	if hasLegacyPasswordColumn {
		var plaintextCount int64
		if err := db.Table("proxy_routes").Where("basic_auth_password <> ''").Count(&plaintextCount).Error; err != nil {
			return fmt.Errorf("validate proxy route basic auth password cleanup failed: %w", err)
		}
		if plaintextCount > 0 {
			return fmt.Errorf("proxy_routes.basic_auth_password still contains %d plaintext values", plaintextCount)
		}
	}
	_ = backend
	return nil
}

func validateDatabaseSchemaV47(db *gorm.DB, backend string) error {
	if err := validateDatabaseSchemaV46(db, backend); err != nil {
		return err
	}
	for _, column := range []string{
		"origin_host_header",
		"origin_sni",
		"origin_tls_verify",
		"origin_ca_bundle",
		"origin_resolve_mode",
	} {
		if !db.Migrator().HasColumn(&ProxyRoute{}, column) {
			return fmt.Errorf("column proxy_routes.%s is missing", column)
		}
	}
	var invalidResolveModeCount int64
	if err := db.Model(&ProxyRoute{}).
		Where("origin_resolve_mode NOT IN ?", []string{"runtime_dns", "publish_resolve", "static_ip", "origin_group"}).
		Count(&invalidResolveModeCount).Error; err != nil {
		return fmt.Errorf("validate proxy route origin_resolve_mode failed: %w", err)
	}
	if invalidResolveModeCount > 0 {
		return fmt.Errorf("proxy_routes.origin_resolve_mode contains %d invalid values", invalidResolveModeCount)
	}
	if !db.Migrator().HasTable(&OriginHealthStatus{}) {
		return errors.New("table origin_health_statuses is missing")
	}
	for _, column := range []string{
		"route_id",
		"node_id",
		"origin_url",
		"status",
		"latency_ms",
		"last_error",
		"reported_at",
		"checked_at",
	} {
		if !db.Migrator().HasColumn(&OriginHealthStatus{}, column) {
			return fmt.Errorf("column origin_health_statuses.%s is missing", column)
		}
	}
	_ = backend
	return nil
}

func validateDatabaseSchemaV48(db *gorm.DB, backend string) error {
	if err := validateDatabaseSchemaV47(db, backend); err != nil {
		return err
	}
	if err := validateProxyRouteRulePathSchema(db, true); err != nil {
		return err
	}
	if err := validateConfigReleaseSchema(db); err != nil {
		return err
	}
	_ = backend
	return nil
}

func validateDatabaseSchemaV49(db *gorm.DB, backend string) error {
	if err := validateDatabaseSchemaV48(db, backend); err != nil {
		return err
	}
	if err := validateProxyRouteReusablePolicySchema(db); err != nil {
		return err
	}
	_ = backend
	return nil
}

func validateDatabaseSchemaV50(db *gorm.DB, backend string) error {
	if err := validateDatabaseSchemaV49(db, backend); err != nil {
		return err
	}
	if err := validateConfigReleaseSchema(db); err != nil {
		return err
	}
	_ = backend
	return nil
}

func validateProxyRouteReusablePolicySchema(db *gorm.DB) error {
	return validateProxyRouteReusablePolicySchemaWithCache(newSchemaIntrospectionCache(db))
}

func validateProxyRouteReusablePolicySchemaWithCache(schemaCache *schemaIntrospectionCache) error {
	reusableColumns := []struct {
		model   any
		table   string
		columns []string
	}{
		{model: &ProxyRoute{}, table: "proxy_routes", columns: []string{"origin_group_id", "cache_policy_id"}},
		{model: &OriginGroup{}, table: "origin_groups", columns: []string{"resolve_mode", "health_check_path", "connect_timeout", "read_timeout"}},
		{model: &OriginServer{}, table: "origin_servers", columns: []string{"weight", "backup", "sni", "host_header", "enabled"}},
		{model: &CachePolicy{}, table: "cache_policies", columns: []string{"default_ttl", "status_ttls", "cache_key", "bypass_cookies", "bypass_headers", "include_query", "ignore_query_params", "cache_methods"}},
	}
	for _, item := range reusableColumns {
		for _, column := range item.columns {
			if !schemaCache.HasColumn(item.model, item.table, column) {
				return fmt.Errorf("column %s.%s is missing", item.table, column)
			}
		}
	}
	return nil
}

func validateProxyRouteRulePathSchema(db *gorm.DB, validateData bool) error {
	return validateProxyRouteRulePathSchemaWithCache(db, newSchemaIntrospectionCache(db), validateData)
}

func validateProxyRouteRulePathSchemaWithCache(db *gorm.DB, schemaCache *schemaIntrospectionCache, validateData bool) error {
	normalizedColumns := []struct {
		model   any
		table   string
		columns []string
	}{
		{model: &OriginGroup{}, table: "origin_groups", columns: []string{"is_default"}},
		{model: &CachePolicy{}, table: "cache_policies", columns: []string{"name", "is_default"}},
		{model: &SecurityPolicy{}, table: "security_policies", columns: []string{"name", "is_default"}},
		{model: &ProxyRouteRule{}, table: "proxy_route_rules", columns: []string{
			"cache_policy_id",
			"security_policy_id",
			"name",
			"match_type",
			"path",
			"priority",
			"enabled",
			"origin_host_header",
			"origin_sni",
			"origin_tls_verify",
			"origin_ca_bundle",
			"origin_resolve_mode",
		}},
	}
	for _, item := range normalizedColumns {
		for _, column := range item.columns {
			if !schemaCache.HasColumn(item.model, item.table, column) {
				return fmt.Errorf("column %s.%s is missing", item.table, column)
			}
		}
	}
	if !validateData {
		return nil
	}

	var invalidRuleCount int64
	if err := db.Model(&ProxyRouteRule{}).
		Where("match_type NOT IN ?", []string{"default", "prefix", "exact", "regex"}).
		Count(&invalidRuleCount).Error; err != nil {
		return fmt.Errorf("validate proxy route rule match_type failed: %w", err)
	}
	if invalidRuleCount > 0 {
		return fmt.Errorf("proxy_route_rules.match_type contains %d invalid values", invalidRuleCount)
	}
	return nil
}

func validateConfigReleaseSchema(db *gorm.DB) error {
	return validateConfigReleaseSchemaWithCache(newSchemaIntrospectionCache(db))
}

func validateConfigReleaseSchemaWithCache(schemaCache *schemaIntrospectionCache) error {
	releaseTables := []struct {
		model   any
		table   string
		columns []string
	}{
		{model: &ConfigPoolActiveVersion{}, table: "config_pool_active_versions", columns: []string{"id", "pool_name", "config_version_id", "artifact_id", "checksum", "activated_by_plan_id", "activated_at"}},
		{model: &ConfigReleasePlan{}, table: "config_release_plans", columns: []string{"id", "config_version_id", "rollback_version_id", "status", "strategy", "canary_pool_name", "current_stage", "canary_percent", "observe_seconds", "checksum", "failure_reason", "created_by", "started_at", "completed_at", "failed_at"}},
		{model: &ConfigReleaseTarget{}, table: "config_release_targets", columns: []string{"id", "plan_id", "config_version_id", "node_id", "pool_name", "checksum", "stage_index", "status", "failure_reason", "started_at", "completed_at"}},
		{model: &ConfigReleaseBlockedChecksum{}, table: "config_release_blocked_checksums", columns: []string{"id", "pool_name", "config_version_id", "plan_id", "version", "checksum", "reason", "expires_at", "unblocked_at", "unblocked_by", "unblock_reason"}},
		{model: &ConfigReleaseBlockedChecksumAudit{}, table: "config_release_blocked_checksum_audits", columns: []string{"id", "blocked_checksum_id", "pool_name", "checksum", "action", "operator", "original_reason", "reason", "created_at"}},
	}
	for _, item := range releaseTables {
		if !schemaCache.HasTable(item.model, item.table) {
			return fmt.Errorf("table %s is missing", item.table)
		}
		for _, column := range item.columns {
			if !schemaCache.HasColumn(item.model, item.table, column) {
				return fmt.Errorf("column %s.%s is missing", item.table, column)
			}
		}
	}
	return nil
}

func validateCurrentDatabaseSchemaLightweight(db *gorm.DB, backend string) error {
	if db == nil {
		return fmt.Errorf("database handle is nil")
	}
	return validateCurrentDatabaseSchemaLightweightWithCache(db, backend, newSchemaIntrospectionCache(db))
}

func validateCurrentDatabaseSchemaLightweightWithCache(db *gorm.DB, backend string, schemaCache *schemaIntrospectionCache) error {
	if db == nil {
		return fmt.Errorf("database handle is nil")
	}
	if schemaCache == nil {
		schemaCache = newSchemaIntrospectionCache(db)
	}
	if !schemaCache.HasTable(&DatabaseSchemaVersion{}, (&DatabaseSchemaVersion{}).TableName()) {
		return fmt.Errorf("table %s is missing", (&DatabaseSchemaVersion{}).TableName())
	}
	if err := validateCurrentModelTables(schemaCache); err != nil {
		return err
	}
	if err := validateProxyRouteNormalizedSchemaWithCache(db, schemaCache, false); err != nil {
		return err
	}
	if err := validateProxyRouteRulePathSchemaWithCache(db, schemaCache, false); err != nil {
		return err
	}
	if err := validateConfigReleaseSchemaWithCache(schemaCache); err != nil {
		return err
	}
	if err := validateCurrentOriginConnectionSchema(schemaCache); err != nil {
		return err
	}
	if err := validateProxyRouteReusablePolicySchemaWithCache(schemaCache); err != nil {
		return err
	}
	if err := validateCurrentObservabilityMaintenanceSchema(schemaCache); err != nil {
		return err
	}
	_ = backend
	return nil
}

func validateCurrentModelTables(schemaCache *schemaIntrospectionCache) error {
	models, err := buildDBModels()
	if err != nil {
		return err
	}
	for _, item := range models {
		if isShardedObservabilityTable(item.tableName) {
			for _, table := range observabilityShardTables(item.tableName) {
				if !schemaCache.HasPhysicalTable(table) {
					return fmt.Errorf("sharded table %s is missing", table)
				}
				for _, column := range item.columns {
					if !schemaCache.HasPhysicalColumn(table, column) {
						return fmt.Errorf("column %s.%s is missing", table, column)
					}
				}
			}
			continue
		}
		if !schemaCache.HasTable(item.value, item.tableName) {
			return fmt.Errorf("table %s is missing", item.tableName)
		}
		for _, column := range item.columns {
			if !schemaCache.HasColumn(item.value, item.tableName, column) {
				return fmt.Errorf("column %s.%s is missing", item.tableName, column)
			}
		}
	}
	return nil
}

func validateCurrentOriginConnectionSchema(schemaCache *schemaIntrospectionCache) error {
	for _, column := range []string{
		"origin_host_header",
		"origin_sni",
		"origin_tls_verify",
		"origin_ca_bundle",
		"origin_resolve_mode",
	} {
		if !schemaCache.HasColumn(&ProxyRoute{}, (&ProxyRoute{}).TableName(), column) {
			return fmt.Errorf("column proxy_routes.%s is missing", column)
		}
	}
	if !schemaCache.HasTable(&OriginHealthStatus{}, (&OriginHealthStatus{}).TableName()) {
		return errors.New("table origin_health_statuses is missing")
	}
	for _, column := range []string{
		"route_id",
		"node_id",
		"origin_url",
		"status",
		"latency_ms",
		"last_error",
		"reported_at",
		"checked_at",
	} {
		if !schemaCache.HasColumn(&OriginHealthStatus{}, (&OriginHealthStatus{}).TableName(), column) {
			return fmt.Errorf("column origin_health_statuses.%s is missing", column)
		}
	}
	return nil
}

func validateCurrentObservabilityMaintenanceSchema(schemaCache *schemaIntrospectionCache) error {
	if schemaCache == nil {
		return nil
	}
	for _, table := range observabilityShardTables("node_access_logs") {
		if !schemaCache.HasPhysicalTable(table) {
			return fmt.Errorf("sharded table %s is missing", table)
		}
		for _, column := range []string{
			"request_bytes",
			"response_bytes",
			"upstream_bytes",
			"reason",
			"operator",
			"cache_status",
		} {
			if !schemaCache.HasPhysicalColumn(table, column) {
				return fmt.Errorf("column %s.%s is missing", table, column)
			}
		}
	}
	return nil
}
