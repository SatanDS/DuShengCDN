package model

import (
	"strings"
	"time"
)

type DNSZone struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	Name        string    `json:"name" gorm:"uniqueIndex;size:255;not null"`
	SOAEmail    string    `json:"soa_email" gorm:"size:255;not null;default:''"`
	PrimaryNS   string    `json:"primary_ns" gorm:"size:255;not null;default:''"`
	NameServers string    `json:"name_servers" gorm:"type:text;not null;default:'[]'"`
	DefaultTTL  int       `json:"default_ttl" gorm:"not null;default:300"`
	Serial      uint64    `json:"serial" gorm:"not null;default:1"`
	Enabled     bool      `json:"enabled" gorm:"not null;default:true"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type DNSRecord struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	ZoneID    uint      `json:"zone_id" gorm:"index;not null"`
	Name      string    `json:"name" gorm:"index:idx_dns_records_zone_name_type;size:255;not null;default:''"`
	Type      string    `json:"type" gorm:"index:idx_dns_records_zone_name_type;size:16;not null"`
	Value     string    `json:"value" gorm:"type:text;not null"`
	TTL       int       `json:"ttl" gorm:"not null;default:300"`
	Priority  int       `json:"priority" gorm:"not null;default:0"`
	Enabled   bool      `json:"enabled" gorm:"not null;default:true"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type DNSRecordCountByZone struct {
	ZoneID uint
	Count  int64
}

type DNSWorker struct {
	ID                       uint       `json:"id" gorm:"primaryKey"`
	WorkerID                 string     `json:"worker_id" gorm:"uniqueIndex;size:64;not null"`
	Name                     string     `json:"name" gorm:"size:128;not null"`
	Token                    string     `json:"-" gorm:"size:128;index;not null"`
	PublicAddress            string     `json:"public_address" gorm:"size:255"`
	Version                  string     `json:"version" gorm:"size:64"`
	Status                   string     `json:"status" gorm:"size:16;not null;default:'offline'"`
	LastSnapshotVersion      string     `json:"last_snapshot_version" gorm:"size:128"`
	LastSnapshotAt           *time.Time `json:"last_snapshot_at"`
	LastSeenAt               *time.Time `json:"last_seen_at"`
	LastHeartbeatAt          *time.Time `json:"last_heartbeat_at" gorm:"index"`
	LastRollupAt             *time.Time `json:"last_rollup_at"`
	LastRollupCount          int64      `json:"last_rollup_count" gorm:"not null;default:0"`
	LastError                string     `json:"last_error" gorm:"type:text"`
	GeoIPEnabled             bool       `json:"geoip_enabled" gorm:"not null;default:false"`
	GeoIPDatabasePath        string     `json:"geoip_database_path" gorm:"size:512;not null;default:''"`
	GeoIPLastError           string     `json:"geoip_last_error" gorm:"type:text;not null;default:''"`
	ASNDatabasePath          string     `json:"asn_database_path" gorm:"column:asn_database_path;size:512;not null;default:''"`
	ASNLastError             string     `json:"asn_last_error" gorm:"column:asn_last_error;type:text;not null;default:''"`
	GeoIPDatabaseType        string     `json:"geoip_database_type" gorm:"column:geo_ip_database_type;size:128;not null;default:''"`
	ASNDatabaseType          string     `json:"asn_database_type" gorm:"column:asn_database_type;size:128;not null;default:''"`
	GeoIPCountryEnabled      bool       `json:"geoip_country_enabled" gorm:"column:geo_ip_country_enabled;not null;default:false"`
	GeoIPASNEnabled          bool       `json:"geoip_asn_enabled" gorm:"column:geo_ip_asn_enabled;not null;default:false"`
	GeoIPOperatorEnabled     bool       `json:"geoip_operator_enabled" gorm:"column:geo_ip_operator_enabled;not null;default:false"`
	OperatorCIDRDatabasePath string     `json:"operator_cidr_database_path" gorm:"column:operator_cidr_database_path;size:512;not null;default:''"`
	OperatorCIDRLastError    string     `json:"operator_cidr_last_error" gorm:"column:operator_cidr_last_error;type:text;not null;default:''"`
	LastProbeAt              *time.Time `json:"last_probe_at"`
	LastProbeQuery           string     `json:"last_probe_query" gorm:"size:255"`
	LastProbeResult          string     `json:"last_probe_result" gorm:"type:text;not null;default:'[]'"`
	CreatedAt                time.Time  `json:"created_at"`
	UpdatedAt                time.Time  `json:"updated_at"`
}

