package model

import (
	"database/sql"
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
	"path/filepath"
	"reflect"
	"strings"
	"sync"
)

var DB *gorm.DB

type dbModel struct {
	value             any
	tableName         string
	columns           []string
	primaryColumnName string
	primaryFieldName  string
	hasIDPK           bool
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
		&ProxySite{},
		&ProxySiteDomain{},
		&OriginGroup{},
		&OriginServer{},
		&ProxyRouteRule{},
		&CachePolicy{},
		&TLSBinding{},
		&DNSBinding{},
		&SecurityPolicy{},
		&GSLBSchedulingState{},
		&DNSZone{},
		&DNSRecord{},
		&DNSWorker{},
		&DNSWorkerNodeProbe{},
		&DNSZoneWorkerAssignment{},
		&DNSSECKey{},
		&DNSQueryRollup{},
		&ConfigVersion{},
		&ConfigVersionArtifact{},
		&ConfigReleasePlan{},
		&ConfigReleaseTarget{},
		&ConfigReleaseBlockedChecksum{},
		&Node{},
		&NodeSystemProfile{},
		&ApplyLog{},
		&NodeMetricSnapshot{},
		&NodeRequestReport{},
		&NodeAccessLog{},
		&NodeAccessLogRollup{},
		&NodeHealthEvent{},
		&OriginHealthStatus{},
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
		primaryColumnName := ""
		primaryFieldName := ""
		if len(parsed.PrimaryFields) == 1 {
			primaryColumnName = parsed.PrimaryFields[0].DBName
			primaryFieldName = parsed.PrimaryFields[0].Name
		}
		columns := make([]string, 0, len(parsed.DBNames))
		for _, column := range parsed.DBNames {
			if strings.TrimSpace(column) != "" {
				columns = append(columns, column)
			}
		}
		result = append(result, dbModel{
			value:             item,
			tableName:         parsed.Table,
			columns:           columns,
			primaryColumnName: primaryColumnName,
			primaryFieldName:  primaryFieldName,
			hasIDPK:           hasIDPK,
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
	passwordFromFile := false
	passwordFilePath := ""
	if password == "" {
		passwordFilePath = initialRootPasswordFilePath()
		filePassword, ok, err := readInitialRootPasswordFile(passwordFilePath)
		if err != nil {
			return fmt.Errorf("read initial root password file failed: %w", err)
		}
		if ok {
			password = filePassword
			passwordFromFile = true
		} else {
			password = security.GenerateRandomString(24)
			generated = true
		}
	}
	if password == "" {
		return fmt.Errorf("generate initial root password failed")
	}
	if generated {
		if err := writeInitialRootPasswordFile(passwordFilePath, password); err != nil {
			if !errors.Is(err, os.ErrExist) {
				return fmt.Errorf("write initial root password file failed: %w", err)
			}
			filePassword, ok, readErr := readInitialRootPasswordFile(passwordFilePath)
			if readErr != nil {
				return fmt.Errorf("read existing initial root password file failed: %w", readErr)
			}
			if !ok {
				return fmt.Errorf("initial root password file already exists but could not be read")
			}
			password = filePassword
			generated = false
			passwordFromFile = true
		}
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
		if isModelUniqueConstraintError(err) {
			slog.Info("root user already exists; skipping bootstrap root creation", "username", "root")
			return nil
		}
		if generated && passwordFilePath != "" {
			_ = os.Remove(passwordFilePath)
		}
		return err
	}
	if generated {
		slog.Warn("no user exists; created root user with a generated one-time password file", "username", "root", "password_file", passwordFilePath, "reset_hint", "run with --reset-root-password-file or --reset-root-password-stdin if this file is not available")
	} else if passwordFromFile {
		slog.Warn("no user exists; created root user with password from file", "username", "root", "password_file", passwordFilePath, "reset_hint", "remove or rotate this bootstrap password after first login")
	} else {
		slog.Warn("no user exists; created root user with DUSHENGCDN_INITIAL_ROOT_PASSWORD", "username", "root", "reset_hint", "remove or rotate this bootstrap password after first login")
	}
	return nil
}

func initialRootPasswordFilePath() string {
	path := strings.TrimSpace(common.InitialRootPasswordFile)
	if path != "" {
		return path
	}
	sqlitePath := strings.TrimSpace(common.SQLitePath)
	if sqlitePath != "" {
		dir := filepath.Dir(sqlitePath)
		if dir != "" && dir != "." {
			return filepath.Join(dir, "initial-root-password.txt")
		}
	}
	return "initial-root-password.txt"
}

func readInitialRootPasswordFile(path string) (string, bool, error) {
	if strings.TrimSpace(path) == "" {
		return "", false, nil
	}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	content := strings.TrimSpace(string(raw))
	if content == "" {
		return "", true, fmt.Errorf("initial root password file is empty")
	}
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "password=") {
			password := strings.TrimSpace(strings.TrimPrefix(line, "password="))
			if password == "" {
				return "", true, fmt.Errorf("initial root password file has an empty password field")
			}
			return password, true, nil
		}
	}
	return content, true, nil
}

