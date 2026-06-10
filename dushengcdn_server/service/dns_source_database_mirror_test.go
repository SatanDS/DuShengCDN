package service

import (
	"context"
	"dushengcdn/common"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
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
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload, ok := payloads[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(payload)
	}))
	t.Cleanup(server.Close)
	defer SetDNSSourceDatabaseMirrorHTTPClientForTest(dnsMirrorClientForTLSServer(t, server))()

	common.DNSSourceDatabaseMirrorPath = root
	common.DNSSourceDatabaseCountryURL = "https://mirror.example.test/country.mmdb"
	common.DNSSourceDatabaseASNURL = "https://mirror.example.test/asn.mmdb"
	common.DNSSourceDatabaseOperatorCIDRBaseURL = "https://mirror.example.test"
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
	status := GetDNSSourceDatabaseMirrorStatus()
	if !status.Available || status.UpdatedAt == nil || status.SourceCount != 3 || status.FileCount != 5 || status.TotalSize <= 0 {
		t.Fatalf("unexpected mirror status: %+v", status)
	}
	if len(status.Sources) != 3 {
		t.Fatalf("expected per-source mirror status, got %+v", status.Sources)
	}
	for _, source := range status.Sources {
		if !source.Available || source.FileCount == 0 || source.TotalSize < 0 || source.UpdatedAt == nil {
			t.Fatalf("unexpected source mirror status: %+v", source)
		}
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

func TestGetDNSSourceDatabaseMirrorStatusMissing(t *testing.T) {
	root := t.TempDir()
	oldMirrorPath := common.DNSSourceDatabaseMirrorPath
	common.DNSSourceDatabaseMirrorPath = root
	t.Cleanup(func() {
		common.DNSSourceDatabaseMirrorPath = oldMirrorPath
	})

	status := GetDNSSourceDatabaseMirrorStatus()
	if status.Available || status.UpdatedAt != nil || len(status.MissingKinds) != 3 {
		t.Fatalf("expected missing mirror status, got %+v", status)
	}
	if len(status.Sources) != 3 {
		t.Fatalf("expected missing per-source mirror status, got %+v", status.Sources)
	}
	for _, source := range status.Sources {
		if source.Available || source.FileCount != 0 || source.TotalSize != 0 || source.UpdatedAt != nil {
			t.Fatalf("expected source mirror status to be missing, got %+v", source)
		}
	}
}

func TestOpenDNSSourceDatabaseMirrorFileRejectsManifestPathTraversal(t *testing.T) {
	root := t.TempDir()
	oldMirrorPath := common.DNSSourceDatabaseMirrorPath
	common.DNSSourceDatabaseMirrorPath = root
	t.Cleanup(func() {
		common.DNSSourceDatabaseMirrorPath = oldMirrorPath
	})

	current := filepath.Join(root, "current")
	if err := os.MkdirAll(filepath.Join(current, "operator"), 0o755); err != nil {
		t.Fatalf("create mirror dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "secret.txt"), []byte("secret"), 0o644); err != nil {
		t.Fatalf("write outside file: %v", err)
	}
	manifest := `{
  "updated_at": "2026-01-01T00:00:00Z",
  "sources": {
    "operator": {
      "kind": "operator",
      "updated_at": "2026-01-01T00:00:00Z",
      "files": [{
        "name": "chinanet.txt",
        "path": "../secret.txt",
        "size": 6,
        "sha256": "unused",
        "updated_at": "2026-01-01T00:00:00Z"
      }]
    }
  }
}`
	if err := os.WriteFile(filepath.Join(current, "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	file, _, err := OpenDNSSourceDatabaseMirrorFile("operator", "chinanet.txt")
	if err == nil {
		_ = file.Close()
		t.Fatal("expected manifest path traversal to be rejected")
	}
}

func TestRefreshDNSSourceDatabaseMirrorRejectsUnsafeDownloadURL(t *testing.T) {
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

	common.DNSSourceDatabaseMirrorPath = root
	common.DNSSourceDatabaseCountryURL = "http://169.254.169.254/latest/meta-data"
	common.DNSSourceDatabaseASNURL = "https://example.com/asn.mmdb"
	common.DNSSourceDatabaseOperatorCIDRBaseURL = "https://example.com"
	common.DNSSourceDatabaseOperatorCIDRFiles = "chinanet.txt"

	err := RefreshDNSSourceDatabaseMirror(context.Background())
	if err == nil {
		t.Fatal("expected unsafe DNS source database URL to be rejected")
	}
}

func TestRefreshDNSSourceDatabaseMirrorRejectsOversizedDownload(t *testing.T) {
	root := t.TempDir()
	oldMirrorPath := common.DNSSourceDatabaseMirrorPath
	oldCountryURL := common.DNSSourceDatabaseCountryURL
	oldASNURL := common.DNSSourceDatabaseASNURL
	oldOperatorBaseURL := common.DNSSourceDatabaseOperatorCIDRBaseURL
	oldOperatorFiles := common.DNSSourceDatabaseOperatorCIDRFiles
	oldMaxBytes := maxDNSSourceDatabaseMirrorBytes
	t.Cleanup(func() {
		common.DNSSourceDatabaseMirrorPath = oldMirrorPath
		common.DNSSourceDatabaseCountryURL = oldCountryURL
		common.DNSSourceDatabaseASNURL = oldASNURL
		common.DNSSourceDatabaseOperatorCIDRBaseURL = oldOperatorBaseURL
		common.DNSSourceDatabaseOperatorCIDRFiles = oldOperatorFiles
		maxDNSSourceDatabaseMirrorBytes = oldMaxBytes
	})

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "9")
		_, _ = w.Write([]byte("123456789"))
	}))
	t.Cleanup(server.Close)
	defer SetDNSSourceDatabaseMirrorHTTPClientForTest(dnsMirrorClientForTLSServer(t, server))()
	maxDNSSourceDatabaseMirrorBytes = 8
	common.DNSSourceDatabaseMirrorPath = root
	common.DNSSourceDatabaseCountryURL = "https://mirror.example.test/country.mmdb"
	common.DNSSourceDatabaseASNURL = "https://mirror.example.test/asn.mmdb"
	common.DNSSourceDatabaseOperatorCIDRBaseURL = "https://mirror.example.test"
	common.DNSSourceDatabaseOperatorCIDRFiles = "chinanet.txt"

	err := RefreshDNSSourceDatabaseMirror(context.Background())
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("expected oversized DNS source database download to be rejected, got %v", err)
	}
}

func bytesOfSize(value byte, size int) []byte {
	data := make([]byte, size)
	for i := range data {
		data[i] = value
	}
	return data
}

func dnsMirrorClientForTLSServer(t *testing.T, server *httptest.Server) *http.Client {
	t.Helper()
	upstream, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse TLS server URL: %v", err)
	}
	baseClient := server.Client()
	baseTransport := baseClient.Transport
	if baseTransport == nil {
		baseTransport = http.DefaultTransport
	}
	baseClient.Transport = dnsMirrorTestTransport{
		base:     baseTransport,
		upstream: upstream,
	}
	return baseClient
}

type dnsMirrorTestTransport struct {
	base     http.RoundTripper
	upstream *url.URL
}

func (transport dnsMirrorTestTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	rewritten := request.Clone(request.Context())
	rewritten.URL = cloneURL(request.URL)
	rewritten.URL.Scheme = transport.upstream.Scheme
	rewritten.URL.Host = transport.upstream.Host
	rewritten.Host = request.URL.Host
	return transport.base.RoundTrip(rewritten)
}
