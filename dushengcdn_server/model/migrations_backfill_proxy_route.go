package model

import (
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"gorm.io/gorm"
	"net"
	"net/url"
	"strings"
	"time"
)

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

func migrateProxyRouteRulePathLevelSchema(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	for _, item := range []struct {
		model any
		index string
	}{
		{model: &ProxyRouteRule{}, index: "idx_proxy_route_rules_proxy_route_id"},
		{model: &OriginGroup{}, index: "idx_origin_groups_proxy_route_id"},
		{model: &CachePolicy{}, index: "idx_cache_policies_proxy_route_id"},
		{model: &SecurityPolicy{}, index: "idx_security_policies_proxy_route_id"},
	} {
		if db.Migrator().HasIndex(item.model, item.index) {
			if err := db.Migrator().DropIndex(item.model, item.index); err != nil {
				return fmt.Errorf("drop legacy unique index %s failed: %w", item.index, err)
			}
		}
	}
	if db.Migrator().HasTable(&ProxyRouteRule{}) {
		// V48 originally upgraded one default rule per route. Keep this bulk
		// defaulting scoped to rows that still look like that legacy shape;
		// path-level rules created by newer code may have one empty column due
		// to partial data, and must not be collapsed back to the root rule.
		defaultRuleUpdates := map[string]any{
			"match_type":          "default",
			"path":                "/",
			"priority":            1000000,
			"enabled":             true,
			"origin_tls_verify":   true,
			"origin_resolve_mode": "publish_resolve",
		}
		if err := db.Model(&ProxyRouteRule{}).
			Where("match_type = '' AND path = ''").
			Updates(defaultRuleUpdates).Error; err != nil {
			return fmt.Errorf("backfill proxy_route_rules path fields failed: %w", err)
		}
		if err := db.Model(&ProxyRouteRule{}).
			Where("origin_resolve_mode = ''").
			Update("origin_resolve_mode", "publish_resolve").Error; err != nil {
			return fmt.Errorf("backfill proxy_route_rules origin_resolve_mode failed: %w", err)
		}
	}
	for _, modelValue := range []any{&OriginGroup{}, &CachePolicy{}, &SecurityPolicy{}} {
		if db.Migrator().HasTable(modelValue) {
			if err := db.Model(modelValue).Where("name = ''").Update("name", "default").Error; err != nil {
				return fmt.Errorf("backfill default policy names failed: %w", err)
			}
			if err := db.Model(modelValue).Where("is_default = ?", false).Update("is_default", true).Error; err != nil {
				return fmt.Errorf("backfill default policy markers failed: %w", err)
			}
		}
	}
	return nil
}

func backfillProxyRouteOriginConnectionFields(db *gorm.DB) error {
	if db == nil || !db.Migrator().HasTable(&ProxyRoute{}) {
		return nil
	}
	if !db.Migrator().HasColumn(&ProxyRoute{}, "origin_host_header") {
		return nil
	}
	if err := db.Model(&ProxyRoute{}).
		Where("origin_host_header = '' AND origin_host <> ''").
		Update("origin_host_header", gorm.Expr("origin_host")).Error; err != nil {
		return fmt.Errorf("backfill proxy route origin_host_header failed: %w", err)
	}
	if db.Migrator().HasColumn(&ProxyRoute{}, "origin_tls_verify") {
		if err := db.Model(&ProxyRoute{}).
			Where("origin_tls_verify = ?", false).
			Update("origin_tls_verify", true).Error; err != nil {
			return fmt.Errorf("backfill proxy route origin_tls_verify failed: %w", err)
		}
	}
	if db.Migrator().HasColumn(&ProxyRoute{}, "origin_resolve_mode") {
		if err := db.Model(&ProxyRoute{}).
			Where("origin_resolve_mode = '' OR origin_resolve_mode NOT IN ?", []string{"runtime_dns", "publish_resolve", "static_ip", "origin_group"}).
			Update("origin_resolve_mode", "publish_resolve").Error; err != nil {
			return fmt.Errorf("backfill proxy route origin_resolve_mode failed: %w", err)
		}
	}
	return nil
}

