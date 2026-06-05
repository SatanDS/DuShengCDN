package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"dushengcdn/model"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestCreateDNSWorkerCLI(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping go run based CLI test in short mode")
	}

	dbPath := filepath.Join(t.TempDir(), "create-dns-worker-cli.db")
	output := runCreateDNSWorkerCLI(t, dbPath, "DNS服务响应端", "203.0.113.10")
	token := lastNonEmptyLine(output)
	if token == "" {
		t.Fatalf("expected CLI to print DNS Worker token, got %q", output)
	}

	db := openDNSWorkerCLIDB(t, dbPath)
	var worker model.DNSWorker
	if err := db.Where("name = ?", "DNS服务响应端").First(&worker).Error; err != nil {
		t.Fatalf("load created DNS worker: %v", err)
	}
	if worker.PublicAddress != "203.0.113.10" {
		t.Fatalf("public address = %q, want 203.0.113.10", worker.PublicAddress)
	}
	if worker.Token != token {
		t.Fatalf("stored token = %q, CLI token = %q", worker.Token, token)
	}
	if worker.WorkerID == "" {
		t.Fatal("expected worker id to be generated")
	}
}

func runCreateDNSWorkerCLI(t *testing.T, dbPath string, name string, publicAddress string) string {
	t.Helper()

	cmd := exec.Command("go", "run", ".",
		"--create-dns-worker-name", name,
		"--create-dns-worker-public-address", publicAddress,
	)
	cmd.Dir = "."
	cmd.Env = append(os.Environ(),
		"SQLITE_PATH="+dbPath,
		"SESSION_SECRET=test-session-secret-1234567890abcdef",
		"LOG_LEVEL=error",
		"GIN_MODE=release",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("create DNS worker CLI failed: %v\n%s", err, output)
	}
	return string(output)
}

func openDNSWorkerCLIDB(t *testing.T, dbPath string) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open create DNS worker CLI database: %v", err)
	}
	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func lastNonEmptyLine(output string) string {
	lines := strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			return line
		}
	}
	return ""
}
