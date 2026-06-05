package service

import (
	"dushengcdn/common"
	"dushengcdn/model"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"

	"gorm.io/gorm"
)

func TestPostgresCommercialLicenseQuotaSerializesConcurrentNodeCreates(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("POSTGRES_TEST_DSN"))
	if dsn == "" {
		t.Skip("set POSTGRES_TEST_DSN to run Postgres commercial quota integration test")
	}
	setupPostgresServiceTestDB(t, dsn)
	withCommercialLicenseTestConfig(t, true, "", true)

	token := buildUnsignedCommercialLicenseToken(t, CommercialLicensePayload{
		LicenseID:    "lic-postgres-node-quota",
		CustomerName: "Quota Ltd.",
		Plan:         "business",
		MaxNodes:     1,
	})
	if _, err := InstallCommercialLicense(CommercialLicenseInstallInput{Token: token}); err != nil {
		t.Fatalf("InstallCommercialLicense failed: %v", err)
	}

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, name := range []string{"pg-edge-a", "pg-edge-b"} {
		wg.Add(1)
		go func(nodeName string) {
			defer wg.Done()
			_, err := CreateNode(NodeInput{Name: nodeName, IP: "203.0.113.10"})
			errs <- err
		}(name)
	}
	wg.Wait()
	close(errs)

	successes := 0
	failures := 0
	for err := range errs {
		if err == nil {
			successes++
			continue
		}
		if !strings.Contains(err.Error(), "节点") {
			t.Fatalf("expected node quota error, got %v", err)
		}
		failures++
	}
	if successes != 1 || failures != 1 {
		t.Fatalf("expected one successful create and one quota failure, got successes=%d failures=%d", successes, failures)
	}
	var count int64
	if err := model.DB.Model(&model.Node{}).Count(&count).Error; err != nil {
		t.Fatalf("count nodes: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected exactly one node after concurrent Postgres creates, got %d", count)
	}
}

func setupPostgresServiceTestDB(t *testing.T, dsn string) {
	t.Helper()
	nodeAgentTokenCache.reset()
	previousSQLDSN := common.SQLDSN
	previousSQLitePath := common.SQLitePath
	common.SQLDSN = dsn
	common.SQLitePath = ""
	if err := model.InitDB(); err != nil {
		t.Fatalf("failed to init postgres test db: %v", err)
	}
	t.Cleanup(func() {
		nodeAgentTokenCache.reset()
		if err := model.CloseDB(); err != nil {
			t.Fatalf("failed to close postgres test db: %v", err)
		}
		common.SQLDSN = previousSQLDSN
		common.SQLitePath = previousSQLitePath
	})
	truncatePostgresServiceTestDB(t)
}

func truncatePostgresServiceTestDB(t *testing.T) {
	t.Helper()
	var tables []string
	if err := model.DB.
		Table("information_schema.tables").
		Where("table_schema = ? AND table_type = ?", "public", "BASE TABLE").
		Pluck("table_name", &tables).Error; err != nil {
		t.Fatalf("list postgres tables: %v", err)
	}
	if len(tables) == 0 {
		return
	}
	quoted := make([]string, 0, len(tables))
	for _, table := range tables {
		table = strings.TrimSpace(table)
		if table == "" {
			continue
		}
		quoted = append(quoted, fmt.Sprintf("%q", table))
	}
	if len(quoted) == 0 {
		return
	}
	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		return tx.Exec("TRUNCATE TABLE " + strings.Join(quoted, ", ") + " RESTART IDENTITY CASCADE").Error
	}); err != nil {
		t.Fatalf("truncate postgres tables: %v", err)
	}
}
