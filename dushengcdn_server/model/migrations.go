package model

import (
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"

	"gorm.io/gorm"
)

type databaseSchemaMigration struct {
	fromVersion int
	toVersion   int
	migrate     func(db *gorm.DB, backend string) error
	validate    func(db *gorm.DB, backend string) error
}

func autoMigrateSchemaMetadata(db *gorm.DB) error {
	for _, item := range schemaMetadataModels() {
		if err := db.AutoMigrate(item); err != nil {
			return err
		}
	}
	return nil
}

func migrateProxyRouteEnableHTTPSColumn(db *gorm.DB) error {
	if !db.Migrator().HasTable(&ProxyRoute{}) {
		return nil
	}
	if db.Migrator().HasColumn(&ProxyRoute{}, "enable_https") || !db.Migrator().HasColumn(&ProxyRoute{}, "enable_http_s") {
		return nil
	}
	return db.Migrator().RenameColumn(&ProxyRoute{}, "enable_http_s", "enable_https")
}

func migrateTextColumns(db *gorm.DB, backend string) error {
	if backend != "postgres" {
		return nil
	}
	type textColumn struct {
		model  any
		table  string
		column string
	}
	columns := []textColumn{
		{model: &Node{}, table: "nodes", column: "openresty_message"},
		{model: &Node{}, table: "nodes", column: "last_error"},
		{model: &ApplyLog{}, table: "apply_logs", column: "message"},
		{model: &NodeHealthEvent{}, table: "node_health_events", column: "message"},
		{model: &ProxyRoute{}, table: "proxy_routes", column: "dns_record_content"},
	}
	for _, item := range columns {
		if !db.Migrator().HasTable(item.model) || !db.Migrator().HasColumn(item.model, item.column) {
			continue
		}
		sql := fmt.Sprintf(`ALTER TABLE "%s" ALTER COLUMN "%s" TYPE text`, item.table, item.column)
		if err := db.Exec(sql).Error; err != nil {
			return fmt.Errorf("migrate column %s.%s to text failed: %w", item.table, item.column, err)
		}
	}
	return nil
}

func migrateObservabilityLegacyColumns(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	if !db.Migrator().HasTable(&NodeHealthEvent{}) || !db.Migrator().HasColumn(&NodeHealthEvent{}, "raw_json") {
		return nil
	}
	type legacyHealthEventRaw struct {
		ID           uint
		RawJSON      string
		MetadataJSON string
	}
	type legacyHealthEventPayload struct {
		Metadata map[string]string `json:"metadata"`
	}

	var rows []legacyHealthEventRaw
	if err := db.Model(&NodeHealthEvent{}).
		Select("id, raw_json, metadata_json").
		Where("raw_json <> '' AND (metadata_json IS NULL OR metadata_json = '')").
		Find(&rows).Error; err != nil {
		return fmt.Errorf("query legacy node health event raw_json failed: %w", err)
	}
	for _, row := range rows {
		var payload legacyHealthEventPayload
		if err := json.Unmarshal([]byte(row.RawJSON), &payload); err != nil {
			continue
		}
		if len(payload.Metadata) == 0 {
			continue
		}
		metadataJSON, err := json.Marshal(payload.Metadata)
		if err != nil {
			continue
		}
		if err := db.Model(&NodeHealthEvent{}).
			Where("id = ?", row.ID).
			Update("metadata_json", string(metadataJSON)).Error; err != nil {
			return fmt.Errorf("migrate node health event metadata_json failed: %w", err)
		}
	}
	return nil
}

func applyCurrentSchema(db *gorm.DB, backend string) error {
	if err := autoMigrateSchemaMetadata(db); err != nil {
		return err
	}
	if err := migrateProxyRouteEnableHTTPSColumn(db); err != nil {
		return err
	}
	if err := autoMigrateAll(db); err != nil {
		return err
	}
	if err := migrateTextColumns(db, backend); err != nil {
		return err
	}
	if err := migrateObservabilityLegacyColumns(db); err != nil {
		return err
	}
	if err := ensureNodeAccessLogCurrentColumns(db); err != nil {
		return err
	}
	if err := ensureDNSRollupObservabilityIndex(db); err != nil {
		return err
	}
	return ensureObservabilityShardQueryIndexes(db, backend)
}

func loadDatabaseSchemaVersion(db *gorm.DB) (int, bool, error) {
	if db == nil {
		return 0, false, nil
	}
	if !db.Migrator().HasTable(&DatabaseSchemaVersion{}) {
		return 0, false, nil
	}
	var state DatabaseSchemaVersion
	err := db.Where("id = ?", databaseSchemaVersionRowID).First(&state).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return state.Version, true, nil
}

func saveDatabaseSchemaVersion(db *gorm.DB, version int) error {
	return db.Save(&DatabaseSchemaVersion{
		ID:      databaseSchemaVersionRowID,
		Version: version,
	}).Error
}

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

func normalizeProxyRouteDomainForMigration(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func normalizeProxyRouteSiteNameForMigration(raw string, primaryDomain string) string {
	siteName := strings.TrimSpace(raw)
	if siteName != "" {
		return siteName
	}
	return primaryDomain
}

func decodeProxyRouteDomainsForMigration(raw string, fallbackDomain string) ([]string, error) {
	primaryDomain := normalizeProxyRouteDomainForMigration(fallbackDomain)
	text := strings.TrimSpace(raw)
	if text == "" {
		if primaryDomain == "" {
			return nil, fmt.Errorf("proxy route primary domain is empty")
		}
		return []string{primaryDomain}, nil
	}

	var domains []string
	if err := json.Unmarshal([]byte(text), &domains); err != nil {
		return nil, fmt.Errorf("decode proxy route domains failed: %w", err)
	}

	normalized := make([]string, 0, len(domains))
	seen := make(map[string]struct{}, len(domains))
	for _, domain := range domains {
		item := normalizeProxyRouteDomainForMigration(domain)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		normalized = append(normalized, item)
	}
	if len(normalized) == 0 {
		if primaryDomain == "" {
			return nil, fmt.Errorf("proxy route domains are empty")
		}
		return []string{primaryDomain}, nil
	}
	if primaryDomain == "" {
		primaryDomain = normalized[0]
	}
	if normalized[0] != primaryDomain {
		rest := make([]string, 0, len(normalized))
		for _, domain := range normalized {
			if domain == primaryDomain {
				continue
			}
			rest = append(rest, domain)
		}
		normalized = append([]string{primaryDomain}, rest...)
	}
	return normalized, nil
}

func backfillProxyRouteSiteFields(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("database handle is nil")
	}
	if !db.Migrator().HasTable(&ProxyRoute{}) {
		return nil
	}
	if !db.Migrator().HasColumn(&ProxyRoute{}, "site_name") || !db.Migrator().HasColumn(&ProxyRoute{}, "domains") {
		return nil
	}

	var routes []ProxyRoute
	if err := db.Order("id asc").Find(&routes).Error; err != nil {
		return fmt.Errorf("list proxy routes for site field backfill failed: %w", err)
	}
	for _, route := range routes {
		domains, err := decodeProxyRouteDomainsForMigration(route.Domains, route.Domain)
		if err != nil {
			return fmt.Errorf("normalize proxy route %d domains failed: %w", route.ID, err)
		}
		domainsJSON, err := json.Marshal(domains)
		if err != nil {
			return fmt.Errorf("encode proxy route %d domains failed: %w", route.ID, err)
		}

		primaryDomain := domains[0]
		siteName := normalizeProxyRouteSiteNameForMigration(route.SiteName, primaryDomain)
		updates := make(map[string]any, 3)
		if route.Domain != primaryDomain {
			updates["domain"] = primaryDomain
		}
		if route.SiteName != siteName {
			updates["site_name"] = siteName
		}
		if strings.TrimSpace(route.Domains) != string(domainsJSON) {
			updates["domains"] = string(domainsJSON)
		}
		if len(updates) == 0 {
			continue
		}
		if err := db.Model(&ProxyRoute{}).Where("id = ?", route.ID).Updates(updates).Error; err != nil {
			return fmt.Errorf("update proxy route %d site fields failed: %w", route.ID, err)
		}
	}
	return nil
}

