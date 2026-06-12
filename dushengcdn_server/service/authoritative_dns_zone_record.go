package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"dushengcdn/model"

	"gorm.io/gorm"
)

func ListAuthoritativeDNSZones() ([]DNSZoneView, error) {
	zones, err := model.ListDNSZones()
	if err != nil {
		return nil, err
	}
	recordCounts, err := dnsZoneRecordCountMap(zones)
	if err != nil {
		return nil, err
	}
	views := make([]DNSZoneView, 0, len(zones))
	for _, zone := range zones {
		var recordCount int64
		if zone != nil {
			recordCount = recordCounts[zone.ID]
		}
		view, err := buildDNSZoneViewWithRecordCount(zone, false, recordCount)
		if err != nil {
			return nil, err
		}
		views = append(views, *view)
	}
	return views, nil
}

func dnsZoneRecordCountMap(zones []*model.DNSZone) (map[uint]int64, error) {
	zoneIDs := make([]uint, 0, len(zones))
	for _, zone := range zones {
		if zone != nil && zone.ID != 0 {
			zoneIDs = append(zoneIDs, zone.ID)
		}
	}
	rows, err := model.ListDNSRecordCountsByZoneIDs(zoneIDs)
	if err != nil {
		return nil, err
	}
	counts := make(map[uint]int64, len(rows))
	for _, row := range rows {
		counts[row.ZoneID] = row.Count
	}
	return counts, nil
}

func GetAuthoritativeDNSZone(id uint) (*DNSZoneView, error) {
	zone, err := model.GetDNSZoneByID(id)
	if err != nil {
		return nil, err
	}
	return buildDNSZoneView(zone, true)
}

func CreateAuthoritativeDNSZone(input DNSZoneInput) (*DNSZoneView, error) {
	if err := EnsureCommercialFeatureEnabled(CommercialFeatureAuthoritativeDNS); err != nil {
		return nil, err
	}
	zone, err := buildDNSZone(nil, input)
	if err != nil {
		return nil, err
	}
	if err := zone.Insert(); err != nil {
		if isUniqueConstraintError(err) {
			return nil, errors.New("DNS zone already exists")
		}
		return nil, err
	}
	if err := assignDNSZoneToAllWorkers(zone.ID); err != nil {
		return nil, err
	}
	return buildDNSZoneView(zone, true)
}

func UpdateAuthoritativeDNSZone(id uint, input DNSZoneInput) (*DNSZoneView, error) {
	if err := EnsureCommercialFeatureEnabled(CommercialFeatureAuthoritativeDNS); err != nil {
		return nil, err
	}
	zone, err := model.GetDNSZoneByID(id)
	if err != nil {
		return nil, err
	}
	zone, err = buildDNSZone(zone, input)
	if err != nil {
		return nil, err
	}
	if err := zone.Update(); err != nil {
		if isUniqueConstraintError(err) {
			return nil, errors.New("DNS zone already exists")
		}
		return nil, err
	}
	return buildDNSZoneView(zone, true)
}

func DeleteAuthoritativeDNSZone(id uint) error {
	if err := EnsureCommercialFeatureEnabled(CommercialFeatureAuthoritativeDNS); err != nil {
		return err
	}
	zone, err := model.GetDNSZoneByID(id)
	if err != nil {
		return err
	}
	var routeCount int64
	if err := model.DB.Model(&model.ProxyRoute{}).Where("dns_zone_id_ref = ?", id).Count(&routeCount).Error; err != nil {
		return err
	}
	if routeCount > 0 {
		return errors.New("DNS zone is used by authoritative proxy routes")
	}
	var certificateCount int64
	if err := model.DB.Model(&model.TLSCertificate{}).Where("dns_provider_mode = ? AND dns_zone_id_ref = ?", DNSProviderModeAuthoritative, id).Count(&certificateCount).Error; err != nil {
		return err
	}
	if certificateCount > 0 {
		return errors.New("DNS zone is used by certificate validation")
	}
	return model.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("zone_id = ?", zone.ID).Delete(&model.DNSRecord{}).Error; err != nil {
			return err
		}
		return tx.Delete(zone).Error
	})
}

