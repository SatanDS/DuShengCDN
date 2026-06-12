package model

import (
	"errors"
	"fmt"
	"gorm.io/gorm"
)

type databaseSchemaMigration struct {
	fromVersion          int
	toVersion            int
	migrate              func(db *gorm.DB, backend string) error
	validate             func(db *gorm.DB, backend string) error
	appliesCurrentSchema bool
	currentSchemaOnly    bool
}

func loadDatabaseSchemaVersion(db *gorm.DB) (int, bool, error) {
	if db == nil {
		return 0, false, nil
	}
	if !db.Migrator().HasTable(&DatabaseSchemaVersion{}) {
		return 0, false, nil
	}
	var state DatabaseSchemaVersion
	err := db.Where("id = ?", databaseSchemaVersionRowID).First(&state).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return state.Version, true, nil
}

func saveDatabaseSchemaVersion(db *gorm.DB, version int) error {
	return db.Save(&DatabaseSchemaVersion{
		ID:      databaseSchemaVersionRowID,
		Version: version,
	}).Error
}

func currentSchemaOnlyMigration(
	fromVersion int,
	toVersion int,
	migrate func(db *gorm.DB, backend string) error,
	validate func(db *gorm.DB, backend string) error,
) databaseSchemaMigration {
	return databaseSchemaMigration{
		fromVersion:          fromVersion,
		toVersion:            toVersion,
		migrate:              migrate,
		validate:             validate,
		appliesCurrentSchema: true,
		currentSchemaOnly:    true,
	}
}

func databaseSchemaMigrations() []databaseSchemaMigration {
	return []databaseSchemaMigration{
		currentSchemaOnlyMigration(1, 2, migrateV2, validateDatabaseSchemaV2),
		{fromVersion: 2, toVersion: 3, migrate: migrateV3, validate: validateDatabaseSchemaV3},
		{fromVersion: 3, toVersion: 4, migrate: migrateV4, validate: validateDatabaseSchemaV4},
		{fromVersion: 4, toVersion: 5, migrate: migrateV5, validate: validateDatabaseSchemaV5},
		{fromVersion: 5, toVersion: 6, migrate: migrateV6, validate: validateDatabaseSchemaV6},
		{fromVersion: 6, toVersion: 7, migrate: migrateV7, validate: validateDatabaseSchemaV7},
		{fromVersion: 7, toVersion: 8, migrate: migrateV8, validate: validateDatabaseSchemaV8},
		{fromVersion: 8, toVersion: 9, migrate: migrateV9, validate: validateDatabaseSchemaV9},
		{fromVersion: 9, toVersion: 10, migrate: migrateV10, validate: validateDatabaseSchemaV10},
		currentSchemaOnlyMigration(10, 11, migrateV11, validateDatabaseSchemaV11),
		currentSchemaOnlyMigration(11, 12, migrateV12, validateDatabaseSchemaV12),
		currentSchemaOnlyMigration(12, 13, migrateV13, validateDatabaseSchemaV13),
		currentSchemaOnlyMigration(13, 14, migrateV14, validateDatabaseSchemaV14),
		currentSchemaOnlyMigration(14, 15, migrateV15, validateDatabaseSchemaV15),
		currentSchemaOnlyMigration(15, 16, migrateV16, validateDatabaseSchemaV16),
		currentSchemaOnlyMigration(16, 17, migrateV17, validateDatabaseSchemaV17),
		currentSchemaOnlyMigration(17, 18, migrateV18, validateDatabaseSchemaV18),
		{fromVersion: 18, toVersion: 19, migrate: migrateV19, validate: validateDatabaseSchemaV19},
		currentSchemaOnlyMigration(19, 20, migrateV20, validateDatabaseSchemaV20),
		currentSchemaOnlyMigration(20, 21, migrateV21, validateDatabaseSchemaV21),
		{fromVersion: 21, toVersion: 22, migrate: migrateV22, validate: validateDatabaseSchemaV22},
		currentSchemaOnlyMigration(22, 23, migrateV23, validateDatabaseSchemaV23),
		currentSchemaOnlyMigration(23, 24, migrateV24, validateDatabaseSchemaV24),
		currentSchemaOnlyMigration(24, 25, migrateV25, validateDatabaseSchemaV25),
		currentSchemaOnlyMigration(25, 26, migrateV26, validateDatabaseSchemaV26),
		currentSchemaOnlyMigration(26, 27, migrateV27, validateDatabaseSchemaV27),
		currentSchemaOnlyMigration(27, 28, migrateV28, validateDatabaseSchemaV28),
		currentSchemaOnlyMigration(28, 29, migrateV29, validateDatabaseSchemaV29),
		currentSchemaOnlyMigration(29, 30, migrateV30, validateDatabaseSchemaV30),
		currentSchemaOnlyMigration(30, 31, migrateV31, validateDatabaseSchemaV31),
		currentSchemaOnlyMigration(31, 32, migrateV32, validateDatabaseSchemaV32),
		currentSchemaOnlyMigration(32, 33, migrateV33, validateDatabaseSchemaV33),
		currentSchemaOnlyMigration(33, 34, migrateV34, validateDatabaseSchemaV34),
		{fromVersion: 34, toVersion: 35, migrate: migrateV35, validate: validateDatabaseSchemaV35},
		{fromVersion: 35, toVersion: 36, migrate: migrateV36, validate: validateDatabaseSchemaV36},
		{fromVersion: 36, toVersion: 37, migrate: migrateV37, validate: validateDatabaseSchemaV37},
		{fromVersion: 37, toVersion: 38, migrate: migrateV38, validate: validateDatabaseSchemaV38},
		{fromVersion: 38, toVersion: 39, migrate: migrateV39, validate: validateDatabaseSchemaV39},
		{fromVersion: 39, toVersion: 40, migrate: migrateV40, validate: validateDatabaseSchemaV40},
		{fromVersion: 40, toVersion: 41, migrate: migrateV41, validate: validateDatabaseSchemaV41},
		{fromVersion: 41, toVersion: 42, migrate: migrateV42, validate: validateDatabaseSchemaV42},
		{fromVersion: 42, toVersion: 43, migrate: migrateV43, validate: validateDatabaseSchemaV43},
		currentSchemaOnlyMigration(43, 44, migrateV44, validateDatabaseSchemaV44),
		{fromVersion: 44, toVersion: 45, migrate: migrateV45, validate: validateDatabaseSchemaV45},
		{fromVersion: 45, toVersion: 46, migrate: migrateV46, validate: validateDatabaseSchemaV46},
		{fromVersion: 46, toVersion: 47, migrate: migrateV47, validate: validateDatabaseSchemaV47},
		{fromVersion: 47, toVersion: 48, migrate: migrateV48, validate: validateDatabaseSchemaV48, appliesCurrentSchema: true},
	}
}