type DNSQueryRollup struct {
	ID              uint      `json:"id" gorm:"primaryKey"`
	WindowStart     time.Time `json:"window_start" gorm:"index;index:idx_dns_rollups_observability,priority:1;not null"`
	WindowMinutes   int       `json:"window_minutes" gorm:"not null;default:1"`
	WorkerID        string    `json:"worker_id" gorm:"index;index:idx_dns_rollups_observability,priority:2;size:64;not null"`
	ZoneID          uint      `json:"zone_id" gorm:"index;index:idx_dns_rollups_observability,priority:3"`
	ProxyRouteID    uint      `json:"proxy_route_id" gorm:"index"`
	SourceScope     string    `json:"source_scope" gorm:"index;size:64;not null;default:'global'"`
	QName           string    `json:"qname" gorm:"index;size:255"`
	QType           string    `json:"qtype" gorm:"size:16"`
	RCode           string    `json:"rcode" gorm:"size:32"`
	QueryCount      int64     `json:"query_count" gorm:"not null;default:0"`
	TotalDurationMs int64     `json:"total_duration_ms" gorm:"not null;default:0"`
	MaxDurationMs   int64     `json:"max_duration_ms" gorm:"not null;default:0"`
	TargetSummary   string    `json:"target_summary" gorm:"type:text;not null;default:'{}'"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func ListDNSZones() (zones []*DNSZone, err error) {
	err = DB.Order("name asc").Find(&zones).Error
	return zones, err
}

func ListDNSZonesByIDs(ids []uint) (zones []*DNSZone, err error) {
	if len(ids) == 0 {
		return []*DNSZone{}, nil
	}
	err = DB.Where("id IN ?", ids).Order("id asc").Find(&zones).Error
	return zones, err
}

func GetDNSZoneByID(id uint) (*DNSZone, error) {
	zone := &DNSZone{}
	err := DB.First(zone, id).Error
	return zone, err
}

func GetDNSZoneByName(name string) (*DNSZone, error) {
	zone := &DNSZone{}
	err := DB.Where("name = ?", name).First(zone).Error
	return zone, err
}

func (zone *DNSZone) Insert() error {
	return DB.Create(zone).Error
}

func (zone *DNSZone) Update() error {
	return DB.Save(zone).Error
}

func (zone *DNSZone) Delete() error {
	return DB.Delete(zone).Error
}

func ListDNSRecordsByZoneID(zoneID uint) (records []*DNSRecord, err error) {
	err = DB.Where("zone_id = ?", zoneID).Order("name asc").Order("type asc").Order("id asc").Find(&records).Error
	return records, err
}

func ListDNSRecordsByZoneIDs(zoneIDs []uint) (records []*DNSRecord, err error) {
	if len(zoneIDs) == 0 {
		return []*DNSRecord{}, nil
	}
	err = DB.Where("zone_id IN ?", zoneIDs).
		Order("zone_id asc").
		Order("name asc").
		Order("type asc").
		Order("id asc").
		Find(&records).Error
	return records, err
}

func ListDNSRecordsByZoneIDNameCandidates(zoneID uint, names []string, suffixes []string) (records []*DNSRecord, err error) {
	if zoneID == 0 || (len(names) == 0 && len(suffixes) == 0) {
		return []*DNSRecord{}, nil
	}
	uniqueNames := make([]string, 0, len(names))
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		uniqueNames = append(uniqueNames, name)
	}
	uniqueSuffixes := make([]string, 0, len(suffixes))
	seenSuffixes := make(map[string]struct{}, len(suffixes))
	for _, suffix := range suffixes {
		if suffix == "" {
			continue
		}
		if _, ok := seenSuffixes[suffix]; ok {
			continue
		}
		seenSuffixes[suffix] = struct{}{}
		uniqueSuffixes = append(uniqueSuffixes, suffix)
	}
	if len(uniqueNames) == 0 && len(uniqueSuffixes) == 0 {
		return []*DNSRecord{}, nil
	}

	conditions := make([]string, 0, 1+len(uniqueSuffixes))
	args := make([]any, 0, 1+len(uniqueSuffixes))
	if len(uniqueNames) > 0 {
		conditions = append(conditions, "name IN ?")
		args = append(args, uniqueNames)
	}
	for _, suffix := range uniqueSuffixes {
		conditions = append(conditions, "name LIKE ?")
		args = append(args, "%."+suffix)
	}
	err = DB.Where("zone_id = ?", zoneID).
		Where("("+strings.Join(conditions, " OR ")+")", args...).
		Order("name asc").
		Order("type asc").
		Order("id asc").
		Find(&records).Error
	return records, err
}

func ListDNSRecordCountsByZoneIDs(zoneIDs []uint) ([]DNSRecordCountByZone, error) {
	if len(zoneIDs) == 0 {
		return []DNSRecordCountByZone{}, nil
	}
	var counts []DNSRecordCountByZone
	err := DB.Model(&DNSRecord{}).
		Select("zone_id, COUNT(*) AS count").
		Where("zone_id IN ?", zoneIDs).
		Group("zone_id").
		Scan(&counts).Error
	return counts, err
}

func GetDNSRecordByID(id uint) (*DNSRecord, error) {
	record := &DNSRecord{}
	err := DB.First(record, id).Error
	return record, err
}

func (record *DNSRecord) Insert() error {
	return DB.Create(record).Error
}

func (record *DNSRecord) Update() error {
	return DB.Save(record).Error
}

func (record *DNSRecord) Delete() error {
	return DB.Delete(record).Error
}

func ListDNSWorkers() (workers []*DNSWorker, err error) {
	err = DB.Order("id desc").Find(&workers).Error
	return workers, err
}

func GetDNSWorkerByID(id uint) (*DNSWorker, error) {
	worker := &DNSWorker{}
	err := DB.First(worker, id).Error
	return worker, err
}

func GetDNSWorkerByToken(token string) (*DNSWorker, error) {
	worker := &DNSWorker{}
	err := DB.Where("token = ?", token).First(worker).Error
	return worker, err
}

func (worker *DNSWorker) Insert() error {
	return DB.Create(worker).Error
}

func (worker *DNSWorker) Update() error {
	return DB.Save(worker).Error
}

func (worker *DNSWorker) UpdateProbeResult() error {
	return DB.Model(worker).Select("last_probe_at", "last_probe_query", "last_probe_result").Updates(worker).Error
}

func (worker *DNSWorker) Delete() error {
	return DB.Delete(worker).Error
}

func (rollup *DNSQueryRollup) Insert() error {
	return DB.Create(rollup).Error
}