func GetAuthoritativeDNSZoneWorkerAssignments(zoneID uint) (*DNSZoneWorkerAssignmentView, error) {
	if _, err := model.GetDNSZoneByID(zoneID); err != nil {
		return nil, err
	}
	assignments, err := model.ListDNSZoneWorkerAssignments(zoneID)
	if err != nil {
		return nil, err
	}
	workers, err := model.ListDNSWorkers()
	if err != nil {
		return nil, err
	}
	workerByID := make(map[uint]*model.DNSWorker, len(workers))
	for _, worker := range workers {
		if worker != nil {
			workerByID[worker.ID] = worker
		}
	}
	view := &DNSZoneWorkerAssignmentView{
		ZoneID:    zoneID,
		WorkerIDs: []uint{},
		Workers:   []DNSWorkerView{},
	}
	for _, assignment := range assignments {
		if assignment == nil || assignment.WorkerID == 0 {
			continue
		}
		view.WorkerIDs = append(view.WorkerIDs, assignment.WorkerID)
		if worker := workerByID[assignment.WorkerID]; worker != nil {
			view.Workers = append(view.Workers, buildDNSWorkerView(worker, false))
		}
	}
	return view, nil
}

func UpdateAuthoritativeDNSZoneWorkerAssignments(zoneID uint, input DNSZoneWorkerAssignmentInput) (*DNSZoneWorkerAssignmentView, error) {
	if err := EnsureCommercialFeatureEnabled(CommercialFeatureAuthoritativeDNS); err != nil {
		return nil, err
	}
	if _, err := model.GetDNSZoneByID(zoneID); err != nil {
		return nil, err
	}
	workerIDs := normalizeUintIDs(input.WorkerIDs)
	if len(workerIDs) == 0 {
		var err error
		workerIDs, err = listAuthoritativeDNSWorkerIDs()
		if err != nil {
			return nil, err
		}
	}
	for _, workerID := range workerIDs {
		if _, err := model.GetDNSWorkerByID(workerID); err != nil {
			return nil, fmt.Errorf("DNS worker %d does not exist", workerID)
		}
	}
	if err := model.ReplaceDNSZoneWorkerAssignments(zoneID, workerIDs); err != nil {
		return nil, err
	}
	return GetAuthoritativeDNSZoneWorkerAssignments(zoneID)
}

func assignDNSZoneToAllWorkers(zoneID uint) error {
	workerIDs, err := listAuthoritativeDNSWorkerIDs()
	if err != nil {
		return err
	}
	if len(workerIDs) == 0 {
		return nil
	}
	return model.ReplaceDNSZoneWorkerAssignments(zoneID, workerIDs)
}

func listAuthoritativeDNSWorkerIDs() ([]uint, error) {
	workers, err := model.ListDNSWorkers()
	if err != nil {
		return nil, err
	}
	workerIDs := make([]uint, 0, len(workers))
	for _, worker := range workers {
		if worker == nil || worker.ID == 0 {
			continue
		}
		workerIDs = append(workerIDs, worker.ID)
	}
	return workerIDs, nil
}

func ListAuthoritativeDNSRecords(zoneID uint) ([]DNSRecordView, error) {
	if _, err := model.GetDNSZoneByID(zoneID); err != nil {
		return nil, err
	}
	records, err := model.ListDNSRecordsByZoneID(zoneID)
	if err != nil {
		return nil, err
	}
	views := make([]DNSRecordView, 0, len(records))
	for _, record := range records {
		views = append(views, buildDNSRecordView(record))
	}
	return views, nil
}

func CreateAuthoritativeDNSRecord(zoneID uint, input DNSRecordInput) (*DNSRecordView, error) {
	if err := EnsureCommercialFeatureEnabled(CommercialFeatureAuthoritativeDNS); err != nil {
		return nil, err
	}
	if input.ZoneID == 0 {
		input.ZoneID = zoneID
	}
	if input.ZoneID != zoneID {
		return nil, errors.New("record zone_id does not match request zone")
	}
	record, err := buildDNSRecord(nil, input)
	if err != nil {
		return nil, err
	}
	if err := validateAuthoritativeDNSRecordDynamicConflicts(record); err != nil {
		return nil, err
	}
	if err := validateAuthoritativeDNSRecordStaticConflicts(record); err != nil {
		return nil, err
	}
	if err := record.Insert(); err != nil {
		return nil, err
	}
	if err := bumpDNSZoneSerial(record.ZoneID); err != nil {
		return nil, err
	}
	return ptrDNSRecordView(buildDNSRecordView(record)), nil
}

