package model

import (
	"dushengcdn/common"
	"testing"
)

func TestInitOptionMapKeepsDefaultServerAddressWhenUnset(t *testing.T) {
	oldDB := DB
	oldServerAddress := common.ServerAddress
	common.ServerAddress = "http://localhost:3000"
	t.Cleanup(func() {
		DB = oldDB
		common.ServerAddress = oldServerAddress
	})

	db := openBareTestSQLiteDB(t, "option-map.db")
	if err := db.AutoMigrate(&Option{}); err != nil {
		t.Fatalf("migrate option table: %v", err)
	}
	DB = db
	InitOptionMap()

	if common.GetOptionValue("ServerAddress") != "http://localhost:3000" {
		t.Fatalf("expected option map default server address, got %q", common.GetOptionValue("ServerAddress"))
	}
	if common.ServerAddress != "http://localhost:3000" {
		t.Fatalf("expected common server address to remain default, got %q", common.ServerAddress)
	}
}

func TestAgentLegacyGlobalTokenOptionHotReloads(t *testing.T) {
	oldDB := DB
	oldLegacyEnabled := common.AgentLegacyGlobalTokenEnabled
	oldOptionMap := common.OptionMapSnapshot()
	common.AgentLegacyGlobalTokenEnabled = false
	t.Cleanup(func() {
		DB = oldDB
		common.AgentLegacyGlobalTokenEnabled = oldLegacyEnabled
		common.OptionMapRWMutex.Lock()
		common.OptionMap = oldOptionMap
		common.OptionMapRWMutex.Unlock()
	})

	db := openBareTestSQLiteDB(t, "agent-legacy-token-option.db")
	if err := db.AutoMigrate(&Option{}); err != nil {
		t.Fatalf("migrate option table: %v", err)
	}
	DB = db
	InitOptionMap()

	if common.GetOptionValue("AgentLegacyGlobalTokenEnabled") != "false" {
		t.Fatalf("expected legacy token option default false, got %q", common.GetOptionValue("AgentLegacyGlobalTokenEnabled"))
	}
	if err := UpdateOption("AgentLegacyGlobalTokenEnabled", "true"); err != nil {
		t.Fatalf("update legacy token option: %v", err)
	}
	if !common.AgentLegacyGlobalTokenEnabled {
		t.Fatal("expected legacy token option to hot-reload common flag")
	}
	common.AgentLegacyGlobalTokenEnabled = false
	if err := UpdateOption("AgentLegacyGlobalAuthEnabled", "true"); err != nil {
		t.Fatalf("update legacy alias option: %v", err)
	}
	if !common.AgentLegacyGlobalTokenEnabled {
		t.Fatal("expected legacy alias option to hot-reload common flag")
	}
}

func TestInitOptionMapDefaultsUploadsToAuthenticatedUsers(t *testing.T) {
	oldDB := DB
	oldOptionMap := common.OptionMapSnapshot()
	oldFileUploadPermission := common.FileUploadPermission
	oldImageUploadPermission := common.ImageUploadPermission
	t.Cleanup(func() {
		DB = oldDB
		common.FileUploadPermission = oldFileUploadPermission
		common.ImageUploadPermission = oldImageUploadPermission
		common.OptionMapRWMutex.Lock()
		common.OptionMap = oldOptionMap
		common.OptionMapRWMutex.Unlock()
	})

	db := openBareTestSQLiteDB(t, "upload-permission-defaults.db")
	if err := db.AutoMigrate(&Option{}); err != nil {
		t.Fatalf("migrate option table: %v", err)
	}
	DB = db
	InitOptionMap()

	expected := "1"
	if common.GetOptionValue("FileUploadPermission") != expected {
		t.Fatalf("expected file upload default permission %s, got %q", expected, common.GetOptionValue("FileUploadPermission"))
	}
	if common.GetOptionValue("ImageUploadPermission") != expected {
		t.Fatalf("expected image upload default permission %s, got %q", expected, common.GetOptionValue("ImageUploadPermission"))
	}
}

