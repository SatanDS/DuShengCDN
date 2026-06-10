package main

import (
	"crypto/sha256"
	"encoding/hex"
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
	output := runCreateDNSWorkerCLI(t, dbPath, "DNS服务响应端", "8.8.8.8")
	token := lastNonEmptyLine(output)
	if token == "" {
		t.Fatalf("expected CLI to print DNS Worker token, got %q", output)
	}

	db := openDNSWorkerCLIDB(t, dbPath)
	var worker model.DNSWorker
	if err := db.Where("name = ?", "DNS服务响应端").First(&worker).Error; err != nil {
		t.Fatalf("load created DNS worker: %v", err)
	}
	if worker.PublicAddress != "8.8.8.8:53" {
		t.Fatalf("public address = %q, want 8.8.8.8:53", worker.PublicAddress)
	}
	if worker.Token != "" {
		t.Fatalf("expected stored plaintext token to be empty, got %q", worker.Token)
	}
	if want := sha256Hex(token); worker.TokenHash != want {
		t.Fatalf("stored token hash = %q, want %q", worker.TokenHash, want)
	}
	if want := tokenPrefix(token); worker.TokenPrefix != want {
		t.Fatalf("stored token prefix = %q, want %q", worker.TokenPrefix, want)
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

func sha256Hex(value string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(sum[:])
}

func tokenPrefix(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 12 {
		return value[:12]
	}
	return value
}
