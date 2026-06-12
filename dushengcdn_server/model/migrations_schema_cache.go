package model

import (
	"gorm.io/gorm"
	"strings"
)

type schemaIntrospectionCache struct {
	db          *gorm.DB
	physicalDB  *gorm.DB
	tableCache  map[string]bool
	columnCache map[string]bool
}

func newSchemaIntrospectionCache(db *gorm.DB) *schemaIntrospectionCache {
	return &schemaIntrospectionCache{
		db:          db,
		physicalDB:  sessionIgnoringSharding(db),
		tableCache:  make(map[string]bool),
		columnCache: make(map[string]bool),
	}
}

func cachedHasTable(db *gorm.DB, schemaCache *schemaIntrospectionCache, model any, table string) bool {
	if schemaCache != nil {
		return schemaCache.HasTable(model, table)
	}
	if db == nil {
		return false
	}
	return db.Migrator().HasTable(model)
}

func cachedHasPhysicalTable(db *gorm.DB, schemaCache *schemaIntrospectionCache, table string) bool {
	if schemaCache != nil {
		return schemaCache.HasPhysicalTable(table)
	}
	if db == nil {
		return false
	}
	return db.Migrator().HasTable(table)
}

func cachedHasColumn(db *gorm.DB, schemaCache *schemaIntrospectionCache, model any, table string, column string) bool {
	if schemaCache != nil {
		return schemaCache.HasColumn(model, table, column)
	}
	if db == nil {
		return false
	}
	return db.Migrator().HasColumn(model, column)
}

func cachedHasPhysicalColumn(db *gorm.DB, schemaCache *schemaIntrospectionCache, table string, column string) bool {
	if schemaCache != nil {
		return schemaCache.HasPhysicalColumn(table, column)
	}
	if db == nil {
		return false
	}
	return db.Migrator().HasColumn(table, column)
}

func (cache *schemaIntrospectionCache) HasTable(model any, table string) bool {
	if cache == nil || cache.db == nil {
		return false
	}
	table = strings.TrimSpace(table)
	if table == "" {
		return false
	}
	if exists, ok := cache.tableCache[table]; ok {
		return exists
	}
	exists := cache.db.Migrator().HasTable(model)
	cache.tableCache[table] = exists
	return exists
}

func (cache *schemaIntrospectionCache) InvalidateTable(table string) {
	if cache == nil {
		return
	}
	table = strings.TrimSpace(table)
	if table == "" {
		return
	}
	delete(cache.tableCache, table)
}

func (cache *schemaIntrospectionCache) HasPhysicalTable(table string) bool {
	if cache == nil || cache.physicalDB == nil {
		return false
	}
	table = strings.TrimSpace(table)
	if table == "" {
		return false
	}
	if exists, ok := cache.tableCache[table]; ok {
		return exists
	}
	exists := cache.physicalDB.Migrator().HasTable(table)
	cache.tableCache[table] = exists
	return exists
}

func (cache *schemaIntrospectionCache) HasColumn(model any, table string, column string) bool {
	if cache == nil || cache.db == nil {
		return false
	}
	table = strings.TrimSpace(table)
	column = strings.TrimSpace(column)
	if table == "" || column == "" {
		return false
	}
	key := table + "." + column
	if exists, ok := cache.columnCache[key]; ok {
		return exists
	}
	exists := cache.db.Migrator().HasColumn(model, column)
	cache.columnCache[key] = exists
	return exists
}

func (cache *schemaIntrospectionCache) InvalidateColumn(table string, column string) {
	if cache == nil {
		return
	}
	table = strings.TrimSpace(table)
	column = strings.TrimSpace(column)
	if table == "" || column == "" {
		return
	}
	delete(cache.columnCache, table+"."+column)
}

func (cache *schemaIntrospectionCache) HasPhysicalColumn(table string, column string) bool {
	if cache == nil || cache.physicalDB == nil {
		return false
	}
	table = strings.TrimSpace(table)
	column = strings.TrimSpace(column)
	if table == "" || column == "" {
		return false
	}
	key := table + "." + column
	if exists, ok := cache.columnCache[key]; ok {
		return exists
	}
	exists := cache.physicalDB.Migrator().HasColumn(table, column)
	cache.columnCache[key] = exists
	return exists
}