func writeInitialRootPasswordFile(path string, password string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("initial root password file path is empty")
	}
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = fmt.Fprintf(file, "username=root\npassword=%s\n", password)
	return err
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
	defer resetProxyRouteRuntimeSchemaCache(db)
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
	tables, err := db.Migrator().GetTables()
	if err != nil {
		return false, err
	}
	tableSet := make(map[string]struct{}, len(tables))
	for _, table := range tables {
		tableSet[strings.ToLower(strings.TrimSpace(table))] = struct{}{}
	}
	for _, item := range models {
		if isShardedObservabilityTable(item.tableName) {
			for _, table := range observabilityShardTables(item.tableName) {
				if !tableExistsInSet(tableSet, table) {
					continue
				}
				hasRows, err := tableHasRows(db, table)
				if err != nil {
					return false, err
				}
				if hasRows {
					return false, nil
				}
			}
			continue
		}
		if !tableExistsInSet(tableSet, item.tableName) {
			continue
		}
		hasRows, err := tableHasRows(db, item.tableName)
		if err != nil {
			return false, err
		}
		if hasRows {
			return false, nil
		}
	}
	return true, nil
}

func tableExistsInSet(tableSet map[string]struct{}, table string) bool {
	_, ok := tableSet[strings.ToLower(strings.TrimSpace(table))]
	return ok
}