func TestInitOptionMapLoadsGeoIPProviderWithoutReload(t *testing.T) {
	oldDB := DB
	oldGeoIPProvider := common.GeoIPProvider
	oldOptionMap := common.OptionMapSnapshot()
	oldRefreshGeoIPProvider := refreshGeoIPProvider
	common.GeoIPProvider = "ipinfo"
	refreshCount := 0
	refreshGeoIPProvider = func() {
		refreshCount++
	}
	t.Cleanup(func() {
		DB = oldDB
		common.GeoIPProvider = oldGeoIPProvider
		common.OptionMapRWMutex.Lock()
		common.OptionMap = oldOptionMap
		common.OptionMapRWMutex.Unlock()
		refreshGeoIPProvider = oldRefreshGeoIPProvider
	})

	db := openBareTestSQLiteDB(t, "geoip-provider-init-option.db")
	if err := db.AutoMigrate(&Option{}); err != nil {
		t.Fatalf("migrate option table: %v", err)
	}
	if err := db.Create(&Option{Key: "GeoIPProvider", Value: "geojs"}).Error; err != nil {
		t.Fatalf("seed geoip provider option: %v", err)
	}
	DB = db

	InitOptionMap()

	if common.GeoIPProvider != "geojs" {
		t.Fatalf("expected GeoIP provider to load from options, got %q", common.GeoIPProvider)
	}
	if common.GetOptionValue("GeoIPProvider") != "geojs" {
		t.Fatalf("expected GeoIP provider in option map, got %q", common.GetOptionValue("GeoIPProvider"))
	}
	if refreshCount != 0 {
		t.Fatalf("expected InitOptionMap not to refresh GeoIP provider, got %d refreshes", refreshCount)
	}
}

func TestUpdateOptionReloadsGeoIPProviderOnlyWhenChanged(t *testing.T) {
	oldDB := DB
	oldGeoIPProvider := common.GeoIPProvider
	oldOptionMap := common.OptionMapSnapshot()
	oldRefreshGeoIPProvider := refreshGeoIPProvider
	common.GeoIPProvider = "ipinfo"
	refreshCount := 0
	refreshGeoIPProvider = func() {
		refreshCount++
	}
	t.Cleanup(func() {
		DB = oldDB
		common.GeoIPProvider = oldGeoIPProvider
		common.OptionMapRWMutex.Lock()
		common.OptionMap = oldOptionMap
		common.OptionMapRWMutex.Unlock()
		refreshGeoIPProvider = oldRefreshGeoIPProvider
	})

	db := openBareTestSQLiteDB(t, "geoip-provider-update-option.db")
	if err := db.AutoMigrate(&Option{}); err != nil {
		t.Fatalf("migrate option table: %v", err)
	}
	DB = db

	if err := UpdateOption("GeoIPProvider", "ipinfo"); err != nil {
		t.Fatalf("update unchanged geoip provider: %v", err)
	}
	if refreshCount != 0 {
		t.Fatalf("expected unchanged GeoIP provider not to refresh, got %d refreshes", refreshCount)
	}
	if err := UpdateOption("GeoIPProvider", "geojs"); err != nil {
		t.Fatalf("update changed geoip provider: %v", err)
	}
	if refreshCount != 1 {
		t.Fatalf("expected changed GeoIP provider to refresh once, got %d refreshes", refreshCount)
	}
	if err := UpdateOption("GeoIPProvider", "geojs"); err != nil {
		t.Fatalf("update same geoip provider again: %v", err)
	}
	if refreshCount != 1 {
		t.Fatalf("expected repeated GeoIP provider update not to refresh again, got %d refreshes", refreshCount)
	}
}

func TestUpdateOptionsUpsertsExistingRowsAndCompactsDuplicateKeys(t *testing.T) {
	oldDB := DB
	oldOptionMap := common.OptionMapSnapshot()
	t.Cleanup(func() {
		DB = oldDB
		common.OptionMapRWMutex.Lock()
		common.OptionMap = oldOptionMap
		common.OptionMapRWMutex.Unlock()
	})

	db := openBareTestSQLiteDB(t, "option-upsert-existing.db")
	if err := db.AutoMigrate(&Option{}); err != nil {
		t.Fatalf("migrate option table: %v", err)
	}
	if err := db.Create(&Option{Key: "ExistingOptionKey", Value: "old"}).Error; err != nil {
		t.Fatalf("seed existing option: %v", err)
	}
	DB = db

	if err := UpdateOptions([]Option{
		{Key: "ExistingOptionKey", Value: "first"},
		{Key: "NewOptionKey", Value: "created"},
		{Key: "ExistingOptionKey", Value: "last"},
	}); err != nil {
		t.Fatalf("UpdateOptions failed: %v", err)
	}

	var options []Option
	if err := db.Order("key asc").Find(&options).Error; err != nil {
		t.Fatalf("load options: %v", err)
	}
	if len(options) != 2 {
		t.Fatalf("expected 2 option rows after upsert, got %+v", options)
	}
	values := map[string]string{}
	for _, option := range options {
		values[option.Key] = option.Value
	}
	if values["ExistingOptionKey"] != "last" {
		t.Fatalf("expected existing option to keep last duplicate value, got %q", values["ExistingOptionKey"])
	}
	if values["NewOptionKey"] != "created" {
		t.Fatalf("expected new option to be inserted, got %q", values["NewOptionKey"])
	}
}
