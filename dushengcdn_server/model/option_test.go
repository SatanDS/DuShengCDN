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
