package model

import (
	"dushengcdn/common"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type legacyProxyRouteV4 struct {
	ID            uint   `gorm:"primaryKey"`
	Domain        string `gorm:"uniqueIndex;size:255;not null"`
	OriginID      *uint  `gorm:"index"`
	OriginURL     string `gorm:"size:2048;not null"`
	OriginHost    string `gorm:"size:255"`
	Upstreams     string `gorm:"type:text;not null;default:'[]'"`
	Enabled       bool   `gorm:"not null;default:true"`
	EnableHTTPS   bool   `gorm:"column:enable_https;not null;default:false"`
	CertID        *uint
	RedirectHTTP  bool   `gorm:"not null;default:false"`
	CacheEnabled  bool   `gorm:"not null;default:false"`
	CachePolicy   string `gorm:"size:32;not null;default:''"`
	CacheRules    string `gorm:"type:text;not null;default:'[]'"`
	CustomHeaders string `gorm:"type:text;not null;default:'[]'"`
	Remark        string `gorm:"size:255"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (legacyProxyRouteV4) TableName() string {
	return "proxy_routes"
}

type legacyProxyRouteV5 struct {
	ID            uint   `gorm:"primaryKey"`
	SiteName      string `gorm:"size:255;not null;default:''"`
	Domain        string `gorm:"uniqueIndex;size:255;not null"`
	Domains       string `gorm:"type:text;not null;default:'[]'"`
	OriginID      *uint  `gorm:"index"`
	OriginURL     string `gorm:"size:2048;not null"`
	OriginHost    string `gorm:"size:255"`
	Upstreams     string `gorm:"type:text;not null;default:'[]'"`
	Enabled       bool   `gorm:"not null;default:true"`
	EnableHTTPS   bool   `gorm:"column:enable_https;not null;default:false"`
	CertID        *uint
	RedirectHTTP  bool   `gorm:"not null;default:false"`
	CacheEnabled  bool   `gorm:"not null;default:false"`
	CachePolicy   string `gorm:"size:32;not null;default:''"`
	CacheRules    string `gorm:"type:text;not null;default:'[]'"`
	CustomHeaders string `gorm:"type:text;not null;default:'[]'"`
	Remark        string `gorm:"size:255"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (legacyProxyRouteV5) TableName() string {
	return "proxy_routes"
}

type legacyProxyRouteV6 struct {
	ID                 uint   `gorm:"primaryKey"`
	SiteName           string `gorm:"size:255;not null;default:''"`
	Domain             string `gorm:"uniqueIndex;size:255;not null"`
	Domains            string `gorm:"type:text;not null;default:'[]'"`
	OriginID           *uint  `gorm:"index"`
	OriginURL          string `gorm:"size:2048;not null"`
	OriginHost         string `gorm:"size:255"`
	Upstreams          string `gorm:"type:text;not null;default:'[]'"`
	Enabled            bool   `gorm:"not null;default:true"`
	EnableHTTPS        bool   `gorm:"column:enable_https;not null;default:false"`
	CertID             *uint
	RedirectHTTP       bool   `gorm:"not null;default:false"`
	LimitConnPerServer int    `gorm:"not null;default:0"`
	LimitConnPerIP     int    `gorm:"not null;default:0"`
	LimitRate          string `gorm:"size:32;not null;default:''"`
	CacheEnabled       bool   `gorm:"not null;default:false"`
	CachePolicy        string `gorm:"size:32;not null;default:''"`
	CacheRules         string `gorm:"type:text;not null;default:'[]'"`
	CustomHeaders      string `gorm:"type:text;not null;default:'[]'"`
	Remark             string `gorm:"size:255"`
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func (legacyProxyRouteV6) TableName() string {
	return "proxy_routes"
}

type legacyProxyRouteV7 struct {
	ID                 uint   `gorm:"primaryKey"`
	SiteName           string `gorm:"size:255;not null;default:''"`
	Domain             string `gorm:"uniqueIndex;size:255;not null"`
	Domains            string `gorm:"type:text;not null;default:'[]'"`
	OriginID           *uint  `gorm:"index"`
	OriginURL          string `gorm:"size:2048;not null"`
	OriginHost         string `gorm:"size:255"`
	Upstreams          string `gorm:"type:text;not null;default:'[]'"`
	Enabled            bool   `gorm:"not null;default:true"`
	EnableHTTPS        bool   `gorm:"column:enable_https;not null;default:false"`
	CertID             *uint
	CertIDs            string `gorm:"type:text;not null;default:'[]'"`
	RedirectHTTP       bool   `gorm:"not null;default:false"`
	LimitConnPerServer int    `gorm:"not null;default:0"`
	LimitConnPerIP     int    `gorm:"not null;default:0"`
	LimitRate          string `gorm:"size:32;not null;default:''"`
	CacheEnabled       bool   `gorm:"not null;default:false"`
	CachePolicy        string `gorm:"size:32;not null;default:''"`
	CacheRules         string `gorm:"type:text;not null;default:'[]'"`
	CustomHeaders      string `gorm:"type:text;not null;default:'[]'"`
	Remark             string `gorm:"size:255"`
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func (legacyProxyRouteV7) TableName() string {
	return "proxy_routes"
}

type legacyNodeAccessLogV16 struct {
	ID         uint      `gorm:"primaryKey"`
	NodeID     string    `gorm:"index:,composite:node_logged_at,priority:1;size:64;not null"`
	LoggedAt   time.Time `gorm:"index;index:,composite:node_logged_at,priority:2"`
	RemoteAddr string    `gorm:"index;size:128"`
	Region     string    `gorm:"size:128"`
	Host       string    `gorm:"index;size:255"`
	Path       string    `gorm:"size:2048"`
	StatusCode int       `gorm:"index"`
	CreatedAt  time.Time
}

func (legacyNodeAccessLogV16) TableName() string {
	return "node_access_logs"
}

func openBareTestSQLiteDB(t *testing.T, name string) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), name)), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql db: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})
	return db
}

func openTestSQLiteDB(t *testing.T, name string) *gorm.DB {
	t.Helper()

	db := openBareTestSQLiteDB(t, name)
	if err := autoMigrateAll(db); err != nil {
		t.Fatalf("auto migrate db: %v", err)
	}
	return db
}

