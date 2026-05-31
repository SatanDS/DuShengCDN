package service

import (
	"crypto/sha256"
	"dushengcdn/model"
	"dushengcdn/utils/geoip/iputil"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
)

const (
	dnsWorkerStatusOnline   = "online"
	dnsWorkerStatusOffline  = "offline"
	defaultAuthoritativeTTL = 30
	defaultDNSZoneTTL       = 300
)

type DNSZoneInput struct {
	Name        string   `json:"name"`
	SOAEmail    string   `json:"soa_email"`
	PrimaryNS   string   `json:"primary_ns"`
	NameServers []string `json:"name_servers"`
	DefaultTTL  int      `json:"default_ttl"`
	Enabled     *bool    `json:"enabled"`
}

type DNSRecordInput struct {
	ZoneID   uint   `json:"zone_id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Value    string `json:"value"`
	TTL      int    `json:"ttl"`
	Priority int    `json:"priority"`
	Enabled  *bool  `json:"enabled"`
}

type DNSWorkerInput struct {
	Name          string `json:"name"`
	PublicAddress string `json:"public_address"`
}

type DNSWorkerHeartbeatInput struct {
	Version             string                `json:"version"`
	Status              string                `json:"status"`
	LastSnapshotVersion string                `json:"last_snapshot_version"`
	LastSnapshotAt      *time.Time            `json:"last_snapshot_at"`
	LastError           string                `json:"last_error"`
	Rollups             []DNSQueryRollupInput `json:"rollups"`
}

type DNSQueryRollupInput struct {
	WindowStart   time.Time        `json:"window_start"`
	WindowMinutes int              `json:"window_minutes"`
	ZoneID        uint             `json:"zone_id"`
	ProxyRouteID  uint             `json:"proxy_route_id"`
	QName         string           `json:"qname"`
	QType         string           `json:"qtype"`
	RCode         string           `json:"rcode"`
	QueryCount    int64            `json:"query_count"`
	TargetSummary map[string]int64 `json:"target_summary"`
}

type DNSZoneView struct {
	ID          uint            `json:"id"`
	Name        string          `json:"name"`
	SOAEmail    string          `json:"soa_email"`
	PrimaryNS   string          `json:"primary_ns"`
	NameServers []string        `json:"name_servers"`
	DefaultTTL  int             `json:"default_ttl"`
	Serial      uint64          `json:"serial"`
	Enabled     bool            `json:"enabled"`
	RecordCount int64           `json:"record_count"`
	Records     []DNSRecordView `json:"records,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

type DNSRecordView struct {
	ID        uint      `json:"id"`
	ZoneID    uint      `json:"zone_id"`
	Name      string    `json:"name"`
	Type      string    `json:"type"`
	Value     string    `json:"value"`
	TTL       int       `json:"ttl"`
	Priority  int       `json:"priority"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type DNSWorkerView struct {
	ID                  uint       `json:"id"`
	WorkerID            string     `json:"worker_id"`
	Name                string     `json:"name"`
	Token               string     `json:"token,omitempty"`
	PublicAddress       string     `json:"public_address"`
	Version             string     `json:"version"`
	Status              string     `json:"status"`
	LastSnapshotVersion string     `json:"last_snapshot_version"`
	LastSnapshotAt      *time.Time `json:"last_snapshot_at"`
	LastSeenAt          *time.Time `json:"last_seen_at"`
	LastError           string     `json:"last_error"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

type AuthoritativeDNSSnapshot struct {
	SnapshotVersion string                          `json:"snapshot_version"`
	GeneratedAt     time.Time                       `json:"generated_at"`
	Zones           []AuthoritativeDNSSnapshotZone  `json:"zones"`
	Routes          []AuthoritativeDNSSnapshotRoute `json:"routes"`
	Nodes           []AuthoritativeDNSSnapshotNode  `json:"nodes"`
}

type AuthoritativeDNSSnapshotZone struct {
	ID          uint                             `json:"id"`
	Name        string                           `json:"name"`
	SOAEmail    string                           `json:"soa_email"`
	PrimaryNS   string                           `json:"primary_ns"`
	NameServers []string                         `json:"name_servers"`
	DefaultTTL  int                              `json:"default_ttl"`
	Serial      uint64                           `json:"serial"`
	Records     []AuthoritativeDNSSnapshotRecord `json:"records"`
}

type AuthoritativeDNSSnapshotRecord struct {
	ID       uint   `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Value    string `json:"value"`
	TTL      int    `json:"ttl"`
	Priority int    `json:"priority"`
}

type AuthoritativeDNSSnapshotRoute struct {
	ID             uint                 `json:"id"`
	SiteName       string               `json:"site_name"`
	Domains        []string             `json:"domains"`
	ZoneID         uint                 `json:"zone_id"`
	NodePool       string               `json:"node_pool"`
	RecordType     string               `json:"record_type"`
	TargetCount    int                  `json:"target_count"`
	ScheduleMode   string               `json:"schedule_mode"`
	TTL            int                  `json:"ttl"`
	GSLBEnabled    bool                 `json:"gslb_enabled"`
	GSLBPolicy     ProxyRouteGSLBPolicy `json:"gslb_policy"`
	CurrentTargets []string             `json:"current_targets"`
	TargetError    string               `json:"target_error,omitempty"`
}

type AuthoritativeDNSSnapshotNode struct {
	NodeID               string     `json:"node_id"`
	Name                 string     `json:"name"`
	PoolName             string     `json:"pool_name"`
	PublicIPs            []string   `json:"public_ips"`
	Weight               int        `json:"weight"`
	SchedulingEnabled    bool       `json:"scheduling_enabled"`
	DrainMode            bool       `json:"drain_mode"`
	Status               string     `json:"status"`
	OpenrestyStatus      string     `json:"openresty_status"`
	LastSeenAt           time.Time  `json:"last_seen_at"`
	OpenrestyConnections int64      `json:"openresty_connections"`
	CPUUsagePercent      float64    `json:"cpu_usage_percent"`
	MemoryUsagePercent   float64    `json:"memory_usage_percent"`
	MetricCapturedAt     *time.Time `json:"metric_captured_at,omitempty"`
}

func ListAuthoritativeDNSZones() ([]DNSZoneView, error) {
	zones, err := model.ListDNSZones()
	if err != nil {
		return nil, err
	}
	views := make([]DNSZoneView, 0, len(zones))
	for _, zone := range zones {
		view, err := buildDNSZoneView(zone, false)
		if err != nil {
			return nil, err
		}
		views = append(views, *view)
	}
	return views, nil
}

func GetAuthoritativeDNSZone(id uint) (*DNSZoneView, error) {
	zone, err := model.GetDNSZoneByID(id)
	if err != nil {
		return nil, err
	}
	return buildDNSZoneView(zone, true)
}

func CreateAuthoritativeDNSZone(input DNSZoneInput) (*DNSZoneView, error) {
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
	return buildDNSZoneView(zone, true)
}

func UpdateAuthoritativeDNSZone(id uint, input DNSZoneInput) (*DNSZoneView, error) {
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
	if err := model.DB.Where("zone_id = ?", zone.ID).Delete(&model.DNSRecord{}).Error; err != nil {
		return err
	}
	return zone.Delete()
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
	if err := record.Insert(); err != nil {
		return nil, err
	}
	if err := bumpDNSZoneSerial(record.ZoneID); err != nil {
		return nil, err
	}
	return ptrDNSRecordView(buildDNSRecordView(record)), nil
}

func UpdateAuthoritativeDNSRecord(id uint, input DNSRecordInput) (*DNSRecordView, error) {
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
	if err := record.Update(); err != nil {
		return nil, err
	}
	if err := bumpDNSZoneSerial(record.ZoneID); err != nil {
		return nil, err
	}
	return ptrDNSRecordView(buildDNSRecordView(record)), nil
}

func DeleteAuthoritativeDNSRecord(id uint) error {
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

func ListAuthoritativeDNSWorkers() ([]DNSWorkerView, error) {
	workers, err := model.ListDNSWorkers()
	if err != nil {
		return nil, err
	}
	views := make([]DNSWorkerView, 0, len(workers))
	for _, worker := range workers {
		views = append(views, buildDNSWorkerView(worker, false))
	}
	return views, nil
}

func CreateAuthoritativeDNSWorker(input DNSWorkerInput) (*DNSWorkerView, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, errors.New("DNS worker name cannot be empty")
	}
	if len(name) > 128 {
		return nil, errors.New("DNS worker name is too long")
	}
	token, err := newRandomToken()
	if err != nil {
		return nil, err
	}
	workerIDSeed, err := newRandomToken()
	if err != nil {
		return nil, err
	}
	worker := &model.DNSWorker{
		WorkerID:      "dns-" + workerIDSeed,
		Name:          name,
		Token:         token,
		PublicAddress: strings.TrimSpace(input.PublicAddress),
		Status:        dnsWorkerStatusOffline,
	}
	if err := worker.Insert(); err != nil {
		if isUniqueConstraintError(err) {
			return nil, errors.New("DNS worker identity collision, please retry")
		}
		return nil, err
	}
	return ptrDNSWorkerView(buildDNSWorkerView(worker, true)), nil
}

func DeleteAuthoritativeDNSWorker(id uint) error {
	worker, err := model.GetDNSWorkerByID(id)
	if err != nil {
		return err
	}
	return worker.Delete()
}

func AuthenticateDNSWorkerToken(token string) (*model.DNSWorker, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, errors.New("missing DNS Worker Token")
	}
	worker, err := model.GetDNSWorkerByToken(token)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("invalid DNS Worker Token")
		}
		return nil, err
	}
	return worker, nil
}

func RecordDNSWorkerHeartbeat(worker *model.DNSWorker, input DNSWorkerHeartbeatInput) (*DNSWorkerView, error) {
	if worker == nil {
		return nil, errors.New("DNS worker is nil")
	}
	now := time.Now()
	worker.Status = normalizeDNSWorkerStatus(input.Status)
	worker.Version = strings.TrimSpace(input.Version)
	worker.LastSnapshotVersion = strings.TrimSpace(input.LastSnapshotVersion)
	worker.LastSnapshotAt = input.LastSnapshotAt
	worker.LastSeenAt = &now
	worker.LastError = truncateForDatabase(strings.TrimSpace(input.LastError), 16000)
	if err := worker.Update(); err != nil {
		return nil, err
	}
	if err := persistDNSQueryRollups(worker.WorkerID, input.Rollups); err != nil {
		return nil, err
	}
	return ptrDNSWorkerView(buildDNSWorkerView(worker, false)), nil
}

func GetAuthoritativeDNSSnapshot(worker *model.DNSWorker) (*AuthoritativeDNSSnapshot, error) {
	zones, err := snapshotDNSZones()
	if err != nil {
		return nil, err
	}
	routes, err := snapshotAuthoritativeRoutes()
	if err != nil {
		return nil, err
	}
	nodes, err := snapshotNodes()
	if err != nil {
		return nil, err
	}
	snapshot := &AuthoritativeDNSSnapshot{
		GeneratedAt: time.Now().UTC(),
		Zones:       zones,
		Routes:      routes,
		Nodes:       nodes,
	}
	version, err := authoritativeDNSSnapshotVersion(snapshot)
	if err != nil {
		return nil, err
	}
	snapshot.SnapshotVersion = version
	if worker != nil {
		_ = recordDNSWorkerSnapshotPull(worker, version)
	}
	return snapshot, nil
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

func buildDNSZoneView(zone *model.DNSZone, includeRecords bool) (*DNSZoneView, error) {
	if zone == nil {
		return nil, errors.New("DNS zone is nil")
	}
	view := &DNSZoneView{
		ID:          zone.ID,
		Name:        zone.Name,
		SOAEmail:    zone.SOAEmail,
		PrimaryNS:   zone.PrimaryNS,
		NameServers: decodeStoredStringList(zone.NameServers),
		DefaultTTL:  normalizeDNSZoneTTL(zone.DefaultTTL),
		Serial:      zone.Serial,
		Enabled:     zone.Enabled,
		CreatedAt:   zone.CreatedAt,
		UpdatedAt:   zone.UpdatedAt,
	}
	if err := model.DB.Model(&model.DNSRecord{}).Where("zone_id = ?", zone.ID).Count(&view.RecordCount).Error; err != nil {
		return nil, err
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

func buildDNSWorkerView(worker *model.DNSWorker, includeToken bool) DNSWorkerView {
	if worker == nil {
		return DNSWorkerView{}
	}
	view := DNSWorkerView{
		ID:                  worker.ID,
		WorkerID:            worker.WorkerID,
		Name:                worker.Name,
		PublicAddress:       worker.PublicAddress,
		Version:             worker.Version,
		Status:              normalizeDNSWorkerStatus(worker.Status),
		LastSnapshotVersion: worker.LastSnapshotVersion,
		LastSnapshotAt:      worker.LastSnapshotAt,
		LastSeenAt:          worker.LastSeenAt,
		LastError:           worker.LastError,
		CreatedAt:           worker.CreatedAt,
		UpdatedAt:           worker.UpdatedAt,
	}
	if includeToken {
		view.Token = worker.Token
	}
	return view
}

func snapshotDNSZones() ([]AuthoritativeDNSSnapshotZone, error) {
	var zones []*model.DNSZone
	if err := model.DB.Where("enabled = ?", true).Order("name asc").Find(&zones).Error; err != nil {
		return nil, err
	}
	result := make([]AuthoritativeDNSSnapshotZone, 0, len(zones))
	for _, zone := range zones {
		records, err := model.ListDNSRecordsByZoneID(zone.ID)
		if err != nil {
			return nil, err
		}
		item := AuthoritativeDNSSnapshotZone{
			ID:          zone.ID,
			Name:        zone.Name,
			SOAEmail:    zone.SOAEmail,
			PrimaryNS:   zone.PrimaryNS,
			NameServers: decodeStoredStringList(zone.NameServers),
			DefaultTTL:  normalizeDNSZoneTTL(zone.DefaultTTL),
			Serial:      zone.Serial,
			Records:     make([]AuthoritativeDNSSnapshotRecord, 0, len(records)),
		}
		for _, record := range records {
			if record == nil || !record.Enabled {
				continue
			}
			item.Records = append(item.Records, AuthoritativeDNSSnapshotRecord{
				ID:       record.ID,
				Name:     record.Name,
				Type:     record.Type,
				Value:    record.Value,
				TTL:      normalizeAuthoritativeTTL(record.TTL, item.DefaultTTL),
				Priority: record.Priority,
			})
		}
		result = append(result, item)
	}
	return result, nil
}

func snapshotAuthoritativeRoutes() ([]AuthoritativeDNSSnapshotRoute, error) {
	var routes []*model.ProxyRoute
	if err := model.DB.
		Where("enabled = ? AND dns_provider_mode = ? AND dns_zone_id_ref IS NOT NULL", true, DNSProviderModeAuthoritative).
		Order("site_name asc").
		Find(&routes).Error; err != nil {
		return nil, err
	}
	result := make([]AuthoritativeDNSSnapshotRoute, 0, len(routes))
	for _, route := range routes {
		if route == nil || route.DNSZoneIDRef == nil || *route.DNSZoneIDRef == 0 {
			continue
		}
		zone, err := model.GetDNSZoneByID(*route.DNSZoneIDRef)
		if err != nil || zone == nil || !zone.Enabled {
			continue
		}
		domains, err := decodeStoredDomains(route.Domains, route.Domain)
		if err != nil {
			return nil, err
		}
		policy, err := decodeStoredGSLBPolicy(route.GSLBPolicy)
		if err != nil {
			return nil, err
		}
		recordType := normalizeDNSRecordType(route.DNSRecordType)
		item := AuthoritativeDNSSnapshotRoute{
			ID:           route.ID,
			SiteName:     normalizeProxyRouteSiteNameInput(route, route.SiteName, route.Domain),
			Domains:      domains,
			ZoneID:       *route.DNSZoneIDRef,
			NodePool:     normalizeNodePoolName(route.NodePool),
			RecordType:   recordType,
			TargetCount:  normalizeDNSTargetCount(route.DNSTargetCount),
			ScheduleMode: normalizeDNSScheduleMode(route.DNSScheduleMode),
			TTL:          normalizeAuthoritativeRouteTTL(route.DNSTTL),
			GSLBEnabled:  route.GSLBEnabled,
			GSLBPolicy:   policy,
		}
		selection, selectErr := selectProxyRouteDNSTargets(route, recordType)
		if selectErr != nil {
			item.TargetError = selectErr.Error()
		} else {
			item.CurrentTargets = selection.Targets
			item.TTL = normalizeAuthoritativeRouteTTL(selection.TTL)
		}
		result = append(result, item)
	}
	return result, nil
}

func snapshotNodes() ([]AuthoritativeDNSSnapshotNode, error) {
	nodes, err := model.ListNodes()
	if err != nil {
		return nil, err
	}
	metrics := latestNodeMetricSnapshots()
	result := make([]AuthoritativeDNSSnapshotNode, 0, len(nodes))
	for _, node := range nodes {
		if node == nil {
			continue
		}
		item := AuthoritativeDNSSnapshotNode{
			NodeID:            node.NodeID,
			Name:              node.Name,
			PoolName:          normalizeNodePoolName(node.PoolName),
			PublicIPs:         publicNodeIPsForSnapshot(node),
			Weight:            normalizeNodeWeight(node.Weight),
			SchedulingEnabled: isNodeSchedulableForDNS(node),
			DrainMode:         node.DrainMode,
			Status:            computeNodeStatus(node),
			OpenrestyStatus:   normalizeOpenrestyStatus(node.OpenrestyStatus),
			LastSeenAt:        node.LastSeenAt,
		}
		if metric := metrics[node.NodeID]; metric != nil {
			capturedAt := metric.CapturedAt
			item.OpenrestyConnections = metric.OpenrestyConnections
			item.CPUUsagePercent = metric.CPUUsagePercent
			item.MemoryUsagePercent = nodeMetricMemoryUsagePercent(metric)
			item.MetricCapturedAt = &capturedAt
		}
		result = append(result, item)
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].PoolName != result[j].PoolName {
			return result[i].PoolName < result[j].PoolName
		}
		return result[i].NodeID < result[j].NodeID
	})
	return result, nil
}

func authoritativeDNSSnapshotVersion(snapshot *AuthoritativeDNSSnapshot) (string, error) {
	payload := struct {
		Zones  []AuthoritativeDNSSnapshotZone  `json:"zones"`
		Routes []AuthoritativeDNSSnapshotRoute `json:"routes"`
		Nodes  []AuthoritativeDNSSnapshotNode  `json:"nodes"`
	}{
		Zones:  snapshot.Zones,
		Routes: snapshot.Routes,
		Nodes:  snapshot.Nodes,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])[:24], nil
}

func recordDNSWorkerSnapshotPull(worker *model.DNSWorker, version string) error {
	if worker == nil {
		return nil
	}
	now := time.Now()
	worker.Status = dnsWorkerStatusOnline
	worker.LastSeenAt = &now
	worker.LastSnapshotAt = &now
	worker.LastSnapshotVersion = strings.TrimSpace(version)
	return worker.Update()
}

func persistDNSQueryRollups(workerID string, inputs []DNSQueryRollupInput) error {
	for _, input := range inputs {
		if input.QueryCount <= 0 {
			continue
		}
		targetSummary := input.TargetSummary
		if targetSummary == nil {
			targetSummary = map[string]int64{}
		}
		targetSummaryJSON, err := json.Marshal(targetSummary)
		if err != nil {
			return err
		}
		rollup := &model.DNSQueryRollup{
			WindowStart:   input.WindowStart,
			WindowMinutes: normalizeDNSRollupWindow(input.WindowMinutes),
			WorkerID:      workerID,
			ZoneID:        input.ZoneID,
			ProxyRouteID:  input.ProxyRouteID,
			QName:         normalizeDNSRecordName(input.QName),
			QType:         normalizeAuthoritativeDNSRecordTypeOrDefault(input.QType),
			RCode:         normalizeDNSRCode(input.RCode),
			QueryCount:    input.QueryCount,
			TargetSummary: string(targetSummaryJSON),
		}
		if rollup.WindowStart.IsZero() {
			rollup.WindowStart = time.Now().UTC().Truncate(time.Minute)
		}
		if err := rollup.Insert(); err != nil {
			return err
		}
	}
	return nil
}

func normalizeDNSZoneName(raw string) (string, error) {
	name := normalizeDNSRecordName(raw)
	if name == "" {
		return "", errors.New("DNS zone name cannot be empty")
	}
	if strings.HasPrefix(name, "*.") || !isValidProxyRouteDomain(name) {
		return "", errors.New("DNS zone name format is invalid")
	}
	return name, nil
}

func normalizeNameServers(values []string) ([]string, error) {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		ns := normalizeDNSRecordName(value)
		if ns == "" {
			continue
		}
		if !isValidProxyRouteDomain(ns) {
			return nil, errors.New("name_servers contains invalid domain")
		}
		if _, ok := seen[ns]; ok {
			continue
		}
		seen[ns] = struct{}{}
		result = append(result, ns)
	}
	return result, nil
}

func normalizeSOAEmail(raw string, zoneName string) string {
	email := strings.TrimSpace(raw)
	if email != "" {
		return email
	}
	return "hostmaster@" + zoneName
}

func normalizeDNSZoneTTL(value int) int {
	if value <= 0 {
		return defaultDNSZoneTTL
	}
	if value > 86400 {
		return 86400
	}
	return value
}

func normalizeAuthoritativeTTL(value int, fallback int) int {
	if fallback <= 0 {
		fallback = defaultDNSZoneTTL
	}
	if value <= 0 {
		return fallback
	}
	if value > 86400 {
		return 86400
	}
	return value
}

func normalizeAuthoritativeRouteTTL(value int) int {
	if value <= 1 {
		return defaultAuthoritativeTTL
	}
	if value < defaultAuthoritativeTTL {
		return defaultAuthoritativeTTL
	}
	if value > 86400 {
		return 86400
	}
	return value
}

func normalizeAuthoritativeDNSRecordType(raw string) (string, error) {
	recordType := strings.ToUpper(strings.TrimSpace(raw))
	switch recordType {
	case "A", "AAAA", "CNAME", "TXT", "MX", "NS", "SOA":
		return recordType, nil
	default:
		return "", errors.New("unsupported DNS record type")
	}
}

func normalizeAuthoritativeDNSRecordTypeOrDefault(raw string) string {
	recordType, err := normalizeAuthoritativeDNSRecordType(raw)
	if err != nil {
		return "A"
	}
	return recordType
}

func normalizeAuthoritativeDNSRecordName(zoneName string, raw string) (string, error) {
	zoneName = normalizeDNSRecordName(zoneName)
	name := normalizeDNSRecordName(raw)
	if name == "" || name == "@" {
		name = zoneName
	} else if !strings.Contains(name, ".") {
		name += "." + zoneName
	}
	if !isValidProxyRouteDomain(name) {
		return "", errors.New("DNS record name format is invalid")
	}
	if !domainBelongsToZone(name, zoneName) {
		return "", errors.New("DNS record name is outside the zone")
	}
	return name, nil
}

func normalizeAuthoritativeDNSRecordValue(recordType string, raw string, priority int) (string, int, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", 0, errors.New("DNS record value cannot be empty")
	}
	switch recordType {
	case "A":
		ip := net.ParseIP(value)
		if ip == nil || ip.To4() == nil {
			return "", 0, errors.New("A record value must be an IPv4 address")
		}
		return ip.String(), 0, nil
	case "AAAA":
		ip := net.ParseIP(value)
		if ip == nil || ip.To4() != nil {
			return "", 0, errors.New("AAAA record value must be an IPv6 address")
		}
		return ip.String(), 0, nil
	case "CNAME", "NS":
		target := normalizeDNSRecordName(value)
		if target == "" || !isValidProxyRouteDomain(target) || net.ParseIP(target) != nil {
			return "", 0, fmt.Errorf("%s record value must be a domain name", recordType)
		}
		return target, 0, nil
	case "MX":
		target := normalizeDNSRecordName(value)
		if target == "" || !isValidProxyRouteDomain(target) || net.ParseIP(target) != nil {
			return "", 0, errors.New("MX record value must be a mail server domain name")
		}
		if priority < 0 {
			priority = 0
		}
		return target, priority, nil
	case "TXT", "SOA":
		return value, priority, nil
	default:
		return "", 0, errors.New("unsupported DNS record type")
	}
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

func normalizeDNSWorkerStatus(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case dnsWorkerStatusOnline:
		return dnsWorkerStatusOnline
	default:
		return dnsWorkerStatusOffline
	}
}

func normalizeDNSRollupWindow(value int) int {
	if value <= 0 {
		return 1
	}
	if value > 1440 {
		return 1440
	}
	return value
}

func normalizeDNSRCode(raw string) string {
	rcode := strings.ToUpper(strings.TrimSpace(raw))
	switch rcode {
	case "NOERROR", "NXDOMAIN", "NODATA", "SERVFAIL", "REFUSED":
		return rcode
	default:
		return "NOERROR"
	}
}

func publicNodeIPsForSnapshot(node *model.Node) []string {
	ips := make([]string, 0)
	seen := map[string]struct{}{}
	for _, value := range resolveNodePublicIPs(node) {
		ip := iputil.NormalizeIP(value)
		if ip == "" || !iputil.IsPublicString(ip) {
			continue
		}
		if _, ok := seen[ip]; ok {
			continue
		}
		seen[ip] = struct{}{}
		ips = append(ips, ip)
	}
	sort.Strings(ips)
	return ips
}

func ptrDNSRecordView(view DNSRecordView) *DNSRecordView {
	return &view
}

func ptrDNSWorkerView(view DNSWorkerView) *DNSWorkerView {
	return &view
}
