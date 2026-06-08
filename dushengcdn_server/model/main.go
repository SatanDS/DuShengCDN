package model

import (
	"dushengcdn/common"
	"dushengcdn/utils/security"
	"errors"
	"fmt"
	"github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
	"log/slog"
	"os"
	"reflect"
	"strings"
	"sync"
)

var DB *gorm.DB

type dbModel struct {
	value     any
	tableName string
	hasIDPK   bool
}

func registeredModels() []any {
	return []any{
		&File{},
		&User{},
		&AuthSource{},
		&ExternalAccount{},
		&Option{},
		&Origin{},
		&ProxyRoute{},
		&GSLBSchedulingState{},
		&DNSZone{},
		&DNSRecord{},
		&DNSWorker{},
		&DNSWorkerNodeProbe{},
		&DNSQueryRollup{},
		&ConfigVersion{},
		&ConfigVersionArtifact{},
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
		&CommercialLicense{},
		&CommercialLicenseActivation{},
		&CommercialLicenseRevocation{},
	}
}

func schemaMetadataModels() []any {
	return []any{
		&DatabaseSchemaVersion{},
	}
}

func buildDBModels() ([]dbModel, error) {
	models := registeredModels()
	result := make([]dbModel, 0, len(models))
	namer := schema.NamingStrategy{}
	cache := &sync.Map{}
	for _, item := range models {
		parsed, err := schema.Parse(item, cache, namer)
		if err != nil {
			return nil, err
		}
		hasIDPK := len(parsed.PrimaryFields) == 1 && parsed.PrimaryFields[0].DBName == "id"
		result = append(result, dbModel{
			value:     item,
			tableName: parsed.Table,
			hasIDPK:   hasIDPK,
		})
	}
	return result, nil
}

