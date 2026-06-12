package common

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

const importSideEffectHelperEnv = "DUSHENGCDN_COMMON_IMPORT_SIDE_EFFECT_HELPER"

func TestImportCommonDoesNotRunServerRuntimeInitialization(t *testing.T) {
	if os.Getenv(importSideEffectHelperEnv) == "1" {
		assertCommonImportHasNoRuntimeSideEffects(t)
		return
	}

	uploadPath := filepath.Join(t.TempDir(), "runtime-upload")
	cmd := exec.Command("go", "test", ".", "-run", "^TestImportCommonDoesNotRunServerRuntimeInitialization$", "-count=1")
	cmd.Env = append(os.Environ(),
		importSideEffectHelperEnv+"=1",
		"UPLOAD_PATH="+uploadPath,
		"SQL_DSN=postgres://dushengcdn:secret@example.invalid:5432/dushengcdn",
		"LOG_LEVEL=debug",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("import side-effect helper failed: %v\n%s", err, output)
	}
	if _, err := os.Stat(uploadPath); !os.IsNotExist(err) {
		t.Fatalf("expected import-only subprocess not to create UPLOAD_PATH %q, stat err=%v", uploadPath, err)
	}
}

func assertCommonImportHasNoRuntimeSideEffects(t *testing.T) {
	t.Helper()

	if UploadPath != "upload" {
		t.Fatalf("UploadPath = %q, want default %q before explicit initialization", UploadPath, "upload")
	}
	if SQLDSN != "" {
		t.Fatalf("SQLDSN = %q, want empty before explicit initialization", SQLDSN)
	}
	if got := GetLogLevel(); got != "info" {
		t.Fatalf("log level = %q, want default %q before explicit initialization", got, "info")
	}
	if _, err := os.Stat(os.Getenv("UPLOAD_PATH")); !os.IsNotExist(err) {
		t.Fatalf("expected import-only subprocess not to create UPLOAD_PATH, stat err=%v", err)
	}
}

func TestPrepareRuntimeDirectoriesCreatesConfiguredDirectories(t *testing.T) {
	previousUploadPath := UploadPath
	previousLogDir := *LogDir
	t.Cleanup(func() {
		UploadPath = previousUploadPath
		*LogDir = previousLogDir
	})

	root := t.TempDir()
	UploadPath = filepath.Join(root, "upload")
	*LogDir = filepath.Join(root, "logs")

	if err := prepareRuntimeDirectories(); err != nil {
		t.Fatalf("prepareRuntimeDirectories failed: %v", err)
	}
	assertDirectoryExists(t, UploadPath)
	assertDirectoryExists(t, *LogDir)
	if !filepath.IsAbs(*LogDir) {
		t.Fatalf("LogDir = %q, want absolute path", *LogDir)
	}
}

func assertDirectoryExists(t *testing.T, path string) {
	t.Helper()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %q: %v", path, err)
	}
	if !info.IsDir() {
		t.Fatalf("%q is not a directory", path)
	}
}
