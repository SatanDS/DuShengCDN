package model

import (
	"dushengcdn/common"
	"os"
	"path/filepath"
	"testing"
)

func TestSafeUploadFilePathRejectsTraversal(t *testing.T) {
	previousUploadPath := common.UploadPath
	common.UploadPath = t.TempDir()
	t.Cleanup(func() {
		common.UploadPath = previousUploadPath
	})

	if _, err := safeUploadFilePath("../secret.txt"); err == nil {
		t.Fatal("expected traversal link to be rejected")
	}
	if _, err := safeUploadFilePath(filepath.Join("nested", "..", "..", "secret.txt")); err == nil {
		t.Fatal("expected cleaned traversal link to be rejected")
	}
}

func TestFileDeleteRemovesOnlyUploadPathFile(t *testing.T) {
	previousUploadPath := common.UploadPath
	common.UploadPath = t.TempDir()
	t.Cleanup(func() {
		common.UploadPath = previousUploadPath
	})
	db := openTestSQLiteDB(t, "file-delete.db")
	previousDB := DB
	DB = db
	t.Cleanup(func() {
		DB = previousDB
	})

	if err := os.MkdirAll(filepath.Join(common.UploadPath, "nested"), 0o755); err != nil {
		t.Fatalf("failed to create nested upload dir: %v", err)
	}
	filePath := filepath.Join(common.UploadPath, "nested", "file.txt")
	if err := os.WriteFile(filePath, []byte("data"), 0o644); err != nil {
		t.Fatalf("failed to write upload file: %v", err)
	}

	file := &File{Filename: "file.txt", Link: filepath.Join("nested", "file.txt")}
	if err := file.Insert(); err != nil {
		t.Fatalf("failed to insert file record: %v", err)
	}
	if err := file.Delete(); err != nil {
		t.Fatalf("expected delete to succeed: %v", err)
	}
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Fatalf("expected upload file to be removed, got %v", err)
	}
}
