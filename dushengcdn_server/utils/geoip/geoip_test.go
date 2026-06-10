package geoip

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
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
	GeoIpUrl = "https://geoip.example.test/GeoLite2-Country.mmdb"
	geoIPDownloadHTTPClient = geoIPDownloadTestClient(t, server)

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
	GeoIpUrl = "https://geoip.example.test/GeoLite2-Country.mmdb"
	geoIPDownloadHTTPClient = geoIPDownloadTestClient(t, server)

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

func TestMaxMindUpdateDatabaseRejectsUnsafeURL(t *testing.T) {
	originalURL := GeoIpUrl
	t.Cleanup(func() {
		GeoIpUrl = originalURL
	})
	GeoIpUrl = "http://127.0.0.1/GeoLite2-Country.mmdb"

	service := &MaxMindGeoIPService{dbFilePath: filepath.Join(t.TempDir(), "GeoLite2-Country.mmdb")}
	err := service.UpdateDatabase()
	if err == nil || !strings.Contains(err.Error(), "url must use https") {
		t.Fatalf("expected unsafe URL rejection, got %v", err)
	}
}

func TestMaxMindUpdateDatabaseCreatesRestrictedDataDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("windows does not expose POSIX directory mode bits consistently")
	}
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
	GeoIpUrl = "https://geoip.example.test/GeoLite2-Country.mmdb"
	geoIPDownloadHTTPClient = geoIPDownloadTestClient(t, server)

	dataDir := filepath.Join(t.TempDir(), "geoip-data")
	service := &MaxMindGeoIPService{dbFilePath: filepath.Join(dataDir, "GeoLite2-Country.mmdb")}
	err := service.UpdateDatabase()
	if err == nil || !strings.Contains(err.Error(), "validate MaxMind database") {
		t.Fatalf("expected invalid database validation error, got %v", err)
	}

	info, statErr := os.Stat(dataDir)
	if statErr != nil {
		t.Fatalf("expected geoip data directory to be created: %v", statErr)
	}
	if got := info.Mode().Perm(); got != 0o750 {
		t.Fatalf("expected geoip data directory mode 0750, got %03o", got)
	}
}

func TestOnlineGeoIPProvidersUseHTTPSAndUserAgent(t *testing.T) {
	tests := []struct {
		name     string
		service  GeoIPService
		response any
	}{
		{
			name:    "ip-api",
			service: &IPAPIService{},
			response: ipAPIResponse{
				Status:      "success",
				Country:     "United States",
				CountryCode: "US",
				ISP:         "Example ISP",
			},
		},
		{
			name:    "geojs",
			service: &GeoJSService{},
			response: geoJSResponse{
				Country:     "United States",
				CountryCode: "US",
			},
		},
		{
			name:    "ipinfo",
			service: &IPInfoService{},
			response: ipInfoResponse{
				Country: "US",
				Org:     "Example Org",
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var originalScheme string
			var originalHost string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("User-Agent") != "DuShengCDN-Server" {
					t.Fatalf("unexpected user agent: %s", r.Header.Get("User-Agent"))
				}
				_ = json.NewEncoder(w).Encode(tc.response)
			}))
			defer server.Close()
			client := onlineGeoIPTestClient(t, server, func(r *http.Request) {
				originalScheme = r.URL.Scheme
				originalHost = r.URL.Host
			})
			switch service := tc.service.(type) {
			case *IPAPIService:
				service.Client = client
			case *GeoJSService:
				service.Client = client
			case *IPInfoService:
				service.Client = client
			default:
				t.Fatalf("unsupported service type %T", tc.service)
			}

			info, err := tc.service.GetGeoInfo(net.ParseIP("8.8.8.8"))
			if err != nil {
				t.Fatalf("GetGeoInfo: %v", err)
			}
			if info == nil || info.ISOCode != "US" {
				t.Fatalf("unexpected geo info: %+v", info)
			}
			if originalScheme != "https" || originalHost == "" {
				t.Fatalf("expected provider request to use HTTPS, got scheme=%q host=%q", originalScheme, originalHost)
			}
		})
	}
}

func geoIPDownloadTestClient(t *testing.T, server *httptest.Server) *http.Client {
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
	client.Transport = geoIPDownloadTestTransport{
		base:     baseTransport,
		upstream: upstream,
	}
	return client
}

type geoIPDownloadTestTransport struct {
	base     http.RoundTripper
	upstream *url.URL
}

func (transport geoIPDownloadTestTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	rewritten := *request.URL
	rewritten.Scheme = transport.upstream.Scheme
	rewritten.Host = transport.upstream.Host
	clone.URL = &rewritten
	clone.Host = transport.upstream.Host
	return transport.base.RoundTrip(clone)
}

func onlineGeoIPTestClient(t *testing.T, server *httptest.Server, observe func(*http.Request)) *http.Client {
	t.Helper()
	upstream, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse test server URL: %v", err)
	}
	return &http.Client{
		Transport: onlineGeoIPTestTransport{
			base:     http.DefaultTransport,
			upstream: upstream,
			observe:  observe,
		},
	}
}

type onlineGeoIPTestTransport struct {
	base     http.RoundTripper
	upstream *url.URL
	observe  func(*http.Request)
}

func (transport onlineGeoIPTestTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if transport.observe != nil {
		transport.observe(request)
	}
	clone := request.Clone(request.Context())
	rewritten := *request.URL
	rewritten.Scheme = transport.upstream.Scheme
	rewritten.Host = transport.upstream.Host
	clone.URL = &rewritten
	clone.Host = transport.upstream.Host
	return transport.base.RoundTrip(clone)
}
