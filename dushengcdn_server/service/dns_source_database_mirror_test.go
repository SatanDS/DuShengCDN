package service

import (
	"context"
	"dushengcdn/common"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRefreshDNSSourceDatabaseMirrorPublishesCurrentAndRemovesPrevious(t *testing.T) {
	root := t.TempDir()
	oldMirrorPath := common.DNSSourceDatabaseMirrorPath
	oldCountryURL := common.DNSSourceDatabaseCountryURL
	oldASNURL := common.DNSSourceDatabaseASNURL
	oldOperatorBaseURL := common.DNSSourceDatabaseOperatorCIDRBaseURL
	oldOperatorFiles := common.DNSSourceDatabaseOperatorCIDRFiles
	t.Cleanup(func() {
		common.DNSSourceDatabaseMirrorPath = oldMirrorPath
		common.DNSSourceDatabaseCountryURL = oldCountryURL
		common.DNSSourceDatabaseASNURL = oldASNURL
		common.DNSSourceDatabaseOperatorCIDRBaseURL = oldOperatorBaseURL
		common.DNSSourceDatabaseOperatorCIDRFiles = oldOperatorFiles
	})

	payloads := map[string][]byte{
		"/country.mmdb": bytesOfSize('c', 2048),
		"/asn.mmdb":     bytesOfSize('a', 2048),
		"/chinanet.txt": []byte("1.0.1.0/24\n1.0.1.1/32\n"),
		"/cmcc.txt":     []byte("1.0.2.0/24\n1.0.2.1/32\n"),
		"/drpeng6.txt":  []byte{},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload, ok := payloads[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(payload)
	}))
	t.Cleanup(server.Close)

	common.DNSSourceDatabaseMirrorPath = root
	common.DNSSourceDatabaseCountryURL = server.URL + "/country.mmdb"
	common.DNSSourceDatabaseASNURL = server.URL + "/asn.mmdb"
	common.DNSSourceDatabaseOperatorCIDRBaseURL = server.URL
	common.DNSSourceDatabaseOperatorCIDRFiles = "chinanet.txt cmcc.txt drpeng6.txt"

	if err := RefreshDNSSourceDatabaseMirror(context.Background()); err != nil {
		t.Fatalf("RefreshDNSSourceDatabaseMirror first run: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "current", "stale.txt"), []byte("old"), 0o644); err != nil {
		t.Fatalf("write stale file: %v", err)
	}
	orphanedStaging := filepath.Join(filepath.Dir(root), ".dns-source-databases.orphaned")
	if err := os.MkdirAll(orphanedStaging, 0o755); err != nil {
		t.Fatalf("create orphaned staging: %v", err)
	}
	oldTime := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(orphanedStaging, oldTime, oldTime); err != nil {
		t.Fatalf("age orphaned staging: %v", err)
	}
	payloads["/chinanet.txt"] = []byte("1.0.3.0/24\n1.0.3.1/32\n")
	if err := RefreshDNSSourceDatabaseMirror(context.Background()); err != nil {
		t.Fatalf("RefreshDNSSourceDatabaseMirror second run: %v", err)
	}

	if _, err := os.Stat(filepath.Join(root, "previous")); !os.IsNotExist(err) {
		t.Fatalf("expected previous mirror directory to be removed, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "current", "stale.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected stale file from old mirror to be removed, err=%v", err)
	}
	if _, err := os.Stat(orphanedStaging); !os.IsNotExist(err) {
		t.Fatalf("expected orphaned staging directory to be removed, err=%v", err)
	}
	manifest, err := GetDNSSourceDatabaseMirrorManifest()
	if err != nil {
		t.Fatalf("GetDNSSourceDatabaseMirrorManifest: %v", err)
	}
	if len(manifest.Sources) != 3 {
		t.Fatalf("expected three mirrored sources, got %+v", manifest.Sources)
	}
	file, meta, err := OpenDNSSourceDatabaseMirrorFile("operator", "chinanet.txt")
	if err != nil {
		t.Fatalf("OpenDNSSourceDatabaseMirrorFile: %v", err)
	}
	defer file.Close()
	if meta.Name != "chinanet.txt" || meta.SHA256 == "" || meta.Size == 0 {
		t.Fatalf("unexpected operator mirror file metadata: %+v", meta)
	}
	emptyFile, emptyMeta, err := OpenDNSSourceDatabaseMirrorFile("operator", "drpeng6.txt")
	if err != nil {
		t.Fatalf("OpenDNSSourceDatabaseMirrorFile empty operator file: %v", err)
	}
	defer emptyFile.Close()
	if emptyMeta.Name != "drpeng6.txt" || emptyMeta.SHA256 == "" || emptyMeta.Size != 0 {
		t.Fatalf("unexpected empty operator mirror file metadata: %+v", emptyMeta)
	}
}

func bytesOfSize(value byte, size int) []byte {
	data := make([]byte, size)
	for i := range data {
		data[i] = value
	}
	return data
}