func TestListProxyRoutesByIDsReturnsMatchingRoutes(t *testing.T) {
	db := openTestSQLiteDB(t, "proxy-routes-by-ids.db")
	previousDB := DB
	DB = db
	t.Cleanup(func() {
		DB = previousDB
	})

	routes := []*ProxyRoute{
		{
			SiteName:  "edge-a",
			Domain:    "a.example.com",
			Domains:   `["a.example.com"]`,
			OriginURL: "https://origin-a.internal",
			Upstreams: `["https://origin-a.internal"]`,
			Enabled:   true,
		},
		{
			SiteName:  "edge-b",
			Domain:    "b.example.com",
			Domains:   `["b.example.com"]`,
			OriginURL: "https://origin-b.internal",
			Upstreams: `["https://origin-b.internal"]`,
			Enabled:   true,
		},
		{
			SiteName:  "edge-c",
			Domain:    "c.example.com",
			Domains:   `["c.example.com"]`,
			OriginURL: "https://origin-c.internal",
			Upstreams: `["https://origin-c.internal"]`,
			Enabled:   true,
		},
	}
	for _, route := range routes {
		if err := route.Insert(); err != nil {
			t.Fatalf("insert proxy route: %v", err)
		}
	}

	empty, err := ListProxyRoutesByIDs(nil)
	if err != nil {
		t.Fatalf("ListProxyRoutesByIDs empty failed: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("expected empty result for empty ids, got %+v", empty)
	}

	matched, err := ListProxyRoutesByIDs([]uint{routes[2].ID, 999999, routes[0].ID, routes[2].ID})
	if err != nil {
		t.Fatalf("ListProxyRoutesByIDs failed: %v", err)
	}
	gotIDs := make([]uint, 0, len(matched))
	for _, route := range matched {
		if route == nil {
			continue
		}
		gotIDs = append(gotIDs, route.ID)
	}
	sort.Slice(gotIDs, func(i, j int) bool {
		return gotIDs[i] < gotIDs[j]
	})
	wantIDs := []uint{routes[0].ID, routes[2].ID}
	if fmt.Sprint(gotIDs) != fmt.Sprint(wantIDs) {
		t.Fatalf("unexpected route ids: got %v want %v", gotIDs, wantIDs)
	}
}

func TestListProxyRouteCertificateReferenceFieldsReturnsOnlyReferenceColumns(t *testing.T) {
	db := openTestSQLiteDB(t, "proxy-route-certificate-reference-fields.db")
	previousDB := DB
	DB = db
	t.Cleanup(func() {
		DB = previousDB
	})

	primaryCertID := uint(101)
	route := &ProxyRoute{
		SiteName:      "edge-a",
		Domain:        "a.example.com",
		Domains:       `["a.example.com"]`,
		OriginURL:     "https://origin-a.internal",
		OriginHost:    "origin-a.internal",
		Upstreams:     `["https://origin-a.internal"]`,
		NodePool:      "premium",
		Enabled:       true,
		EnableHTTPS:   true,
		CertID:        &primaryCertID,
		CertIDs:       `[101,102]`,
		DomainCertIDs: `{"a.example.com":102}`,
	}
	if err := route.Insert(); err != nil {
		t.Fatalf("insert proxy route: %v", err)
	}

	references, err := ListProxyRouteCertificateReferenceFields()
	if err != nil {
		t.Fatalf("ListProxyRouteCertificateReferenceFields failed: %v", err)
	}
	if len(references) != 1 {
		t.Fatalf("expected one route reference row, got %+v", references)
	}
	reference := references[0]
	if reference.ID != route.ID {
		t.Fatalf("unexpected route id: got %d want %d", reference.ID, route.ID)
	}
	if reference.CertID == nil || *reference.CertID != primaryCertID {
		t.Fatalf("unexpected primary certificate id: got %+v want %d", reference.CertID, primaryCertID)
	}
	if reference.CertIDs != route.CertIDs {
		t.Fatalf("unexpected certificate list: got %q want %q", reference.CertIDs, route.CertIDs)
	}
	if reference.DomainCertIDs != route.DomainCertIDs {
		t.Fatalf("unexpected domain certificate list: got %q want %q", reference.DomainCertIDs, route.DomainCertIDs)
	}
	if reference.Domain != "" || reference.OriginURL != "" || reference.NodePool != "" || reference.EnableHTTPS {
		t.Fatalf("expected non-reference fields to stay unloaded, got %+v", reference)
	}
}

func TestListProxyRouteIdentityCandidatesReturnsRelevantRoutes(t *testing.T) {
	db := openTestSQLiteDB(t, "proxy-route-identity-candidates.db")
	previousDB := DB
	DB = db
	t.Cleanup(func() {
		DB = previousDB
	})

	routes := []*ProxyRoute{
		{
			SiteName:  "edge-a",
			Domain:    "a.example.com",
			Domains:   `["a.example.com","www.example.com"]`,
			OriginURL: "https://origin-a.internal",
			Upstreams: `["https://origin-a.internal"]`,
			Enabled:   true,
		},
		{
			SiteName:  "edge-b",
			Domain:    "b.example.com",
			Domains:   `["b.example.com"]`,
			OriginURL: "https://origin-b.internal",
			Upstreams: `["https://origin-b.internal"]`,
			Enabled:   true,
		},
		{
			SiteName:  "",
			Domain:    "legacy.example.com",
			Domains:   `["legacy.example.com"]`,
			OriginURL: "https://origin-legacy.internal",
			Upstreams: `["https://origin-legacy.internal"]`,
			Enabled:   true,
		},
		{
			SiteName:  "edge-c",
			Domain:    "c.example.com",
			Domains:   `["c.example.com","not-www.example.com"]`,
			OriginURL: "https://origin-c.internal",
			Upstreams: `["https://origin-c.internal"]`,
			Enabled:   true,
		},
	}
	for _, route := range routes {
		if err := route.Insert(); err != nil {
			t.Fatalf("insert proxy route: %v", err)
		}
	}

	matched, err := ListProxyRouteIdentityCandidates("edge-a", []string{"www.example.com", "legacy.example.com", "missing.example.com"})
	if err != nil {
		t.Fatalf("ListProxyRouteIdentityCandidates failed: %v", err)
	}
	gotIDs := make([]uint, 0, len(matched))
	for _, route := range matched {
		if route == nil {
			continue
		}
		gotIDs = append(gotIDs, route.ID)
	}
	sort.Slice(gotIDs, func(i, j int) bool {
		return gotIDs[i] < gotIDs[j]
	})
	wantIDs := []uint{routes[0].ID, routes[2].ID}
	if fmt.Sprint(gotIDs) != fmt.Sprint(wantIDs) {
		t.Fatalf("unexpected candidate route ids: got %v want %v", gotIDs, wantIDs)
	}

	fallbackSiteName, err := ListProxyRouteIdentityCandidates("legacy.example.com", nil)
	if err != nil {
		t.Fatalf("ListProxyRouteIdentityCandidates fallback failed: %v", err)
	}
	if len(fallbackSiteName) != 1 || fallbackSiteName[0].ID != routes[2].ID {
		t.Fatalf("expected legacy primary domain to match fallback site name, got %+v", fallbackSiteName)
	}

	falsePositive, err := ListProxyRouteIdentityCandidates("", []string{"www.example.com"})
	if err != nil {
		t.Fatalf("ListProxyRouteIdentityCandidates exact alias failed: %v", err)
	}
	if len(falsePositive) != 1 || falsePositive[0].ID != routes[0].ID {
		t.Fatalf("expected exact alias match without matching not-www.example.com, got %+v", falsePositive)
	}

	empty, err := ListProxyRouteIdentityCandidates("", nil)
	if err != nil {
		t.Fatalf("ListProxyRouteIdentityCandidates empty failed: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("expected empty result for empty identity filters, got %+v", empty)
	}
}

func TestListTLSCertificatesByIDsReturnsMatchingCertificates(t *testing.T) {
	db := openTestSQLiteDB(t, "tls-certificates-by-ids.db")
	previousDB := DB
	DB = db
	t.Cleanup(func() {
		DB = previousDB
	})

	certificates := []*TLSCertificate{
		{Name: "cert-a", CertPEM: "cert-a", KeyPEM: "key-a", PrimaryDomain: "a.example.com"},
		{Name: "cert-b", CertPEM: "cert-b", KeyPEM: "key-b", PrimaryDomain: "b.example.com"},
		{Name: "cert-c", CertPEM: "cert-c", KeyPEM: "key-c", PrimaryDomain: "c.example.com"},
	}
	for _, certificate := range certificates {
		if err := certificate.Insert(); err != nil {
			t.Fatalf("insert tls certificate: %v", err)
		}
	}

	empty, err := ListTLSCertificatesByIDs(nil)
	if err != nil {
		t.Fatalf("ListTLSCertificatesByIDs empty failed: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("expected empty result for empty ids, got %+v", empty)
	}

	matched, err := ListTLSCertificatesByIDs([]uint{certificates[2].ID, 999999, certificates[0].ID, certificates[2].ID})
	if err != nil {
		t.Fatalf("ListTLSCertificatesByIDs failed: %v", err)
	}
	gotIDs := make([]uint, 0, len(matched))
	for _, certificate := range matched {
		if certificate == nil {
			continue
		}
		gotIDs = append(gotIDs, certificate.ID)
	}
	sort.Slice(gotIDs, func(i, j int) bool {
		return gotIDs[i] < gotIDs[j]
	})
	wantIDs := []uint{certificates[0].ID, certificates[2].ID}
	if fmt.Sprint(gotIDs) != fmt.Sprint(wantIDs) {
		t.Fatalf("unexpected certificate ids: got %v want %v", gotIDs, wantIDs)
	}
}

func TestListDnsAccountsByIDsReturnsMatchingAccounts(t *testing.T) {
	db := openTestSQLiteDB(t, "dns-accounts-by-ids.db")
	previousDB := DB
	DB = db
	t.Cleanup(func() {
		DB = previousDB
	})

	accounts := []*DnsAccount{
		{Name: "cf-a", Type: "cloudflare", Authorization: `{"api_token":"a"}`},
		{Name: "cf-b", Type: "cloudflare", Authorization: `{"api_token":"b"}`},
		{Name: "cf-c", Type: "cloudflare", Authorization: `{"api_token":"c"}`},
	}
	for _, account := range accounts {
		if err := account.Insert(); err != nil {
			t.Fatalf("insert dns account: %v", err)
		}
	}

	empty, err := ListDnsAccountsByIDs(nil)
	if err != nil {
		t.Fatalf("ListDnsAccountsByIDs empty failed: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("expected empty result for empty ids, got %+v", empty)
	}

	matched, err := ListDnsAccountsByIDs([]uint{accounts[2].ID, 999999, accounts[0].ID, accounts[2].ID})
	if err != nil {
		t.Fatalf("ListDnsAccountsByIDs failed: %v", err)
	}
	gotIDs := make([]uint, 0, len(matched))
	for _, account := range matched {
		if account == nil {
			continue
		}
		gotIDs = append(gotIDs, account.ID)
	}
	sort.Slice(gotIDs, func(i, j int) bool {
		return gotIDs[i] < gotIDs[j]
	})
	wantIDs := []uint{accounts[0].ID, accounts[2].ID}
	if fmt.Sprint(gotIDs) != fmt.Sprint(wantIDs) {
		t.Fatalf("unexpected dns account ids: got %v want %v", gotIDs, wantIDs)
	}
}

func TestListDNSRecordCountsByZoneIDsReturnsCounts(t *testing.T) {
	db := openTestSQLiteDB(t, "dns-record-counts-by-zone-ids.db")
	previousDB := DB
	DB = db
	t.Cleanup(func() {
		DB = previousDB
	})

	zones := []*DNSZone{
		{
			Name:        "a.example.com",
			SOAEmail:    "admin.example.com",
			PrimaryNS:   "ns1.example.com",
			NameServers: `["ns1.example.com"]`,
			DefaultTTL:  300,
			Serial:      1,
			Enabled:     true,
		},
		{
			Name:        "b.example.com",
			SOAEmail:    "admin.example.com",
			PrimaryNS:   "ns1.example.com",
			NameServers: `["ns1.example.com"]`,
			DefaultTTL:  300,
			Serial:      1,
			Enabled:     true,
		},
		{
			Name:        "empty.example.com",
			SOAEmail:    "admin.example.com",
			PrimaryNS:   "ns1.example.com",
			NameServers: `["ns1.example.com"]`,
			DefaultTTL:  300,
			Serial:      1,
			Enabled:     true,
		},
	}
	for _, zone := range zones {
		if err := zone.Insert(); err != nil {
			t.Fatalf("insert dns zone: %v", err)
		}
	}

	records := []*DNSRecord{
		{ZoneID: zones[0].ID, Name: "www.a.example.com", Type: "A", Value: "192.0.2.1", TTL: 300, Enabled: true},
		{ZoneID: zones[0].ID, Name: "api.a.example.com", Type: "A", Value: "192.0.2.2", TTL: 300, Enabled: true},
		{ZoneID: zones[1].ID, Name: "www.b.example.com", Type: "A", Value: "192.0.2.3", TTL: 300, Enabled: true},
	}
	for _, record := range records {
		if err := record.Insert(); err != nil {
			t.Fatalf("insert dns record: %v", err)
		}
	}

	empty, err := ListDNSRecordCountsByZoneIDs(nil)
	if err != nil {
		t.Fatalf("ListDNSRecordCountsByZoneIDs empty failed: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("expected empty result for empty zone ids, got %+v", empty)
	}

	countRows, err := ListDNSRecordCountsByZoneIDs([]uint{zones[1].ID, zones[0].ID, zones[2].ID, 999999, zones[0].ID})
	if err != nil {
		t.Fatalf("ListDNSRecordCountsByZoneIDs failed: %v", err)
	}
	counts := make(map[uint]int64, len(countRows))
	for _, row := range countRows {
		counts[row.ZoneID] = row.Count
	}
	if counts[zones[0].ID] != 2 || counts[zones[1].ID] != 1 {
		t.Fatalf("unexpected dns record counts: %+v", counts)
	}
	if _, ok := counts[zones[2].ID]; ok {
		t.Fatalf("expected zone without records to be omitted, got %+v", counts)
	}
	if _, ok := counts[999999]; ok {
		t.Fatalf("expected unknown zone id to be omitted, got %+v", counts)
	}
}

func TestListDNSRecordsByZoneIDNameCandidatesReturnsMatchingRecords(t *testing.T) {
	db := openTestSQLiteDB(t, "dns-record-name-candidates.db")
	previousDB := DB
	DB = db
	t.Cleanup(func() {
		DB = previousDB
	})

	zoneA := &DNSZone{
		Name:        "a.example.com",
		SOAEmail:    "admin.example.com",
		PrimaryNS:   "ns1.example.com",
		NameServers: `["ns1.example.com"]`,
		DefaultTTL:  300,
		Serial:      1,
		Enabled:     true,
	}
	zoneB := &DNSZone{
		Name:        "b.example.com",
		SOAEmail:    "admin.example.com",
		PrimaryNS:   "ns1.example.com",
		NameServers: `["ns1.example.com"]`,
		DefaultTTL:  300,
		Serial:      1,
		Enabled:     true,
	}
	if err := zoneA.Insert(); err != nil {
		t.Fatalf("insert dns zone A: %v", err)
	}
	if err := zoneB.Insert(); err != nil {
		t.Fatalf("insert dns zone B: %v", err)
	}
	records := []*DNSRecord{
		{ZoneID: zoneA.ID, Name: "api.a.example.com", Type: "A", Value: "192.0.2.1", TTL: 300, Enabled: true},
		{ZoneID: zoneA.ID, Name: "www.a.example.com", Type: "A", Value: "192.0.2.2", TTL: 300, Enabled: true},
		{ZoneID: zoneA.ID, Name: "www.a.example.com", Type: "AAAA", Value: "2001:db8::1", TTL: 300, Enabled: true},
		{ZoneID: zoneA.ID, Name: "deep.api.a.example.com", Type: "A", Value: "192.0.2.4", TTL: 300, Enabled: true},
		{ZoneID: zoneB.ID, Name: "www.a.example.com", Type: "A", Value: "192.0.2.3", TTL: 300, Enabled: true},
	}
	for _, record := range records {
		if err := record.Insert(); err != nil {
			t.Fatalf("insert dns record: %v", err)
		}
	}

	empty, err := ListDNSRecordsByZoneIDNameCandidates(zoneA.ID, nil, nil)
	if err != nil {
		t.Fatalf("ListDNSRecordsByZoneIDNameCandidates empty failed: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("expected empty result for empty names, got %+v", empty)
	}

	matched, err := ListDNSRecordsByZoneIDNameCandidates(zoneA.ID, []string{"www.a.example.com", "missing.a.example.com", "www.a.example.com"}, nil)
	if err != nil {
		t.Fatalf("ListDNSRecordsByZoneIDNameCandidates failed: %v", err)
	}
	if len(matched) != 2 {
		t.Fatalf("expected two matching records in zone A, got %+v", matched)
	}
	gotTypes := []string{matched[0].Type, matched[1].Type}
	if fmt.Sprint(gotTypes) != fmt.Sprint([]string{"A", "AAAA"}) {
		t.Fatalf("unexpected matching record types: %v", gotTypes)
	}
	for _, record := range matched {
		if record.ZoneID != zoneA.ID || record.Name != "www.a.example.com" {
			t.Fatalf("unexpected matched record: %+v", record)
		}
	}

	suffixMatches, err := ListDNSRecordsByZoneIDNameCandidates(zoneA.ID, nil, []string{"a.example.com"})
	if err != nil {
		t.Fatalf("ListDNSRecordsByZoneIDNameCandidates suffix failed: %v", err)
	}
	gotNames := make([]string, 0, len(suffixMatches))
	for _, record := range suffixMatches {
		gotNames = append(gotNames, record.Name)
	}
	if fmt.Sprint(gotNames) != fmt.Sprint([]string{"api.a.example.com", "deep.api.a.example.com", "www.a.example.com", "www.a.example.com"}) {
		t.Fatalf("unexpected suffix candidate names: %v", gotNames)
	}
}

func TestListDNSRecordsByZoneIDsReturnsOrderedRecords(t *testing.T) {
	db := openTestSQLiteDB(t, "dns-records-by-zone-ids.db")
	previousDB := DB
	DB = db
	t.Cleanup(func() {
		DB = previousDB
	})

	zones := []*DNSZone{
		{Name: "a.example.com", SOAEmail: "admin.example.com", PrimaryNS: "ns1.example.com", NameServers: `["ns1.example.com"]`, DefaultTTL: 300, Serial: 1, Enabled: true},
		{Name: "b.example.com", SOAEmail: "admin.example.com", PrimaryNS: "ns1.example.com", NameServers: `["ns1.example.com"]`, DefaultTTL: 300, Serial: 1, Enabled: true},
	}
	for _, zone := range zones {
		if err := zone.Insert(); err != nil {
			t.Fatalf("insert dns zone: %v", err)
		}
	}
	records := []*DNSRecord{
		{ZoneID: zones[1].ID, Name: "www.b.example.com", Type: "A", Value: "192.0.2.2", TTL: 300, Enabled: true},
		{ZoneID: zones[0].ID, Name: "www.a.example.com", Type: "AAAA", Value: "2001:db8::1", TTL: 300, Enabled: true},
		{ZoneID: zones[0].ID, Name: "api.a.example.com", Type: "A", Value: "192.0.2.1", TTL: 300, Enabled: true},
	}
	for _, record := range records {
		if err := record.Insert(); err != nil {
			t.Fatalf("insert dns record: %v", err)
		}
	}

	empty, err := ListDNSRecordsByZoneIDs(nil)
	if err != nil {
		t.Fatalf("ListDNSRecordsByZoneIDs empty failed: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("expected empty result for empty zone ids, got %+v", empty)
	}

	matched, err := ListDNSRecordsByZoneIDs([]uint{zones[1].ID, 999999, zones[0].ID, zones[1].ID})
	if err != nil {
		t.Fatalf("ListDNSRecordsByZoneIDs failed: %v", err)
	}
	got := make([]string, 0, len(matched))
	for _, record := range matched {
		got = append(got, fmt.Sprintf("%d:%s:%s", record.ZoneID, record.Name, record.Type))
	}
	want := []string{
		fmt.Sprintf("%d:api.a.example.com:A", zones[0].ID),
		fmt.Sprintf("%d:www.a.example.com:AAAA", zones[0].ID),
		fmt.Sprintf("%d:www.b.example.com:A", zones[1].ID),
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("unexpected ordered dns records: got %v want %v", got, want)
	}
}

func TestListDNSZonesByIDsReturnsMatchingZones(t *testing.T) {
	db := openTestSQLiteDB(t, "dns-zones-by-ids.db")
	previousDB := DB
	DB = db
	t.Cleanup(func() {
		DB = previousDB
	})

	zones := []*DNSZone{
		{Name: "a.example.com", SOAEmail: "admin.example.com", PrimaryNS: "ns1.example.com", NameServers: `["ns1.example.com"]`, DefaultTTL: 300, Serial: 1, Enabled: true},
		{Name: "b.example.com", SOAEmail: "admin.example.com", PrimaryNS: "ns1.example.com", NameServers: `["ns1.example.com"]`, DefaultTTL: 300, Serial: 1, Enabled: true},
		{Name: "c.example.com", SOAEmail: "admin.example.com", PrimaryNS: "ns1.example.com", NameServers: `["ns1.example.com"]`, DefaultTTL: 300, Serial: 1, Enabled: false},
	}
	for _, zone := range zones {
		if err := zone.Insert(); err != nil {
			t.Fatalf("insert dns zone: %v", err)
		}
	}

	empty, err := ListDNSZonesByIDs(nil)
	if err != nil {
		t.Fatalf("ListDNSZonesByIDs empty failed: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("expected empty result for empty ids, got %+v", empty)
	}

	matched, err := ListDNSZonesByIDs([]uint{zones[2].ID, zones[0].ID, 999999, zones[2].ID})
	if err != nil {
		t.Fatalf("ListDNSZonesByIDs failed: %v", err)
	}
	gotIDs := make([]uint, 0, len(matched))
	for _, zone := range matched {
		gotIDs = append(gotIDs, zone.ID)
	}
	wantIDs := []uint{zones[0].ID, zones[2].ID}
	if fmt.Sprint(gotIDs) != fmt.Sprint(wantIDs) {
		t.Fatalf("unexpected dns zone ids: got %v want %v", gotIDs, wantIDs)
	}
}

func findDBModelByTableName(t *testing.T, tableName string) dbModel {
	t.Helper()

	models, err := buildDBModels()
	if err != nil {
		t.Fatalf("build db models: %v", err)
	}
	for _, item := range models {
		if item.tableName == tableName {
			return item
		}
	}
	t.Fatalf("db model not found for table %s", tableName)
	return dbModel{}
}

func TestNodeAccessLogAggregationsAcrossShards(t *testing.T) {
	db := openBareTestSQLiteDB(t, "access-log-aggregations.db")
	if err := registerSharding(db, "sqlite"); err != nil {
		t.Fatalf("register sharding: %v", err)
	}
	if err := applyCurrentSchema(db, "sqlite"); err != nil {
		t.Fatalf("applyCurrentSchema: %v", err)
	}

	previousDB := DB
	DB = db
	t.Cleanup(func() {
		DB = previousDB
	})

	rawDB := sessionIgnoringSharding(db)
	baseTime := time.Date(2026, 6, 4, 8, 12, 0, 0, time.UTC)
	fixtures := []NodeAccessLog{
		{
			ID:            20,
			NodeID:        "node-a",
			LoggedAt:      baseTime.Add(-4 * time.Minute),
			RemoteAddr:    "203.0.113.1",
			Region:        "HK",
			Operator:      "EdgeNet",
			Host:          "app.example.com",
			Path:          "/index",
			StatusCode:    200,
			CacheStatus:   "HIT",
			RequestBytes:  100,
			ResponseBytes: 1000,
			UpstreamBytes: 0,
		},
		{
			ID:            21,
			NodeID:        "node-b",
			LoggedAt:      baseTime.Add(-3 * time.Minute),
			RemoteAddr:    "203.0.113.1",
			Region:        "HK",
			Operator:      "EdgeNet",
			Host:          "app.example.com",
			Path:          "/asset.js",
			StatusCode:    502,
			CacheStatus:   "MISS",
			RequestBytes:  200,
			ResponseBytes: 2000,
			UpstreamBytes: 1500,
		},
		{
			ID:            22,
			NodeID:        "node-a",
			LoggedAt:      baseTime.Add(-2 * time.Minute),
			RemoteAddr:    "203.0.113.2",
			Region:        "SG",
			Operator:      "SeaNet",
			Host:          "api.example.com",
			Path:          "/v1",
			StatusCode:    404,
			CacheStatus:   "BYPASS",
			RequestBytes:  300,
			ResponseBytes: 500,
			UpstreamBytes: 500,
		},
	}
	for _, item := range fixtures {
		table := observabilityShardTableForID("node_access_logs", item.ID)
		if err := rawDB.Table(table).Create(&item).Error; err != nil {
			t.Fatalf("seed access log %d into %s: %v", item.ID, table, err)
		}
	}

	logs, err := ListNodeAccessLogs(NodeAccessLogQuery{Page: 0, PageSize: 2})
	if err != nil {
		t.Fatalf("ListNodeAccessLogs failed: %v", err)
	}
	if len(logs) != 2 || logs[0].ID != 22 || logs[1].ID != 21 {
		t.Fatalf("expected newest two logs across shards, got %+v", logs)
	}
	nextLogs, err := ListNodeAccessLogs(NodeAccessLogQuery{Page: 1, PageSize: 2})
	if err != nil {
		t.Fatalf("ListNodeAccessLogs second page failed: %v", err)
	}
	if len(nextLogs) != 1 || nextLogs[0].ID != 20 {
		t.Fatalf("expected second page to contain the oldest log only, got %+v", nextLogs)
	}
	filteredLogs, err := ListNodeAccessLogs(NodeAccessLogQuery{Host: "app.example.com", Page: 0, PageSize: 10, SortBy: "status_code", SortOrder: "desc"})
	if err != nil {
		t.Fatalf("ListNodeAccessLogs filtered failed: %v", err)
	}
	if len(filteredLogs) != 2 || filteredLogs[0].ID != 21 || filteredLogs[1].ID != 20 {
		t.Fatalf("expected filtered logs sorted by status across shards, got %+v", filteredLogs)
	}

	totalRecords, totalIPs, err := CountNodeAccessLogs(NodeAccessLogQuery{})
	if err != nil {
		t.Fatalf("CountNodeAccessLogs failed: %v", err)
	}
	if totalRecords != 3 || totalIPs != 2 {
		t.Fatalf("unexpected access log counts: records=%d ips=%d", totalRecords, totalIPs)
	}
	filteredRecords, filteredIPs, err := CountNodeAccessLogs(NodeAccessLogQuery{Host: "app.example.com"})
	if err != nil {
		t.Fatalf("CountNodeAccessLogs filtered failed: %v", err)
	}
	if filteredRecords != 2 || filteredIPs != 1 {
		t.Fatalf("unexpected filtered access log counts: records=%d ips=%d", filteredRecords, filteredIPs)
	}
	regions, err := ListNodeAccessLogRegionCounts("", baseTime.Add(-time.Hour), 1)
	if err != nil {
		t.Fatalf("ListNodeAccessLogRegionCounts failed: %v", err)
	}
	if len(regions) != 1 || regions[0].Region != "HK" || regions[0].Count != 2 {
		t.Fatalf("unexpected top region counts: %+v", regions)
	}

	buckets, err := ListNodeAccessLogBuckets(NodeAccessLogBucketQuery{FoldMinutes: 5, PageSize: 10})
	if err != nil {
		t.Fatalf("ListNodeAccessLogBuckets failed: %v", err)
	}
	if len(buckets) != 2 {
		t.Fatalf("expected two folded buckets, got %+v", buckets)
	}
	var bucketRecords int64
	var foundDedupedBucket bool
	for _, bucket := range buckets {
		bucketRecords += bucket.RequestCount
		if bucket.RequestCount == 2 && bucket.UniqueIPCount == 1 {
			foundDedupedBucket = true
		}
	}
	if bucketRecords != 3 || !foundDedupedBucket {
		t.Fatalf("unexpected folded bucket totals: %+v", buckets)
	}
	bucketTotal, err := CountNodeAccessLogBuckets(NodeAccessLogBucketQuery{FoldMinutes: 5})
	if err != nil {
		t.Fatalf("CountNodeAccessLogBuckets failed: %v", err)
	}
	if bucketTotal != 2 {
		t.Fatalf("expected two folded bucket count, got %d", bucketTotal)
	}
	topBucketPage, err := ListNodeAccessLogBuckets(NodeAccessLogBucketQuery{
		FoldMinutes: 5,
		Page:        0,
		PageSize:    1,
		SortBy:      "request_count",
		SortOrder:   "desc",
	})
	if err != nil {
		t.Fatalf("ListNodeAccessLogBuckets top page failed: %v", err)
	}
	if len(topBucketPage) != 1 ||
		topBucketPage[0].RequestCount != 2 ||
		topBucketPage[0].UniqueIPCount != 1 ||
		topBucketPage[0].UniqueHostCount != 1 ||
		topBucketPage[0].SuccessCount != 1 ||
		topBucketPage[0].ServerErrorCount != 1 {
		t.Fatalf("unexpected folded bucket top page: %+v", topBucketPage)
	}
	secondBucketPage, err := ListNodeAccessLogBuckets(NodeAccessLogBucketQuery{
		FoldMinutes: 5,
		Page:        1,
		PageSize:    1,
		SortBy:      "request_count",
		SortOrder:   "desc",
	})
	if err != nil {
		t.Fatalf("ListNodeAccessLogBuckets second page failed: %v", err)
	}
	if len(secondBucketPage) != 1 || secondBucketPage[0].RequestCount != 1 {
		t.Fatalf("expected second folded bucket page to contain the single-request bucket, got %+v", secondBucketPage)
	}
	filteredBucketTotal, err := CountNodeAccessLogBuckets(NodeAccessLogBucketQuery{Host: "app.example.com", FoldMinutes: 5})
	if err != nil {
		t.Fatalf("CountNodeAccessLogBuckets filtered failed: %v", err)
	}
	if filteredBucketTotal != 1 {
		t.Fatalf("expected one filtered folded bucket, got %d", filteredBucketTotal)
	}

	ipSummaries, err := ListNodeAccessLogIPSummaries(NodeAccessLogIPSummaryQuery{
		Page:      0,
		PageSize:  10,
		SortBy:    "total_requests",
		SortOrder: "desc",
	}, baseTime.Add(-4*time.Hour))
	if err != nil {
		t.Fatalf("ListNodeAccessLogIPSummaries failed: %v", err)
	}
	if len(ipSummaries) != 2 ||
		ipSummaries[0].RemoteAddr != "203.0.113.1" ||
		ipSummaries[0].TotalRequests != 2 ||
		ipSummaries[0].Region != "HK" ||
		ipSummaries[0].RecentRequests != 2 {
		t.Fatalf("unexpected ip summaries: %+v", ipSummaries)
	}
	secondIPPage, err := ListNodeAccessLogIPSummaries(NodeAccessLogIPSummaryQuery{
		Page:      1,
		PageSize:  1,
		SortBy:    "total_requests",
		SortOrder: "desc",
	}, baseTime.Add(-4*time.Hour))
	if err != nil {
		t.Fatalf("ListNodeAccessLogIPSummaries second page failed: %v", err)
	}
	if len(secondIPPage) != 1 || secondIPPage[0].RemoteAddr != "203.0.113.2" {
		t.Fatalf("expected second ip summary page to contain 203.0.113.2, got %+v", secondIPPage)
	}
	filteredIPTotal, err := CountNodeAccessLogIPSummaries(NodeAccessLogIPSummaryQuery{Host: "app.example.com"})
	if err != nil {
		t.Fatalf("CountNodeAccessLogIPSummaries filtered failed: %v", err)
	}
	if filteredIPTotal != 1 {
		t.Fatalf("expected one filtered ip summary, got %d", filteredIPTotal)
	}

	trend, err := ListNodeAccessLogIPTrend(NodeAccessLogIPTrendQuery{
		RemoteAddr:    "203.0.113.1",
		Since:         baseTime.Add(-time.Hour),
		BucketMinutes: 5,
	})
	if err != nil {
		t.Fatalf("ListNodeAccessLogIPTrend failed: %v", err)
	}
	if len(trend) != 1 || trend[0].RequestCount != 2 {
		t.Fatalf("unexpected ip trend: %+v", trend)
	}

	domains, err := ListNodeAccessLogHostDistributions(NodeAccessLogDistributionQuery{Limit: 2})
	if err != nil {
		t.Fatalf("ListNodeAccessLogHostDistributions failed: %v", err)
	}
	if len(domains) == 0 || domains[0].Key != "app.example.com" || domains[0].Value != 2 {
		t.Fatalf("unexpected domain distributions: %+v", domains)
	}
	urls, err := ListNodeAccessLogURLDistributions(NodeAccessLogDistributionQuery{Limit: 1})
	if err != nil {
		t.Fatalf("ListNodeAccessLogURLDistributions failed: %v", err)
	}
	if len(urls) != 1 || urls[0].Key != "api.example.com/v1" || urls[0].Value != 1 {
		t.Fatalf("unexpected url distributions: %+v", urls)
	}
	statuses, err := ListNodeAccessLogStatusDistributions(NodeAccessLogDistributionQuery{Limit: 2})
	if err != nil {
		t.Fatalf("ListNodeAccessLogStatusDistributions failed: %v", err)
	}
	if len(statuses) != 2 || statuses[0].Key != "200" || statuses[1].Key != "404" {
		t.Fatalf("expected status distribution ties to sort by status key, got %+v", statuses)
	}

	meteringSummary, err := GetNodeAccessLogMeteringSummary(baseTime.Add(-time.Hour))
	if err != nil {
		t.Fatalf("GetNodeAccessLogMeteringSummary failed: %v", err)
	}
	if meteringSummary.RequestCount != 3 ||
		meteringSummary.RequestBytes != 600 ||
		meteringSummary.ResponseBytes != 3500 ||
		meteringSummary.UpstreamBytes != 2000 ||
		meteringSummary.UpstreamBytesHitCount != 2 ||
		meteringSummary.CacheHitCount != 1 ||
		meteringSummary.CacheMissCount != 1 ||
		meteringSummary.CacheBypassCount != 1 ||
		meteringSummary.CacheClassifiedCount != 3 {
		t.Fatalf("unexpected metering summary: %+v", meteringSummary)
	}

	siteTraffic, err := ListNodeAccessLogMeteringTrafficByHost(baseTime.Add(-time.Hour), 2)
	if err != nil {
		t.Fatalf("ListNodeAccessLogMeteringTrafficByHost failed: %v", err)
	}
	if len(siteTraffic) == 0 ||
		siteTraffic[0].Key != "app.example.com" ||
		siteTraffic[0].RequestCount != 2 ||
		siteTraffic[0].ResponseBytes != 3000 ||
		siteTraffic[0].UpstreamBytes != 1500 {
		t.Fatalf("unexpected site metering traffic: %+v", siteTraffic)
	}

	nodeTraffic, err := ListNodeAccessLogMeteringTrafficByNode(baseTime.Add(-time.Hour), 2)
	if err != nil {
		t.Fatalf("ListNodeAccessLogMeteringTrafficByNode failed: %v", err)
	}
	if len(nodeTraffic) != 2 ||
		nodeTraffic[0].Key != "node-b" ||
		nodeTraffic[0].RequestCount != 1 ||
		nodeTraffic[0].ResponseBytes != 2000 ||
		nodeTraffic[1].Key != "node-a" ||
		nodeTraffic[1].RequestCount != 2 ||
		nodeTraffic[1].ResponseBytes != 1500 {
		t.Fatalf("unexpected node metering traffic: %+v", nodeTraffic)
	}
}

func TestListNodeAccessLogIPSummariesIncludesLatestRegion(t *testing.T) {
	db := openBareTestSQLiteDB(t, "access-log-ip-summary-latest-region.db")
	if err := registerSharding(db, "sqlite"); err != nil {
		t.Fatalf("register sharding: %v", err)
	}
	if err := applyCurrentSchema(db, "sqlite"); err != nil {
		t.Fatalf("applyCurrentSchema: %v", err)
	}

	previousDB := DB
	DB = db
	t.Cleanup(func() {
		DB = previousDB
	})

	rawDB := sessionIgnoringSharding(db)
	baseTime := time.Date(2026, 6, 4, 8, 12, 0, 0, time.UTC)
	fixtures := []NodeAccessLog{
		{
			ID:         30,
			NodeID:     "node-a",
			LoggedAt:   baseTime.Add(-3 * time.Minute),
			RemoteAddr: "203.0.113.9",
			Region:     "HK",
			Operator:   "OldNet",
			Host:       "app.example.com",
			Path:       "/old",
			StatusCode: 200,
		},
		{
			ID:         31,
			NodeID:     "node-b",
			LoggedAt:   baseTime.Add(-time.Minute),
			RemoteAddr: "203.0.113.9",
			Region:     "TW",
			Operator:   "NewNet",
			Host:       "app.example.com",
			Path:       "/new",
			StatusCode: 200,
		},
		{
			ID:         32,
			NodeID:     "node-a",
			LoggedAt:   baseTime.Add(-2 * time.Minute),
			RemoteAddr: "203.0.113.8",
			Region:     "SG",
			Operator:   "SeaNet",
			Host:       "api.example.com",
			Path:       "/api",
			StatusCode: 200,
		},
	}
	for _, item := range fixtures {
		table := observabilityShardTableForID("node_access_logs", item.ID)
		if err := rawDB.Table(table).Create(&item).Error; err != nil {
			t.Fatalf("seed access log %d into %s: %v", item.ID, table, err)
		}
	}

	summaries, err := ListNodeAccessLogIPSummaries(NodeAccessLogIPSummaryQuery{
		Page:      0,
		PageSize:  10,
		SortBy:    "total_requests",
		SortOrder: "desc",
	}, baseTime.Add(-4*time.Hour))
	if err != nil {
		t.Fatalf("ListNodeAccessLogIPSummaries failed: %v", err)
	}
	if len(summaries) != 2 {
		t.Fatalf("expected two ip summaries, got %+v", summaries)
	}
	if summaries[0].RemoteAddr != "203.0.113.9" ||
		summaries[0].TotalRequests != 2 ||
		summaries[0].Region != "TW" ||
		summaries[0].Operator != "NewNet" {
		t.Fatalf("expected latest region/operator on top summary, got %+v", summaries[0])
	}
}

func TestExistingNodeAccessLogDedupKeysSQLFindsKeysAcrossShards(t *testing.T) {
	db := openBareTestSQLiteDB(t, "access-log-dedup-fast-path.db")
	if err := registerSharding(db, "sqlite"); err != nil {
		t.Fatalf("register sharding: %v", err)
	}
	if err := applyCurrentSchema(db, "sqlite"); err != nil {
		t.Fatalf("applyCurrentSchema: %v", err)
	}

	rawDB := sessionIgnoringSharding(db)
	baseTime := time.Date(2026, 6, 4, 9, 0, 0, 0, time.UTC)
	existing := []*NodeAccessLog{
		{
			ID:         30,
			NodeID:     "node-fast",
			LoggedAt:   baseTime,
			RemoteAddr: "198.51.100.1",
			Host:       "fast.example.com",
			Path:       "/same",
			StatusCode: 200,
		},
		{
			ID:         31,
			NodeID:     "node-fast-other",
			LoggedAt:   baseTime.Add(time.Minute),
			RemoteAddr: "198.51.100.2",
			Host:       "fast.example.com",
			Path:       "/other",
			StatusCode: 201,
		},
	}
	for _, record := range existing {
		table := observabilityShardTableForID("node_access_logs", record.ID)
		if err := rawDB.Table(table).Create(record).Error; err != nil {
			t.Fatalf("seed access log %d into %s: %v", record.ID, table, err)
		}
	}

	keys, err := existingNodeAccessLogDedupKeysSQL(db, []string{"node-fast", "node-fast-other"}, nodeAccessLogTimeRange{
		min: baseTime.Add(-time.Second),
		max: baseTime.Add(2 * time.Minute),
	})
	if err != nil {
		t.Fatalf("existingNodeAccessLogDedupKeysSQL failed: %v", err)
	}
	for _, record := range existing {
		if _, ok := keys[nodeAccessLogDedupKeyFor(record)]; !ok {
			t.Fatalf("expected fast path to include access log key %+v, got %+v", nodeAccessLogDedupKeyFor(record), keys)
		}
	}
	if _, ok := keys[nodeAccessLogDedupKey{
		nodeID:     "node-fast",
		loggedAtNS: baseTime.Add(10 * time.Minute).UnixNano(),
		remoteAddr: "198.51.100.99",
		host:       "fast.example.com",
		path:       "/missing",
		statusCode: 404,
	}]; ok {
		t.Fatalf("unexpected missing access log key in fast path result: %+v", keys)
	}
}

func TestInsertNewNodeAccessLogsDeduplicatesBatchAndExistingRows(t *testing.T) {
	db := openBareTestSQLiteDB(t, "access-log-bulk-insert.db")
	if err := registerSharding(db, "sqlite"); err != nil {
		t.Fatalf("register sharding: %v", err)
	}
	if err := applyCurrentSchema(db, "sqlite"); err != nil {
		t.Fatalf("applyCurrentSchema: %v", err)
	}

	previousDB := DB
	DB = db
	t.Cleanup(func() {
		DB = previousDB
	})

	existing := &NodeAccessLog{
		ID:         30,
		NodeID:     "node-bulk",
		LoggedAt:   time.Date(2026, 6, 4, 9, 0, 0, 0, time.UTC),
		RemoteAddr: "198.51.100.1",
		Host:       "bulk.example.com",
		Path:       "/same",
		StatusCode: 200,
	}
	rawDB := sessionIgnoringSharding(db)
	if err := rawDB.Table(observabilityShardTableForID("node_access_logs", existing.ID)).Create(existing).Error; err != nil {
		t.Fatalf("seed existing access log: %v", err)
	}

	newRecord := &NodeAccessLog{
		NodeID:     "node-bulk",
		LoggedAt:   existing.LoggedAt.Add(time.Minute),
		RemoteAddr: "198.51.100.2",
		Host:       "bulk.example.com",
		Path:       "/new",
		StatusCode: 201,
	}
	otherNodeRecord := &NodeAccessLog{
		NodeID:     "node-bulk-other",
		LoggedAt:   existing.LoggedAt.Add(2 * time.Minute),
		RemoteAddr: "198.51.100.3",
		Host:       "bulk.example.com",
		Path:       "/other",
		StatusCode: 202,
	}
	inserted, err := InsertNewNodeAccessLogs(db, []*NodeAccessLog{
		{
			NodeID:     existing.NodeID,
			LoggedAt:   existing.LoggedAt,
			RemoteAddr: existing.RemoteAddr,
			Host:       existing.Host,
			Path:       existing.Path,
			StatusCode: existing.StatusCode,
		},
		newRecord,
		{
			NodeID:     newRecord.NodeID,
			LoggedAt:   newRecord.LoggedAt,
			RemoteAddr: newRecord.RemoteAddr,
			Host:       newRecord.Host,
			Path:       newRecord.Path,
			StatusCode: newRecord.StatusCode,
		},
		otherNodeRecord,
		{
			NodeID:     otherNodeRecord.NodeID,
			LoggedAt:   otherNodeRecord.LoggedAt,
			RemoteAddr: otherNodeRecord.RemoteAddr,
			Host:       otherNodeRecord.Host,
			Path:       otherNodeRecord.Path,
			StatusCode: otherNodeRecord.StatusCode,
		},
	})
	if err != nil {
		t.Fatalf("InsertNewNodeAccessLogs failed: %v", err)
	}
	if inserted != 2 {
		t.Fatalf("expected two inserted access logs, got %d", inserted)
	}

	totalRecords, totalIPs, err := CountNodeAccessLogs(NodeAccessLogQuery{NodeID: "node-bulk"})
	if err != nil {
		t.Fatalf("CountNodeAccessLogs failed: %v", err)
	}
	if totalRecords != 3 || totalIPs != 3 {
		t.Fatalf("unexpected totals after bulk insert: records=%d ips=%d", totalRecords, totalIPs)
	}
}

func TestNodeMetricAndRequestReportLimitsMergeAcrossShards(t *testing.T) {
	db := openBareTestSQLiteDB(t, "observability-limits.db")
	if err := registerSharding(db, "sqlite"); err != nil {
		t.Fatalf("register sharding: %v", err)
	}
	if err := applyCurrentSchema(db, "sqlite"); err != nil {
		t.Fatalf("applyCurrentSchema: %v", err)
	}

	previousDB := DB
	DB = db
	t.Cleanup(func() {
		DB = previousDB
	})

	baseTime := time.Date(2026, 6, 4, 10, 0, 0, 0, time.UTC)
	snapshots := []NodeMetricSnapshot{
		{ID: 40, NodeID: "node-limit", CapturedAt: baseTime.Add(-4 * time.Minute), CPUUsagePercent: 10},
		{ID: 41, NodeID: "node-limit", CapturedAt: baseTime.Add(-3 * time.Minute), CPUUsagePercent: 20},
		{ID: 42, NodeID: "node-limit", CapturedAt: baseTime.Add(-2 * time.Minute), CPUUsagePercent: 30},
	}
	for _, item := range snapshots {
		record := item
		if err := db.Create(&record).Error; err != nil {
			t.Fatalf("seed metric snapshot %d: %v", item.ID, err)
		}
	}
	reports := []NodeRequestReport{
		{ID: 50, NodeID: "node-limit", WindowStartedAt: baseTime.Add(-5 * time.Minute), WindowEndedAt: baseTime.Add(-4 * time.Minute), RequestCount: 10},
		{ID: 51, NodeID: "node-limit", WindowStartedAt: baseTime.Add(-4 * time.Minute), WindowEndedAt: baseTime.Add(-3 * time.Minute), RequestCount: 20},
		{ID: 52, NodeID: "node-limit", WindowStartedAt: baseTime.Add(-3 * time.Minute), WindowEndedAt: baseTime.Add(-2 * time.Minute), RequestCount: 30},
	}
	for _, item := range reports {
		record := item
		if err := db.Create(&record).Error; err != nil {
			t.Fatalf("seed request report %d: %v", item.ID, err)
		}
	}

	gotSnapshots, err := ListNodeMetricSnapshots("node-limit", time.Time{}, 2)
	if err != nil {
		t.Fatalf("ListNodeMetricSnapshots failed: %v", err)
	}
	if len(gotSnapshots) != 2 || gotSnapshots[0].ID != 42 || gotSnapshots[1].ID != 41 {
		t.Fatalf("expected latest two snapshots across shards, got %+v", gotSnapshots)
	}

	gotReports, err := ListNodeRequestReports("node-limit", time.Time{}, 2)
	if err != nil {
		t.Fatalf("ListNodeRequestReports failed: %v", err)
	}
	if len(gotReports) != 2 || gotReports[0].ID != 52 || gotReports[1].ID != 51 {
		t.Fatalf("expected latest two request reports across shards, got %+v", gotReports)
	}
}

func TestMetricSnapshotTrendAndCounterBucketsMergeAcrossShards(t *testing.T) {
	db := openBareTestSQLiteDB(t, "metric-snapshot-trends.db")
	if err := registerSharding(db, "sqlite"); err != nil {
		t.Fatalf("register sharding: %v", err)
	}
	if err := applyCurrentSchema(db, "sqlite"); err != nil {
		t.Fatalf("applyCurrentSchema: %v", err)
	}

	previousDB := DB
	DB = db
	t.Cleanup(func() {
		DB = previousDB
	})

	baseTime := time.Date(2026, 6, 4, 10, 30, 0, 0, time.UTC)
	snapshots := []NodeMetricSnapshot{
		{
			ID:               101,
			NodeID:           "node-shard-metric",
			CapturedAt:       baseTime.Add(-time.Hour),
			CPUUsagePercent:  20,
			MemoryUsedBytes:  2,
			MemoryTotalBytes: 10,
			DiskReadBytes:    100,
			DiskWriteBytes:   200,
			NetworkRxBytes:   1000,
			NetworkTxBytes:   2000,
			OpenrestyRxBytes: 3000,
			OpenrestyTxBytes: 4000,
		},
		{
			ID:               112,
			NodeID:           "node-shard-metric",
			CapturedAt:       baseTime,
			CPUUsagePercent:  40,
			MemoryUsedBytes:  5,
			MemoryTotalBytes: 10,
			DiskReadBytes:    170,
			DiskWriteBytes:   350,
			NetworkRxBytes:   1600,
			NetworkTxBytes:   2400,
			OpenrestyRxBytes: 4500,
			OpenrestyTxBytes: 5200,
		},
		{
			ID:               113,
			NodeID:           "node-shard-other",
			CapturedAt:       baseTime,
			CPUUsagePercent:  60,
			MemoryUsedBytes:  4,
			MemoryTotalBytes: 8,
		},
	}
	for _, item := range snapshots {
		record := item
		if err := db.Create(&record).Error; err != nil {
			t.Fatalf("seed metric snapshot %d: %v", item.ID, err)
		}
	}

	latestRows, err := ListLatestMetricSnapshotsByNodeSince(baseTime.Add(-2 * time.Hour))
	if err != nil {
		t.Fatalf("ListLatestMetricSnapshotsByNodeSince failed: %v", err)
	}
	latestByNode := make(map[string]*NodeMetricSnapshot, len(latestRows))
	for _, row := range latestRows {
		latestByNode[row.NodeID] = row
	}
	if len(latestByNode) != 2 {
		t.Fatalf("expected two latest metric rows, got %+v", latestRows)
	}
	if latestByNode["node-shard-metric"] == nil || latestByNode["node-shard-metric"].ID != 112 {
		t.Fatalf("expected global latest metric snapshot to come from shard 02, got %+v", latestByNode["node-shard-metric"])
	}

	trendBuckets, err := ListMetricSnapshotTrendBuckets("", baseTime.Add(-2*time.Hour), baseTime.Add(time.Minute), 60)
	if err != nil {
		t.Fatalf("ListMetricSnapshotTrendBuckets failed: %v", err)
	}
	currentTrendBucket := metricSnapshotTrendBucketByEpoch(trendBuckets, metricSnapshotBucketEpoch(baseTime, 60))
	if currentTrendBucket == nil {
		t.Fatalf("missing current trend bucket: %+v", trendBuckets)
	}
	if currentTrendBucket.CPUUsageSum != 100 || currentTrendBucket.CPUUsageCount != 2 || currentTrendBucket.MemoryUsageSum != 100 || currentTrendBucket.MemoryUsageCount != 2 || currentTrendBucket.ReportedNodes != 2 {
		t.Fatalf("unexpected current trend bucket: %+v", currentTrendBucket)
	}
	filteredTrendBuckets, err := ListMetricSnapshotTrendBuckets("node-shard-metric", baseTime.Add(-2*time.Hour), baseTime.Add(time.Minute), 60)
	if err != nil {
		t.Fatalf("ListMetricSnapshotTrendBuckets filtered failed: %v", err)
	}
	filteredTrendBucket := metricSnapshotTrendBucketByEpoch(filteredTrendBuckets, metricSnapshotBucketEpoch(baseTime, 60))
	if filteredTrendBucket == nil {
		t.Fatalf("missing filtered trend bucket: %+v", filteredTrendBuckets)
	}
	if filteredTrendBucket.CPUUsageSum != 40 || filteredTrendBucket.CPUUsageCount != 1 || filteredTrendBucket.MemoryUsageSum != 50 || filteredTrendBucket.MemoryUsageCount != 1 || filteredTrendBucket.ReportedNodes != 1 {
		t.Fatalf("unexpected filtered trend bucket: %+v", filteredTrendBucket)
	}

	counterBuckets, err := ListMetricSnapshotCounterDeltaBuckets("", baseTime.Add(-2*time.Hour), baseTime.Add(time.Minute), 60)
	if err != nil {
		t.Fatalf("ListMetricSnapshotCounterDeltaBuckets failed: %v", err)
	}
	currentCounterBucket := metricSnapshotCounterBucketByEpoch(counterBuckets, metricSnapshotBucketEpoch(baseTime, 60))
	if currentCounterBucket == nil {
		t.Fatalf("missing current counter bucket: %+v", counterBuckets)
	}
	if currentCounterBucket.DiskWriteBytes != 150 ||
		currentCounterBucket.NetworkRxBytes != 600 ||
		currentCounterBucket.OpenrestyTxBytes != 1200 ||
		currentCounterBucket.ReportedNodeCount != 1 ||
		currentCounterBucket.SamplesWithPrevious != 1 {
		t.Fatalf("unexpected current counter bucket: %+v", currentCounterBucket)
	}

	filteredCounterBuckets, err := ListMetricSnapshotCounterDeltaBuckets("node-shard-metric", baseTime.Add(-2*time.Hour), baseTime.Add(time.Minute), 60)
	if err != nil {
		t.Fatalf("ListMetricSnapshotCounterDeltaBuckets filtered failed: %v", err)
	}
	filteredCounterBucket := metricSnapshotCounterBucketByEpoch(filteredCounterBuckets, metricSnapshotBucketEpoch(baseTime, 60))
	if filteredCounterBucket == nil {
		t.Fatalf("missing filtered counter bucket: %+v", filteredCounterBuckets)
	}
	if filteredCounterBucket.NetworkRxBytes != 600 || filteredCounterBucket.ReportedNodeCount != 1 || filteredCounterBucket.SamplesWithPrevious != 1 {
		t.Fatalf("unexpected filtered counter bucket: %+v", filteredCounterBucket)
	}
}

func metricSnapshotTrendBucketByEpoch(buckets []*NodeMetricSnapshotTrendBucket, bucketEpoch int64) *NodeMetricSnapshotTrendBucket {
	for _, bucket := range buckets {
		if bucket != nil && bucket.BucketEpoch == bucketEpoch {
			return bucket
		}
	}
	return nil
}

func metricSnapshotCounterBucketByEpoch(buckets []*NodeMetricSnapshotCounterDeltaBucket, bucketEpoch int64) *NodeMetricSnapshotCounterDeltaBucket {
	for _, bucket := range buckets {
		if bucket != nil && bucket.BucketEpoch == bucketEpoch {
			return bucket
		}
	}
	return nil
}

func TestExistingNodeMetricSnapshotDedupKeysSQLFindsKeysAcrossShards(t *testing.T) {
	db := openBareTestSQLiteDB(t, "metric-snapshot-dedup-fast-path.db")
	if err := registerSharding(db, "sqlite"); err != nil {
		t.Fatalf("register sharding: %v", err)
	}
	if err := applyCurrentSchema(db, "sqlite"); err != nil {
		t.Fatalf("applyCurrentSchema: %v", err)
	}

	rawDB := sessionIgnoringSharding(db)
	baseTime := time.Date(2026, 6, 4, 10, 30, 0, 0, time.UTC)
	existing := []*NodeMetricSnapshot{
		{ID: 70, NodeID: "node-fast-metric", CapturedAt: baseTime, NetworkTxBytes: 100},
		{ID: 71, NodeID: "node-fast-metric-other", CapturedAt: baseTime.Add(time.Minute), NetworkTxBytes: 200},
	}
	for _, snapshot := range existing {
		table := observabilityShardTableForID("node_metric_snapshots", snapshot.ID)
		if err := rawDB.Table(table).Create(snapshot).Error; err != nil {
			t.Fatalf("seed metric snapshot %d into %s: %v", snapshot.ID, table, err)
		}
	}

	keys, err := existingNodeMetricSnapshotDedupKeysSQL(db, []string{"node-fast-metric", "node-fast-metric-other"}, nodeAccessLogTimeRange{
		min: baseTime.Add(-time.Second),
		max: baseTime.Add(2 * time.Minute),
	})
	if err != nil {
		t.Fatalf("existingNodeMetricSnapshotDedupKeysSQL failed: %v", err)
	}
	for _, snapshot := range existing {
		if _, ok := keys[nodeMetricSnapshotDedupKeyFor(snapshot)]; !ok {
			t.Fatalf("expected fast path to include metric snapshot key %+v, got %+v", nodeMetricSnapshotDedupKeyFor(snapshot), keys)
		}
	}
	if _, ok := keys[nodeMetricSnapshotDedupKey{
		nodeID:       "node-fast-metric",
		capturedAtNS: baseTime.Add(10 * time.Minute).UnixNano(),
	}]; ok {
		t.Fatalf("unexpected missing metric snapshot key in fast path result: %+v", keys)
	}
}

func TestExistingNodeRequestReportDedupKeysSQLFindsKeysAcrossShards(t *testing.T) {
	db := openBareTestSQLiteDB(t, "request-report-dedup-fast-path.db")
	if err := registerSharding(db, "sqlite"); err != nil {
		t.Fatalf("register sharding: %v", err)
	}
	if err := applyCurrentSchema(db, "sqlite"); err != nil {
		t.Fatalf("applyCurrentSchema: %v", err)
	}

	rawDB := sessionIgnoringSharding(db)
	baseTime := time.Date(2026, 6, 4, 10, 30, 0, 0, time.UTC)
	existing := []*NodeRequestReport{
		{ID: 80, NodeID: "node-fast-report", WindowStartedAt: baseTime.Add(-time.Minute), WindowEndedAt: baseTime, RequestCount: 10},
		{ID: 81, NodeID: "node-fast-report-other", WindowStartedAt: baseTime, WindowEndedAt: baseTime.Add(time.Minute), RequestCount: 20},
	}
	for _, report := range existing {
		table := observabilityShardTableForID("node_request_reports", report.ID)
		if err := rawDB.Table(table).Create(report).Error; err != nil {
			t.Fatalf("seed request report %d into %s: %v", report.ID, table, err)
		}
	}

	keys, err := existingNodeRequestReportDedupKeysSQL(db, []string{"node-fast-report", "node-fast-report-other"}, nodeAccessLogTimeRange{
		min: baseTime.Add(-time.Second),
		max: baseTime.Add(2 * time.Minute),
	})
	if err != nil {
		t.Fatalf("existingNodeRequestReportDedupKeysSQL failed: %v", err)
	}
	for _, report := range existing {
		if _, ok := keys[nodeRequestReportDedupKeyFor(report)]; !ok {
			t.Fatalf("expected fast path to include request report key %+v, got %+v", nodeRequestReportDedupKeyFor(report), keys)
		}
	}
	if _, ok := keys[nodeRequestReportDedupKey{
		nodeID:            "node-fast-report",
		windowStartedAtNS: baseTime.Add(9 * time.Minute).UnixNano(),
		windowEndedAtNS:   baseTime.Add(10 * time.Minute).UnixNano(),
	}]; ok {
		t.Fatalf("unexpected missing request report key in fast path result: %+v", keys)
	}
}

func TestInsertNewMetricSnapshotsAndRequestReportsDeduplicateBatchAndExistingRows(t *testing.T) {
	db := openBareTestSQLiteDB(t, "observability-bulk-insert.db")
	if err := registerSharding(db, "sqlite"); err != nil {
		t.Fatalf("register sharding: %v", err)
	}
	if err := applyCurrentSchema(db, "sqlite"); err != nil {
		t.Fatalf("applyCurrentSchema: %v", err)
	}

	previousDB := DB
	DB = db
	t.Cleanup(func() {
		DB = previousDB
	})

	baseTime := time.Date(2026, 6, 4, 10, 30, 0, 0, time.UTC)
	existingSnapshot := &NodeMetricSnapshot{
		ID:             70,
		NodeID:         "node-bulk-observe",
		CapturedAt:     baseTime,
		NetworkTxBytes: 100,
	}
	if err := db.Create(existingSnapshot).Error; err != nil {
		t.Fatalf("seed existing metric snapshot: %v", err)
	}
	existingReport := &NodeRequestReport{
		ID:              80,
		NodeID:          "node-bulk-observe",
		WindowStartedAt: baseTime.Add(-time.Minute),
		WindowEndedAt:   baseTime,
		RequestCount:    10,
	}
	if err := db.Create(existingReport).Error; err != nil {
		t.Fatalf("seed existing request report: %v", err)
	}

	newSnapshot := &NodeMetricSnapshot{
		NodeID:         "node-bulk-observe",
		CapturedAt:     baseTime.Add(time.Minute),
		NetworkTxBytes: 200,
	}
	otherNodeSnapshot := &NodeMetricSnapshot{
		NodeID:         "node-bulk-observe-other",
		CapturedAt:     baseTime.Add(2 * time.Minute),
		NetworkTxBytes: 300,
	}
	insertedSnapshots, err := InsertNewNodeMetricSnapshots(db, []*NodeMetricSnapshot{
		{NodeID: existingSnapshot.NodeID, CapturedAt: existingSnapshot.CapturedAt, NetworkTxBytes: existingSnapshot.NetworkTxBytes},
		newSnapshot,
		{NodeID: newSnapshot.NodeID, CapturedAt: newSnapshot.CapturedAt, NetworkTxBytes: newSnapshot.NetworkTxBytes},
		otherNodeSnapshot,
		{NodeID: otherNodeSnapshot.NodeID, CapturedAt: otherNodeSnapshot.CapturedAt, NetworkTxBytes: otherNodeSnapshot.NetworkTxBytes},
	})
	if err != nil {
		t.Fatalf("InsertNewNodeMetricSnapshots failed: %v", err)
	}
	if insertedSnapshots != 2 {
		t.Fatalf("expected two inserted metric snapshots, got %d", insertedSnapshots)
	}

	newReport := &NodeRequestReport{
		NodeID:          "node-bulk-observe",
		WindowStartedAt: baseTime,
		WindowEndedAt:   baseTime.Add(time.Minute),
		RequestCount:    20,
	}
	otherNodeReport := &NodeRequestReport{
		NodeID:          "node-bulk-observe-other",
		WindowStartedAt: baseTime.Add(time.Minute),
		WindowEndedAt:   baseTime.Add(2 * time.Minute),
		RequestCount:    30,
	}
	insertedReports, err := InsertNewNodeRequestReports(db, []*NodeRequestReport{
		{NodeID: existingReport.NodeID, WindowStartedAt: existingReport.WindowStartedAt, WindowEndedAt: existingReport.WindowEndedAt, RequestCount: existingReport.RequestCount},
		newReport,
		{NodeID: newReport.NodeID, WindowStartedAt: newReport.WindowStartedAt, WindowEndedAt: newReport.WindowEndedAt, RequestCount: newReport.RequestCount},
		otherNodeReport,
		{NodeID: otherNodeReport.NodeID, WindowStartedAt: otherNodeReport.WindowStartedAt, WindowEndedAt: otherNodeReport.WindowEndedAt, RequestCount: otherNodeReport.RequestCount},
	})
	if err != nil {
		t.Fatalf("InsertNewNodeRequestReports failed: %v", err)
	}
	if insertedReports != 2 {
		t.Fatalf("expected two inserted request reports, got %d", insertedReports)
	}

	snapshots, err := ListNodeMetricSnapshots("node-bulk-observe", time.Time{}, 10)
	if err != nil {
		t.Fatalf("ListNodeMetricSnapshots failed: %v", err)
	}
	if len(snapshots) != 2 {
		t.Fatalf("expected two metric snapshots after dedupe, got %+v", snapshots)
	}
	reports, err := ListNodeRequestReports("node-bulk-observe", time.Time{}, 10)
	if err != nil {
		t.Fatalf("ListNodeRequestReports failed: %v", err)
	}
	if len(reports) != 2 {
		t.Fatalf("expected two request reports after dedupe, got %+v", reports)
	}
}

func TestRequestReportTrendBucketsAggregateAcrossShards(t *testing.T) {
	db := openBareTestSQLiteDB(t, "request-report-trend-buckets.db")
	if err := registerSharding(db, "sqlite"); err != nil {
		t.Fatalf("register sharding: %v", err)
	}
	if err := applyCurrentSchema(db, "sqlite"); err != nil {
		t.Fatalf("applyCurrentSchema: %v", err)
	}

	previousDB := DB
	DB = db
	t.Cleanup(func() {
		DB = previousDB
	})

	baseTime := time.Date(2026, 6, 4, 11, 50, 0, 0, time.UTC)
	reports := []NodeRequestReport{
		{ID: 60, NodeID: "node-trend", WindowStartedAt: baseTime.Add(-20 * time.Minute), WindowEndedAt: baseTime.Add(-15 * time.Minute), RequestCount: 10, ErrorCount: 1, UniqueVisitorCount: 3},
		{ID: 61, NodeID: "node-trend", WindowStartedAt: baseTime.Add(-10 * time.Minute), WindowEndedAt: baseTime.Add(-5 * time.Minute), RequestCount: 20, ErrorCount: 2, UniqueVisitorCount: 5},
		{ID: 62, NodeID: "other-node", WindowStartedAt: baseTime.Add(-10 * time.Minute), WindowEndedAt: baseTime.Add(-5 * time.Minute), RequestCount: 90, ErrorCount: 9, UniqueVisitorCount: 11},
		{ID: 63, NodeID: "node-trend", WindowStartedAt: baseTime.Add(time.Hour), WindowEndedAt: baseTime.Add(time.Hour), RequestCount: 99, ErrorCount: 9, UniqueVisitorCount: 9},
	}
	for _, item := range reports {
		record := item
		if err := db.Create(&record).Error; err != nil {
			t.Fatalf("seed request report %d: %v", item.ID, err)
		}
	}

	buckets, err := ListRequestReportTrendBuckets("node-trend", baseTime.Add(-time.Hour), baseTime, 60)
	if err != nil {
		t.Fatalf("ListRequestReportTrendBuckets failed: %v", err)
	}
	if len(buckets) != 1 {
		t.Fatalf("expected one hourly bucket, got %+v", buckets)
	}
	if buckets[0].RequestCount != 30 || buckets[0].ErrorCount != 3 || buckets[0].UniqueVisitorCount != 8 {
		t.Fatalf("unexpected trend bucket totals: %+v", buckets[0])
	}

	trafficSummary, err := GetRequestReportTrafficSummary(baseTime.Add(-time.Hour), baseTime)
	if err != nil {
		t.Fatalf("GetRequestReportTrafficSummary failed: %v", err)
	}
	if trafficSummary.RequestCount != 120 || trafficSummary.ErrorCount != 12 {
		t.Fatalf("unexpected request report traffic summary: %+v", trafficSummary)
	}

	latestRows, err := ListLatestRequestReportsByNode(baseTime.Add(-time.Hour), baseTime)
	if err != nil {
		t.Fatalf("ListLatestRequestReportsByNode failed: %v", err)
	}
	latestByNode := make(map[string]*NodeRequestReport, len(latestRows))
	for _, row := range latestRows {
		latestByNode[row.NodeID] = row
	}
	if len(latestByNode) != 2 {
		t.Fatalf("expected latest report for two nodes, got %+v", latestRows)
	}
	if latestByNode["node-trend"] == nil || latestByNode["node-trend"].ID != 61 {
		t.Fatalf("expected latest node-trend report from shard 01, got %+v", latestByNode["node-trend"])
	}

	cacheSummary, err := GetRequestReportCacheSummary(baseTime.Add(-time.Hour), baseTime)
	if err != nil {
		t.Fatalf("GetRequestReportCacheSummary failed: %v", err)
	}
	if cacheSummary.CacheHitCount != 0 || cacheSummary.CacheClassifiedCount != 0 {
		t.Fatalf("expected empty cache summary for reports without cache counts, got %+v", cacheSummary)
	}

	cacheReports := []NodeRequestReport{
		{ID: 64, NodeID: "cache-node-a", WindowStartedAt: baseTime.Add(-time.Minute), WindowEndedAt: baseTime, CacheHitCount: 5, CacheMissCount: 2, CacheBypassCount: 1},
		{ID: 65, NodeID: "cache-node-b", WindowStartedAt: baseTime.Add(-time.Minute), WindowEndedAt: baseTime, CacheHitCount: 7, CacheMissCount: 3, CacheExpiredCount: 2, CacheStaleCount: 1},
		{ID: 66, NodeID: "cache-node-a", WindowStartedAt: baseTime.Add(time.Hour), WindowEndedAt: baseTime.Add(time.Hour), CacheHitCount: 99},
	}
	for _, item := range cacheReports {
		record := item
		if err := db.Create(&record).Error; err != nil {
			t.Fatalf("seed cache request report %d: %v", item.ID, err)
		}
	}
	cacheSummary, err = GetRequestReportCacheSummary(baseTime.Add(-time.Hour), baseTime)
	if err != nil {
		t.Fatalf("GetRequestReportCacheSummary after cache fixtures failed: %v", err)
	}
	if cacheSummary.CacheHitCount != 12 ||
		cacheSummary.CacheMissCount != 5 ||
		cacheSummary.CacheBypassCount != 1 ||
		cacheSummary.CacheExpiredCount != 2 ||
		cacheSummary.CacheStaleCount != 1 ||
		cacheSummary.CacheClassifiedCount != 21 {
		t.Fatalf("unexpected cache summary: %+v", cacheSummary)
	}
}

func TestIsDatabaseEmpty(t *testing.T) {
	db := openTestSQLiteDB(t, "empty.db")

	empty, err := isDatabaseEmpty(db)
	if err != nil {
		t.Fatalf("isDatabaseEmpty returned error: %v", err)
	}
	if !empty {
		t.Fatal("expected database to be empty")
	}

	if err := db.Create(&User{
		Username:    "alice",
		Password:    "secret",
		DisplayName: "Alice",
		Role:        1,
		Status:      1,
	}).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}

	empty, err = isDatabaseEmpty(db)
	if err != nil {
		t.Fatalf("isDatabaseEmpty after seed returned error: %v", err)
	}
	if empty {
		t.Fatal("expected database to be non-empty")
	}
}

func TestResetUserPasswordByUsername(t *testing.T) {
	db := openTestSQLiteDB(t, "reset-password.db")
	previousDB := DB
	DB = db
	t.Cleanup(func() {
		DB = previousDB
	})

	root := &User{
		Username:    "root",
		Password:    "old-password",
		DisplayName: "Root User",
		Role:        100,
		Status:      1,
	}
	if err := root.Insert(); err != nil {
		t.Fatalf("insert root user: %v", err)
	}

	if err := ResetUserPasswordByUsername("root", "new-password"); err != nil {
		t.Fatalf("reset root password: %v", err)
	}

	user := &User{Username: "root", Password: "new-password"}
	if err := user.ValidateAndFill(); err != nil {
		t.Fatalf("expected new password to validate: %v", err)
	}

	oldUser := &User{Username: "root", Password: "old-password"}
	if err := oldUser.ValidateAndFill(); err == nil {
		t.Fatal("expected old password to be rejected")
	}

	if err := ResetUserPasswordByUsername("missing", "new-password"); err == nil {
		t.Fatal("expected missing user reset to fail")
	}
}

func TestUserFillMethodsReturnRecordNotFound(t *testing.T) {
	db := openTestSQLiteDB(t, "user-fill-missing.db")
	previousDB := DB
	DB = db
	t.Cleanup(func() {
		DB = previousDB
	})

	cases := []struct {
		name string
		user User
		fill func(*User) error
	}{
		{
			name: "id",
			user: User{Id: 404},
			fill: func(user *User) error { return user.FillUserById() },
		},
		{
			name: "email",
			user: User{Email: "missing@example.com"},
			fill: func(user *User) error { return user.FillUserByEmail() },
		},
		{
			name: "github",
			user: User{GitHubId: "missing-github"},
			fill: func(user *User) error { return user.FillUserByGitHubId() },
		},
		{
			name: "wechat",
			user: User{WeChatId: "missing-wechat"},
			fill: func(user *User) error { return user.FillUserByWeChatId() },
		},
		{
			name: "username",
			user: User{Username: "missing"},
			fill: func(user *User) error { return user.FillUserByUsername() },
		},
	}

	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			user := item.user
			if err := item.fill(&user); err == nil {
				t.Fatal("expected missing user fill to return an error")
			}
		})
	}
}

func TestResetRootPasswordCreatesAndEnablesRoot(t *testing.T) {
	db := openTestSQLiteDB(t, "reset-root.db")
	previousDB := DB
	DB = db
	t.Cleanup(func() {
		DB = previousDB
	})

	if err := ResetRootPassword("new-password"); err != nil {
		t.Fatalf("create root with reset password: %v", err)
	}

	user := &User{Username: "root", Password: "new-password"}
	if err := user.ValidateAndFill(); err != nil {
		t.Fatalf("expected created root to validate: %v", err)
	}
	if user.Role != 100 || user.Status != 1 {
		t.Fatalf("expected enabled root role, got role=%d status=%d", user.Role, user.Status)
	}

	user.Status = 2
	user.Role = 1
	if err := user.Update(false); err != nil {
		t.Fatalf("disable/demote root for reset test: %v", err)
	}
	if err := ResetRootPassword("another-password"); err != nil {
		t.Fatalf("reset existing root: %v", err)
	}
	resetUser := &User{Username: "root", Password: "another-password"}
	if err := resetUser.ValidateAndFill(); err != nil {
		t.Fatalf("expected reset root to validate: %v", err)
	}
	if resetUser.Role != 100 || resetUser.Status != 1 {
		t.Fatalf("expected reset root to be enabled root role, got role=%d status=%d", resetUser.Role, resetUser.Status)
	}
}

func TestCreateRootAccountUsesConfiguredInitialPassword(t *testing.T) {
	db := openTestSQLiteDB(t, "initial-root-password.db")
	previousDB := DB
	DB = db
	previousInitialPassword := common.InitialRootPassword
	common.InitialRootPassword = "configured-root-password"
	t.Cleanup(func() {
		DB = previousDB
		common.InitialRootPassword = previousInitialPassword
	})

	if err := createRootAccountIfNeed(); err != nil {
		t.Fatalf("create root account: %v", err)
	}

	user := &User{Username: "root", Password: "configured-root-password"}
	if err := user.ValidateAndFill(); err != nil {
		t.Fatalf("expected configured initial root password to validate: %v", err)
	}
	defaultPasswordUser := &User{Username: "root", Password: "123456"}
	if err := defaultPasswordUser.ValidateAndFill(); err == nil {
		t.Fatal("fixed default root password should not validate")
	}
}

func TestMigrateTableDataCopiesRows(t *testing.T) {
	source := openTestSQLiteDB(t, "source.db")
	target := openTestSQLiteDB(t, "target.db")

	user := User{
		Id:          1,
		Username:    "root",
		Password:    "hashed",
		DisplayName: "Root User",
		Role:        100,
		Status:      1,
	}
	option := Option{
		Key:   "AgentHeartbeatInterval",
		Value: "10000",
	}

	if err := source.Create(&user).Error; err != nil {
		t.Fatalf("seed source user: %v", err)
	}
	if err := source.Create(&option).Error; err != nil {
		t.Fatalf("seed source option: %v", err)
	}

	if err := migrateTableData(source, target, findDBModelByTableName(t, "users")); err != nil {
		t.Fatalf("migrate users: %v", err)
	}
	if err := migrateTableData(source, target, findDBModelByTableName(t, "options")); err != nil {
		t.Fatalf("migrate options: %v", err)
	}

	var gotUser User
	if err := target.First(&gotUser, 1).Error; err != nil {
		t.Fatalf("query migrated user: %v", err)
	}
	if gotUser.Username != user.Username || gotUser.DisplayName != user.DisplayName {
		t.Fatalf("unexpected migrated user: %+v", gotUser)
	}

	var gotOption Option
	if err := target.First(&gotOption, "key = ?", option.Key).Error; err != nil {
		t.Fatalf("query migrated option: %v", err)
	}
	if gotOption.Value != option.Value {
		t.Fatalf("unexpected migrated option value: %s", gotOption.Value)
	}
}

func TestRegisterShardingAutoMigratesShardTables(t *testing.T) {
	db := openBareTestSQLiteDB(t, "sharded.db")
	if err := registerSharding(db, "sqlite"); err != nil {
		t.Fatalf("register sharding: %v", err)
	}
	if err := autoMigrateAll(db); err != nil {
		t.Fatalf("auto migrate db: %v", err)
	}

	for _, table := range []string{
		"node_metric_snapshots_00",
		"node_metric_snapshots_09",
		"node_request_reports_00",
		"node_request_reports_09",
		"node_access_logs_00",
		"node_access_logs_09",
	} {
		if !db.Migrator().HasTable(table) {
			t.Fatalf("expected sharded table %s to exist", table)
		}
	}
}

func TestApplyCurrentSchemaCreatesObservabilityShardQueryIndexes(t *testing.T) {
	db := openBareTestSQLiteDB(t, "observability-query-indexes.db")
	if err := registerSharding(db, "sqlite"); err != nil {
		t.Fatalf("register sharding: %v", err)
	}
	if err := applyCurrentSchema(db, "sqlite"); err != nil {
		t.Fatalf("applyCurrentSchema: %v", err)
	}

	expectSQLiteIndexWithColumns(t, db, "node_access_logs_00", []string{"remote_addr", "logged_at"})
	expectSQLiteIndexWithColumns(t, db, "node_access_logs_00", []string{"host", "logged_at"})
	expectSQLiteIndexWithColumns(t, db, "node_access_logs_09", []string{"remote_addr", "logged_at"})
	expectSQLiteIndexWithColumns(t, db, "node_access_logs_09", []string{"host", "logged_at"})
	expectSQLiteIndexWithColumns(t, db, "node_metric_snapshots_00", []string{"node_id", "captured_at"})
	expectSQLiteIndexWithColumns(t, db, "node_request_reports_00", []string{"node_id", "window_ended_at"})
}

func expectSQLiteIndexWithColumns(t *testing.T, db *gorm.DB, table string, columns []string) {
	t.Helper()
	type indexListRow struct {
		Name string
	}
	type indexInfoRow struct {
		Seqno int
		Name  string
	}
	var indexes []indexListRow
	if err := db.Raw(fmt.Sprintf(`PRAGMA index_list("%s")`, table)).Scan(&indexes).Error; err != nil {
		t.Fatalf("list indexes for %s: %v", table, err)
	}
	for _, index := range indexes {
		var info []indexInfoRow
		if err := db.Raw(fmt.Sprintf(`PRAGMA index_info("%s")`, index.Name)).Scan(&info).Error; err != nil {
			t.Fatalf("read index %s info: %v", index.Name, err)
		}
		sort.Slice(info, func(i int, j int) bool {
			return info[i].Seqno < info[j].Seqno
		})
		if len(info) != len(columns) {
			continue
		}
		matched := true
		for idx, column := range columns {
			if info[idx].Name != column {
				matched = false
				break
			}
		}
		if matched {
			return
		}
	}
	t.Fatalf("expected %s to have index on columns %v", table, columns)
}

func TestMigrateObservabilityLegacyColumnsBackfillsHealthEventMetadata(t *testing.T) {
	db := openTestSQLiteDB(t, "legacy-health-events.db")

	if err := db.Exec("ALTER TABLE node_health_events ADD COLUMN raw_json TEXT").Error; err != nil {
		t.Fatalf("add raw_json column: %v", err)
	}
	rawJSON, err := json.Marshal(map[string]any{
		"event_type": "sync_error",
		"metadata": map[string]string{
			"reason": "checksum_mismatch",
			"scope":  "routes",
		},
	})
	if err != nil {
		t.Fatalf("marshal raw json: %v", err)
	}
	event := &NodeHealthEvent{
		NodeID:           "node-legacy",
		EventType:        "sync_error",
		Severity:         "warning",
		Status:           "active",
		Message:          "checksum mismatch",
		FirstTriggeredAt: time.Now().Add(-time.Minute),
		LastTriggeredAt:  time.Now(),
		ReportedAt:       time.Now(),
	}
	if err := db.Create(event).Error; err != nil {
		t.Fatalf("create health event: %v", err)
	}
	if err := db.Exec("UPDATE node_health_events SET raw_json = ? WHERE id = ?", string(rawJSON), event.ID).Error; err != nil {
		t.Fatalf("seed legacy raw_json: %v", err)
	}

	if err := migrateObservabilityLegacyColumns(db); err != nil {
		t.Fatalf("migrateObservabilityLegacyColumns: %v", err)
	}

	var got NodeHealthEvent
	if err := db.First(&got, event.ID).Error; err != nil {
		t.Fatalf("query health event: %v", err)
	}
	if got.MetadataJSON == "" {
		t.Fatal("expected metadata_json to be backfilled")
	}
}

func TestEnsureDatabaseSchemaUpToDateInitializesFreshDatabase(t *testing.T) {
	db := openBareTestSQLiteDB(t, "fresh-schema.db")
	if err := registerSharding(db, "sqlite"); err != nil {
		t.Fatalf("register sharding: %v", err)
	}

	if err := ensureDatabaseSchemaUpToDate(db, "sqlite"); err != nil {
		t.Fatalf("ensureDatabaseSchemaUpToDate: %v", err)
	}

	version, exists, err := loadDatabaseSchemaVersion(db)
	if err != nil {
		t.Fatalf("loadDatabaseSchemaVersion: %v", err)
	}
	if !exists {
		t.Fatal("expected database schema version to be recorded")
	}
	if version != currentDatabaseSchemaVersion {
		t.Fatalf("unexpected schema version: got %d want %d", version, currentDatabaseSchemaVersion)
	}
}

func TestEnsureDatabaseSchemaUpToDateUpgradesLegacyDatabase(t *testing.T) {
	db := openBareTestSQLiteDB(t, "legacy-schema.db")
	if err := registerSharding(db, "sqlite"); err != nil {
		t.Fatalf("register sharding: %v", err)
	}
	if err := autoMigrateAll(db); err != nil {
		t.Fatalf("auto migrate db: %v", err)
	}
	if err := db.Create(&User{
		Username:    "legacy",
		Password:    "secret",
		DisplayName: "Legacy User",
		Role:        1,
		Status:      1,
	}).Error; err != nil {
		t.Fatalf("seed legacy user: %v", err)
	}

	if err := ensureDatabaseSchemaUpToDate(db, "sqlite"); err != nil {
		t.Fatalf("ensureDatabaseSchemaUpToDate: %v", err)
	}

	version, exists, err := loadDatabaseSchemaVersion(db)
	if err != nil {
		t.Fatalf("loadDatabaseSchemaVersion: %v", err)
	}
	if !exists {
		t.Fatal("expected legacy database to gain a schema version record")
	}
	if version != currentDatabaseSchemaVersion {
		t.Fatalf("unexpected schema version: got %d want %d", version, currentDatabaseSchemaVersion)
	}
}

func TestEnsureDatabaseSchemaUpToDateMigratesObservabilityShardsToID(t *testing.T) {
	db := openBareTestSQLiteDB(t, "legacy-observability-shards.db")
	if err := registerSharding(db, "sqlite"); err != nil {
		t.Fatalf("register sharding: %v", err)
	}
	if err := autoMigrateAll(db); err != nil {
		t.Fatalf("auto migrate db: %v", err)
	}
	if err := autoMigrateSchemaMetadata(db); err != nil {
		t.Fatalf("auto migrate schema metadata: %v", err)
	}

	now := time.Now().UTC()
	if err := db.Table("node_metric_snapshots_00").Create(&NodeMetricSnapshot{
		ID:               1,
		NodeID:           "node-a",
		CapturedAt:       now.Add(-2 * time.Minute),
		CPUUsagePercent:  22,
		MemoryUsedBytes:  2,
		MemoryTotalBytes: 8,
	}).Error; err != nil {
		t.Fatalf("seed metric snapshot shard 00: %v", err)
	}
	if err := db.Table("node_metric_snapshots_01").Create(&NodeMetricSnapshot{
		ID:               1,
		NodeID:           "node-b",
		CapturedAt:       now.Add(-time.Minute),
		CPUUsagePercent:  44,
		MemoryUsedBytes:  4,
		MemoryTotalBytes: 8,
	}).Error; err != nil {
		t.Fatalf("seed metric snapshot shard 01: %v", err)
	}
	if err := db.Table("node_request_reports_00").Create(&NodeRequestReport{
		ID:                 1,
		NodeID:             "node-a",
		WindowStartedAt:    now.Add(-3 * time.Minute),
		WindowEndedAt:      now.Add(-2 * time.Minute),
		RequestCount:       12,
		ErrorCount:         1,
		UniqueVisitorCount: 6,
	}).Error; err != nil {
		t.Fatalf("seed request report shard 00: %v", err)
	}
	if err := db.Table("node_request_reports_01").Create(&NodeRequestReport{
		ID:                 1,
		NodeID:             "node-b",
		WindowStartedAt:    now.Add(-2 * time.Minute),
		WindowEndedAt:      now.Add(-time.Minute),
		RequestCount:       21,
		ErrorCount:         2,
		UniqueVisitorCount: 9,
	}).Error; err != nil {
		t.Fatalf("seed request report shard 01: %v", err)
	}
	if err := db.Table("node_access_logs_00").Create(&NodeAccessLog{
		ID:         1,
		NodeID:     "node-a",
		LoggedAt:   now.Add(-90 * time.Second),
		RemoteAddr: "203.0.113.10",
		Host:       "a.example.com",
		Path:       "/alpha",
		StatusCode: 200,
	}).Error; err != nil {
		t.Fatalf("seed access log shard 00: %v", err)
	}
	if err := db.Table("node_access_logs_01").Create(&NodeAccessLog{
		ID:         1,
		NodeID:     "node-b",
		LoggedAt:   now.Add(-60 * time.Second),
		RemoteAddr: "203.0.113.11",
		Host:       "b.example.com",
		Path:       "/beta",
		StatusCode: 502,
	}).Error; err != nil {
		t.Fatalf("seed access log shard 01: %v", err)
	}
	if err := saveDatabaseSchemaVersion(db, 2); err != nil {
		t.Fatalf("save schema version: %v", err)
	}

	previousDB := DB
	DB = db
	t.Cleanup(func() {
		DB = previousDB
	})

	if err := ensureDatabaseSchemaUpToDate(db, "sqlite"); err != nil {
		t.Fatalf("ensureDatabaseSchemaUpToDate: %v", err)
	}

	version, exists, err := loadDatabaseSchemaVersion(db)
	if err != nil {
		t.Fatalf("loadDatabaseSchemaVersion: %v", err)
	}
	if !exists {
		t.Fatal("expected migrated database to keep schema version record")
	}
	if version != currentDatabaseSchemaVersion {
		t.Fatalf("unexpected schema version: got %d want %d", version, currentDatabaseSchemaVersion)
	}

	for _, baseTable := range shardedObservabilityBaseTables() {
		for _, table := range observabilityShardTables(baseTable) {
			legacyTable := legacyObservabilityShardTableName(table)
			if db.Migrator().HasTable(legacyTable) {
				t.Fatalf("expected legacy shard table %s to be removed", legacyTable)
			}
		}
	}

	snapshots, err := ListMetricSnapshotsSince(time.Time{})
	if err != nil {
		t.Fatalf("ListMetricSnapshotsSince failed: %v", err)
	}
	if len(snapshots) != 2 {
		t.Fatalf("expected 2 migrated metric snapshots, got %+v", snapshots)
	}
	reports, err := ListRequestReportsSince(time.Time{})
	if err != nil {
		t.Fatalf("ListRequestReportsSince failed: %v", err)
	}
	if len(reports) != 2 {
		t.Fatalf("expected 2 migrated request reports, got %+v", reports)
	}
	logs, err := ListNodeAccessLogs(NodeAccessLogQuery{Page: 0, PageSize: 10})
	if err != nil {
		t.Fatalf("ListNodeAccessLogs failed: %v", err)
	}
	if len(logs) != 2 {
		t.Fatalf("expected 2 migrated access logs, got %+v", logs)
	}

	seenSnapshotIDs := make(map[uint]struct{}, len(snapshots))
	for _, item := range snapshots {
		if item == nil || item.ID == 0 {
			t.Fatalf("expected migrated metric snapshot to have a new non-zero id: %+v", item)
		}
		if _, exists := seenSnapshotIDs[item.ID]; exists {
			t.Fatalf("expected migrated metric snapshot ids to be unique, got duplicate %d", item.ID)
		}
		seenSnapshotIDs[item.ID] = struct{}{}
		targetTable := observabilityShardTableForID("node_metric_snapshots", item.ID)
		var count int64
		if err := db.Table(targetTable).Where("id = ?", item.ID).Count(&count).Error; err != nil {
			t.Fatalf("count migrated metric snapshot in target shard: %v", err)
		}
		if count != 1 {
			t.Fatalf("expected migrated metric snapshot id %d to be stored in %s", item.ID, targetTable)
		}
	}
}

func TestMigrateOriginsSchemaBackfillsOrigins(t *testing.T) {
	db := openBareTestSQLiteDB(t, "legacy-origins.db")
	if err := registerSharding(db, "sqlite"); err != nil {
		t.Fatalf("register sharding: %v", err)
	}
	if err := applyCurrentSchema(db, "sqlite"); err != nil {
		t.Fatalf("applyCurrentSchema: %v", err)
	}
	now := time.Now().UTC()
	route := &ProxyRoute{
		Domain:    "app.example.com",
		OriginURL: "https://origin-a.internal:8443/api",
		Upstreams: `["https://origin-a.internal:8443/api"]`,
		Enabled:   true,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := db.Create(route).Error; err != nil {
		t.Fatalf("seed proxy route: %v", err)
	}
	if err := db.Exec(`DELETE FROM origins`).Error; err != nil {
		t.Fatalf("clear origins: %v", err)
	}
	if err := db.Model(&ProxyRoute{}).Where("id = ?", route.ID).Update("origin_id", nil).Error; err != nil {
		t.Fatalf("clear route origin_id: %v", err)
	}

	if err := backfillOriginsFromProxyRoutes(db); err != nil {
		t.Fatalf("backfillOriginsFromProxyRoutes: %v", err)
	}

	if !db.Migrator().HasTable(&Origin{}) {
		t.Fatal("expected origins table to exist")
	}
	if !db.Migrator().HasColumn(&ProxyRoute{}, "origin_id") {
		t.Fatal("expected proxy_routes.origin_id column to exist")
	}

	reloadedRoute := &ProxyRoute{}
	if err := db.First(reloadedRoute, route.ID).Error; err != nil {
		t.Fatalf("query proxy route: %v", err)
	}
	if reloadedRoute.OriginID == nil || *reloadedRoute.OriginID == 0 {
		t.Fatal("expected migrated route to be linked to a backfilled origin")
	}

	origin := &Origin{}
	if err := db.First(origin, *reloadedRoute.OriginID).Error; err != nil {
		t.Fatalf("query origin: %v", err)
	}
	if origin.Address != "origin-a.internal" {
		t.Fatalf("unexpected backfilled origin address: %s", origin.Address)
	}
}

func TestEnsureDatabaseSchemaUpToDateBackfillsProxyRouteSiteFields(t *testing.T) {
	db := openBareTestSQLiteDB(t, "legacy-proxy-route-sites.db")
	if err := registerSharding(db, "sqlite"); err != nil {
		t.Fatalf("register sharding: %v", err)
	}
	if err := autoMigrateSchemaMetadata(db); err != nil {
		t.Fatalf("auto migrate schema metadata: %v", err)
	}

	for _, item := range registeredModels() {
		if _, ok := item.(*ProxyRoute); ok {
			continue
		}
		if err := db.AutoMigrate(item); err != nil {
			t.Fatalf("auto migrate supporting table: %v", err)
		}
	}
	if err := db.AutoMigrate(&legacyProxyRouteV4{}); err != nil {
		t.Fatalf("auto migrate legacy proxy_routes: %v", err)
	}

	now := time.Now().UTC()
	if err := db.Create(&legacyProxyRouteV4{
		Domain:        "app.example.com",
		OriginURL:     "https://origin-a.internal:8443",
		Upstreams:     `["https://origin-a.internal:8443","https://origin-b.internal:8443"]`,
		Enabled:       true,
		EnableHTTPS:   false,
		RedirectHTTP:  false,
		CacheEnabled:  false,
		CachePolicy:   "",
		CacheRules:    `[]`,
		CustomHeaders: `[]`,
		CreatedAt:     now,
		UpdatedAt:     now,
	}).Error; err != nil {
		t.Fatalf("seed legacy proxy route: %v", err)
	}
	if err := saveDatabaseSchemaVersion(db, 4); err != nil {
		t.Fatalf("save schema version: %v", err)
	}

	previousDB := DB
	DB = db
	t.Cleanup(func() {
		DB = previousDB
	})

	if err := ensureDatabaseSchemaUpToDate(db, "sqlite"); err != nil {
		t.Fatalf("ensureDatabaseSchemaUpToDate: %v", err)
	}

	var route ProxyRoute
	if err := db.First(&route).Error; err != nil {
		t.Fatalf("query migrated proxy route: %v", err)
	}
	if route.SiteName != "app.example.com" {
		t.Fatalf("unexpected site_name after migration: %s", route.SiteName)
	}
	if route.Domain != "app.example.com" {
		t.Fatalf("unexpected domain mirror after migration: %s", route.Domain)
	}

	var domains []string
	if err := json.Unmarshal([]byte(route.Domains), &domains); err != nil {
		t.Fatalf("decode migrated domains: %v", err)
	}
	if len(domains) != 1 || domains[0] != "app.example.com" {
		t.Fatalf("unexpected migrated domains: %#v", domains)
	}
}

func TestEnsureDatabaseSchemaUpToDateAddsProxyRouteRateLimitFields(t *testing.T) {
	db := openBareTestSQLiteDB(t, "legacy-proxy-route-rate-limits.db")
	if err := registerSharding(db, "sqlite"); err != nil {
		t.Fatalf("register sharding: %v", err)
	}
	if err := autoMigrateSchemaMetadata(db); err != nil {
		t.Fatalf("auto migrate schema metadata: %v", err)
	}

	for _, item := range registeredModels() {
		if _, ok := item.(*ProxyRoute); ok {
			continue
		}
		if err := db.AutoMigrate(item); err != nil {
			t.Fatalf("auto migrate supporting table: %v", err)
		}
	}
	if err := db.AutoMigrate(&legacyProxyRouteV5{}); err != nil {
		t.Fatalf("auto migrate legacy proxy_routes v5: %v", err)
	}

	now := time.Now().UTC()
	if err := db.Create(&legacyProxyRouteV5{
		SiteName:      "main-site",
		Domain:        "app.example.com",
		Domains:       `["app.example.com","www.example.com"]`,
		OriginURL:     "https://origin-a.internal:8443",
		Upstreams:     `["https://origin-a.internal:8443"]`,
		Enabled:       true,
		EnableHTTPS:   false,
		RedirectHTTP:  false,
		CacheEnabled:  false,
		CachePolicy:   "",
		CacheRules:    `[]`,
		CustomHeaders: `[]`,
		CreatedAt:     now,
		UpdatedAt:     now,
	}).Error; err != nil {
		t.Fatalf("seed legacy proxy route v5: %v", err)
	}
	if err := saveDatabaseSchemaVersion(db, 5); err != nil {
		t.Fatalf("save schema version: %v", err)
	}

	previousDB := DB
	DB = db
	t.Cleanup(func() {
		DB = previousDB
	})

	if err := ensureDatabaseSchemaUpToDate(db, "sqlite"); err != nil {
		t.Fatalf("ensureDatabaseSchemaUpToDate: %v", err)
	}

	var route ProxyRoute
	if err := db.First(&route).Error; err != nil {
		t.Fatalf("query migrated proxy route: %v", err)
	}
	if route.LimitConnPerServer != 0 || route.LimitConnPerIP != 0 || route.LimitRate != "" {
		t.Fatalf("expected new rate limit fields to default to disabled values, got %+v", route)
	}
}

func TestEnsureDatabaseSchemaUpToDateAddsProxyRouteCertificateListFields(t *testing.T) {
	db := openBareTestSQLiteDB(t, "legacy-proxy-route-cert-ids.db")
	if err := registerSharding(db, "sqlite"); err != nil {
		t.Fatalf("register sharding: %v", err)
	}
	if err := autoMigrateSchemaMetadata(db); err != nil {
		t.Fatalf("auto migrate schema metadata: %v", err)
	}

	for _, item := range registeredModels() {
		if _, ok := item.(*ProxyRoute); ok {
			continue
		}
		if err := db.AutoMigrate(item); err != nil {
			t.Fatalf("auto migrate supporting table: %v", err)
		}
	}
	if err := db.AutoMigrate(&legacyProxyRouteV6{}); err != nil {
		t.Fatalf("auto migrate legacy proxy_routes v6: %v", err)
	}

	now := time.Now().UTC()
	certID := uint(9)
	if err := db.Create(&legacyProxyRouteV6{
		SiteName:           "secure-site",
		Domain:             "secure.example.com",
		Domains:            `["secure.example.com","www.secure.example.com"]`,
		OriginURL:          "https://origin-secure.internal:8443",
		Upstreams:          `["https://origin-secure.internal:8443"]`,
		Enabled:            true,
		EnableHTTPS:        true,
		CertID:             &certID,
		RedirectHTTP:       true,
		LimitConnPerServer: 120,
		LimitConnPerIP:     12,
		LimitRate:          "512k",
		CacheEnabled:       false,
		CachePolicy:        "",
		CacheRules:         `[]`,
		CustomHeaders:      `[]`,
		CreatedAt:          now,
		UpdatedAt:          now,
	}).Error; err != nil {
		t.Fatalf("seed legacy proxy route v6: %v", err)
	}
	if err := saveDatabaseSchemaVersion(db, 6); err != nil {
		t.Fatalf("save schema version: %v", err)
	}

	previousDB := DB
	DB = db
	t.Cleanup(func() {
		DB = previousDB
	})

	if err := ensureDatabaseSchemaUpToDate(db, "sqlite"); err != nil {
		t.Fatalf("ensureDatabaseSchemaUpToDate: %v", err)
	}

	var route ProxyRoute
	if err := db.First(&route).Error; err != nil {
		t.Fatalf("query migrated proxy route: %v", err)
	}
	if route.CertID == nil || *route.CertID != certID {
		t.Fatalf("expected cert_id mirror to be preserved, got %+v", route.CertID)
	}

	var certIDs []uint
	if err := json.Unmarshal([]byte(route.CertIDs), &certIDs); err != nil {
		t.Fatalf("decode migrated cert_ids: %v", err)
	}
	if len(certIDs) != 1 || certIDs[0] != certID {
		t.Fatalf("unexpected migrated cert_ids: %#v", certIDs)
	}
}

func TestEnsureDatabaseSchemaUpToDateAddsProxyRouteDomainCertificateFields(t *testing.T) {
	db := openBareTestSQLiteDB(t, "legacy-proxy-route-domain-cert-ids.db")
	if err := registerSharding(db, "sqlite"); err != nil {
		t.Fatalf("register sharding: %v", err)
	}
	if err := autoMigrateSchemaMetadata(db); err != nil {
		t.Fatalf("auto migrate schema metadata: %v", err)
	}

	for _, item := range registeredModels() {
		if _, ok := item.(*ProxyRoute); ok {
			continue
		}
		if err := db.AutoMigrate(item); err != nil {
			t.Fatalf("auto migrate supporting table: %v", err)
		}
	}
	if err := db.AutoMigrate(&legacyProxyRouteV7{}); err != nil {
		t.Fatalf("auto migrate legacy proxy_routes v7: %v", err)
	}

	now := time.Now().UTC()
	certID := uint(9)
	if err := db.Create(&legacyProxyRouteV7{
		SiteName:           "secure-site",
		Domain:             "secure.example.com",
		Domains:            `["secure.example.com","www.secure.example.com"]`,
		OriginURL:          "https://origin-secure.internal:8443",
		Upstreams:          `["https://origin-secure.internal:8443"]`,
		Enabled:            true,
		EnableHTTPS:        true,
		CertID:             &certID,
		CertIDs:            `[9]`,
		RedirectHTTP:       true,
		LimitConnPerServer: 120,
		LimitConnPerIP:     12,
		LimitRate:          "512k",
		CacheEnabled:       false,
		CachePolicy:        "",
		CacheRules:         `[]`,
		CustomHeaders:      `[]`,
		CreatedAt:          now,
		UpdatedAt:          now,
	}).Error; err != nil {
		t.Fatalf("seed legacy proxy route v7: %v", err)
	}
	if err := saveDatabaseSchemaVersion(db, 7); err != nil {
		t.Fatalf("save schema version: %v", err)
	}

	previousDB := DB
	DB = db
	t.Cleanup(func() {
		DB = previousDB
	})

	if err := ensureDatabaseSchemaUpToDate(db, "sqlite"); err != nil {
		t.Fatalf("ensureDatabaseSchemaUpToDate: %v", err)
	}

	var route ProxyRoute
	if err := db.First(&route).Error; err != nil {
		t.Fatalf("query migrated proxy route: %v", err)
	}

	var domainCertIDs []uint
	if err := json.Unmarshal([]byte(route.DomainCertIDs), &domainCertIDs); err != nil {
		t.Fatalf("decode migrated domain_cert_ids: %v", err)
	}
	if len(domainCertIDs) != 2 || domainCertIDs[0] != certID || domainCertIDs[1] != certID {
		t.Fatalf("unexpected migrated domain_cert_ids: %#v", domainCertIDs)
	}
}

func TestEnsureDatabaseSchemaUpToDateAddsAccessLogByteFields(t *testing.T) {
	db := openBareTestSQLiteDB(t, "access-log-bytes.db")
	if err := registerSharding(db, "sqlite"); err != nil {
		t.Fatalf("register sharding: %v", err)
	}
	if err := db.AutoMigrate(
		&DatabaseSchemaVersion{},
		&File{},
		&User{},
		&AuthSource{},
		&ExternalAccount{},
		&Option{},
		&Origin{},
		&ProxyRoute{},
		&ConfigVersion{},
		&Node{},
		&NodeSystemProfile{},
		&ApplyLog{},
		&NodeMetricSnapshot{},
		&NodeRequestReport{},
		&legacyNodeAccessLogV16{},
		&NodeHealthEvent{},
		&TLSCertificate{},
		&ManagedDomain{},
		&AcmeAccount{},
		&DnsAccount{},
	); err != nil {
		t.Fatalf("AutoMigrate legacy schema: %v", err)
	}
	if err := saveDatabaseSchemaVersion(db, 16); err != nil {
		t.Fatalf("save schema version: %v", err)
	}

	if err := ensureDatabaseSchemaUpToDate(db, "sqlite"); err != nil {
		t.Fatalf("ensureDatabaseSchemaUpToDate: %v", err)
	}

	expectAccessLogCurrentColumns(t, db)
	version, exists, err := loadDatabaseSchemaVersion(db)
	if err != nil {
		t.Fatalf("loadDatabaseSchemaVersion: %v", err)
	}
	if !exists || version != currentDatabaseSchemaVersion {
		t.Fatalf("unexpected schema version: exists=%v version=%d", exists, version)
	}
}

func TestEnsureDatabaseSchemaUpToDateRepairsCurrentAccessLogShardColumns(t *testing.T) {
	db := openBareTestSQLiteDB(t, "current-access-log-columns.db")
	if err := registerSharding(db, "sqlite"); err != nil {
		t.Fatalf("register sharding: %v", err)
	}
	if err := applyCurrentSchema(db, "sqlite"); err != nil {
		t.Fatalf("applyCurrentSchema: %v", err)
	}
	if err := saveDatabaseSchemaVersion(db, currentDatabaseSchemaVersion); err != nil {
		t.Fatalf("save schema version: %v", err)
	}

	rawDB := sessionIgnoringSharding(db)
	targetTable := "node_access_logs_00"
	if err := rawDB.Table(targetTable).Migrator().DropColumn(&NodeAccessLog{}, "Operator"); err != nil {
		t.Fatalf("drop operator column: %v", err)
	}
	if rawDB.Migrator().HasColumn(targetTable, "operator") {
		t.Fatalf("expected %s.operator to be missing before repair", targetTable)
	}

	if err := ensureDatabaseSchemaUpToDate(db, "sqlite"); err != nil {
		t.Fatalf("ensureDatabaseSchemaUpToDate: %v", err)
	}

	expectAccessLogCurrentColumns(t, db)
	version, exists, err := loadDatabaseSchemaVersion(db)
	if err != nil {
		t.Fatalf("loadDatabaseSchemaVersion: %v", err)
	}
	if !exists || version != currentDatabaseSchemaVersion {
		t.Fatalf("unexpected schema version: exists=%v version=%d", exists, version)
	}
}

func TestEnsureDatabaseSchemaUpToDateAddsCommercialLicenseTable(t *testing.T) {
	db := openBareTestSQLiteDB(t, "license-schema.db")
	if err := registerSharding(db, "sqlite"); err != nil {
		t.Fatalf("register sharding: %v", err)
	}
	if err := db.AutoMigrate(
		&DatabaseSchemaVersion{},
		&Option{},
		&Node{},
		&ProxyRoute{},
	); err != nil {
		t.Fatalf("auto migrate legacy db: %v", err)
	}
	if err := saveDatabaseSchemaVersion(db, 29); err != nil {
		t.Fatalf("save schema version: %v", err)
	}

	if err := ensureDatabaseSchemaUpToDate(db, "sqlite"); err != nil {
		t.Fatalf("ensureDatabaseSchemaUpToDate: %v", err)
	}

	if !db.Migrator().HasTable(&CommercialLicense{}) {
		t.Fatal("expected commercial_licenses table to exist")
	}
	if !db.Migrator().HasColumn(&CommercialLicense{}, "token_hash") {
		t.Fatal("expected commercial_licenses.token_hash column to exist")
	}
	if !db.Migrator().HasTable(&CommercialLicenseActivation{}) {
		t.Fatal("expected commercial_license_activations table to exist")
	}
	version, exists, err := loadDatabaseSchemaVersion(db)
	if err != nil {
		t.Fatalf("loadDatabaseSchemaVersion: %v", err)
	}
	if !exists || version != currentDatabaseSchemaVersion {
		t.Fatalf("unexpected schema version: exists=%v version=%d", exists, version)
	}
}

func expectAccessLogCurrentColumns(t *testing.T, db *gorm.DB) {
	t.Helper()
	for _, table := range observabilityShardTables("node_access_logs") {
		for _, column := range []string{"request_bytes", "response_bytes", "upstream_bytes", "reason", "operator", "cache_status"} {
			if !db.Migrator().HasColumn(table, column) {
				t.Fatalf("expected column %s.%s to exist", table, column)
			}
		}
	}
}

func TestEnsureDatabaseSchemaUpToDateAddsGSLBFields(t *testing.T) {
	db := openBareTestSQLiteDB(t, "gslb-fields.db")
	if err := registerSharding(db, "sqlite"); err != nil {
		t.Fatalf("register sharding: %v", err)
	}
	if err := db.AutoMigrate(
		&DatabaseSchemaVersion{},
		&File{},
		&User{},
		&AuthSource{},
		&ExternalAccount{},
		&Option{},
		&Origin{},
		&ProxyRoute{},
		&ConfigVersion{},
		&Node{},
		&NodeSystemProfile{},
		&ApplyLog{},
		&NodeMetricSnapshot{},
		&NodeRequestReport{},
		&NodeAccessLog{},
		&NodeHealthEvent{},
		&TLSCertificate{},
		&ManagedDomain{},
		&AcmeAccount{},
		&DnsAccount{},
	); err != nil {
		t.Fatalf("AutoMigrate legacy schema: %v", err)
	}
	if err := db.Migrator().DropTable(&GSLBSchedulingState{}); err != nil {
		t.Fatalf("drop gslb state table: %v", err)
	}
	if db.Migrator().HasColumn(&ProxyRoute{}, "dns_ttl") {
		if err := db.Migrator().DropColumn(&ProxyRoute{}, "dns_ttl"); err != nil {
			t.Fatalf("drop dns_ttl: %v", err)
		}
	}
	if db.Migrator().HasColumn(&ProxyRoute{}, "gslb_enabled") {
		if err := db.Migrator().DropColumn(&ProxyRoute{}, "gslb_enabled"); err != nil {
			t.Fatalf("drop gslb_enabled: %v", err)
		}
	}
	if db.Migrator().HasColumn(&ProxyRoute{}, "gslb_policy") {
		if err := db.Migrator().DropColumn(&ProxyRoute{}, "gslb_policy"); err != nil {
			t.Fatalf("drop gslb_policy: %v", err)
		}
	}
	if err := saveDatabaseSchemaVersion(db, 17); err != nil {
		t.Fatalf("save schema version: %v", err)
	}

	if err := ensureDatabaseSchemaUpToDate(db, "sqlite"); err != nil {
		t.Fatalf("ensureDatabaseSchemaUpToDate: %v", err)
	}

	for _, column := range []string{"dns_ttl", "gslb_enabled", "gslb_policy"} {
		if !db.Migrator().HasColumn(&ProxyRoute{}, column) {
			t.Fatalf("expected proxy_routes.%s column to exist", column)
		}
	}
	if !db.Migrator().HasTable(&GSLBSchedulingState{}) {
		t.Fatal("expected gslb_scheduling_states table to exist")
	}
	version, exists, err := loadDatabaseSchemaVersion(db)
	if err != nil {
		t.Fatalf("loadDatabaseSchemaVersion: %v", err)
	}
	if !exists || version != currentDatabaseSchemaVersion {
		t.Fatalf("unexpected schema version: exists=%v version=%d", exists, version)
	}
}

func TestEnsureDatabaseSchemaUpToDateAddsAuthoritativeDNSFields(t *testing.T) {
	db := openBareTestSQLiteDB(t, "authoritative-dns-fields.db")
	if err := registerSharding(db, "sqlite"); err != nil {
		t.Fatalf("register sharding: %v", err)
	}
	if err := db.AutoMigrate(
		&DatabaseSchemaVersion{},
		&File{},
		&User{},
		&AuthSource{},
		&ExternalAccount{},
		&Option{},
		&Origin{},
		&ProxyRoute{},
		&ConfigVersion{},
		&Node{},
		&NodeSystemProfile{},
		&ApplyLog{},
		&NodeMetricSnapshot{},
		&NodeRequestReport{},
		&NodeAccessLog{},
		&NodeHealthEvent{},
		&TLSCertificate{},
		&ManagedDomain{},
		&AcmeAccount{},
		&DnsAccount{},
		&GSLBSchedulingState{},
	); err != nil {
		t.Fatalf("AutoMigrate legacy schema: %v", err)
	}
	for _, table := range []any{
		&DNSZone{},
		&DNSRecord{},
		&DNSWorker{},
		&DNSQueryRollup{},
	} {
		if db.Migrator().HasTable(table) {
			if err := db.Migrator().DropTable(table); err != nil {
				t.Fatalf("drop authoritative dns table %T: %v", table, err)
			}
		}
	}
	for _, column := range []string{"dns_provider_mode", "dns_zone_id_ref"} {
		if db.Migrator().HasColumn(&ProxyRoute{}, column) {
			if err := db.Migrator().DropColumn(&ProxyRoute{}, column); err != nil {
				t.Fatalf("drop %s: %v", column, err)
			}
		}
	}
	if db.Migrator().HasColumn(&GSLBSchedulingState{}, "scope_key") {
		if err := db.Migrator().DropColumn(&GSLBSchedulingState{}, "scope_key"); err != nil {
			t.Fatalf("drop gslb scope_key: %v", err)
		}
	}
	if err := saveDatabaseSchemaVersion(db, 18); err != nil {
		t.Fatalf("save schema version: %v", err)
	}

	if err := ensureDatabaseSchemaUpToDate(db, "sqlite"); err != nil {
		t.Fatalf("ensureDatabaseSchemaUpToDate: %v", err)
	}

	for _, table := range []any{
		&DNSZone{},
		&DNSRecord{},
		&DNSWorker{},
		&DNSQueryRollup{},
	} {
		if !db.Migrator().HasTable(table) {
			t.Fatalf("expected table for %T to exist", table)
		}
	}
	for _, column := range []string{"dns_provider_mode", "dns_zone_id_ref"} {
		if !db.Migrator().HasColumn(&ProxyRoute{}, column) {
			t.Fatalf("expected proxy_routes.%s column to exist", column)
		}
	}
	if !db.Migrator().HasColumn(&GSLBSchedulingState{}, "scope_key") {
		t.Fatal("expected gslb_scheduling_states.scope_key column to exist")
	}
	version, exists, err := loadDatabaseSchemaVersion(db)
	if err != nil {
		t.Fatalf("loadDatabaseSchemaVersion: %v", err)
	}
	if !exists || version != currentDatabaseSchemaVersion {
		t.Fatalf("unexpected schema version: exists=%v version=%d", exists, version)
	}
}

func TestEnsureDatabaseSchemaUpToDateAddsDNSRollupDurationFields(t *testing.T) {
	db := openBareTestSQLiteDB(t, "dns-rollup-duration-fields.db")
	if err := registerSharding(db, "sqlite"); err != nil {
		t.Fatalf("register sharding: %v", err)
	}
	if err := autoMigrateAll(db); err != nil {
		t.Fatalf("auto migrate current schema: %v", err)
	}
	for _, column := range []string{"total_duration_ms", "max_duration_ms"} {
		if db.Migrator().HasColumn(&DNSQueryRollup{}, column) {
			if err := db.Migrator().DropColumn(&DNSQueryRollup{}, column); err != nil {
				t.Fatalf("drop dns_query_rollups.%s: %v", column, err)
			}
		}
	}
	if err := autoMigrateSchemaMetadata(db); err != nil {
		t.Fatalf("auto migrate schema metadata: %v", err)
	}
	if err := saveDatabaseSchemaVersion(db, 19); err != nil {
		t.Fatalf("save schema version: %v", err)
	}

	if err := ensureDatabaseSchemaUpToDate(db, "sqlite"); err != nil {
		t.Fatalf("ensureDatabaseSchemaUpToDate: %v", err)
	}

	for _, column := range []string{"total_duration_ms", "max_duration_ms"} {
		if !db.Migrator().HasColumn(&DNSQueryRollup{}, column) {
			t.Fatalf("expected dns_query_rollups.%s column to exist", column)
		}
	}
	version, exists, err := loadDatabaseSchemaVersion(db)
	if err != nil {
		t.Fatalf("loadDatabaseSchemaVersion: %v", err)
	}
	if !exists || version != currentDatabaseSchemaVersion {
		t.Fatalf("unexpected schema version: exists=%v version=%d", exists, version)
	}
}

func TestEnsureDatabaseSchemaUpToDateAddsDNSWorkerProbeFields(t *testing.T) {
	db := openBareTestSQLiteDB(t, "dns-worker-probe-fields.db")
	if err := registerSharding(db, "sqlite"); err != nil {
		t.Fatalf("register sharding: %v", err)
	}
	if err := autoMigrateAll(db); err != nil {
		t.Fatalf("auto migrate current schema: %v", err)
	}
	for _, column := range []string{"last_probe_at", "last_probe_query", "last_probe_result"} {
		if db.Migrator().HasColumn(&DNSWorker{}, column) {
			if err := db.Migrator().DropColumn(&DNSWorker{}, column); err != nil {
				t.Fatalf("drop dns_workers.%s: %v", column, err)
			}
		}
	}
	if err := autoMigrateSchemaMetadata(db); err != nil {
		t.Fatalf("auto migrate schema metadata: %v", err)
	}
	if err := saveDatabaseSchemaVersion(db, 20); err != nil {
		t.Fatalf("save schema version: %v", err)
	}

	if err := ensureDatabaseSchemaUpToDate(db, "sqlite"); err != nil {
		t.Fatalf("ensureDatabaseSchemaUpToDate: %v", err)
	}

	for _, column := range []string{"last_probe_at", "last_probe_query", "last_probe_result"} {
		if !db.Migrator().HasColumn(&DNSWorker{}, column) {
			t.Fatalf("expected dns_workers.%s column to exist", column)
		}
	}
	version, exists, err := loadDatabaseSchemaVersion(db)
	if err != nil {
		t.Fatalf("loadDatabaseSchemaVersion: %v", err)
	}
	if !exists || version != currentDatabaseSchemaVersion {
		t.Fatalf("unexpected schema version: exists=%v version=%d", exists, version)
	}
}

func TestEnsureDatabaseSchemaUpToDateAddsDNSRollupSourceScope(t *testing.T) {
	db := openBareTestSQLiteDB(t, "dns-rollup-source-scope.db")
	if err := registerSharding(db, "sqlite"); err != nil {
		t.Fatalf("register sharding: %v", err)
	}
	if err := autoMigrateAll(db); err != nil {
		t.Fatalf("auto migrate current schema: %v", err)
	}
	if db.Migrator().HasColumn(&DNSQueryRollup{}, "source_scope") {
		if err := db.Migrator().DropColumn(&DNSQueryRollup{}, "source_scope"); err != nil {
			t.Fatalf("drop dns_query_rollups.source_scope: %v", err)
		}
	}
	if err := autoMigrateSchemaMetadata(db); err != nil {
		t.Fatalf("auto migrate schema metadata: %v", err)
	}
	if err := saveDatabaseSchemaVersion(db, 21); err != nil {
		t.Fatalf("save schema version: %v", err)
	}

	if err := ensureDatabaseSchemaUpToDate(db, "sqlite"); err != nil {
		t.Fatalf("ensureDatabaseSchemaUpToDate: %v", err)
	}

	if !db.Migrator().HasColumn(&DNSQueryRollup{}, "source_scope") {
		t.Fatal("expected dns_query_rollups.source_scope column to exist")
	}
	version, exists, err := loadDatabaseSchemaVersion(db)
	if err != nil {
		t.Fatalf("loadDatabaseSchemaVersion: %v", err)
	}
	if !exists || version != currentDatabaseSchemaVersion {
		t.Fatalf("unexpected schema version: exists=%v version=%d", exists, version)
	}
}

func TestAutoMigrateCreatesDNSRollupObservabilityIndex(t *testing.T) {
	db := openBareTestSQLiteDB(t, "dns-rollup-observability-index.db")
	if err := registerSharding(db, "sqlite"); err != nil {
		t.Fatalf("register sharding: %v", err)
	}
	if err := autoMigrateAll(db); err != nil {
		t.Fatalf("auto migrate current schema: %v", err)
	}
	if !db.Migrator().HasIndex(&DNSQueryRollup{}, "idx_dns_rollups_observability") {
		t.Fatal("expected dns_query_rollups observability index to exist")
	}
}

func TestEnsureDatabaseSchemaUpToDateAddsDNSWorkerGeoIPFields(t *testing.T) {
	db := openBareTestSQLiteDB(t, "dns-worker-geoip-fields.db")
	if err := registerSharding(db, "sqlite"); err != nil {
		t.Fatalf("register sharding: %v", err)
	}
	if err := autoMigrateAll(db); err != nil {
		t.Fatalf("auto migrate current schema: %v", err)
	}
	for _, column := range []string{"geo_ip_enabled", "geo_ip_database_path", "geo_ip_last_error"} {
		if db.Migrator().HasColumn(&DNSWorker{}, column) {
			if err := db.Migrator().DropColumn(&DNSWorker{}, column); err != nil {
				t.Fatalf("drop dns_workers.%s: %v", column, err)
			}
		}
	}
	if err := autoMigrateSchemaMetadata(db); err != nil {
		t.Fatalf("auto migrate schema metadata: %v", err)
	}
	if err := saveDatabaseSchemaVersion(db, 22); err != nil {
		t.Fatalf("save schema version: %v", err)
	}

	if err := ensureDatabaseSchemaUpToDate(db, "sqlite"); err != nil {
		t.Fatalf("ensureDatabaseSchemaUpToDate: %v", err)
	}

	for _, column := range []string{"geo_ip_enabled", "geo_ip_database_path", "geo_ip_last_error"} {
		if !db.Migrator().HasColumn(&DNSWorker{}, column) {
			t.Fatalf("expected dns_workers.%s column to exist", column)
		}
	}
	version, exists, err := loadDatabaseSchemaVersion(db)
	if err != nil {
		t.Fatalf("loadDatabaseSchemaVersion: %v", err)
	}
	if !exists || version != currentDatabaseSchemaVersion {
		t.Fatalf("unexpected schema version: exists=%v version=%d", exists, version)
	}
}

func TestEnsureDatabaseSchemaUpToDateAddsDNSWorkerObservabilityFields(t *testing.T) {
	db := openBareTestSQLiteDB(t, "dns-worker-observability-fields.db")
	if err := registerSharding(db, "sqlite"); err != nil {
		t.Fatalf("register sharding: %v", err)
	}
	if err := autoMigrateAll(db); err != nil {
		t.Fatalf("auto migrate current schema: %v", err)
	}
	for _, column := range []string{"last_heartbeat_at", "last_rollup_at", "last_rollup_count"} {
		if db.Migrator().HasColumn(&DNSWorker{}, column) {
			if err := db.Migrator().DropColumn(&DNSWorker{}, column); err != nil {
				t.Fatalf("drop dns_workers.%s: %v", column, err)
			}
		}
	}
	if err := autoMigrateSchemaMetadata(db); err != nil {
		t.Fatalf("auto migrate schema metadata: %v", err)
	}
	if err := saveDatabaseSchemaVersion(db, 33); err != nil {
		t.Fatalf("save schema version: %v", err)
	}

	if err := ensureDatabaseSchemaUpToDate(db, "sqlite"); err != nil {
		t.Fatalf("ensureDatabaseSchemaUpToDate: %v", err)
	}

	for _, column := range []string{"last_heartbeat_at", "last_rollup_at", "last_rollup_count"} {
		if !db.Migrator().HasColumn(&DNSWorker{}, column) {
			t.Fatalf("expected dns_workers.%s column to exist", column)
		}
	}
	version, exists, err := loadDatabaseSchemaVersion(db)
	if err != nil {
		t.Fatalf("loadDatabaseSchemaVersion: %v", err)
	}
	if !exists || version != currentDatabaseSchemaVersion {
		t.Fatalf("unexpected schema version: exists=%v version=%d", exists, version)
	}
}

func TestEnsureDatabaseSchemaUpToDateCompletesPartialDNSWorkerSourceDatabaseMigration(t *testing.T) {
	db := openBareTestSQLiteDB(t, "dns-worker-source-database-fields.db")
	if err := registerSharding(db, "sqlite"); err != nil {
		t.Fatalf("register sharding: %v", err)
	}
	if err := autoMigrateAll(db); err != nil {
		t.Fatalf("auto migrate current schema: %v", err)
	}
	for _, column := range []string{
		"asn_last_error",
		"geo_ip_database_type",
		"asn_database_type",
		"geo_ip_country_enabled",
		"geo_ip_asn_enabled",
		"geo_ip_operator_enabled",
		"operator_cidr_database_path",
		"operator_cidr_last_error",
	} {
		if db.Migrator().HasColumn(&DNSWorker{}, column) {
			if err := db.Migrator().DropColumn(&DNSWorker{}, column); err != nil {
				t.Fatalf("drop dns_workers.%s: %v", column, err)
			}
		}
	}
	if !db.Migrator().HasColumn(&DNSWorker{}, "asn_database_path") {
		t.Fatal("expected partial migration column dns_workers.asn_database_path to exist")
	}
	if err := autoMigrateSchemaMetadata(db); err != nil {
		t.Fatalf("auto migrate schema metadata: %v", err)
	}
	if err := saveDatabaseSchemaVersion(db, 34); err != nil {
		t.Fatalf("save schema version: %v", err)
	}

	if err := ensureDatabaseSchemaUpToDate(db, "sqlite"); err != nil {
		t.Fatalf("ensureDatabaseSchemaUpToDate: %v", err)
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
			t.Fatalf("expected dns_workers.%s column to exist", column)
		}
	}
	version, exists, err := loadDatabaseSchemaVersion(db)
	if err != nil {
		t.Fatalf("loadDatabaseSchemaVersion: %v", err)
	}
	if !exists || version != currentDatabaseSchemaVersion {
		t.Fatalf("unexpected schema version: exists=%v version=%d", exists, version)
	}
}

func TestEnsureDatabaseSchemaUpToDateAddsDNSWorkerNodeProbes(t *testing.T) {
	db := openBareTestSQLiteDB(t, "dns-worker-node-probes.db")
	if err := registerSharding(db, "sqlite"); err != nil {
		t.Fatalf("register sharding: %v", err)
	}
	if err := autoMigrateAll(db); err != nil {
		t.Fatalf("auto migrate current schema: %v", err)
	}
	if db.Migrator().HasTable(&DNSWorkerNodeProbe{}) {
		if err := db.Migrator().DropTable(&DNSWorkerNodeProbe{}); err != nil {
			t.Fatalf("drop dns_worker_node_probes: %v", err)
		}
	}
	if err := autoMigrateSchemaMetadata(db); err != nil {
		t.Fatalf("auto migrate schema metadata: %v", err)
	}
	if err := saveDatabaseSchemaVersion(db, 23); err != nil {
		t.Fatalf("save schema version: %v", err)
	}

	if err := ensureDatabaseSchemaUpToDate(db, "sqlite"); err != nil {
		t.Fatalf("ensureDatabaseSchemaUpToDate: %v", err)
	}

	if !db.Migrator().HasTable(&DNSWorkerNodeProbe{}) {
		t.Fatal("expected dns_worker_node_probes table to exist")
	}
	for _, column := range []string{"worker_id", "node_id", "checked_at", "results_json", "healthy", "average_rtt_ms", "max_rtt_ms"} {
		if !db.Migrator().HasColumn(&DNSWorkerNodeProbe{}, column) {
			t.Fatalf("expected dns_worker_node_probes.%s column to exist", column)
		}
	}
	version, exists, err := loadDatabaseSchemaVersion(db)
	if err != nil {
		t.Fatalf("loadDatabaseSchemaVersion: %v", err)
	}
	if !exists || version != currentDatabaseSchemaVersion {
		t.Fatalf("unexpected schema version: exists=%v version=%d", exists, version)
	}
}

func TestEnsureDatabaseSchemaUpToDateAddsDDOSProtectionTargetFields(t *testing.T) {
	db := openBareTestSQLiteDB(t, "ddos-protection-target-fields.db")
	if err := registerSharding(db, "sqlite"); err != nil {
		t.Fatalf("register sharding: %v", err)
	}
	if err := autoMigrateAll(db); err != nil {
		t.Fatalf("auto migrate current schema: %v", err)
	}
	for _, column := range []string{"ddos_protection_provider", "ddos_protection_target"} {
		if db.Migrator().HasColumn(&ProxyRoute{}, column) {
			if err := db.Migrator().DropColumn(&ProxyRoute{}, column); err != nil {
				t.Fatalf("drop proxy_routes.%s: %v", column, err)
			}
		}
	}
	if err := autoMigrateSchemaMetadata(db); err != nil {
		t.Fatalf("auto migrate schema metadata: %v", err)
	}
	if err := saveDatabaseSchemaVersion(db, 24); err != nil {
		t.Fatalf("save schema version: %v", err)
	}

	if err := ensureDatabaseSchemaUpToDate(db, "sqlite"); err != nil {
		t.Fatalf("ensureDatabaseSchemaUpToDate: %v", err)
	}

	for _, column := range []string{"ddos_protection_provider", "ddos_protection_target"} {
		if !db.Migrator().HasColumn(&ProxyRoute{}, column) {
			t.Fatalf("expected proxy_routes.%s column to exist", column)
		}
	}
	version, exists, err := loadDatabaseSchemaVersion(db)
	if err != nil {
		t.Fatalf("loadDatabaseSchemaVersion: %v", err)
	}
	if !exists || version != currentDatabaseSchemaVersion {
		t.Fatalf("unexpected schema version: exists=%v version=%d", exists, version)
	}
}

func TestEnsureDatabaseSchemaUpToDateAddsDNSWorkerNodeProbeNodeIndex(t *testing.T) {
	db := openBareTestSQLiteDB(t, "dns-worker-node-probe-node-index.db")
	if err := registerSharding(db, "sqlite"); err != nil {
		t.Fatalf("register sharding: %v", err)
	}
	if err := autoMigrateAll(db); err != nil {
		t.Fatalf("auto migrate current schema: %v", err)
	}
	if db.Migrator().HasIndex(&DNSWorkerNodeProbe{}, "idx_dns_worker_node_probe_node") {
		if err := db.Migrator().DropIndex(&DNSWorkerNodeProbe{}, "idx_dns_worker_node_probe_node"); err != nil {
			t.Fatalf("drop dns worker node probe node index: %v", err)
		}
	}
	if err := autoMigrateSchemaMetadata(db); err != nil {
		t.Fatalf("auto migrate schema metadata: %v", err)
	}
	if err := saveDatabaseSchemaVersion(db, 30); err != nil {
		t.Fatalf("save schema version: %v", err)
	}

	if err := ensureDatabaseSchemaUpToDate(db, "sqlite"); err != nil {
		t.Fatalf("ensureDatabaseSchemaUpToDate: %v", err)
	}

	if !db.Migrator().HasIndex(&DNSWorkerNodeProbe{}, "idx_dns_worker_node_probe_node") {
		t.Fatal("expected dns_worker_node_probes node index to exist")
	}
	version, exists, err := loadDatabaseSchemaVersion(db)
	if err != nil {
		t.Fatalf("loadDatabaseSchemaVersion: %v", err)
	}
	if !exists || version != currentDatabaseSchemaVersion {
		t.Fatalf("unexpected schema version: exists=%v version=%d", exists, version)
	}
}

func TestListDNSWorkerNodeProbesByScopeFiltersWorkersAndNodes(t *testing.T) {
	db := openTestSQLiteDB(t, "dns-worker-node-probe-scope.db")
	previousDB := DB
	DB = db
	t.Cleanup(func() {
		DB = previousDB
	})
	now := time.Now().UTC()
	for _, probe := range []*DNSWorkerNodeProbe{
		{WorkerID: "worker-a", NodeID: "node-a", CheckedAt: now.Add(-time.Second), ResultsJSON: "[]"},
		{WorkerID: "worker-a", NodeID: "node-b", CheckedAt: now.Add(-2 * time.Second), ResultsJSON: "[]"},
		{WorkerID: "worker-b", NodeID: "node-a", CheckedAt: now.Add(-3 * time.Second), ResultsJSON: "[]"},
		{WorkerID: "worker-c", NodeID: "node-c", CheckedAt: now.Add(-4 * time.Second), ResultsJSON: "[]"},
	} {
		if err := UpsertDNSWorkerNodeProbe(DB, probe); err != nil {
			t.Fatalf("UpsertDNSWorkerNodeProbe: %v", err)
		}
	}

	probes, err := ListDNSWorkerNodeProbesByScope([]string{"worker-a", "worker-b", "worker-a"}, []string{"node-a"})
	if err != nil {
		t.Fatalf("ListDNSWorkerNodeProbesByScope: %v", err)
	}
	if len(probes) != 2 {
		t.Fatalf("expected two scoped probes, got %+v", probes)
	}
	for _, probe := range probes {
		if probe.NodeID != "node-a" || (probe.WorkerID != "worker-a" && probe.WorkerID != "worker-b") {
			t.Fatalf("unexpected scoped probe: %+v", probe)
		}
	}
	empty, err := ListDNSWorkerNodeProbesByScope([]string{"worker-a"}, []string{})
	if err != nil {
		t.Fatalf("ListDNSWorkerNodeProbesByScope empty: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("expected empty scope to avoid table scan, got %+v", empty)
	}
}

func TestRunDatabaseSchemaMigrationDoesNotAdvanceVersionWhenValidationFails(t *testing.T) {
	db := openBareTestSQLiteDB(t, "failed-validation.db")

	err := runDatabaseSchemaMigration(db, "sqlite", databaseSchemaMigration{
		fromVersion: legacyDatabaseSchemaVersion,
		toVersion:   11,
		migrate: func(tx *gorm.DB, backend string) error {
			return autoMigrateSchemaMetadata(tx)
		},
		validate: func(tx *gorm.DB, backend string) error {
			return gorm.ErrInvalidDB
		},
	})
	if err == nil {
		t.Fatal("expected migration validation to fail")
	}

	_, exists, loadErr := loadDatabaseSchemaVersion(db)
	if loadErr != nil {
		t.Fatalf("loadDatabaseSchemaVersion: %v", loadErr)
	}
	if exists {
		t.Fatal("expected schema version to remain unset after failed validation")
	}
}