func createRootAccountIfNeed() error {
	var user User
	//if user.Status != common.UserStatusEnabled {
	err := DB.First(&user).Error
	if err == nil {
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	password := strings.TrimSpace(common.InitialRootPassword)
	generated := false
	if password == "" {
		password = security.GenerateRandomString(24)
		generated = true
	}
	if password == "" {
		return fmt.Errorf("generate initial root password failed")
	}
	hashedPassword, err := security.Password2Hash(password)
	if err != nil {
		return err
	}
	rootUser := User{
		Username:    "root",
		Password:    hashedPassword,
		Role:        common.RoleRootUser,
		Status:      common.UserStatusEnabled,
		DisplayName: "Root User",
	}
	if err := DB.Create(&rootUser).Error; err != nil {
		return err
	}
	if generated {
		slog.Warn("no user exists; created root user with a generated one-time password", "username", "root", "initial_password", password, "reset_hint", "run with --reset-root-password if this password was not captured")
	} else {
		slog.Warn("no user exists; created root user with DUSHENGCDN_INITIAL_ROOT_PASSWORD", "username", "root", "reset_hint", "remove or rotate this bootstrap password after first login")
	}
	return nil
}

func CountTable(tableName string) (num int64) {
	DB.Table(tableName).Count(&num)
	return
}

func openDatabase() (*gorm.DB, string, error) {
	if common.SQLDSN != "" {
		db, err := gorm.Open(postgres.Open(common.SQLDSN), &gorm.Config{})
		if err != nil {
			return nil, "", err
		}
		if err := configureDatabasePool(db); err != nil {
			return nil, "", err
		}
		return db, "postgres", nil
	}
	db, err := gorm.Open(sqlite.Open(common.SQLitePath), &gorm.Config{})
	if err != nil {
		return nil, "", err
	}
	if err := configureDatabasePool(db); err != nil {
		return nil, "", err
	}
	slog.Info("database DSN not set, using SQLite as database", "sqlite_path", common.SQLitePath)
	return db, "sqlite", nil
}

func configureDatabasePool(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	sqlDB.SetMaxOpenConns(common.DatabaseMaxOpenConns)
	sqlDB.SetMaxIdleConns(common.DatabaseMaxIdleConns)
	sqlDB.SetConnMaxLifetime(common.DatabaseConnMaxLifetime)
	return nil
}

func migrationSession(db *gorm.DB) *gorm.DB {
	if db == nil {
		return nil
	}
	return db.Session(&gorm.Session{DisableNestedTransaction: true})
}

func autoMigrateAll(db *gorm.DB) error {
	db = migrationSession(db)
	models, err := buildDBModels()
	if err != nil {
		return err
	}
	for _, item := range models {
		if isShardedObservabilityTable(item.tableName) {
			continue
		}
		if err := db.AutoMigrate(item.value); err != nil {
			return err
		}
	}
	return autoMigrateObservabilityShardTables(db)
}

func isDatabaseEmpty(db *gorm.DB) (bool, error) {
	models, err := buildDBModels()
	if err != nil {
		return false, err
	}
	for _, item := range models {
		if isShardedObservabilityTable(item.tableName) {
			for _, table := range observabilityShardTables(item.tableName) {
				if !db.Migrator().HasTable(table) {
					continue
				}
				var count int64
				if err := db.Table(table).Limit(1).Count(&count).Error; err != nil {
					return false, err
				}
				if count > 0 {
					return false, nil
				}
			}
			continue
		}
		if !db.Migrator().HasTable(item.value) {
			continue
		}
		var count int64
		if err := db.Model(item.value).Limit(1).Count(&count).Error; err != nil {
			return false, err
		}
		if count > 0 {
			return false, nil
		}
	}
	return true, nil
}

func sqliteSourceExists() bool {
	info, err := os.Stat(common.SQLitePath)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func migrateSQLiteDataIfNeeded(target *gorm.DB, backend string) error {
	if backend != "postgres" {
		return nil
	}
	empty, err := isDatabaseEmpty(target)
	if err != nil {
		return err
	}
	if !empty {
		slog.Info("skip sqlite migration because target database already has data", "backend", backend)
		return nil
	}
	if !sqliteSourceExists() {
		slog.Info("skip sqlite migration because sqlite source file was not found", "sqlite_path", common.SQLitePath)
		return nil
	}

	source, err := gorm.Open(sqlite.Open(common.SQLitePath), &gorm.Config{
		PrepareStmt: true,
	})
	if err != nil {
		return fmt.Errorf("open sqlite source database failed: %w", err)
	}
	sourceSQLDB, err := source.DB()
	if err != nil {
		return fmt.Errorf("get sqlite source database handle failed: %w", err)
	}
	defer func() {
		_ = sourceSQLDB.Close()
	}()

	models, err := buildDBModels()
	if err != nil {
		return err
	}

	slog.Info("starting sqlite to postgres database migration", "sqlite_path", common.SQLitePath)
	err = target.Transaction(func(tx *gorm.DB) error {
		for _, item := range models {
			if err := migrateTableData(source, tx, item); err != nil {
				return err
			}
			if item.hasIDPK {
				if err := resetPostgresSequence(tx, item.tableName); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	slog.Info("sqlite to postgres database migration completed", "sqlite_path", common.SQLitePath)
	return nil
}

func migrateTableData(source *gorm.DB, target *gorm.DB, item dbModel) error {
	if !source.Migrator().HasTable(item.value) {
		slog.Info("database migration progress", "table", item.tableName, "migrated", 0, "total", 0, "status", "skipped_missing_source_table")
		return nil
	}
	var total int64
	if err := source.Model(item.value).Count(&total).Error; err != nil {
		return fmt.Errorf("count sqlite table %s failed: %w", item.tableName, err)
	}
	slog.Info("database migration progress", "table", item.tableName, "migrated", 0, "total", total, "status", "starting")
	if total == 0 {
		slog.Info("database migration progress", "table", item.tableName, "migrated", 0, "total", total, "status", "completed")
		return nil
	}

	modelType := reflect.TypeOf(item.value).Elem()
	sliceType := reflect.SliceOf(modelType)
	migrated := int64(0)
	offset := 0
	const batchSize = 200

	for {
		batchPtr := reflect.New(sliceType)
		query := source.Model(item.value).Limit(batchSize).Offset(offset)
		if item.hasIDPK {
			query = query.Order("id ASC")
		}
		if err := query.Find(batchPtr.Interface()).Error; err != nil {
			return fmt.Errorf("read sqlite table %s failed: %w", item.tableName, err)
		}
		batchLen := batchPtr.Elem().Len()
		if batchLen == 0 {
			break
		}
		if isShardedObservabilityTable(item.tableName) {
			for index := 0; index < batchLen; index++ {
				record := batchPtr.Elem().Index(index)
				if err := target.Create(record.Addr().Interface()).Error; err != nil {
					return fmt.Errorf("write target sharded table %s failed: %w", item.tableName, err)
				}
			}
		} else {
			if err := target.Create(batchPtr.Interface()).Error; err != nil {
				return fmt.Errorf("write target table %s failed: %w", item.tableName, err)
			}
		}
		migrated += int64(batchLen)
		offset += batchLen
		slog.Info("database migration progress", "table", item.tableName, "migrated", migrated, "total", total, "status", "running")
	}

	slog.Info("database migration progress", "table", item.tableName, "migrated", migrated, "total", total, "status", "completed")
	return nil
}

func resetPostgresSequence(db *gorm.DB, tableName string) error {
	sql := fmt.Sprintf(
		"SELECT setval(pg_get_serial_sequence('%s', 'id'), COALESCE(MAX(id), 1), MAX(id) IS NOT NULL) FROM \"%s\"",
		tableName,
		tableName,
	)
	return db.Exec(sql).Error
}

func InitDB() (err error) {
	db, backend, err := openDatabase()
	if err != nil {
		slog.Error("open database failed", "error", err)
		os.Exit(1)
	}
	DB = db
	if err = registerSharding(db, backend); err != nil {
		return err
	}
	if err = ensureDatabaseSchemaUpToDate(migrationSession(db), backend); err != nil {
		return err
	}
	return createRootAccountIfNeed()
}

func CloseDB() error {
	sqlDB, err := DB.DB()
	if err != nil {
		return err
	}
	err = sqlDB.Close()
	return err
}