func UpdateAuthoritativeDNSRecord(id uint, input DNSRecordInput) (*DNSRecordView, error) {
	if err := EnsureCommercialFeatureEnabled(CommercialFeatureAuthoritativeDNS); err != nil {
		return nil, err
	}
	record, err := model.GetDNSRecordByID(id)
	if err != nil {
		return nil, err
	}
	if input.ZoneID == 0 {
		input.ZoneID = record.ZoneID
	}
	if input.ZoneID != record.ZoneID {
		return nil, errors.New("moving DNS records between zones is not supported")
	}
	record, err = buildDNSRecord(record, input)
	if err != nil {
		return nil, err
	}
	if err := validateAuthoritativeDNSRecordDynamicConflicts(record); err != nil {
		return nil, err
	}
	if err := validateAuthoritativeDNSRecordStaticConflicts(record); err != nil {
		return nil, err
	}
	if err := record.Update(); err != nil {
		return nil, err
	}
	if err := bumpDNSZoneSerial(record.ZoneID); err != nil {
		return nil, err
	}
	return ptrDNSRecordView(buildDNSRecordView(record)), nil
}

func DeleteAuthoritativeDNSRecord(id uint) error {
	if err := EnsureCommercialFeatureEnabled(CommercialFeatureAuthoritativeDNS); err != nil {
		return err
	}
	record, err := model.GetDNSRecordByID(id)
	if err != nil {
		return err
	}
	zoneID := record.ZoneID
	if err := record.Delete(); err != nil {
		return err
	}
	return bumpDNSZoneSerial(zoneID)
}

func buildDNSZone(zone *model.DNSZone, input DNSZoneInput) (*model.DNSZone, error) {
	name, err := normalizeDNSZoneName(input.Name)
	if err != nil {
		return nil, err
	}
	nameServers, err := normalizeNameServers(input.NameServers)
	if err != nil {
		return nil, err
	}
	primaryNS := normalizeDNSRecordName(input.PrimaryNS)
	if primaryNS == "" && len(nameServers) > 0 {
		primaryNS = nameServers[0]
	}
	if primaryNS != "" && !isValidProxyRouteDomain(primaryNS) {
		return nil, errors.New("primary_ns format is invalid")
	}
	soaEmail := normalizeSOAEmail(input.SOAEmail, name)
	enabled := true
	if zone != nil {
		enabled = zone.Enabled
	}
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	nameServersJSON, err := json.Marshal(nameServers)
	if err != nil {
		return nil, err
	}
	if zone == nil {
		zone = &model.DNSZone{
			Serial: nextDNSZoneSerial(0),
		}
	}
	zoneChanged := zone.Name != "" && (zone.Name != name ||
		zone.SOAEmail != soaEmail ||
		zone.PrimaryNS != primaryNS ||
		zone.NameServers != string(nameServersJSON) ||
		normalizeDNSZoneTTL(zone.DefaultTTL) != normalizeDNSZoneTTL(input.DefaultTTL) ||
		zone.Enabled != enabled)
	if zoneChanged {
		zone.Serial = nextDNSZoneSerial(zone.Serial)
	}
	zone.Name = name
	zone.SOAEmail = soaEmail
	zone.PrimaryNS = primaryNS
	zone.NameServers = string(nameServersJSON)
	zone.DefaultTTL = normalizeDNSZoneTTL(input.DefaultTTL)
	zone.Enabled = enabled
	if zone.Serial == 0 {
		zone.Serial = nextDNSZoneSerial(0)
	}
	return zone, nil
}

func buildDNSRecord(record *model.DNSRecord, input DNSRecordInput) (*model.DNSRecord, error) {
	zone, err := model.GetDNSZoneByID(input.ZoneID)
	if err != nil {
		return nil, errors.New("selected DNS zone does not exist")
	}
	recordType, err := normalizeAuthoritativeDNSRecordType(input.Type)
	if err != nil {
		return nil, err
	}
	name, err := normalizeAuthoritativeDNSRecordName(zone.Name, input.Name)
	if err != nil {
		return nil, err
	}
	value, priority, err := normalizeAuthoritativeDNSRecordValue(recordType, input.Value, input.Priority)
	if err != nil {
		return nil, err
	}
	enabled := true
	if record != nil {
		enabled = record.Enabled
	}
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	if record == nil {
		record = &model.DNSRecord{}
	}
	record.ZoneID = input.ZoneID
	record.Name = name
	record.Type = recordType
	record.Value = value
	record.TTL = normalizeAuthoritativeTTL(input.TTL, zone.DefaultTTL)
	record.Priority = priority
	record.Enabled = enabled
	return record, nil
}

