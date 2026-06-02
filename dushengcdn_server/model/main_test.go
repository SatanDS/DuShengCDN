package model

import (
	"encoding/json"
	"path/filepath"
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

	for _, table := range observabilityShardTables("node_access_logs") {
		for _, column := range []string{"request_bytes", "response_bytes", "upstream_bytes", "reason"} {
			if !db.Migrator().HasColumn(table, column) {
				t.Fatalf("expected column %s.%s to exist", table, column)
			}
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
