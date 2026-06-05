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