func databaseSchemaMigrationMap() map[int]databaseSchemaMigration {
	migrations := make(map[int]databaseSchemaMigration, len(databaseSchemaMigrations()))
	for _, item := range databaseSchemaMigrations() {
		migrations[item.fromVersion] = item
	}
	return migrations
}

func runDatabaseSchemaMigration(db *gorm.DB, backend string, migration databaseSchemaMigration) error {
	if backend == "sqlite" {
		if err := migration.migrate(db, backend); err != nil {
			return fmt.Errorf("migrate database schema from v%d to v%d failed: %w", migration.fromVersion, migration.toVersion, err)
		}
		if err := migration.validate(db, backend); err != nil {
			return fmt.Errorf("validate database schema v%d failed: %w", migration.toVersion, err)
		}
		if err := saveDatabaseSchemaVersion(db, migration.toVersion); err != nil {
			return fmt.Errorf("persist database schema version v%d failed: %w", migration.toVersion, err)
		}
		return nil
	}

	return db.Transaction(func(tx *gorm.DB) error {
		if err := migration.migrate(tx, backend); err != nil {
			return fmt.Errorf("migrate database schema from v%d to v%d failed: %w", migration.fromVersion, migration.toVersion, err)
		}
		if err := migration.validate(tx, backend); err != nil {
			return fmt.Errorf("validate database schema v%d failed: %w", migration.toVersion, err)
		}
		if err := saveDatabaseSchemaVersion(tx, migration.toVersion); err != nil {
			return fmt.Errorf("persist database schema version v%d failed: %w", migration.toVersion, err)
		}
		return nil
	})
}

func skipAppliedCurrentSchemaOnlyMigrations(db *gorm.DB, migrationMap map[int]databaseSchemaMigration, version int, appliedCurrentSchema bool) (int, error) {
	if !appliedCurrentSchema {
		return version, nil
	}
	startVersion := version
	skipped := 0
	for version < currentDatabaseSchemaVersion {
		migration, ok := migrationMap[version]
		if !ok || !migration.currentSchemaOnly {
			break
		}
		version = migration.toVersion
		skipped++
	}
	if skipped == 0 {
		return startVersion, nil
	}
	if err := saveDatabaseSchemaVersion(db, version); err != nil {
		return startVersion, fmt.Errorf("persist database schema version v%d after skipping current-schema-only migrations failed: %w", version, err)
	}
	if state := currentSchemaApplicationStateFromDB(db); state != nil {
		state.skippedCurrentSchemaOnlyCount += skipped
	}
	return version, nil
}

