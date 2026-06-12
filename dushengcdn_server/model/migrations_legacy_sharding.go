package model

import (
	"fmt"
	"gorm.io/gorm"
)

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
	return migrateLegacyObservabilityShardRows(db, "node_metric_snapshots",
		func(row *NodeMetricSnapshot) uint { return row.ID },
		func(row *NodeMetricSnapshot, id uint) { row.ID = id },
	)
}

func migrateLegacyNodeRequestReports(db *gorm.DB) error {
	return migrateLegacyObservabilityShardRows(db, "node_request_reports",
		func(row *NodeRequestReport) uint { return row.ID },
		func(row *NodeRequestReport, id uint) { row.ID = id },
	)
}

func migrateLegacyNodeAccessLogs(db *gorm.DB) error {
	return migrateLegacyObservabilityShardRows(db, "node_access_logs",
		func(row *NodeAccessLog) uint { return row.ID },
		func(row *NodeAccessLog, id uint) { row.ID = id },
	)
}

func migrateLegacyObservabilityShardRows[T any](db *gorm.DB, baseTable string, getID func(*T) uint, setID func(*T, uint)) error {
	const batchSize = 500
	for _, table := range observabilityShardTables(baseTable) {
		legacyTable := legacyObservabilityShardTableName(table)
		if !db.Migrator().HasTable(legacyTable) {
			continue
		}
		var lastSeenID uint
		for {
			var rows []T
			query := db.Table(legacyTable).Order("id ASC").Limit(batchSize)
			if lastSeenID > 0 {
				query = query.Where("id > ?", lastSeenID)
			}
			if err := query.Find(&rows).Error; err != nil {
				return fmt.Errorf("query legacy sharded table %s failed: %w", legacyTable, err)
			}
			if len(rows) == 0 {
				break
			}
			lastSeenID = getID(&rows[len(rows)-1])
			grouped := make(map[string][]T, observabilityShardCount)
			for index := range rows {
				var id uint
				if err := assignObservabilityID(&id); err != nil {
					return err
				}
				setID(&rows[index], id)
				targetTable := observabilityShardTableForID(baseTable, id)
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
