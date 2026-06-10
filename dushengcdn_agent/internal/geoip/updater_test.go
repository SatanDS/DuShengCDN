package geoip

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestUpdaterDownloadsCountryDatabase(t *testing.T) {
	payload := readTestMMDB(t)
	if int64(len(payload)) < minDatabaseSize {
		t.Fatalf("test mmdb fixture is too small: %d", len(payload))
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	target := filepath.Join(t.TempDir(), "GeoLite2-Country.mmdb")
	updater := &Updater{
		URL:      "https://geoip.example.test/GeoLite2-Country.mmdb",
		Path:     target,
		Interval: time.Hour,
		Client:   geoIPUpdaterTestClient(t, server),
	}
	if err := updater.Ensure(context.Background()); err != nil {
		t.Fatalf("Ensure failed: %v", err)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("expected database file to exist: %v", err)
	}
	if info.Size() != int64(len(payload)) {
		t.Fatalf("unexpected database size: got %d want %d", info.Size(), len(payload))
	}
}

func TestUpdaterRejectsNonMMDBDatabase(t *testing.T) {
	payload := make([]byte, minDatabaseSize+16)
	for index := range payload {
		payload[index] = byte(index % 251)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	target := filepath.Join(t.TempDir(), "GeoLite2-Country.mmdb")
	updater := &Updater{URL: "https://geoip.example.test/GeoLite2-Country.mmdb", Path: target, Client: geoIPUpdaterTestClient(t, server)}
	if err := updater.Ensure(context.Background()); err == nil {
		t.Fatal("expected non-mmdb database download to fail")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("expected non-mmdb database not to be activated, stat err = %v", err)
	}
}

func TestUpdaterRejectsTinyDatabase(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("tiny"))
	}))
	defer server.Close()

	target := filepath.Join(t.TempDir(), "GeoLite2-Country.mmdb")
	updater := &Updater{URL: "https://geoip.example.test/GeoLite2-Country.mmdb", Path: target, Client: geoIPUpdaterTestClient(t, server)}
	if err := updater.Ensure(context.Background()); err == nil {
		t.Fatal("expected tiny database download to fail")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("expected tiny database not to be activated, stat err = %v", err)
	}
}

func TestUpdaterRejectsOversizedDatabase(t *testing.T) {
	originalMax := maxDatabaseSize
	maxDatabaseSize = minDatabaseSize + 8
	t.Cleanup(func() {
		maxDatabaseSize = originalMax
	})

	payload := make([]byte, int(maxDatabaseSize)+1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	target := filepath.Join(t.TempDir(), "GeoLite2-Country.mmdb")
	updater := &Updater{URL: "https://geoip.example.test/GeoLite2-Country.mmdb", Path: target, Client: geoIPUpdaterTestClient(t, server)}
	if err := updater.Ensure(context.Background()); err == nil {
		t.Fatal("expected oversized database download to fail")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("expected oversized database not to be activated, stat err = %v", err)
	}
}

func TestUpdaterRejectsUnsafeDatabaseURL(t *testing.T) {
	target := filepath.Join(t.TempDir(), "GeoLite2-Country.mmdb")
	updater := &Updater{URL: "http://127.0.0.1/GeoLite2-Country.mmdb", Path: target}
	if err := updater.Download(context.Background()); err == nil {
		t.Fatal("expected unsafe database url to be rejected")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("expected unsafe download not to create target, stat err = %v", err)
	}
}

func readTestMMDB(t *testing.T) []byte {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to locate test file")
	}
	path := filepath.Join(filepath.Dir(file), "..", "..", "..", "dushengcdn_server", "service", "data", "GeoLite2-Country.mmdb")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read mmdb fixture: %v", err)
	}
	return data
}

func geoIPUpdaterTestClient(t *testing.T, server *httptest.Server) *http.Client {
	t.Helper()
	upstream, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse test server URL: %v", err)
	}
	client := server.Client()
	baseTransport := client.Transport
	if baseTransport == nil {
		baseTransport = http.DefaultTransport
	}
	client.Transport = geoIPUpdaterTestTransport{
		base:     baseTransport,
		upstream: upstream,
	}
	return client
}

type geoIPUpdaterTestTransport struct {
	base     http.RoundTripper
	upstream *url.URL
}

func (transport geoIPUpdaterTestTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	rewritten := request.Clone(request.Context())
	rewritten.URL = cloneTestURL(request.URL)
	rewritten.URL.Scheme = transport.upstream.Scheme
	rewritten.URL.Host = transport.upstream.Host
	rewritten.Host = request.URL.Host
	return transport.base.RoundTrip(rewritten)
}

func cloneTestURL(value *url.URL) *url.URL {
	if value == nil {
		return &url.URL{}
	}
	clone := *value
	return &clone
}