func upgradeDatabaseSchema(db *gorm.DB, backend string, version int) error {
	if version > currentDatabaseSchemaVersion {
		return fmt.Errorf("database schema version %d is newer than application version %d", version, currentDatabaseSchemaVersion)
	}
	if version == currentDatabaseSchemaVersion {
		schemaCache := newSchemaIntrospectionCache(db)
		if err := applyCurrentSchemaMaintenanceWithCache(db, backend, schemaCache); err != nil {
			return err
		}
		return validateCurrentDatabaseSchemaLightweightWithCache(db, backend, schemaCache)
	}
	// Older validators check a final-schema superset instead of strict
	// per-version deltas, so the historical chain still runs in order. The
	// context state only removes duplicate full AutoMigrate/apply work once
	// one migration has already brought the schema to the current shape.
	db = withCurrentSchemaApplicationState(db)
	currentSchemaState := currentSchemaApplicationStateFromDB(db)
	migrationMap := databaseSchemaMigrationMap()
	appliedCurrentSchema := currentSchemaState != nil && currentSchemaState.applied
	for version < currentDatabaseSchemaVersion {
		skippedVersion, err := skipAppliedCurrentSchemaOnlyMigrations(db, migrationMap, version, appliedCurrentSchema)
		if err != nil {
			return err
		}
		if skippedVersion != version {
			version = skippedVersion
			if version >= currentDatabaseSchemaVersion {
				break
			}
		}
		migration, ok := migrationMap[version]
		if !ok {
			return fmt.Errorf("database schema migration from v%d is not defined", version)
		}
		if err := runDatabaseSchemaMigration(db, backend, migration); err != nil {
			return err
		}
		appliedCurrentSchema = appliedCurrentSchema || migration.appliesCurrentSchema || (currentSchemaState != nil && currentSchemaState.applied)
		version = migration.toVersion
	}
	if !appliedCurrentSchema {
		if err := applyCurrentSchema(db, backend); err != nil {
			return err
		}
	}
	return validateCompletedDatabaseSchemaUpgrade(db, backend, appliedCurrentSchema)
}

func validateCompletedDatabaseSchemaUpgrade(db *gorm.DB, backend string, appliedCurrentSchema bool) error {
	if appliedCurrentSchema {
		return validateCurrentDatabaseSchemaLightweight(db, backend)
	}
	return validateDatabaseSchemaV48(db, backend)
}

func initializeFreshDatabaseSchema(db *gorm.DB, backend string) error {
	if err := applyCurrentSchema(db, backend); err != nil {
		return err
	}
	if err := migrateSQLiteDataIfNeeded(db, backend); err != nil {
		return err
	}
	if err := backfillOriginsFromProxyRoutes(db); err != nil {
		return err
	}
	if err := backfillProxyRouteSiteFields(db); err != nil {
		return err
	}
	if err := ensureProxyRouteSiteNameUniqueIndex(db); err != nil {
		return err
	}
	if err := backfillProxyRouteCertificateFields(db); err != nil {
		return err
	}
	if err := backfillProxyRouteDomainCertificateFields(db); err != nil {
		return err
	}
	if err := migrateProxyRouteBasicAuthPasswordHashes(db); err != nil {
		return err
	}
	if err := BackfillProxyRouteNormalizedTables(db); err != nil {
		return err
	}
	if err := ensureDefaultGitHubAuthSource(db); err != nil {
		return err
	}
	if err := ensureGSLBSchedulingStateScopeIndex(db); err != nil {
		return err
	}
	if err := validateDatabaseSchemaV48(db, backend); err != nil {
		return err
	}
	return saveDatabaseSchemaVersion(db, currentDatabaseSchemaVersion)
}

func ensureDatabaseSchemaUpToDate(db *gorm.DB, backend string) error {
	version, exists, err := loadDatabaseSchemaVersion(db)
	if err != nil {
		return err
	}
	if exists {
		return upgradeDatabaseSchema(db, backend, version)
	}
	empty, err := isDatabaseEmpty(db)
	if err != nil {
		return err
	}
	if empty {
		return initializeFreshDatabaseSchema(db, backend)
	}
	if err := autoMigrateSchemaMetadata(db); err != nil {
		return err
	}
	return upgradeDatabaseSchema(db, backend, legacyDatabaseSchemaVersion)
}