func tableHasRows(db *gorm.DB, table string) (bool, error) {
	var value int
	if err := db.Table(table).Select("1").Limit(1).Scan(&value).Error; err != nil {
		return false, err
	}
	return value == 1, nil
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
			migratedTables, err := migrateTableData(source, tx, item)
			if err != nil {
				return err
			}
			if item.hasIDPK {
				for _, table := range migratedTables {
					if err := resetPostgresSequence(tx, table); err != nil {
						return err
					}
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

func migrateTableData(source *gorm.DB, target *gorm.DB, item dbModel) ([]string, error) {
	if isShardedObservabilityTable(item.tableName) {
		if sourceHasObservabilityShardTables(source, item.tableName) || !source.Migrator().HasTable(item.value) {
			return migrateShardedTableData(source, target, item)
		}
		migratedTableSet := make(map[string]struct{}, observabilityShardCount)
		migrated, err := migrateTableDataFromSource(source.Model(item.value), target, item, item.tableName, item.tableName, func(rows reflect.Value) error {
			for index := 0; index < rows.Len(); index++ {
				row := rows.Index(index)
				id, err := primaryIDValue(row)
				if err != nil {
					return err
				}
				migratedTableSet[observabilityShardTableForID(item.tableName, id)] = struct{}{}
				if err := target.Create(row.Addr().Interface()).Error; err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		if migrated {
			return migratedShardTables(item.tableName, migratedTableSet), nil
		}
		return nil, nil
	}
	if !source.Migrator().HasTable(item.value) {
		slog.Info("database migration progress", "table", item.tableName, "migrated", 0, "total", 0, "status", "skipped_missing_source_table")
		return nil, nil
	}
	if _, err := migrateTableDataFromSource(source.Model(item.value), target.Table(item.tableName), item, item.tableName, item.tableName, nil); err != nil {
		return nil, err
	}
	return []string{item.tableName}, nil
}

func sourceHasObservabilityShardTables(source *gorm.DB, baseTable string) bool {
	if source == nil {
		return false
	}
	for _, table := range observabilityShardTables(baseTable) {
		if source.Migrator().HasTable(table) {
			return true
		}
	}
	return false
}

func migrateShardedTableData(source *gorm.DB, target *gorm.DB, item dbModel) ([]string, error) {
	target = sessionIgnoringSharding(target)
	migratedTables := make([]string, 0)
	for _, table := range observabilityShardTables(item.tableName) {
		if !source.Migrator().HasTable(table) {
			slog.Info("database migration progress", "table", table, "migrated", 0, "total", 0, "status", "skipped_missing_source_table")
			continue
		}
		migrated, err := migrateTableDataFromSource(source.Table(table), target.Table(table), item, table, table, nil)
		if err != nil {
			return nil, err
		}
		if migrated {
			migratedTables = append(migratedTables, table)
		}
	}
	if len(migratedTables) == 0 {
		slog.Info("database migration progress", "table", item.tableName, "migrated", 0, "total", 0, "status", "skipped_missing_source_table")
	}
	return migratedTables, nil
}

func migratedShardTables(baseTable string, tableSet map[string]struct{}) []string {
	if len(tableSet) == 0 {
		return nil
	}
	tables := make([]string, 0, len(tableSet))
	for _, table := range observabilityShardTables(baseTable) {
		if _, ok := tableSet[table]; ok {
			tables = append(tables, table)
		}
	}
	return tables
}

func migrateTableDataFromSource(sourceQuery *gorm.DB, targetQuery *gorm.DB, item dbModel, sourceTable string, targetTable string, writeRows func(rows reflect.Value) error) (bool, error) {
	var total int64
	if err := sourceQuery.Count(&total).Error; err != nil {
		return false, fmt.Errorf("count sqlite table %s failed: %w", sourceTable, err)
	}
	slog.Info("database migration progress", "table", sourceTable, "target_table", targetTable, "migrated", 0, "total", total, "status", "starting")
	if total == 0 {
		slog.Info("database migration progress", "table", sourceTable, "target_table", targetTable, "migrated", 0, "total", total, "status", "completed")
		return false, nil
	}

	modelType := reflect.TypeOf(item.value).Elem()
	sliceType := reflect.SliceOf(modelType)
	migrated := int64(0)
	var lastPrimaryKey any
	hasLastPrimaryKey := false
	const batchSize = 200

	for {
		batchPtr := reflect.New(sliceType)
		query := sourceQuery.Limit(batchSize)
		if item.primaryColumnName != "" && item.primaryFieldName != "" {
			quotedPrimaryColumn := quoteIdentifier(item.primaryColumnName)
			query = query.Order(quotedPrimaryColumn + " ASC")
			if hasLastPrimaryKey {
				query = query.Where(quotedPrimaryColumn+" > ?", lastPrimaryKey)
			}
		} else {
			// This fallback is only for legacy tables without a single-column
			// primary key. All registered runtime tables currently have one.
			query = query.Offset(int(migrated))
		}
		if err := query.Find(batchPtr.Interface()).Error; err != nil {
			return false, fmt.Errorf("read sqlite table %s failed: %w", sourceTable, err)
		}
		batchLen := batchPtr.Elem().Len()
		if batchLen == 0 {
			break
		}
		batchRows := batchPtr.Elem()
		if item.primaryColumnName != "" && item.primaryFieldName != "" {
			value, err := primaryKeyCursorValue(batchRows.Index(batchLen-1), item.primaryFieldName)
			if err != nil {
				return false, fmt.Errorf("read primary key for sqlite table %s failed: %w", sourceTable, err)
			}
			lastPrimaryKey = value
			hasLastPrimaryKey = true
		}
		if writeRows != nil {
			if err := writeRows(batchRows); err != nil {
				return false, fmt.Errorf("write target table %s failed: %w", targetTable, err)
			}
		} else {
			if err := targetQuery.Create(batchPtr.Interface()).Error; err != nil {
				return false, fmt.Errorf("write target table %s failed: %w", targetTable, err)
			}
		}
		migrated += int64(batchLen)
		slog.Info("database migration progress", "table", sourceTable, "target_table", targetTable, "migrated", migrated, "total", total, "status", "running")
	}

	slog.Info("database migration progress", "table", sourceTable, "target_table", targetTable, "migrated", migrated, "total", total, "status", "completed")
	return migrated > 0, nil
}

func primaryKeyCursorValue(record reflect.Value, fieldName string) (any, error) {
	if record.Kind() == reflect.Pointer {
		record = record.Elem()
	}
	if record.Kind() != reflect.Struct {
		return nil, fmt.Errorf("expected struct record, got %s", record.Kind())
	}
	field := record.FieldByName(fieldName)
	if !field.IsValid() {
		return nil, fmt.Errorf("primary key field %s is missing", fieldName)
	}
	if field.Kind() == reflect.Pointer {
		if field.IsNil() {
			return nil, fmt.Errorf("primary key field %s is nil", fieldName)
		}
		field = field.Elem()
	}
	if !field.CanInterface() {
		return nil, fmt.Errorf("primary key field %s is not accessible", fieldName)
	}
	return field.Interface(), nil
}

func primaryIDValue(record reflect.Value) (uint, error) {
	if record.Kind() == reflect.Pointer {
		record = record.Elem()
	}
	if record.Kind() != reflect.Struct {
		return 0, fmt.Errorf("expected struct record, got %s", record.Kind())
	}
	field := record.FieldByName("ID")
	if !field.IsValid() {
		field = record.FieldByName("Id")
	}
	if !field.IsValid() {
		return 0, fmt.Errorf("id field is missing")
	}
	switch field.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		value := field.Int()
		if value < 0 {
			return 0, fmt.Errorf("id field is negative")
		}
		return uint(value), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return uint(field.Uint()), nil
	default:
		return 0, fmt.Errorf("id field has unsupported kind %s", field.Kind())
	}
}

func resetPostgresSequence(db *gorm.DB, tableName string) error {
	dialector := baseDialector(db)
	if dialector == nil || dialector.Name() != "postgres" {
		return nil
	}
	var sequence sql.NullString
	if err := db.Raw(`SELECT pg_get_serial_sequence(?, 'id')`, tableName).Scan(&sequence).Error; err != nil {
		return fmt.Errorf("lookup postgres sequence for %s failed: %w", tableName, err)
	}
	if !sequence.Valid || strings.TrimSpace(sequence.String) == "" {
		return nil
	}
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
