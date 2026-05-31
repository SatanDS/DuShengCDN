package model

import "time"

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

type DNSWorker struct {
	ID                  uint       `json:"id" gorm:"primaryKey"`
	WorkerID            string     `json:"worker_id" gorm:"uniqueIndex;size:64;not null"`
	Name                string     `json:"name" gorm:"size:128;not null"`
	Token               string     `json:"-" gorm:"size:128;index;not null"`
	PublicAddress       string     `json:"public_address" gorm:"size:255"`
	Version             string     `json:"version" gorm:"size:64"`
	Status              string     `json:"status" gorm:"size:16;not null;default:'offline'"`
	LastSnapshotVersion string     `json:"last_snapshot_version" gorm:"size:128"`
	LastSnapshotAt      *time.Time `json:"last_snapshot_at"`
	LastSeenAt          *time.Time `json:"last_seen_at"`
	LastError           string     `json:"last_error" gorm:"type:text"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

type DNSQueryRollup struct {
	ID            uint      `json:"id" gorm:"primaryKey"`
	WindowStart   time.Time `json:"window_start" gorm:"index;not null"`
	WindowMinutes int       `json:"window_minutes" gorm:"not null;default:1"`
	WorkerID      string    `json:"worker_id" gorm:"index;size:64;not null"`
	ZoneID        uint      `json:"zone_id" gorm:"index"`
	ProxyRouteID  uint      `json:"proxy_route_id" gorm:"index"`
	QName         string    `json:"qname" gorm:"index;size:255"`
	QType         string    `json:"qtype" gorm:"size:16"`
	RCode         string    `json:"rcode" gorm:"size:32"`
	QueryCount    int64     `json:"query_count" gorm:"not null;default:0"`
	TargetSummary string    `json:"target_summary" gorm:"type:text;not null;default:'{}'"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func ListDNSZones() (zones []*DNSZone, err error) {
	err = DB.Order("name asc").Find(&zones).Error
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

func (worker *DNSWorker) Delete() error {
	return DB.Delete(worker).Error
}

func (rollup *DNSQueryRollup) Insert() error {
	return DB.Create(rollup).Error
}
