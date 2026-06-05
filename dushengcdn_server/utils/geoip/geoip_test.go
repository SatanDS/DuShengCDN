package geoip

import (
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeProvider struct {
	calls int
}

func (f *fakeProvider) Name() string {
	return "fake"
}

func (f *fakeProvider) GetGeoInfo(ip net.IP) (*GeoInfo, error) {
	f.calls++
	return &GeoInfo{
		ISOCode: "CN",
		Name:    "China",
	}, nil
}

func (f *fakeProvider) UpdateDatabase() error {
	return nil
}

func (f *fakeProvider) Close() error {
	return nil
}

func TestGetGeoInfoCachesByProviderAndIP(t *testing.T) {
	originalProvider := CurrentProvider
	geoCache.Flush()
	fake := &fakeProvider{}
	CurrentProvider = fake
	defer func() {
		CurrentProvider = originalProvider
	}()

	ip := net.ParseIP("8.8.8.8")
	record, err := GetGeoInfo(ip)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if record == nil || record.ISOCode != "CN" {
		t.Fatalf("expected cached record, got %#v", record)
	}

	_, err = GetGeoInfo(ip)
	if err != nil {
		t.Fatalf("expected nil error on second call, got %v", err)
	}
	if fake.calls != 1 {
		t.Fatalf("expected provider to be called once, got %d", fake.calls)
	}
}

func TestGetGeoInfoWorksWhenCacheUnavailable(t *testing.T) {
	originalProvider := CurrentProvider
	originalCache := geoCache
	fake := &fakeProvider{}
	CurrentProvider = fake
	geoCache = &providerCache{duration: time.Hour}
	defer func() {
		CurrentProvider = originalProvider
		geoCache = originalCache
	}()

	ip := net.ParseIP("8.8.4.4")
	first, err := GetGeoInfo(ip)
	if err != nil {
		t.Fatalf("expected first lookup without cache to succeed, got %v", err)
	}
	second, err := GetGeoInfo(ip)
	if err != nil {
		t.Fatalf("expected second lookup without cache to succeed, got %v", err)
	}
	if first == nil || second == nil || first.ISOCode != "CN" || second.ISOCode != "CN" {
		t.Fatalf("unexpected geo records: %#v %#v", first, second)
	}
	if fake.calls != 2 {
		t.Fatalf("expected provider to be called for each uncached lookup, got %d", fake.calls)
	}
}

func TestUnicodeEmoji(t *testing.T) {
	emoji := GetRegionUnicodeEmoji("CN")
	if emoji != "🇨🇳" {
		t.Errorf("expected emoji for CN, got %s", emoji)
	}
}

func TestIsValidProvider(t *testing.T) {
	cases := map[string]bool{
		"disabled": true,
		"mmdb":     true,
		"ip-api":   true,
		"geojs":    true,
		"ipinfo":   true,
		"unknown":  false,
	}

	for provider, want := range cases {
		if got := IsValidProvider(provider); got != want {
			t.Fatalf("provider %s validity mismatch: want %v, got %v", provider, want, got)
		}
	}
}

func TestLookupGeoInfoWithProviderUsesTemporaryProvider(t *testing.T) {
	previousFactory := providerFactory
	providerFactory = func(provider string) (GeoIPService, error) {
		return &fakeProvider{}, nil
	}
	defer func() {
		providerFactory = previousFactory
	}()

	info, err := LookupGeoInfoWithProvider("ipinfo", net.ParseIP("8.8.8.8"))
	if err != nil {
		t.Fatalf("expected lookup to succeed, got %v", err)
	}
	if info == nil || info.ISOCode != "CN" || info.Name != "China" {
		t.Fatalf("unexpected geo info: %#v", info)
	}
}

func TestMaxMindUpdateDatabaseRejectsOversizedDownload(t *testing.T) {
	originalURL := GeoIpUrl
	originalMax := maxGeoIPDatabaseDownloadBytes
	originalClient := geoIPDownloadHTTPClient
	maxGeoIPDatabaseDownloadBytes = 8
	t.Cleanup(func() {
		GeoIpUrl = originalURL
		maxGeoIPDatabaseDownloadBytes = originalMax
		geoIPDownloadHTTPClient = originalClient
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != "DuShengCDN-Server" {
			t.Fatalf("unexpected user agent: %s", r.Header.Get("User-Agent"))
		}
		w.Header().Set("Content-Length", "9")
		_, _ = w.Write([]byte("123456789"))
	}))
	defer server.Close()
	GeoIpUrl = server.URL

	service := &MaxMindGeoIPService{dbFilePath: filepath.Join(t.TempDir(), "GeoLite2-Country.mmdb")}
	err := service.UpdateDatabase()
	if err == nil || !strings.Contains(err.Error(), "response exceeds") {
		t.Fatalf("expected oversized database error, got %v", err)
	}
}

func TestMaxMindUpdateDatabaseRejectsInvalidDatabaseWithoutReplacingExisting(t *testing.T) {
	originalURL := GeoIpUrl
	originalClient := geoIPDownloadHTTPClient
	t.Cleanup(func() {
		GeoIpUrl = originalURL
		geoIPDownloadHTTPClient = originalClient
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not a mmdb"))
	}))
	defer server.Close()
	GeoIpUrl = server.URL

	target := filepath.Join(t.TempDir(), "GeoLite2-Country.mmdb")
	if err := os.WriteFile(target, []byte("existing database"), 0o644); err != nil {
		t.Fatalf("write existing database: %v", err)
	}

	service := &MaxMindGeoIPService{dbFilePath: target}
	err := service.UpdateDatabase()
	if err == nil || !strings.Contains(err.Error(), "validate MaxMind database") {
		t.Fatalf("expected invalid database validation error, got %v", err)
	}
	data, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatalf("read existing database: %v", readErr)
	}
	if string(data) != "existing database" {
		t.Fatalf("expected existing database to remain unchanged, got %q", string(data))
	}
}