func validateAuthoritativeDNSRecordDynamicConflicts(record *model.DNSRecord) error {
	if record == nil || !record.Enabled {
		return nil
	}
	recordType := strings.ToUpper(strings.TrimSpace(record.Type))
	if recordType != "A" && recordType != "AAAA" && recordType != "CNAME" {
		return nil
	}
	var routes []*model.ProxyRoute
	if err := model.DB.
		Where("enabled = ? AND dns_provider_mode = ? AND dns_zone_id_ref = ?", true, DNSProviderModeAuthoritative, record.ZoneID).
		Order("site_name asc").
		Find(&routes).Error; err != nil {
		return err
	}
	for _, route := range routes {
		if route == nil {
			continue
		}
		routeType := normalizeDNSRecordType(route.DNSRecordType)
		if recordType != "CNAME" && routeType != recordType {
			continue
		}
		domains, err := decodeStoredDomains(route.Domains, route.Domain)
		if err != nil {
			return fmt.Errorf("existing authoritative route %d domains are invalid: %w", route.ID, err)
		}
		for _, domain := range domains {
			if authoritativeDomainMatchesQName(domain, record.Name) {
				return fmt.Errorf("静态 DNS 记录 %s %s 与网站配置「%s」的本地自建解析自动 %s 记录冲突。请到左侧「网站配置」打开该站点的「负载均衡」检查自动解析，或禁用该网站自动解析后再添加静态记录", record.Name, recordType, route.SiteName, routeType)
			}
		}
	}
	return nil
}

func validateAuthoritativeDNSRecordStaticConflicts(record *model.DNSRecord) error {
	if record == nil || !record.Enabled {
		return nil
	}
	var records []*model.DNSRecord
	if err := model.DB.
		Where("zone_id = ? AND name = ? AND enabled = ?", record.ZoneID, record.Name, true).
		Order("id asc").
		Find(&records).Error; err != nil {
		return err
	}
	recordType := strings.ToUpper(strings.TrimSpace(record.Type))
	recordValue := strings.TrimSpace(record.Value)
	for _, existing := range records {
		if existing == nil || existing.ID == record.ID {
			continue
		}
		existingType := strings.ToUpper(strings.TrimSpace(existing.Type))
		if recordType == "CNAME" || existingType == "CNAME" {
			return fmt.Errorf("静态 DNS 记录 %s 存在 CNAME 独占冲突。CNAME 不能与同名其它记录共存", record.Name)
		}
		if existingType == recordType && strings.TrimSpace(existing.Value) == recordValue && existing.Priority == record.Priority {
			return fmt.Errorf("静态 DNS 记录 %s %s 已存在相同记录值", record.Name, recordType)
		}
	}
	return nil
}

func validateAuthoritativeProxyRouteStaticRecordConflicts(zoneID uint, domains []string, recordType string, enabled bool) error {
	if !enabled || zoneID == 0 {
		return nil
	}
	recordType = normalizeDNSRecordType(recordType)
	if recordType != "A" && recordType != "AAAA" {
		return errors.New("本地自建解析自动选 IP 只支持 A/AAAA")
	}
	normalizedDomains := make([]string, 0, len(domains))
	wildcardSuffixes := make([]string, 0, len(domains))
	for _, domain := range domains {
		normalized := normalizeDNSRecordName(domain)
		if normalized == "" {
			continue
		}
		normalizedDomains = append(normalizedDomains, normalized)
		if strings.HasPrefix(normalized, "*.") {
			suffix := strings.TrimPrefix(normalized, "*.")
			if suffix != "" {
				wildcardSuffixes = append(wildcardSuffixes, suffix)
			}
		}
	}
	if len(normalizedDomains) == 0 && len(wildcardSuffixes) == 0 {
		return nil
	}
	records, err := model.ListDNSRecordsByZoneIDNameCandidates(zoneID, normalizedDomains, wildcardSuffixes)
	if err != nil {
		return err
	}
	for _, record := range records {
		if record == nil || !record.Enabled {
			continue
		}
		if !authoritativeDomainsMatchRecordName(normalizedDomains, record.Name) {
			continue
		}
		staticType := strings.ToUpper(strings.TrimSpace(record.Type))
		if staticType != "CNAME" && staticType != recordType {
			continue
		}
		return fmt.Errorf("本地自建解析网站的自动 %s 记录与托管域名「%s」里的静态记录「%s %s」冲突。位置：左侧「本地自建解析」-> 托管域名「%s」-> 静态记录。请删除或禁用该静态记录后再创建网站配置；如果希望网站配置自动接管解析，不要保留同名 A/AAAA/CNAME 静态记录", recordType, authoritativeDNSZoneDisplayName(zoneID), record.Name, staticType, authoritativeDNSZoneDisplayName(zoneID))
	}
	return nil
}