type legacyProxyRouteBasicAuthPassword struct {
	ID                         uint
	BasicAuthEnabled           bool
	BasicAuthUsername          string
	BasicAuthPassword          string
	BasicAuthPasswordHash      string
	BasicAuthPasswordUpdatedAt *time.Time
	UpdatedAt                  time.Time
}

func migrateProxyRouteBasicAuthPasswordHashes(db *gorm.DB) error {
	if db == nil || !db.Migrator().HasTable(&ProxyRoute{}) {
		return nil
	}
	db = sessionIgnoringSharding(db)
	hasLegacyPasswordColumn, err := databaseTableHasColumn(db, "proxy_routes", "basic_auth_password")
	if err != nil {
		return err
	}
	if !hasLegacyPasswordColumn {
		return nil
	}
	var rows []legacyProxyRouteBasicAuthPassword
	if err := db.Table("proxy_routes").
		Select("id, basic_auth_enabled, basic_auth_username, basic_auth_password, basic_auth_password_hash, basic_auth_password_updated_at, updated_at").
		Where("basic_auth_password <> '' OR (basic_auth_enabled = ? AND basic_auth_password_hash <> '')", true).
		Find(&rows).Error; err != nil {
		return fmt.Errorf("query proxy route basic auth passwords failed: %w", err)
	}
	for _, row := range rows {
		passwordHash := strings.TrimSpace(row.BasicAuthPasswordHash)
		passwordUpdatedAt := row.BasicAuthPasswordUpdatedAt
		if strings.TrimSpace(row.BasicAuthPassword) != "" {
			passwordHash = BasicAuthCredentialHash(row.BasicAuthUsername, row.BasicAuthPassword)
			if passwordUpdatedAt == nil {
				value := row.UpdatedAt
				if value.IsZero() {
					value = time.Now()
				}
				passwordUpdatedAt = &value
			}
		}
		updates := map[string]any{
			"basic_auth_password":            "",
			"basic_auth_password_hash":       passwordHash,
			"basic_auth_password_updated_at": passwordUpdatedAt,
		}
		if !row.BasicAuthEnabled {
			updates["basic_auth_password_hash"] = ""
			updates["basic_auth_password_updated_at"] = gorm.Expr("NULL")
		}
		if err := db.Session(&gorm.Session{NewDB: true}).Table("proxy_routes").Where("id = ?", row.ID).Updates(updates).Error; err != nil {
			return fmt.Errorf("migrate proxy route %d basic auth password failed: %w", row.ID, err)
		}
	}
	return nil
}

func databaseTableHasColumn(db *gorm.DB, table string, column string) (bool, error) {
	db = sessionIgnoringSharding(db)
	if db == nil || strings.TrimSpace(table) == "" || strings.TrimSpace(column) == "" {
		return false, nil
	}
	if !db.Migrator().HasTable(table) {
		return false, nil
	}
	switch databaseDialectorName(db) {
	case "sqlite":
		type pragmaColumn struct {
			Name string
		}
		var columns []pragmaColumn
		if err := db.Raw(fmt.Sprintf("PRAGMA table_info(%s)", quoteIdentifier(table))).Scan(&columns).Error; err != nil {
			return false, fmt.Errorf("query sqlite table %s columns failed: %w", table, err)
		}
		for _, item := range columns {
			if strings.EqualFold(item.Name, column) {
				return true, nil
			}
		}
		return false, nil
	case "postgres":
		var count int64
		if err := db.Raw(
			"SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = ? AND column_name = ?",
			table,
			column,
		).Scan(&count).Error; err != nil {
			return false, fmt.Errorf("query postgres table %s column %s failed: %w", table, column, err)
		}
		return count > 0, nil
	default:
		return db.Migrator().HasColumn(table, column), nil
	}
}