func ensureProxyRouteSiteNameUniqueIndex(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("database handle is nil")
	}
	if !db.Migrator().HasTable(&ProxyRoute{}) || !db.Migrator().HasColumn(&ProxyRoute{}, "site_name") {
		return nil
	}
	return db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_proxy_routes_site_name ON proxy_routes(site_name)`).Error
}

func decodeProxyRouteCertIDsForMigration(raw string, fallbackCertID *uint) ([]uint, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		if fallbackCertID == nil || *fallbackCertID == 0 {
			return []uint{}, nil
		}
		return []uint{*fallbackCertID}, nil
	}

	var certIDs []uint
	if err := json.Unmarshal([]byte(text), &certIDs); err != nil {
		return nil, fmt.Errorf("decode proxy route cert_ids failed: %w", err)
	}

	normalized := make([]uint, 0, len(certIDs))
	seen := make(map[uint]struct{}, len(certIDs))
	for _, certID := range certIDs {
		if certID == 0 {
			continue
		}
		if _, ok := seen[certID]; ok {
			continue
		}
		seen[certID] = struct{}{}
		normalized = append(normalized, certID)
	}
	if len(normalized) == 0 && fallbackCertID != nil && *fallbackCertID != 0 {
		return []uint{*fallbackCertID}, nil
	}
	return normalized, nil
}

func backfillProxyRouteCertificateFields(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("database handle is nil")
	}
	if !db.Migrator().HasTable(&ProxyRoute{}) {
		return nil
	}
	if !db.Migrator().HasColumn(&ProxyRoute{}, "cert_ids") {
		return nil
	}

	var routes []ProxyRoute
	if err := db.Order("id asc").Find(&routes).Error; err != nil {
		return fmt.Errorf("list proxy routes for certificate field backfill failed: %w", err)
	}
	for _, route := range routes {
		certIDs, err := decodeProxyRouteCertIDsForMigration(route.CertIDs, route.CertID)
		if err != nil {
			return fmt.Errorf("normalize proxy route %d cert_ids failed: %w", route.ID, err)
		}
		certIDsJSON, err := json.Marshal(certIDs)
		if err != nil {
			return fmt.Errorf("encode proxy route %d cert_ids failed: %w", route.ID, err)
		}

		var primaryCertID *uint
		if len(certIDs) > 0 {
			primaryCertID = &certIDs[0]
		}

		updates := make(map[string]any, 2)
		if strings.TrimSpace(route.CertIDs) != string(certIDsJSON) {
			updates["cert_ids"] = string(certIDsJSON)
		}
		if (route.CertID == nil) != (primaryCertID == nil) || (route.CertID != nil && primaryCertID != nil && *route.CertID != *primaryCertID) {
			updates["cert_id"] = primaryCertID
		}
		if len(updates) == 0 {
			continue
		}
		if err := db.Model(&ProxyRoute{}).Where("id = ?", route.ID).Updates(updates).Error; err != nil {
			return fmt.Errorf("update proxy route %d certificate fields failed: %w", route.ID, err)
		}
	}
	return nil
}

func decodeProxyRouteDomainCertIDsForMigration(
	raw string,
	domainCount int,
) ([]uint, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return []uint{}, nil
	}

	var domainCertIDs []uint
	if err := json.Unmarshal([]byte(text), &domainCertIDs); err != nil {
		return nil, fmt.Errorf("decode proxy route domain_cert_ids failed: %w", err)
	}
	if len(domainCertIDs) == 0 {
		return []uint{}, nil
	}
	if domainCount > 0 && len(domainCertIDs) != domainCount {
		return nil, fmt.Errorf("proxy route domain_cert_ids length does not match domains")
	}

	normalized := make([]uint, len(domainCertIDs))
	copy(normalized, domainCertIDs)
	return normalized, nil
}

func parseLeafCertificateForMigration(certPEM string) (*x509.Certificate, error) {
	var firstErr error
	rest := []byte(certPEM)
	for len(rest) > 0 {
		block, remaining := pem.Decode(rest)
		if block == nil {
			break
		}
		rest = remaining
		if block.Type != "CERTIFICATE" {
			continue
		}
		certificate, err := x509.ParseCertificate(block.Bytes)
		if err == nil {
			return certificate, nil
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	if firstErr != nil {
		return nil, firstErr
	}
	return nil, fmt.Errorf("parse certificate pem failed")
}

func deriveProxyRouteDomainCertIDsForMigration(
	db *gorm.DB,
	domains []string,
	certIDs []uint,
) ([]uint, error) {
	if len(certIDs) == 0 {
		return []uint{}, nil
	}
	if len(certIDs) == 1 {
		result := make([]uint, len(domains))
		for index := range result {
			result[index] = certIDs[0]
		}
		return result, nil
	}
	if len(certIDs) == len(domains) {
		result := make([]uint, len(certIDs))
		copy(result, certIDs)
		return result, nil
	}

	var certificates []TLSCertificate
	if err := db.Where("id IN ?", certIDs).Find(&certificates).Error; err != nil {
		return nil, fmt.Errorf("load certificates for proxy route migration failed: %w", err)
	}
	certificateByID := make(map[uint]*x509.Certificate, len(certificates))
	for index := range certificates {
		leaf, err := parseLeafCertificateForMigration(certificates[index].CertPEM)
		if err != nil {
			return nil, fmt.Errorf("parse certificate %d for proxy route migration failed: %w", certificates[index].ID, err)
		}
		certificateByID[certificates[index].ID] = leaf
	}

	result := make([]uint, len(domains))
	for domainIndex, domain := range domains {
		if domainIndex < len(certIDs) {
			certificate := certificateByID[certIDs[domainIndex]]
			if certificate != nil && certificate.VerifyHostname(domain) == nil {
				result[domainIndex] = certIDs[domainIndex]
				continue
			}
		}

		assigned := uint(0)
		for _, certID := range certIDs {
			certificate := certificateByID[certID]
			if certificate != nil && certificate.VerifyHostname(domain) == nil {
				assigned = certID
				break
			}
		}
		if assigned == 0 {
			return nil, fmt.Errorf("no certificate covers domain %s", domain)
		}
		result[domainIndex] = assigned
	}
	return result, nil
}

func uniqueProxyRouteCertIDsFromDomainAssignments(domainCertIDs []uint) []uint {
	unique := make([]uint, 0, len(domainCertIDs))
	seen := make(map[uint]struct{}, len(domainCertIDs))
	for _, certID := range domainCertIDs {
		if certID == 0 {
			continue
		}
		if _, ok := seen[certID]; ok {
			continue
		}
		seen[certID] = struct{}{}
		unique = append(unique, certID)
	}
	return unique
}

func backfillProxyRouteDomainCertificateFields(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("database handle is nil")
	}
	if !db.Migrator().HasTable(&ProxyRoute{}) {
		return nil
	}
	if !db.Migrator().HasColumn(&ProxyRoute{}, "domain_cert_ids") {
		return nil
	}

	var routes []ProxyRoute
	if err := db.Order("id asc").Find(&routes).Error; err != nil {
		return fmt.Errorf("list proxy routes for domain certificate field backfill failed: %w", err)
	}
	for _, route := range routes {
		domains, err := decodeProxyRouteDomainsForMigration(route.Domains, route.Domain)
		if err != nil {
			return fmt.Errorf("normalize proxy route %d domains failed: %w", route.ID, err)
		}
		certIDs, err := decodeProxyRouteCertIDsForMigration(route.CertIDs, route.CertID)
		if err != nil {
			return fmt.Errorf("normalize proxy route %d cert_ids failed: %w", route.ID, err)
		}

		domainCertIDs, err := decodeProxyRouteDomainCertIDsForMigration(
			route.DomainCertIDs,
			len(domains),
		)
		if err != nil {
			return fmt.Errorf("normalize proxy route %d domain_cert_ids failed: %w", route.ID, err)
		}
		if len(domainCertIDs) == 0 && len(certIDs) > 0 {
			domainCertIDs, err = deriveProxyRouteDomainCertIDsForMigration(
				db,
				domains,
				certIDs,
			)
			if err != nil {
				return fmt.Errorf("derive proxy route %d domain_cert_ids failed: %w", route.ID, err)
			}
		}
		if !route.EnableHTTPS {
			domainCertIDs = []uint{}
			certIDs = []uint{}
		}

		domainCertIDsJSON, err := json.Marshal(domainCertIDs)
		if err != nil {
			return fmt.Errorf("encode proxy route %d domain_cert_ids failed: %w", route.ID, err)
		}
		normalizedCertIDs := uniqueProxyRouteCertIDsFromDomainAssignments(domainCertIDs)
		if len(domainCertIDs) == 0 {
			normalizedCertIDs = []uint{}
		}
		certIDsJSON, err := json.Marshal(normalizedCertIDs)
		if err != nil {
			return fmt.Errorf("encode proxy route %d cert_ids failed: %w", route.ID, err)
		}

		var primaryCertID *uint
		if len(normalizedCertIDs) > 0 {
			primaryCertID = &normalizedCertIDs[0]
		}

		updates := make(map[string]any, 3)
		if strings.TrimSpace(route.DomainCertIDs) != string(domainCertIDsJSON) {
			updates["domain_cert_ids"] = string(domainCertIDsJSON)
		}
		if strings.TrimSpace(route.CertIDs) != string(certIDsJSON) {
			updates["cert_ids"] = string(certIDsJSON)
		}
		if (route.CertID == nil) != (primaryCertID == nil) || (route.CertID != nil && primaryCertID != nil && *route.CertID != *primaryCertID) {
			updates["cert_id"] = primaryCertID
		}
		if len(updates) == 0 {
			continue
		}
		if err := db.Model(&ProxyRoute{}).Where("id = ?", route.ID).Updates(updates).Error; err != nil {
			return fmt.Errorf("update proxy route %d domain certificate fields failed: %w", route.ID, err)
		}
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

func uintSlicesEqualForMigration(left []uint, right []uint) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func renameLegacyObservabilityShardTables(db *gorm.DB) error {
	for _, baseTable := range shardedObservabilityBaseTables() {
		for _, table := range observabilityShardTables(baseTable) {
			legacyTable := legacyObservabilityShardTableName(table)
			if db.Migrator().HasTable(legacyTable) {
				return fmt.Errorf("legacy sharded table %s already exists", legacyTable)
			}
			if !db.Migrator().HasTable(table) {
				continue
			}
			if err := db.Migrator().RenameTable(table, legacyTable); err != nil {
				return fmt.Errorf("rename sharded table %s to %s failed: %w", table, legacyTable, err)
			}
			if err := dropLegacyObservabilitySecondaryIndexes(db, legacyTable); err != nil {
				return err
			}
		}
	}
	return nil
}

func dropLegacyObservabilitySecondaryIndexes(db *gorm.DB, table string) error {
	db = sessionIgnoringSharding(db)
	if db == nil {
		return fmt.Errorf("database handle is nil")
	}
	backend := baseDialector(db).Name()
	indexes := make([]string, 0)
	switch backend {
	case "sqlite":
		if err := db.Raw(
			`SELECT name FROM sqlite_master WHERE type = 'index' AND tbl_name = ? AND name LIKE 'idx_%'`,
			table,
		).Scan(&indexes).Error; err != nil {
			return fmt.Errorf("list indexes for %s failed: %w", table, err)
		}
	case "postgres":
		if err := db.Raw(
			`SELECT indexname FROM pg_indexes WHERE schemaname = current_schema() AND tablename = ? AND indexname LIKE 'idx_%'`,
			table,
		).Scan(&indexes).Error; err != nil {
			return fmt.Errorf("list indexes for %s failed: %w", table, err)
		}
	default:
		return fmt.Errorf("unsupported database backend %s", backend)
	}
	for _, indexName := range indexes {
		if err := db.Exec(fmt.Sprintf(`DROP INDEX IF EXISTS "%s"`, indexName)).Error; err != nil {
			return fmt.Errorf("drop legacy index %s failed: %w", indexName, err)
		}
	}
	return nil
}

func autoMigrateObservabilityShardTables(db *gorm.DB) error {
	db = sessionIgnoringSharding(db)
	if db == nil {
		return fmt.Errorf("database handle is nil")
	}
	dialector := baseDialector(db)
	if dialector == nil {
		return fmt.Errorf("database dialector is nil")
	}
	type shardedTable struct {
		model any
		base  string
	}
	tables := []shardedTable{
		{model: &NodeMetricSnapshot{}, base: "node_metric_snapshots"},
		{model: &NodeRequestReport{}, base: "node_request_reports"},
		{model: &NodeAccessLog{}, base: "node_access_logs"},
	}
	for _, item := range tables {
		for _, table := range observabilityShardTables(item.base) {
			tx := db.Table(table)
			if err := dialector.Migrator(tx).AutoMigrate(item.model); err != nil {
				return fmt.Errorf("auto migrate sharded table %s failed: %w", table, err)
			}
		}
	}
	return nil
}

func dropLegacyObservabilityShardTables(db *gorm.DB) error {
	db = sessionIgnoringSharding(db)
	if db == nil {
		return fmt.Errorf("database handle is nil")
	}
	for _, baseTable := range shardedObservabilityBaseTables() {
		for _, table := range observabilityShardTables(baseTable) {
			legacyTable := legacyObservabilityShardTableName(table)
			if !db.Migrator().HasTable(legacyTable) {
				continue
			}
			if err := db.Exec(fmt.Sprintf(`DROP TABLE IF EXISTS "%s"`, legacyTable)).Error; err != nil {
				return fmt.Errorf("drop legacy sharded table %s failed: %w", legacyTable, err)
			}
		}
	}
	return nil
}

func migrateLegacyNodeMetricSnapshots(db *gorm.DB) error {
	for _, table := range observabilityShardTables("node_metric_snapshots") {
		legacyTable := legacyObservabilityShardTableName(table)
		if !db.Migrator().HasTable(legacyTable) {
			continue
		}
		var lastSeenID uint
		for {
			var rows []NodeMetricSnapshot
			query := db.Table(legacyTable).Order("id ASC").Limit(500)
			if lastSeenID > 0 {
				query = query.Where("id > ?", lastSeenID)
			}
			if err := query.Find(&rows).Error; err != nil {
				return fmt.Errorf("query legacy sharded table %s failed: %w", legacyTable, err)
			}
			if len(rows) == 0 {
				break
			}
			lastSeenID = rows[len(rows)-1].ID
			grouped := make(map[string][]NodeMetricSnapshot, observabilityShardCount)
			for index := range rows {
				rows[index].ID = 0
				if err := assignObservabilityID(&rows[index].ID); err != nil {
					return err
				}
				targetTable := observabilityShardTableForID("node_metric_snapshots", rows[index].ID)
				grouped[targetTable] = append(grouped[targetTable], rows[index])
			}
			for targetTable, batch := range grouped {
				if err := db.Table(targetTable).Create(&batch).Error; err != nil {
					return fmt.Errorf("write migrated rows into %s failed: %w", targetTable, err)
				}
			}
		}
	}
	return nil
}

func migrateLegacyNodeRequestReports(db *gorm.DB) error {
	for _, table := range observabilityShardTables("node_request_reports") {
		legacyTable := legacyObservabilityShardTableName(table)
		if !db.Migrator().HasTable(legacyTable) {
			continue
		}
		var lastSeenID uint
		for {
			var rows []NodeRequestReport
			query := db.Table(legacyTable).Order("id ASC").Limit(500)
			if lastSeenID > 0 {
				query = query.Where("id > ?", lastSeenID)
			}
			if err := query.Find(&rows).Error; err != nil {
				return fmt.Errorf("query legacy sharded table %s failed: %w", legacyTable, err)
			}
			if len(rows) == 0 {
				break
			}
			lastSeenID = rows[len(rows)-1].ID
			grouped := make(map[string][]NodeRequestReport, observabilityShardCount)
			for index := range rows {
				rows[index].ID = 0
				if err := assignObservabilityID(&rows[index].ID); err != nil {
					return err
				}
				targetTable := observabilityShardTableForID("node_request_reports", rows[index].ID)
				grouped[targetTable] = append(grouped[targetTable], rows[index])
			}
			for targetTable, batch := range grouped {
				if err := db.Table(targetTable).Create(&batch).Error; err != nil {
					return fmt.Errorf("write migrated rows into %s failed: %w", targetTable, err)
				}
			}
		}
	}
	return nil
}

func migrateLegacyNodeAccessLogs(db *gorm.DB) error {
	for _, table := range observabilityShardTables("node_access_logs") {
		legacyTable := legacyObservabilityShardTableName(table)
		if !db.Migrator().HasTable(legacyTable) {
			continue
		}
		var lastSeenID uint
		for {
			var rows []NodeAccessLog
			query := db.Table(legacyTable).Order("id ASC").Limit(500)
			if lastSeenID > 0 {
				query = query.Where("id > ?", lastSeenID)
			}
			if err := query.Find(&rows).Error; err != nil {
				return fmt.Errorf("query legacy sharded table %s failed: %w", legacyTable, err)
			}
			if len(rows) == 0 {
				break
			}
			lastSeenID = rows[len(rows)-1].ID
			grouped := make(map[string][]NodeAccessLog, observabilityShardCount)
			for index := range rows {
				rows[index].ID = 0
				if err := assignObservabilityID(&rows[index].ID); err != nil {
					return err
				}
				targetTable := observabilityShardTableForID("node_access_logs", rows[index].ID)
				grouped[targetTable] = append(grouped[targetTable], rows[index])
			}
			for targetTable, batch := range grouped {
				if err := db.Table(targetTable).Create(&batch).Error; err != nil {
					return fmt.Errorf("write migrated rows into %s failed: %w", targetTable, err)
				}
			}
		}
	}
	return nil
}

func normalizeOriginAddressForMigration(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func extractOriginAddressForMigration(rawURL string) string {
	parsed, err := url.ParseRequestURI(strings.TrimSpace(rawURL))
	if err != nil {
		return ""
	}
	return normalizeOriginAddressForMigration(parsed.Hostname())
}

func backfillOriginsFromProxyRoutes(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("database handle is nil")
	}
	if !db.Migrator().HasTable(&Origin{}) || !db.Migrator().HasTable(&ProxyRoute{}) {
		return nil
	}

	var routes []ProxyRoute
	if err := db.Order("id asc").Find(&routes).Error; err != nil {
		return fmt.Errorf("list proxy routes for origin backfill failed: %w", err)
	}

	type originSeed struct {
		ID      uint
		Address string
	}

	originByAddress := make(map[string]originSeed)
	var origins []Origin
	if err := db.Order("id asc").Find(&origins).Error; err != nil {
		return fmt.Errorf("list origins for backfill failed: %w", err)
	}
	for _, origin := range origins {
		address := normalizeOriginAddressForMigration(origin.Address)
		if address == "" {
			continue
		}
		originByAddress[address] = originSeed{ID: origin.ID, Address: address}
	}

	for _, route := range routes {
		address := extractOriginAddressForMigration(route.OriginURL)
		if address == "" {
			continue
		}
		origin, ok := originByAddress[address]
		if !ok {
			name := address
			if ip := net.ParseIP(address); ip != nil {
				name = ip.String()
			}
			record := Origin{
				Name:    name,
				Address: address,
				Remark:  "",
			}
			if err := db.Create(&record).Error; err != nil {
				return fmt.Errorf("create origin for address %s failed: %w", address, err)
			}
			origin = originSeed{ID: record.ID, Address: address}
			originByAddress[address] = origin
		}
		if route.OriginID != nil && *route.OriginID == origin.ID {
			continue
		}
		if err := db.Model(&ProxyRoute{}).
			Where("id = ?", route.ID).
			Update("origin_id", origin.ID).Error; err != nil {
			return fmt.Errorf("backfill proxy route %d origin_id failed: %w", route.ID, err)
		}
	}

	return nil
}

// migrateV2 upgrades the legacy schema to the first versioned schema by
// creating schema metadata, applying the current tables, and backfilling
// compatibility columns.
func migrateV2(db *gorm.DB, backend string) error {
	return applyCurrentSchema(db, backend)
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

func ensureDefaultGitHubAuthSource(db *gorm.DB) error {
	if db == nil || !db.Migrator().HasTable(&AuthSource{}) || !db.Migrator().HasTable(&ExternalAccount{}) {
		return nil
	}

	var githubUserCount int64
	if db.Migrator().HasColumn(&User{}, "github_id") {
		if err := db.Model(&User{}).Where("github_id <> ''").Count(&githubUserCount).Error; err != nil {
			return fmt.Errorf("count legacy github users failed: %w", err)
		}
	}

	optionMap := map[string]string{}
	if db.Migrator().HasTable(&Option{}) {
		var options []Option
		if err := db.Find(&options).Error; err != nil {
			return fmt.Errorf("query options for github auth source migration failed: %w", err)
		}
		for _, option := range options {
			optionMap[option.Key] = option.Value
		}
	}

	clientID := strings.TrimSpace(optionMap["GitHubClientId"])
	clientSecret := strings.TrimSpace(optionMap["GitHubClientSecret"])
	enabled := optionMap["GitHubOAuthEnabled"] == "true" && clientID != "" && clientSecret != ""
	if githubUserCount == 0 && clientID == "" && clientSecret == "" {
		return nil
	}

	source := AuthSource{}
	err := db.Where("type = ? AND name = ?", AuthSourceTypeGitHub, "GitHub").First(&source).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		source = AuthSource{
			Name:         "GitHub",
			Type:         AuthSourceTypeGitHub,
			DisplayName:  "GitHub",
			IsActive:     enabled,
			ClientID:     clientID,
			ClientSecret: clientSecret,
			Scopes:       "user:email",
		}
		if err := db.Create(&source).Error; err != nil {
			return fmt.Errorf("create default github auth source failed: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("query default github auth source failed: %w", err)
	} else {
		updates := map[string]any{}
		if source.ClientID == "" && clientID != "" {
			updates["client_id"] = clientID
		}
		if source.ClientSecret == "" && clientSecret != "" {
			updates["client_secret"] = clientSecret
		}
		if source.Scopes == "" {
			updates["scopes"] = "user:email"
		}
		if enabled && !source.IsActive {
			updates["is_active"] = true
		}
		if len(updates) > 0 {
			if err := db.Model(&source).Updates(updates).Error; err != nil {
				return fmt.Errorf("update default github auth source failed: %w", err)
			}
		}
	}

	if githubUserCount == 0 {
		return nil
	}

	var users []User
	if err := db.Select("id", "github_id", "username", "email").Where("github_id <> ''").Find(&users).Error; err != nil {
		return fmt.Errorf("query legacy github users failed: %w", err)
	}
	for _, user := range users {
		account := ExternalAccount{
			AuthSourceID:     source.ID,
			UserID:           user.Id,
			ExternalID:       user.GitHubId,
			ExternalUsername: user.GitHubId,
			Email:            user.Email,
		}
		if err := db.Where(ExternalAccount{
			AuthSourceID: source.ID,
			ExternalID:   user.GitHubId,
		}).FirstOrCreate(&account).Error; err != nil {
			return fmt.Errorf("migrate github external account for user %d failed: %w", user.Id, err)
		}
	}
	return nil
}

// migrateV10 adds configurable auth sources and external account bindings.
func migrateV10(db *gorm.DB, backend string) error {
	if err := applyCurrentSchema(db, backend); err != nil {
		return err
	}
	return ensureDefaultGitHubAuthSource(db)
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

// migrateV11 adds acme and dns accounts and extends tls_certificates.
func migrateV11(db *gorm.DB, backend string) error {
	if err := applyCurrentSchema(db, backend); err != nil {
		return err
	}
	// Default values will be applied by gorm for new columns automatically during AutoMigrate.
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

// migrateV12 adds basic authentication fields to proxy_routes.
func migrateV12(db *gorm.DB, backend string) error {
	if err := applyCurrentSchema(db, backend); err != nil {
		return err
	}
	// Default values will be applied by gorm for new columns automatically during AutoMigrate.
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

// migrateV13 adds Cloudflare DNS automation fields to proxy_routes.
func migrateV13(db *gorm.DB, backend string) error {
	if err := applyCurrentSchema(db, backend); err != nil {
		return err
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

// migrateV14 adds country/region restriction fields to proxy_routes.
func migrateV14(db *gorm.DB, backend string) error {
	if err := applyCurrentSchema(db, backend); err != nil {
		return err
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

// migrateV15 adds optional local WAF fields to proxy_routes.
func migrateV15(db *gorm.DB, backend string) error {
	if err := applyCurrentSchema(db, backend); err != nil {
		return err
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

// migrateV16 adds basic CDN scheduling metadata and cache/upstream observability counters.
func migrateV16(db *gorm.DB, backend string) error {
	if err := applyCurrentSchema(db, backend); err != nil {
		return err
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

// migrateV17 adds byte-level access log fields used by observability metering.
func migrateV17(db *gorm.DB, backend string) error {
	if err := applyCurrentSchema(db, backend); err != nil {
		return err
	}
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

// migrateV18 adds GSLB policy fields, DNS TTL, and scheduler debounce state.
func migrateV18(db *gorm.DB, backend string) error {
	if err := applyCurrentSchema(db, backend); err != nil {
		return err
	}
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

func ensureGSLBSchedulingStateScopeIndex(db *gorm.DB) error {
	if db == nil || !db.Migrator().HasTable(&GSLBSchedulingState{}) {
		return nil
	}
	if db.Migrator().HasColumn(&GSLBSchedulingState{}, "scope_key") {
		if err := db.Model(&GSLBSchedulingState{}).
			Where("scope_key = '' OR scope_key IS NULL").
			Update("scope_key", "global").Error; err != nil {
			return fmt.Errorf("backfill gslb_scheduling_states.scope_key failed: %w", err)
		}
	}
	legacyIndexNames := []string{
		"idx_gslb_scheduling_states_proxy_route_id",
		"idx_gslb_scheduling_states_proxy_route_id_unique",
	}
	for _, indexName := range legacyIndexNames {
		if db.Migrator().HasIndex(&GSLBSchedulingState{}, indexName) {
			if err := db.Migrator().DropIndex(&GSLBSchedulingState{}, indexName); err != nil {
				return fmt.Errorf("drop legacy gslb scheduling state index %s failed: %w", indexName, err)
			}
		}
	}
	return nil
}

func ensureDNSRollupObservabilityIndex(db *gorm.DB) error {
	if db == nil || !db.Migrator().HasTable(&DNSQueryRollup{}) {
		return nil
	}
	const indexName = "idx_dns_rollups_observability"
	if db.Migrator().HasIndex(&DNSQueryRollup{}, indexName) {
		return nil
	}
	if err := db.Migrator().CreateIndex(&DNSQueryRollup{}, indexName); err != nil {
		return fmt.Errorf("create dns query rollup observability index failed: %w", err)
	}
	return nil
}

func ensureObservabilityShardQueryIndexes(db *gorm.DB, backend string) error {
	db = sessionIgnoringSharding(db)
	if db == nil {
		return nil
	}
	indexes := []struct {
		baseTable string
		name      string
		columns   []string
	}{
		{baseTable: "node_access_logs", name: "idx_node_access_logs_remote_addr_logged_at", columns: []string{"remote_addr", "logged_at"}},
		{baseTable: "node_access_logs", name: "idx_node_access_logs_host_logged_at", columns: []string{"host", "logged_at"}},
		{baseTable: "node_metric_snapshots", name: "idx_node_metric_snapshots_node_captured_at", columns: []string{"node_id", "captured_at"}},
		{baseTable: "node_request_reports", name: "idx_node_request_reports_node_window_ended_at", columns: []string{"node_id", "window_ended_at"}},
	}
	for _, item := range indexes {
		for _, table := range observabilityShardTables(item.baseTable) {
			if !db.Migrator().HasTable(table) {
				continue
			}
			indexName := fmt.Sprintf("%s_%s", item.name, strings.TrimPrefix(table, item.baseTable+"_"))
			if err := createObservabilityShardIndexIfMissing(db, backend, table, indexName, item.columns); err != nil {
				return err
			}
		}
	}
	return nil
}

func createObservabilityShardIndexIfMissing(db *gorm.DB, backend string, table string, indexName string, columns []string) error {
	if len(columns) == 0 {
		return nil
	}
	columnSQL := make([]string, 0, len(columns))
	for _, column := range columns {
		columnSQL = append(columnSQL, quoteIdentifier(column))
	}
	sql := fmt.Sprintf(
		"CREATE INDEX IF NOT EXISTS %s ON %s (%s)",
		quoteIdentifier(indexName),
		quoteIdentifier(table),
		strings.Join(columnSQL, ", "),
	)
	switch backend {
	case "sqlite", "postgres":
		if err := db.Exec(sql).Error; err != nil {
			return fmt.Errorf("create observability shard index %s on %s failed: %w", indexName, table, err)
		}
		return nil
	default:
		return fmt.Errorf("unsupported database backend %s", backend)
	}
}

func quoteIdentifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

func ensureNodeAccessLogCurrentColumns(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	columns := []struct {
		field  string
		column string
	}{
		{field: "RequestBytes", column: "request_bytes"},
		{field: "ResponseBytes", column: "response_bytes"},
		{field: "UpstreamBytes", column: "upstream_bytes"},
		{field: "Reason", column: "reason"},
		{field: "Operator", column: "operator"},
		{field: "CacheStatus", column: "cache_status"},
	}
	db = sessionIgnoringSharding(db)
	if db == nil {
		return nil
	}
	for _, table := range observabilityShardTables("node_access_logs") {
		if !db.Migrator().HasTable(table) {
			continue
		}
		for _, column := range columns {
			if db.Migrator().HasColumn(table, column.column) {
				continue
			}
			if err := db.Table(table).Migrator().AddColumn(&NodeAccessLog{}, column.field); err != nil {
				return fmt.Errorf("add node access log %s column to %s failed: %w", column.column, table, err)
			}
		}
	}
	return nil
}

func ensureNodeAccessLogOperatorColumn(db *gorm.DB) error {
	return ensureNodeAccessLogCurrentColumns(db)
}

func ensureNodeAccessLogCacheStatusColumn(db *gorm.DB) error {
	return ensureNodeAccessLogCurrentColumns(db)
}

// migrateV19 adds authoritative DNS control-plane tables and source-scoped GSLB state.
func migrateV19(db *gorm.DB, backend string) error {
	if err := applyCurrentSchema(db, backend); err != nil {
		return err
	}
	return ensureGSLBSchedulingStateScopeIndex(db)
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

// migrateV20 adds DNS Worker query duration rollup fields.
func migrateV20(db *gorm.DB, backend string) error {
	return applyCurrentSchema(db, backend)
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

// migrateV21 adds persisted DNS Worker probe status fields.
func migrateV21(db *gorm.DB, backend string) error {
	return applyCurrentSchema(db, backend)
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

// migrateV23 adds DNS Worker GeoIP runtime status fields.
func migrateV23(db *gorm.DB, backend string) error {
	return applyCurrentSchema(db, backend)
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

// migrateV24 adds node-side DNS Worker active probe status.
func migrateV24(db *gorm.DB, backend string) error {
	return applyCurrentSchema(db, backend)
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

// migrateV25 adds DDoS protection provider and target fields to proxy_routes.
func migrateV25(db *gorm.DB, backend string) error {
	return applyCurrentSchema(db, backend)
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

// migrateV26 adds ACME certificate DNS provider selection fields.
func migrateV26(db *gorm.DB, backend string) error {
	return applyCurrentSchema(db, backend)
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

// migrateV27 adds access log reason fields for protection hit explanations.
func migrateV27(db *gorm.DB, backend string) error {
	return applyCurrentSchema(db, backend)
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

// migrateV28 adds node-side CC protection fields to proxy_routes.
func migrateV28(db *gorm.DB, backend string) error {
	return applyCurrentSchema(db, backend)
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

// migrateV29 adds per-request cache status to access logs for cache diagnostics.
func migrateV29(db *gorm.DB, backend string) error {
	return applyCurrentSchema(db, backend)
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

// migrateV30 adds the commercial license singleton used by private deployments.
func migrateV30(db *gorm.DB, backend string) error {
	return applyCurrentSchema(db, backend)
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

// migrateV31 adds lookup indexes for DNS Worker node probe health summaries.
func migrateV31(db *gorm.DB, backend string) error {
	return applyCurrentSchema(db, backend)
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

// migrateV32 adds online activation lease state to commercial licenses.
func migrateV32(db *gorm.DB, backend string) error {
	return applyCurrentSchema(db, backend)
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

// migrateV33 adds license-level commercial activation revocations.
func migrateV33(db *gorm.DB, backend string) error {
	return applyCurrentSchema(db, backend)
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

// migrateV34 adds DNS Worker heartbeat and query rollup health markers.
func migrateV34(db *gorm.DB, backend string) error {
	return applyCurrentSchema(db, backend)
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

// migrateV35 adds DNS Worker source database capability fields.
func migrateV35(db *gorm.DB, backend string) error {
	if backend == "postgres" {
		for _, column := range []struct {
			name       string
			definition string
		}{
			{name: "asn_database_path", definition: "varchar(512) NOT NULL DEFAULT ''"},
			{name: "asn_last_error", definition: "text NOT NULL DEFAULT ''"},
			{name: "geo_ip_database_type", definition: "varchar(128) NOT NULL DEFAULT ''"},
			{name: "asn_database_type", definition: "varchar(128) NOT NULL DEFAULT ''"},
			{name: "geo_ip_country_enabled", definition: "boolean NOT NULL DEFAULT false"},
			{name: "geo_ip_asn_enabled", definition: "boolean NOT NULL DEFAULT false"},
			{name: "geo_ip_operator_enabled", definition: "boolean NOT NULL DEFAULT false"},
			{name: "operator_cidr_database_path", definition: "varchar(512) NOT NULL DEFAULT ''"},
			{name: "operator_cidr_last_error", definition: "text NOT NULL DEFAULT ''"},
		} {
			sql := fmt.Sprintf(`ALTER TABLE "dns_workers" ADD COLUMN IF NOT EXISTS "%s" %s`, column.name, column.definition)
			if err := db.Exec(sql).Error; err != nil {
				return fmt.Errorf("add dns_workers.%s column failed: %w", column.name, err)
			}
		}
		return nil
	}
	return applyCurrentSchema(db, backend)
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

// migrateV36 adds manual DNS Worker update request fields.
func migrateV36(db *gorm.DB, backend string) error {
	if backend == "postgres" {
		for _, column := range []struct {
			name       string
			definition string
		}{
			{name: "update_requested", definition: "boolean NOT NULL DEFAULT false"},
			{name: "update_channel", definition: "varchar(32) NOT NULL DEFAULT 'stable'"},
			{name: "update_tag", definition: "varchar(128) NOT NULL DEFAULT ''"},
		} {
			sql := fmt.Sprintf(`ALTER TABLE "dns_workers" ADD COLUMN IF NOT EXISTS "%s" %s`, column.name, column.definition)
			if err := db.Exec(sql).Error; err != nil {
				return fmt.Errorf("add dns_workers.%s column failed: %w", column.name, err)
			}
		}
		return nil
	}
	return applyCurrentSchema(db, backend)
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

// migrateV37 adds DNS Worker self-update capability state.
func migrateV37(db *gorm.DB, backend string) error {
	if backend == "postgres" {
		for _, column := range []struct {
			name       string
			definition string
		}{
			{name: "update_supported", definition: "boolean NOT NULL DEFAULT false"},
			{name: "last_update_supported_at", definition: "timestamptz"},
		} {
			sql := fmt.Sprintf(`ALTER TABLE "dns_workers" ADD COLUMN IF NOT EXISTS "%s" %s`, column.name, column.definition)
			if err := db.Exec(sql).Error; err != nil {
				return fmt.Errorf("add dns_workers.%s column failed: %w", column.name, err)
			}
		}
		return nil
	}
	return applyCurrentSchema(db, backend)
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

// migrateV38 adds DNS Worker operator-facing remarks.
func migrateV38(db *gorm.DB, backend string) error {
	if backend == "postgres" {
		sql := `ALTER TABLE "dns_workers" ADD COLUMN IF NOT EXISTS "remark" varchar(255) NOT NULL DEFAULT ''`
		if err := db.Exec(sql).Error; err != nil {
			return fmt.Errorf("add dns_workers.remark column failed: %w", err)
		}
		return nil
	}
	return applyCurrentSchema(db, backend)
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

// migrateV39 adds DNS Worker remote uninstall request state.
func migrateV39(db *gorm.DB, backend string) error {
	if backend == "postgres" {
		for _, column := range []struct {
			name       string
			definition string
		}{
			{name: "uninstall_supported", definition: "boolean NOT NULL DEFAULT false"},
			{name: "last_uninstall_supported_at", definition: "timestamptz"},
			{name: "uninstall_requested", definition: "boolean NOT NULL DEFAULT false"},
			{name: "uninstall_requested_at", definition: "timestamptz"},
		} {
			sql := fmt.Sprintf(`ALTER TABLE "dns_workers" ADD COLUMN IF NOT EXISTS "%s" %s`, column.name, column.definition)
			if err := db.Exec(sql).Error; err != nil {
				return fmt.Errorf("add dns_workers.%s column failed: %w", column.name, err)
			}
		}
		return nil
	}
	return applyCurrentSchema(db, backend)
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

func databaseSchemaMigrations() []databaseSchemaMigration {
	return []databaseSchemaMigration{
		{fromVersion: 1, toVersion: 2, migrate: migrateV2, validate: validateDatabaseSchemaV2},
		{fromVersion: 2, toVersion: 3, migrate: migrateV3, validate: validateDatabaseSchemaV3},
		{fromVersion: 3, toVersion: 4, migrate: migrateV4, validate: validateDatabaseSchemaV4},
		{fromVersion: 4, toVersion: 5, migrate: migrateV5, validate: validateDatabaseSchemaV5},
		{fromVersion: 5, toVersion: 6, migrate: migrateV6, validate: validateDatabaseSchemaV6},
		{fromVersion: 6, toVersion: 7, migrate: migrateV7, validate: validateDatabaseSchemaV7},
		{fromVersion: 7, toVersion: 8, migrate: migrateV8, validate: validateDatabaseSchemaV8},
		{fromVersion: 8, toVersion: 9, migrate: migrateV9, validate: validateDatabaseSchemaV9},
		{fromVersion: 9, toVersion: 10, migrate: migrateV10, validate: validateDatabaseSchemaV10},
		{fromVersion: 10, toVersion: 11, migrate: migrateV11, validate: validateDatabaseSchemaV11},
		{fromVersion: 11, toVersion: 12, migrate: migrateV12, validate: validateDatabaseSchemaV12},
		{fromVersion: 12, toVersion: 13, migrate: migrateV13, validate: validateDatabaseSchemaV13},
		{fromVersion: 13, toVersion: 14, migrate: migrateV14, validate: validateDatabaseSchemaV14},
		{fromVersion: 14, toVersion: 15, migrate: migrateV15, validate: validateDatabaseSchemaV15},
		{fromVersion: 15, toVersion: 16, migrate: migrateV16, validate: validateDatabaseSchemaV16},
		{fromVersion: 16, toVersion: 17, migrate: migrateV17, validate: validateDatabaseSchemaV17},
		{fromVersion: 17, toVersion: 18, migrate: migrateV18, validate: validateDatabaseSchemaV18},
		{fromVersion: 18, toVersion: 19, migrate: migrateV19, validate: validateDatabaseSchemaV19},
		{fromVersion: 19, toVersion: 20, migrate: migrateV20, validate: validateDatabaseSchemaV20},
		{fromVersion: 20, toVersion: 21, migrate: migrateV21, validate: validateDatabaseSchemaV21},
		{fromVersion: 21, toVersion: 22, migrate: migrateV22, validate: validateDatabaseSchemaV22},
		{fromVersion: 22, toVersion: 23, migrate: migrateV23, validate: validateDatabaseSchemaV23},
		{fromVersion: 23, toVersion: 24, migrate: migrateV24, validate: validateDatabaseSchemaV24},
		{fromVersion: 24, toVersion: 25, migrate: migrateV25, validate: validateDatabaseSchemaV25},
		{fromVersion: 25, toVersion: 26, migrate: migrateV26, validate: validateDatabaseSchemaV26},
		{fromVersion: 26, toVersion: 27, migrate: migrateV27, validate: validateDatabaseSchemaV27},
		{fromVersion: 27, toVersion: 28, migrate: migrateV28, validate: validateDatabaseSchemaV28},
		{fromVersion: 28, toVersion: 29, migrate: migrateV29, validate: validateDatabaseSchemaV29},
		{fromVersion: 29, toVersion: 30, migrate: migrateV30, validate: validateDatabaseSchemaV30},
		{fromVersion: 30, toVersion: 31, migrate: migrateV31, validate: validateDatabaseSchemaV31},
		{fromVersion: 31, toVersion: 32, migrate: migrateV32, validate: validateDatabaseSchemaV32},
		{fromVersion: 32, toVersion: 33, migrate: migrateV33, validate: validateDatabaseSchemaV33},
		{fromVersion: 33, toVersion: 34, migrate: migrateV34, validate: validateDatabaseSchemaV34},
		{fromVersion: 34, toVersion: 35, migrate: migrateV35, validate: validateDatabaseSchemaV35},
		{fromVersion: 35, toVersion: 36, migrate: migrateV36, validate: validateDatabaseSchemaV36},
		{fromVersion: 36, toVersion: 37, migrate: migrateV37, validate: validateDatabaseSchemaV37},
		{fromVersion: 37, toVersion: 38, migrate: migrateV38, validate: validateDatabaseSchemaV38},
		{fromVersion: 38, toVersion: 39, migrate: migrateV39, validate: validateDatabaseSchemaV39},
	}
}

func databaseSchemaMigrationMap() map[int]databaseSchemaMigration {
	migrations := make(map[int]databaseSchemaMigration, len(databaseSchemaMigrations()))
	for _, item := range databaseSchemaMigrations() {
		migrations[item.fromVersion] = item
	}
	return migrations
}

func runDatabaseSchemaMigration(db *gorm.DB, backend string, migration databaseSchemaMigration) error {
	if backend == "sqlite" {
		if err := migration.migrate(db, backend); err != nil {
			return fmt.Errorf("migrate database schema from v%d to v%d failed: %w", migration.fromVersion, migration.toVersion, err)
		}
		if err := migration.validate(db, backend); err != nil {
			return fmt.Errorf("validate database schema v%d failed: %w", migration.toVersion, err)
		}
		if err := saveDatabaseSchemaVersion(db, migration.toVersion); err != nil {
			return fmt.Errorf("persist database schema version v%d failed: %w", migration.toVersion, err)
		}
		return nil
	}

	return db.Transaction(func(tx *gorm.DB) error {
		if err := migration.migrate(tx, backend); err != nil {
			return fmt.Errorf("migrate database schema from v%d to v%d failed: %w", migration.fromVersion, migration.toVersion, err)
		}
		if err := migration.validate(tx, backend); err != nil {
			return fmt.Errorf("validate database schema v%d failed: %w", migration.toVersion, err)
		}
		if err := saveDatabaseSchemaVersion(tx, migration.toVersion); err != nil {
			return fmt.Errorf("persist database schema version v%d failed: %w", migration.toVersion, err)
		}
		return nil
	})
}

func upgradeDatabaseSchema(db *gorm.DB, backend string, version int) error {
	if version > currentDatabaseSchemaVersion {
		return fmt.Errorf("database schema version %d is newer than application version %d", version, currentDatabaseSchemaVersion)
	}
	if version == currentDatabaseSchemaVersion {
		if err := applyCurrentSchema(db, backend); err != nil {
			return err
		}
		return validateDatabaseSchemaV39(db, backend)
	}
	migrationMap := databaseSchemaMigrationMap()
	for version < currentDatabaseSchemaVersion {
		migration, ok := migrationMap[version]
		if !ok {
			return fmt.Errorf("database schema migration from v%d is not defined", version)
		}
		if err := runDatabaseSchemaMigration(db, backend, migration); err != nil {
			return err
		}
		version = migration.toVersion
	}
	if err := applyCurrentSchema(db, backend); err != nil {
		return err
	}
	return validateDatabaseSchemaV39(db, backend)
}

func initializeFreshDatabaseSchema(db *gorm.DB, backend string) error {
	if err := applyCurrentSchema(db, backend); err != nil {
		return err
	}
	if err := migrateSQLiteDataIfNeeded(db, backend); err != nil {
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
	if err := backfillProxyRouteDomainCertificateFields(db); err != nil {
		return err
	}
	if err := ensureDefaultGitHubAuthSource(db); err != nil {
		return err
	}
	if err := ensureGSLBSchedulingStateScopeIndex(db); err != nil {
		return err
	}
	if err := validateDatabaseSchemaV39(db, backend); err != nil {
		return err
	}
	return saveDatabaseSchemaVersion(db, currentDatabaseSchemaVersion)
}

func ensureDatabaseSchemaUpToDate(db *gorm.DB, backend string) error {
	version, exists, err := loadDatabaseSchemaVersion(db)
	if err != nil {
		return err
	}
	if exists {
		return upgradeDatabaseSchema(db, backend, version)
	}
	empty, err := isDatabaseEmpty(db)
	if err != nil {
		return err
	}
	if empty {
		return initializeFreshDatabaseSchema(db, backend)
	}
	if err := autoMigrateSchemaMetadata(db); err != nil {
		return err
	}
	return upgradeDatabaseSchema(db, backend, legacyDatabaseSchemaVersion)
}