func authoritativeDNSZoneDisplayName(zoneID uint) string {
	zone, err := model.GetDNSZoneByID(zoneID)
	if err == nil && zone != nil && strings.TrimSpace(zone.Name) != "" {
		return strings.TrimSpace(zone.Name)
	}
	if zoneID > 0 {
		return fmt.Sprintf("ID %d", zoneID)
	}
	return "未选择托管域名"
}

func authoritativeDomainsMatchRecordName(domains []string, recordName string) bool {
	for _, domain := range domains {
		if authoritativeDomainMatchesQName(domain, recordName) {
			return true
		}
	}
	return false
}

func buildDNSZoneView(zone *model.DNSZone, includeRecords bool) (*DNSZoneView, error) {
	if zone == nil {
		return nil, errors.New("DNS zone is nil")
	}
	var recordCount int64
	if err := model.DB.Model(&model.DNSRecord{}).Where("zone_id = ?", zone.ID).Count(&recordCount).Error; err != nil {
		return nil, err
	}
	return buildDNSZoneViewWithRecordCount(zone, includeRecords, recordCount)
}

func buildDNSZoneViewWithRecordCount(zone *model.DNSZone, includeRecords bool, recordCount int64) (*DNSZoneView, error) {
	if zone == nil {
		return nil, errors.New("DNS zone is nil")
	}
	view := &DNSZoneView{
		ID:                      zone.ID,
		Name:                    zone.Name,
		SOAEmail:                zone.SOAEmail,
		PrimaryNS:               zone.PrimaryNS,
		NameServers:             decodeStoredStringList(zone.NameServers),
		DefaultTTL:              normalizeDNSZoneTTL(zone.DefaultTTL),
		Serial:                  zone.Serial,
		DNSSECEnabled:           zone.DNSSECEnabled,
		DNSSECDenialMode:        normalizeDNSSECDenialMode(zone.DNSSECDenialMode),
		DNSSECNSEC3Salt:         strings.TrimSpace(zone.DNSSECNSEC3Salt),
		DNSSECNSEC3Iterations:   normalizeDNSSECNSEC3Iterations(zone.DNSSECNSEC3Iterations),
		DNSSECSignatureValidity: normalizeDNSSECSignatureValidity(zone.DNSSECSignatureValidity),
		Enabled:                 zone.Enabled,
		RecordCount:             recordCount,
		CreatedAt:               zone.CreatedAt,
		UpdatedAt:               zone.UpdatedAt,
	}
	if includeRecords {
		records, err := model.ListDNSRecordsByZoneID(zone.ID)
		if err != nil {
			return nil, err
		}
		view.Records = make([]DNSRecordView, 0, len(records))
		for _, record := range records {
			view.Records = append(view.Records, buildDNSRecordView(record))
		}
	}
	return view, nil
}

func buildDNSRecordView(record *model.DNSRecord) DNSRecordView {
	if record == nil {
		return DNSRecordView{}
	}
	return DNSRecordView{
		ID:        record.ID,
		ZoneID:    record.ZoneID,
		Name:      record.Name,
		Type:      record.Type,
		Value:     record.Value,
		TTL:       record.TTL,
		Priority:  record.Priority,
		Enabled:   record.Enabled,
		CreatedAt: record.CreatedAt,
		UpdatedAt: record.UpdatedAt,
	}
}

func normalizeUintIDs(values []uint) []uint {
	result := make([]uint, 0, len(values))
	seen := make(map[uint]struct{}, len(values))
	for _, value := range values {
		if value == 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func bumpDNSZoneSerial(zoneID uint) error {
	zone, err := model.GetDNSZoneByID(zoneID)
	if err != nil {
		return err
	}
	zone.Serial = nextDNSZoneSerial(zone.Serial)
	return zone.Update()
}

func nextDNSZoneSerial(current uint64) uint64 {
	next := uint64(time.Now().UTC().Unix())
	if next <= current {
		return current + 1
	}
	return next
}

func ptrDNSRecordView(view DNSRecordView) *DNSRecordView {
	return &view
}
