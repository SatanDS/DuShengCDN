package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"dushengcdn/common"
	"dushengcdn/model"
	"dushengcdn/utils/security"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestResetRootPasswordCLI(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping go run based CLI test in short mode")
	}

	dbPath := filepath.Join(t.TempDir(), "reset-root-cli.db")
	runResetRootPasswordCLI(t, dbPath, "cli-new-password")

	db := openResetRootCLIDB(t, dbPath)
	assertRootPassword(t, db, "cli-new-password", true)
	assertRootPassword(t, db, "123456", false)

	if err := db.Model(&model.User{}).Where("username = ?", "root").Updates(map[string]any{
		"role":   common.RoleCommonUser,
		"status": common.UserStatusDisabled,
	}).Error; err != nil {
		t.Fatalf("demote root before second reset: %v", err)
	}

	runResetRootPasswordCLI(t, dbPath, "cli-second-password")
	assertRootPassword(t, db, "cli-second-password", true)

	var root model.User
	if err := db.Where("username = ?", "root").First(&root).Error; err != nil {
		t.Fatalf("load root after second reset: %v", err)
	}
	if root.Role != common.RoleRootUser || root.Status != common.UserStatusEnabled {
		t.Fatalf("expected reset CLI to enable root role, got role=%d status=%d", root.Role, root.Status)
	}
}

func runResetRootPasswordCLI(t *testing.T, dbPath string, password string) {
	t.Helper()

	cmd := exec.Command("go", "run", ".", "--reset-root-password", password)
	cmd.Dir = "."
	cmd.Env = append(os.Environ(),
		"SQLITE_PATH="+dbPath,
		"SESSION_SECRET=test-session-secret",
		"LOG_LEVEL=error",
		"GIN_MODE=release",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("reset root CLI failed: %v\n%s", err, output)
	}
}

func openResetRootCLIDB(t *testing.T, dbPath string) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open reset root CLI database: %v", err)
	}
	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func assertRootPassword(t *testing.T, db *gorm.DB, password string, wantValid bool) {
	t.Helper()

	var root model.User
	if err := db.Where("username = ?", "root").First(&root).Error; err != nil {
		t.Fatalf("load root user: %v", err)
	}
	valid := security.ValidatePasswordAndHash(password, root.Password)
	if valid != wantValid {
		t.Fatalf("password %q validity = %v, want %v", password, valid, wantValid)
	}
}
