package model

import (
	"context"
	"encoding/json"
	"fmt"
	"gorm.io/gorm"
	"strings"
)

type currentSchemaApplicationState struct {
	applied                       bool
	fullApplyCount                int
	skippedCurrentSchemaOnlyCount int
}

type currentSchemaApplicationStateKey struct{}

func withCurrentSchemaApplicationState(db *gorm.DB) *gorm.DB {
	if db == nil {
		return nil
	}
	if currentSchemaApplicationStateFromDB(db) != nil {
		return db
	}
	ctx := db.Statement.Context
	if ctx == nil {
		ctx = context.Background()
	}
	return db.WithContext(context.WithValue(ctx, currentSchemaApplicationStateKey{}, &currentSchemaApplicationState{}))
}

func currentSchemaApplicationStateFromDB(db *gorm.DB) *currentSchemaApplicationState {
	if db == nil || db.Statement == nil || db.Statement.Context == nil {
		return nil
	}
	state, _ := db.Statement.Context.Value(currentSchemaApplicationStateKey{}).(*currentSchemaApplicationState)
	return state
}

func autoMigrateSchemaMetadata(db *gorm.DB) error {
	db = migrationSession(db)
	for _, item := range schemaMetadataModels() {
		if err := db.AutoMigrate(item); err != nil {
			return err
		}
	}
	return nil
}

func migrateProxyRouteEnableHTTPSColumn(db *gorm.DB) error {
	return migrateProxyRouteEnableHTTPSColumnWithCache(db, nil)
}

func migrateProxyRouteEnableHTTPSColumnWithCache(db *gorm.DB, schemaCache *schemaIntrospectionCache) error {
	if !cachedHasTable(db, schemaCache, &ProxyRoute{}, (&ProxyRoute{}).TableName()) {
		return nil
	}
	if cachedHasColumn(db, schemaCache, &ProxyRoute{}, (&ProxyRoute{}).TableName(), "enable_https") || !cachedHasColumn(db, schemaCache, &ProxyRoute{}, (&ProxyRoute{}).TableName(), "enable_http_s") {
		return nil
	}
	if err := db.Migrator().RenameColumn(&ProxyRoute{}, "enable_http_s", "enable_https"); err != nil {
		return err
	}
	if schemaCache != nil {
		schemaCache.InvalidateColumn((&ProxyRoute{}).TableName(), "enable_http_s")
		schemaCache.InvalidateColumn((&ProxyRoute{}).TableName(), "enable_https")
	}
	return nil
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
	state := currentSchemaApplicationStateFromDB(db)
	if state != nil && state.applied {
		return nil
	}
	db = migrationSession(db)
	defer resetProxyRouteRuntimeSchemaCache(db)
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
	if err := ensureNodeAccessLogRollupSchema(db); err != nil {
		return err
	}
	if err := ensureDNSRollupObservabilityIndex(db); err != nil {
		return err
	}
	if err := ensureObservabilityShardQueryIndexes(db, backend); err != nil {
		return err
	}
	if err := EnsureProxyRouteNormalizedTablesBackfilled(db); err != nil {
		return err
	}
	if state != nil {
		state.applied = true
		state.fullApplyCount++
	}
	return nil
}

func migrateCurrentSchemaOnly(db *gorm.DB, backend string) error {
	return applyCurrentSchema(db, backend)
}

func applyCurrentSchemaMaintenance(db *gorm.DB, backend string) error {
	return applyCurrentSchemaMaintenanceWithCache(db, backend, nil)
}

func applyCurrentSchemaMaintenanceWithCache(db *gorm.DB, backend string, schemaCache *schemaIntrospectionCache) error {
	db = migrationSession(db)
	defer resetProxyRouteRuntimeSchemaCache(db)
	if schemaCache == nil {
		schemaCache = newSchemaIntrospectionCache(db)
	}
	if err := autoMigrateSchemaMetadata(db); err != nil {
		return err
	}
	if schemaCache != nil {
		for _, item := range schemaMetadataModels() {
			if named, ok := item.(interface{ TableName() string }); ok {
				schemaCache.InvalidateTable(named.TableName())
			}
		}
	}
	if err := migrateProxyRouteEnableHTTPSColumnWithCache(db, schemaCache); err != nil {
		return err
	}
	if err := ensureNodeAccessLogCurrentColumnsWithCache(db, schemaCache); err != nil {
		return err
	}
	if err := ensureNodeAccessLogRollupSchema(db); err != nil {
		return err
	}
	if err := ensureDNSRollupObservabilityIndex(db); err != nil {
		return err
	}
	return ensureObservabilityShardQueryIndexesWithCache(db, backend, schemaCache)
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
	return ensureObservabilityShardQueryIndexesWithCache(db, backend, nil)
}

func ensureObservabilityShardQueryIndexesWithCache(db *gorm.DB, backend string, schemaCache *schemaIntrospectionCache) error {
	db = sessionIgnoringSharding(db)
	if db == nil {
		return nil
	}
	indexes := []struct {
		baseTable string
		name      string
		columns   []string
	}{
		{baseTable: "node_access_logs", name: "idx_node_access_logs_node_logged_at", columns: []string{"node_id", "logged_at"}},
		{baseTable: "node_access_logs", name: "idx_node_access_logs_remote_addr_logged_at", columns: []string{"remote_addr", "logged_at"}},
		{baseTable: "node_access_logs", name: "idx_node_access_logs_host_logged_at", columns: []string{"host", "logged_at"}},
		{baseTable: "node_access_logs", name: "idx_node_access_logs_node_remote_logged_at", columns: []string{"node_id", "remote_addr", "logged_at"}},
		{baseTable: "node_access_logs", name: "idx_node_access_logs_node_host_logged_at", columns: []string{"node_id", "host", "logged_at"}},
		{baseTable: "node_metric_snapshots", name: "idx_node_metric_snapshots_node_captured_at", columns: []string{"node_id", "captured_at"}},
		{baseTable: "node_request_reports", name: "idx_node_request_reports_node_window_ended_at", columns: []string{"node_id", "window_ended_at"}},
	}
	for _, item := range indexes {
		for _, table := range observabilityShardTables(item.baseTable) {
			if !cachedHasPhysicalTable(db, schemaCache, table) {
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

type postgresColumnDefinition struct {
	name       string
	definition string
}

func addPostgresColumnsIfMissing(db *gorm.DB, table string, columns []postgresColumnDefinition) error {
	for _, column := range columns {
		sql := fmt.Sprintf(
			"ALTER TABLE %s ADD COLUMN IF NOT EXISTS %s %s",
			quoteIdentifier(table),
			quoteIdentifier(column.name),
			column.definition,
		)
		if err := db.Exec(sql).Error; err != nil {
			return fmt.Errorf("add %s.%s column failed: %w", table, column.name, err)
		}
	}
	return nil
}

func ensureNodeAccessLogCurrentColumns(db *gorm.DB) error {
	return ensureNodeAccessLogCurrentColumnsWithCache(db, nil)
}

func ensureNodeAccessLogCurrentColumnsWithCache(db *gorm.DB, schemaCache *schemaIntrospectionCache) error {
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
		if !cachedHasPhysicalTable(db, schemaCache, table) {
			continue
		}
		for _, column := range columns {
			if cachedHasPhysicalColumn(db, schemaCache, table, column.column) {
				continue
			}
			if err := db.Table(table).Migrator().AddColumn(&NodeAccessLog{}, column.field); err != nil {
				return fmt.Errorf("add node access log %s column to %s failed: %w", column.column, table, err)
			}
			if schemaCache != nil {
				schemaCache.InvalidateColumn(table, column.column)
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
