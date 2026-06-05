package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"dushengcdn/common"
	"dushengcdn/model"

	"gorm.io/gorm"
)

func TestNormalizeProxyRouteDNSSettingsRequiresCloudflareAccount(t *testing.T) {
	setupServiceTestDB(t)

	if _, _, _, _, _, _, _, err := normalizeProxyRouteDNSSettings(ProxyRouteInput{
		DNSAutoSync: true,
	}); err == nil {
		t.Fatal("expected missing dns account to fail")
	}

	account := &model.DnsAccount{
		Name:          "cf",
		Type:          "cloudflare",
		Authorization: `{"api_token":"token"}`,
	}
	if err := account.Insert(); err != nil {
		t.Fatalf("insert dns account: %v", err)
	}

	accountID, zoneID, recordType, recordName, recordContent, autoTarget, ddosMode, err := normalizeProxyRouteDNSSettings(ProxyRouteInput{
		DNSAutoSync:        true,
		DNSAccountID:       &account.ID,
		DNSZoneID:          "zone-a",
		DNSRecordType:      "AAAA",
		DNSRecordName:      "app.example.com",
		DNSRecordContent:   "2001:4860:4860::8888",
		DDOSProtectionMode: "auto",
	})
	if err != nil {
		t.Fatalf("normalize dns settings: %v", err)
	}
	if accountID == nil || *accountID != account.ID {
		t.Fatalf("unexpected account id: %#v", accountID)
	}
	if zoneID != "zone-a" || recordType != "AAAA" || recordName != "app.example.com" || recordContent != "2001:4860:4860::8888" || autoTarget || ddosMode != DDOSProtectionModeAuto {
		t.Fatalf("unexpected normalized values: %q %q %q %q %t %q", zoneID, recordType, recordName, recordContent, autoTarget, ddosMode)
	}
}

func TestSelectHealthyNodeDNSContent(t *testing.T) {
	setupServiceTestDB(t)

	oldThreshold := common.NodeOfflineThreshold
	common.NodeOfflineThreshold = time.Minute
	t.Cleanup(func() {
		common.NodeOfflineThreshold = oldThreshold
	})

	offline := &model.Node{
		NodeID:       "node-offline",
		Name:         "offline",
		IP:           "8.8.4.4",
		AgentToken:   "token-offline",
		AgentVersion: "dev",
		Status:       NodeStatusOnline,
		LastSeenAt:   time.Now().Add(-5 * time.Minute),
	}
	if err := offline.Insert(); err != nil {
		t.Fatalf("insert offline node: %v", err)
	}

	online := &model.Node{
		NodeID:          "node-online",
		Name:            "online",
		IP:              "8.8.8.8",
		AgentToken:      "token-online",
		AgentVersion:    "dev",
		OpenrestyStatus: OpenrestyStatusHealthy,
		Status:          NodeStatusOnline,
		LastSeenAt:      time.Now(),
	}
	if err := online.Insert(); err != nil {
		t.Fatalf("insert online node: %v", err)
	}

	content, err := selectHealthyNodeDNSContent("A")
	if err != nil {
		t.Fatalf("select healthy node: %v", err)
	}
	if content != "8.8.8.8" {
		t.Fatalf("expected online node ip, got %s", content)
	}
}

func TestSelectHealthyNodeDNSContentSwitchesAwayFromOfflineNode(t *testing.T) {
	setupServiceTestDB(t)

	oldThreshold := common.NodeOfflineThreshold
	common.NodeOfflineThreshold = time.Minute
	t.Cleanup(func() {
		common.NodeOfflineThreshold = oldThreshold
	})

	previousTarget := &model.Node{
		NodeID:          "node-previous-target",
		Name:            "previous-target",
		IP:              "8.8.4.4",
		AgentToken:      "token-previous-target",
		AgentVersion:    "dev",
		OpenrestyStatus: OpenrestyStatusHealthy,
		Status:          NodeStatusOnline,
		LastSeenAt:      time.Now().Add(-10 * time.Minute),
	}
	if err := previousTarget.Insert(); err != nil {
		t.Fatalf("insert previous target node: %v", err)
	}

	nextTarget := &model.Node{
		NodeID:          "node-next-target",
		Name:            "next-target",
		IP:              "1.1.1.1",
		AgentToken:      "token-next-target",
		AgentVersion:    "dev",
		OpenrestyStatus: OpenrestyStatusHealthy,
		Status:          NodeStatusOnline,
		LastSeenAt:      time.Now(),
	}
	if err := nextTarget.Insert(); err != nil {
		t.Fatalf("insert next target node: %v", err)
	}

	content, err := selectHealthyNodeDNSContent("A")
	if err != nil {
		t.Fatalf("select healthy node: %v", err)
	}
	if content != "1.1.1.1" {
		t.Fatalf("expected DNS target to switch to online node, got %s", content)
	}
}

func TestShouldEnableCloudflareProxyForDDOS(t *testing.T) {
	setupServiceTestDB(t)
	setCloudflareDDoSThresholdsForTest(t, "100", "50")

	now := time.Now()
	if err := (&model.NodeRequestReport{
		NodeID:          "node-ddos",
		WindowStartedAt: now.Add(-time.Minute),
		WindowEndedAt:   now,
		RequestCount:    120,
		ErrorCount:      1,
	}).Insert(); err != nil {
		t.Fatalf("insert request report: %v", err)
	}

	if !shouldEnableCloudflareProxyForDDOS() {
		t.Fatal("expected request threshold to enable Cloudflare proxy")
	}
}

func TestShouldEnableCloudflareProxyForDDOSErrorRate(t *testing.T) {
	setupServiceTestDB(t)
	setCloudflareDDoSThresholdsForTest(t, "1000", "30")

	now := time.Now()
	if err := (&model.NodeRequestReport{
		NodeID:          "node-ddos-errors",
		WindowStartedAt: now.Add(-time.Minute),
		WindowEndedAt:   now,
		RequestCount:    100,
		ErrorCount:      40,
	}).Insert(); err != nil {
		t.Fatalf("insert request report: %v", err)
	}

	if !shouldEnableCloudflareProxyForDDOS() {
		t.Fatal("expected error rate threshold to enable Cloudflare proxy")
	}
}

func TestEffectiveCloudflareProxiedForDDOSProviders(t *testing.T) {
	route := &model.ProxyRoute{
		CloudflareProxied:      false,
		DDOSProtectionProvider: DDOSProtectionProviderCloudflare,
	}
	if !effectiveCloudflareProxied(route, true) {
		t.Fatal("expected Cloudflare DDoS provider to force proxied records")
	}

	route.CloudflareProxied = true
	route.DDOSProtectionProvider = DDOSProtectionProviderCustom
	if effectiveCloudflareProxied(route, true) {
		t.Fatal("expected custom DDoS provider to disable Cloudflare proxy during override")
	}

	if !effectiveCloudflareProxied(route, false) {
		t.Fatal("expected normal sync to keep configured Cloudflare proxy state")
	}
}

func TestProxyRouteDNSSyncContextBatchesDNSAccounts(t *testing.T) {
	setupServiceTestDB(t)

	account := &model.DnsAccount{
		Name:          "cf",
		Type:          "cloudflare",
		Authorization: `{"api_token":"token"}`,
	}
	if err := account.Insert(); err != nil {
		t.Fatalf("insert dns account: %v", err)
	}
	routes := []*model.ProxyRoute{
		{
			DNSAutoSync:     true,
			DNSProviderMode: DNSProviderModeCloudflare,
			DNSAccountID:    &account.ID,
		},
		{
			DNSAutoSync:     true,
			DNSProviderMode: DNSProviderModeCloudflare,
			DNSAccountID:    &account.ID,
		},
		{
			DNSAutoSync:            true,
			DNSProviderMode:        DNSProviderModeCloudflare,
			DNSAccountID:           &account.ID,
			DDOSProtectionMode:     DDOSProtectionModeAuto,
			DDOSProtectionProvider: DDOSProtectionProviderCloudflare,
			DDOSProtectionTarget:   "999999",
		},
	}

	const callbackName = "dushengcdn:test_dns_account_query_counter"
	var accountQueries int64
	queryCallback := model.DB.Callback().Query()
	if err := queryCallback.After("gorm:query").Register(callbackName, func(db *gorm.DB) {
		if db == nil || db.Statement == nil {
			return
		}
		if db.Statement.Table == "dns_accounts" ||
			(db.Statement.Schema != nil && db.Statement.Schema.Table == "dns_accounts") ||
			strings.Contains(db.Statement.SQL.String(), "dns_accounts") {
			atomic.AddInt64(&accountQueries, 1)
		}
	}); err != nil {
		t.Fatalf("register query callback: %v", err)
	}
	t.Cleanup(func() {
		_ = queryCallback.Remove(callbackName)
	})

	context, err := newProxyRouteDNSSyncContext(routes)
	if err != nil {
		t.Fatalf("newProxyRouteDNSSyncContext failed: %v", err)
	}
	if got := atomic.LoadInt64(&accountQueries); got != 1 {
		t.Fatalf("expected one batched DNS account query, got %d", got)
	}
	resolved, err := proxyRouteDNSAccountForSyncWithContext(routes[0], false, context)
	if err != nil {
		t.Fatalf("resolve route account from context failed: %v", err)
	}
	if resolved.ID != account.ID {
		t.Fatalf("expected account %d, got %d", account.ID, resolved.ID)
	}
	if _, err := proxyRouteDNSAccountForSyncWithContext(routes[2], true, context); err == nil {
		t.Fatal("expected missing DDoS override account to fail from context")
	}
	if got := atomic.LoadInt64(&accountQueries); got != 1 {
		t.Fatalf("expected context lookups to avoid extra DNS account queries, got %d", got)
	}
}

func TestProxyRouteDNSSyncContextCachesDDoSProtectionSummary(t *testing.T) {
	previous := getRequestReportTrafficSummaryForDNSProtection
	var calls int64
	getRequestReportTrafficSummaryForDNSProtection = func(since time.Time, until time.Time) (*model.NodeRequestReportTrafficSummary, error) {
		atomic.AddInt64(&calls, 1)
		return &model.NodeRequestReportTrafficSummary{RequestCount: 120, ErrorCount: 1}, nil
	}
	t.Cleanup(func() {
		getRequestReportTrafficSummaryForDNSProtection = previous
	})
	setCloudflareDDoSThresholdsForTest(t, "100", "50")

	context := &proxyRouteDNSSyncContext{}
	route := &model.ProxyRoute{DDOSProtectionMode: DDOSProtectionModeAuto}
	if !routeDDOSProtectionActiveWithContext(route, context) {
		t.Fatal("expected first DDoS protection decision to be active")
	}
	if !routeDDOSProtectionActiveWithContext(route, context) {
		t.Fatal("expected cached DDoS protection decision to remain active")
	}
	if got := atomic.LoadInt64(&calls); got != 1 {
		t.Fatalf("expected one traffic summary load, got %d", got)
	}
}

func TestSelectProxyRouteDDOSProtectionTargetsUsesCustomPool(t *testing.T) {
	setupServiceTestDB(t)

	oldThreshold := common.NodeOfflineThreshold
	common.NodeOfflineThreshold = time.Minute
	t.Cleanup(func() {
		common.NodeOfflineThreshold = oldThreshold
	})

	now := time.Now()
	nodes := []*model.Node{
		{
			NodeID:          "node-default",
			Name:            "default",
			IP:              "8.8.8.8",
			PoolName:        "default",
			PublicIPs:       `["8.8.8.8"]`,
			AgentToken:      "token-default",
			AgentVersion:    "dev",
			OpenrestyStatus: OpenrestyStatusHealthy,
			Status:          NodeStatusOnline,
			LastSeenAt:      now,
		},
		{
			NodeID:          "node-clean",
			Name:            "clean",
			IP:              "1.1.1.1",
			PoolName:        "anti-ddos",
			PublicIPs:       `["1.1.1.1"]`,
			AgentToken:      "token-clean",
			AgentVersion:    "dev",
			OpenrestyStatus: OpenrestyStatusHealthy,
			Status:          NodeStatusOnline,
			LastSeenAt:      now,
		},
	}
	for _, node := range nodes {
		if err := node.Insert(); err != nil {
			t.Fatalf("insert node: %v", err)
		}
	}

	route := &model.ProxyRoute{
		NodePool:               "default",
		DDOSProtectionTarget:   "anti-ddos",
		DNSTargetCount:         1,
		DNSScheduleMode:        "healthy",
		DNSTTL:                 60,
		DDOSProtectionProvider: DDOSProtectionProviderCustom,
		DDOSProtectionMode:     DDOSProtectionModeAuto,
		DNSRecordType:          "A",
		DNSProviderMode:        DNSProviderModeCloudflare,
		CloudflareProxied:      true,
	}

	selection, err := selectProxyRouteDDOSProtectionTargets(route, "A")
	if err != nil {
		t.Fatalf("select custom ddos targets: %v", err)
	}
	if len(selection.Targets) != 1 || selection.Targets[0] != "1.1.1.1" {
		t.Fatalf("expected custom pool target, got %#v", selection.Targets)
	}
	if selection.GSLB {
		t.Fatal("expected DDoS override to pause GSLB")
	}
}

func setCloudflareDDoSThresholdsForTest(t *testing.T, requestThreshold string, errorRateThreshold string) {
	t.Helper()

	common.OptionMapRWMutex.Lock()
	wasNil := common.OptionMap == nil
	if wasNil {
		common.OptionMap = make(map[string]string)
	}
	oldRequestThreshold, hadRequestThreshold := common.OptionMap["CloudflareDDoSRequestThreshold"]
	oldErrorRateThreshold, hadErrorRateThreshold := common.OptionMap["CloudflareDDoSErrorRateThreshold"]
	common.OptionMap["CloudflareDDoSRequestThreshold"] = requestThreshold
	common.OptionMap["CloudflareDDoSErrorRateThreshold"] = errorRateThreshold
	common.OptionMapRWMutex.Unlock()

	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		defer common.OptionMapRWMutex.Unlock()
		if wasNil {
			common.OptionMap = nil
			return
		}
		if hadRequestThreshold {
			common.OptionMap["CloudflareDDoSRequestThreshold"] = oldRequestThreshold
		} else {
			delete(common.OptionMap, "CloudflareDDoSRequestThreshold")
		}
		if hadErrorRateThreshold {
			common.OptionMap["CloudflareDDoSErrorRateThreshold"] = oldErrorRateThreshold
		} else {
			delete(common.OptionMap, "CloudflareDDoSErrorRateThreshold")
		}
	})
}

func TestValidateDNSRecordContent(t *testing.T) {
	if err := validateDNSRecordContent("A", "2001:4860:4860::8888"); err == nil {
		t.Fatal("expected ipv6 content to fail for A record")
	}
	if err := validateDNSRecordContent("AAAA", "8.8.8.8"); err == nil {
		t.Fatal("expected ipv4 content to fail for AAAA record")
	}
	if err := validateDNSRecordContent("CNAME", "target.example.com"); err != nil {
		t.Fatalf("expected cname target to pass: %v", err)
	}
}

func TestParseCloudflareAPITokenVariants(t *testing.T) {
	cases := map[string]string{
		`raw-token`:                  "raw-token",
		` Bearer bearer-token `:      "bearer-token",
		`{"api_token":"json-token"}`: "json-token",
		`{"apiToken":"camel-token"}`: "camel-token",
		`{"token":"short-token"}`:    "short-token",
		`"quoted-token"`:             "quoted-token",
		"line-\nwrapped":             "line-wrapped",
	}
	for raw, want := range cases {
		if got := parseCloudflareAPIToken(raw); got != want {
			t.Fatalf("parseCloudflareAPIToken(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestNormalizeDNSAccountAuthorization(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{name: "raw", raw: "raw-token", want: `{"api_token":"raw-token"}`},
		{name: "bearer", raw: " Bearer bearer-token ", want: `{"api_token":"bearer-token"}`},
		{name: "json api_token", raw: `{"api_token":"json-token"}`, want: `{"api_token":"json-token"}`},
		{name: "json apiToken", raw: `{"apiToken":"camel-token"}`, want: `{"api_token":"camel-token"}`},
		{name: "quoted", raw: `"quoted-token"`, want: `{"api_token":"quoted-token"}`},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			account := &model.DnsAccount{
				Type:          " CloudFlare ",
				Authorization: tt.raw,
			}
			if err := NormalizeDNSAccountAuthorization(account); err != nil {
				t.Fatalf("normalize authorization: %v", err)
			}
			if account.Type != "cloudflare" {
				t.Fatalf("expected normalized type cloudflare, got %q", account.Type)
			}
			if account.Authorization != tt.want {
				t.Fatalf("authorization = %q, want %q", account.Authorization, tt.want)
			}
		})
	}
}

func TestNormalizeDNSAccountAuthorizationRejectsInvalidInput(t *testing.T) {
	cases := []struct {
		name    string
		account *model.DnsAccount
	}{
		{name: "nil", account: nil},
		{name: "empty type", account: &model.DnsAccount{Type: "", Authorization: "token"}},
		{name: "unsupported type", account: &model.DnsAccount{Type: "route53", Authorization: "token"}},
		{name: "empty token", account: &model.DnsAccount{Type: "cloudflare", Authorization: " "}},
		{name: "invalid json token", account: &model.DnsAccount{Type: "cloudflare", Authorization: `{"api_token":`}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if err := NormalizeDNSAccountAuthorization(tt.account); err == nil {
				t.Fatal("expected invalid input to fail")
			}
		})
	}
}

func TestCloudflareListDNSRecordsFetchesAllPages(t *testing.T) {
	requestedPages := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/zones/zone-a/dns_records" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if r.URL.Query().Get("type") != "A" || r.URL.Query().Get("name") != "app.example.com" || r.URL.Query().Get("per_page") != "100" {
			t.Fatalf("unexpected query %s", r.URL.RawQuery)
		}
		page := r.URL.Query().Get("page")
		requestedPages = append(requestedPages, page)
		w.Header().Set("Content-Type", "application/json")
		switch page {
		case "1":
			_, _ = w.Write([]byte(`{"success":true,"result":[{"id":"r1","type":"A","name":"app.example.com","content":"1.1.1.1"}],"result_info":{"page":1,"per_page":100,"total_pages":2,"count":1,"total_count":2}}`))
		case "2":
			_, _ = w.Write([]byte(`{"success":true,"result":[{"id":"r2","type":"A","name":"app.example.com","content":"8.8.8.8"}],"result_info":{"page":2,"per_page":100,"total_pages":2,"count":1,"total_count":2}}`))
		default:
			t.Fatalf("unexpected page %q", page)
		}
	}))
	t.Cleanup(server.Close)

	client := &cloudflareClient{
		apiToken:   "token",
		baseURL:    server.URL,
		httpClient: server.Client(),
	}
	records, err := client.ListDNSRecords(context.Background(), "zone-a", "A", "App.Example.Com.")
	if err != nil {
		t.Fatalf("list dns records: %v", err)
	}
	if len(records) != 2 || records[0].ID != "r1" || records[1].ID != "r2" {
		t.Fatalf("unexpected records: %#v", records)
	}
	if strings.Join(requestedPages, ",") != "1,2" {
		t.Fatalf("expected pages 1,2, got %v", requestedPages)
	}
}

func TestCloudflareListDNSRecordsRejectsTooManyPages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"result":[],"result_info":{"page":1,"per_page":100,"total_pages":101,"count":0,"total_count":10001}}`))
	}))
	t.Cleanup(server.Close)

	client := &cloudflareClient{
		apiToken:   "token",
		baseURL:    server.URL,
		httpClient: server.Client(),
	}
	_, err := client.ListDNSRecords(context.Background(), "zone-a", "A", "app.example.com")
	if err == nil || !strings.Contains(err.Error(), "page limit") {
		t.Fatalf("expected page limit error, got %v", err)
	}
}

func TestNormalizeGSLBPolicyRejectsHTTPSourceProviderForAuthoritativeDNS(t *testing.T) {
	_, err := normalizeGSLBPolicy(ProxyRouteGSLBPolicy{
		SourceIP: ProxyRouteGSLBSourceIPProvider{
			Provider: "http",
			APIURL:   "https://geo.example.com/{ip}",
		},
	}, "default", 1, "healthy", 60)
	if err == nil || !strings.Contains(err.Error(), "provider=http is not supported") {
		t.Fatalf("expected unsupported http source provider error, got %v", err)
	}
}

func TestNormalizeGSLBPolicyRejectsSourceProviderSecretsWithoutProvider(t *testing.T) {
	_, err := normalizeGSLBPolicy(ProxyRouteGSLBPolicy{
		SourceIP: ProxyRouteGSLBSourceIPProvider{
			APIURL:   "https://geo.example.com/{ip}",
			APIToken: "secret",
		},
	}, "default", 1, "healthy", 60)
	if err == nil || !strings.Contains(err.Error(), "provider is required") {
		t.Fatalf("expected source provider requirement error, got %v", err)
	}

	_, err = normalizeGSLBPolicy(ProxyRouteGSLBPolicy{
		SourceIP: ProxyRouteGSLBSourceIPProvider{
			Provider: "none",
			APIURL:   "https://geo.example.com/{ip}",
		},
	}, "default", 1, "healthy", 60)
	if err == nil || !strings.Contains(err.Error(), "provider=none cannot set api_url") {
		t.Fatalf("expected source provider none secret error, got %v", err)
	}
}

func TestDecodeStoredGSLBPolicyDowngradesHTTPSourceProvider(t *testing.T) {
	policy, err := decodeStoredGSLBPolicy(`{
		"mode":"cloudflare_dns",
		"strategy":"healthy",
		"target_count":1,
		"ttl":60,
		"source_ip":{"provider":"http","api_url":"https://geo.example.com/{ip}","api_token":"secret"},
		"pools":[{"name":"default","weight":100,"enabled":true}]
	}`)
	if err != nil {
		t.Fatalf("decode stored gslb policy: %v", err)
	}
	if policy.SourceIP.Provider != gslbSourceProviderNone || policy.SourceIP.APIURL != "" || policy.SourceIP.APIToken != "" {
		t.Fatalf("expected stored http provider to downgrade to none, got %+v", policy.SourceIP)
	}
}

func TestDecodeStoredGSLBPolicyClearsUnusedSourceProviderFields(t *testing.T) {
	policy, err := decodeStoredGSLBPolicy(`{
		"source_ip":{"provider":"none","api_url":"https://geo.example.com/{ip}","api_token":"secret"},
		"pools":[{"name":"default","weight":100,"enabled":true}]
	}`)
	if err != nil {
		t.Fatalf("decode stored gslb policy: %v", err)
	}
	if policy.SourceIP.Provider != gslbSourceProviderNone || policy.SourceIP.APIURL != "" || policy.SourceIP.APIToken != "" {
		t.Fatalf("expected unused provider fields to be cleared, got %+v", policy.SourceIP)
	}
}

func TestSelectHealthyNodeDNSTargetsByPoolAndWeight(t *testing.T) {
	setupServiceTestDB(t)

	oldThreshold := common.NodeOfflineThreshold
	common.NodeOfflineThreshold = time.Minute
	t.Cleanup(func() {
		common.NodeOfflineThreshold = oldThreshold
	})

	nodes := []*model.Node{
		{
			NodeID:          "node-a",
			Name:            "a",
			IP:              "8.8.8.8",
			PoolName:        "edge",
			PublicIPs:       `["8.8.8.8","2001:4860:4860::8888"]`,
			Weight:          100,
			AgentToken:      "token-a",
			AgentVersion:    "dev",
			OpenrestyStatus: OpenrestyStatusHealthy,
			Status:          NodeStatusOnline,
			LastSeenAt:      time.Now(),
		},
		{
			NodeID:          "node-b",
			Name:            "b",
			IP:              "1.1.1.1",
			PoolName:        "edge",
			PublicIPs:       `["1.1.1.1"]`,
			Weight:          500,
			AgentToken:      "token-b",
			AgentVersion:    "dev",
			OpenrestyStatus: OpenrestyStatusHealthy,
			Status:          NodeStatusOnline,
			LastSeenAt:      time.Now().Add(-time.Second),
		},
		{
			NodeID:          "node-c",
			Name:            "c",
			IP:              "9.9.9.9",
			PoolName:        "other",
			Weight:          1000,
			AgentToken:      "token-c",
			AgentVersion:    "dev",
			OpenrestyStatus: OpenrestyStatusHealthy,
			Status:          NodeStatusOnline,
			LastSeenAt:      time.Now(),
		},
	}
	for _, node := range nodes {
		if err := node.Insert(); err != nil {
			t.Fatalf("insert node: %v", err)
		}
	}

	targets, err := selectHealthyNodeDNSTargets("A", "edge", 2, "weighted")
	if err != nil {
		t.Fatalf("select targets: %v", err)
	}
	if len(targets) != 2 || targets[0] != "1.1.1.1" || targets[1] != "8.8.8.8" {
		t.Fatalf("unexpected targets: %#v", targets)
	}
}

func TestSelectGSLBDNSTargetsAcrossWeightedPools(t *testing.T) {
	setupServiceTestDB(t)

	oldThreshold := common.NodeOfflineThreshold
	common.NodeOfflineThreshold = time.Minute
	t.Cleanup(func() {
		common.NodeOfflineThreshold = oldThreshold
	})

	now := time.Now()
	nodes := []*model.Node{
		{
			NodeID:          "node-hk-a",
			Name:            "hk-a",
			IP:              "8.8.8.8",
			PoolName:        "hk",
			PublicIPs:       `["8.8.8.8"]`,
			Weight:          100,
			AgentToken:      "token-hk-a",
			AgentVersion:    "dev",
			OpenrestyStatus: OpenrestyStatusHealthy,
			Status:          NodeStatusOnline,
			LastSeenAt:      now,
		},
		{
			NodeID:          "node-hk-b",
			Name:            "hk-b",
			IP:              "1.1.1.1",
			PoolName:        "hk",
			PublicIPs:       `["1.1.1.1"]`,
			Weight:          90,
			AgentToken:      "token-hk-b",
			AgentVersion:    "dev",
			OpenrestyStatus: OpenrestyStatusHealthy,
			Status:          NodeStatusOnline,
			LastSeenAt:      now.Add(-time.Second),
		},
		{
			NodeID:          "node-eu-a",
			Name:            "eu-a",
			IP:              "9.9.9.9",
			PoolName:        "eu",
			PublicIPs:       `["9.9.9.9"]`,
			Weight:          100,
			AgentToken:      "token-eu-a",
			AgentVersion:    "dev",
			OpenrestyStatus: OpenrestyStatusHealthy,
			Status:          NodeStatusOnline,
			LastSeenAt:      now,
		},
	}
	for _, node := range nodes {
		if err := node.Insert(); err != nil {
			t.Fatalf("insert node: %v", err)
		}
	}

	policy := defaultGSLBPolicy("hk", 10, "weighted", 60)
	policy.Pools = []ProxyRouteGSLBPoolPolicy{
		{Name: "hk", Weight: 80, Enabled: true},
		{Name: "eu", Weight: 20, Enabled: true},
	}
	rawPolicy, err := json.Marshal(policy)
	if err != nil {
		t.Fatalf("marshal policy: %v", err)
	}
	route := &model.ProxyRoute{
		ID:              99,
		NodePool:        "hk",
		DNSTargetCount:  10,
		DNSScheduleMode: "weighted",
		DNSTTL:          60,
		GSLBEnabled:     true,
		GSLBPolicy:      string(rawPolicy),
	}

	selection, err := selectGSLBDNSTargets(route, "A")
	if err != nil {
		t.Fatalf("select gslb targets: %v", err)
	}
	if len(selection.Targets) != 3 {
		t.Fatalf("expected all available targets, got %#v", selection.Targets)
	}
	if selection.Targets[0] != "8.8.8.8" || selection.Targets[1] != "1.1.1.1" || selection.Targets[2] != "9.9.9.9" {
		t.Fatalf("unexpected weighted target order: %#v", selection.Targets)
	}
	if selection.TTL != 60 || !selection.GSLB {
		t.Fatalf("unexpected selection metadata: %#v", selection)
	}
}

func TestSelectGSLBDNSTargetsSkipsOverloadedNode(t *testing.T) {
	setupServiceTestDB(t)

	oldThreshold := common.NodeOfflineThreshold
	common.NodeOfflineThreshold = time.Minute
	t.Cleanup(func() {
		common.NodeOfflineThreshold = oldThreshold
	})

	now := time.Now()
	overloaded := &model.Node{
		NodeID:          "node-overloaded",
		Name:            "overloaded",
		IP:              "8.8.8.8",
		PoolName:        "hk",
		PublicIPs:       `["8.8.8.8"]`,
		Weight:          1000,
		AgentToken:      "token-overloaded",
		AgentVersion:    "dev",
		OpenrestyStatus: OpenrestyStatusHealthy,
		Status:          NodeStatusOnline,
		LastSeenAt:      now,
	}
	normal := &model.Node{
		NodeID:          "node-normal",
		Name:            "normal",
		IP:              "1.1.1.1",
		PoolName:        "hk",
		PublicIPs:       `["1.1.1.1"]`,
		Weight:          100,
		AgentToken:      "token-normal",
		AgentVersion:    "dev",
		OpenrestyStatus: OpenrestyStatusHealthy,
		Status:          NodeStatusOnline,
		LastSeenAt:      now,
	}
	for _, node := range []*model.Node{overloaded, normal} {
		if err := node.Insert(); err != nil {
			t.Fatalf("insert node: %v", err)
		}
	}
	if err := (&model.NodeMetricSnapshot{
		NodeID:               overloaded.NodeID,
		CapturedAt:           now,
		OpenrestyConnections: 9,
	}).Insert(); err != nil {
		t.Fatalf("insert overloaded metrics: %v", err)
	}
	if err := (&model.NodeMetricSnapshot{
		NodeID:               normal.NodeID,
		CapturedAt:           now,
		OpenrestyConnections: 2,
	}).Insert(); err != nil {
		t.Fatalf("insert normal metrics: %v", err)
	}

	policy := defaultGSLBPolicy("hk", 2, "load_aware", 60)
	policy.LoadThresholds.MaxOpenrestyConnections = 5
	rawPolicy, err := json.Marshal(policy)
	if err != nil {
		t.Fatalf("marshal policy: %v", err)
	}
	route := &model.ProxyRoute{
		ID:              100,
		NodePool:        "hk",
		DNSTargetCount:  2,
		DNSScheduleMode: "load_aware",
		DNSTTL:          60,
		GSLBEnabled:     true,
		GSLBPolicy:      string(rawPolicy),
	}

	selection, err := selectGSLBDNSTargets(route, "A")
	if err != nil {
		t.Fatalf("select gslb targets: %v", err)
	}
	if len(selection.Targets) != 1 || selection.Targets[0] != "1.1.1.1" {
		t.Fatalf("expected overloaded node to be skipped, got %#v", selection.Targets)
	}
}

func TestSelectGSLBDNSTargetsPrefersFreshMetricsForLoadAware(t *testing.T) {
	setupServiceTestDB(t)

	oldThreshold := common.NodeOfflineThreshold
	oldFreshness := common.GSLBMetricFreshnessSeconds
	common.NodeOfflineThreshold = time.Minute
	common.GSLBMetricFreshnessSeconds = 120
	t.Cleanup(func() {
		common.NodeOfflineThreshold = oldThreshold
		common.GSLBMetricFreshnessSeconds = oldFreshness
	})

	now := time.Now()
	withMetric := &model.Node{
		NodeID:          "node-with-metric",
		Name:            "with-metric",
		IP:              "8.8.4.4",
		PoolName:        "hk",
		PublicIPs:       `["8.8.4.4"]`,
		Weight:          10,
		AgentToken:      "token-with-metric",
		AgentVersion:    "dev",
		OpenrestyStatus: OpenrestyStatusHealthy,
		Status:          NodeStatusOnline,
		LastSeenAt:      now.Add(-time.Second),
	}
	withoutMetric := &model.Node{
		NodeID:          "node-without-metric",
		Name:            "without-metric",
		IP:              "1.1.1.1",
		PoolName:        "hk",
		PublicIPs:       `["1.1.1.1"]`,
		Weight:          1000,
		AgentToken:      "token-without-metric",
		AgentVersion:    "dev",
		OpenrestyStatus: OpenrestyStatusHealthy,
		Status:          NodeStatusOnline,
		LastSeenAt:      now,
	}
	for _, node := range []*model.Node{withMetric, withoutMetric} {
		if err := node.Insert(); err != nil {
			t.Fatalf("insert node: %v", err)
		}
	}
	if err := (&model.NodeMetricSnapshot{
		NodeID:               withMetric.NodeID,
		CapturedAt:           now,
		OpenrestyConnections: 8,
		CPUUsagePercent:      12,
		MemoryUsedBytes:      30,
		MemoryTotalBytes:     100,
	}).Insert(); err != nil {
		t.Fatalf("insert fresh metrics: %v", err)
	}

	policy := defaultGSLBPolicy("hk", 1, "load_aware", 60)
	rawPolicy, err := json.Marshal(policy)
	if err != nil {
		t.Fatalf("marshal policy: %v", err)
	}
	route := &model.ProxyRoute{
		ID:              102,
		NodePool:        "hk",
		DNSTargetCount:  1,
		DNSScheduleMode: "load_aware",
		DNSTTL:          60,
		GSLBEnabled:     true,
		GSLBPolicy:      string(rawPolicy),
	}

	selection, err := selectGSLBDNSTargets(route, "A")
	if err != nil {
		t.Fatalf("select gslb targets: %v", err)
	}
	if len(selection.Targets) != 1 || selection.Targets[0] != "8.8.4.4" {
		t.Fatalf("expected fresh metric node before missing metric fallback, got %#v", selection.Targets)
	}
}

func TestUpdateProxyRouteRejectsGSLBPoolWithoutAvailableTargets(t *testing.T) {
	setupServiceTestDB(t)

	oldThreshold := common.NodeOfflineThreshold
	common.NodeOfflineThreshold = time.Minute
	t.Cleanup(func() {
		common.NodeOfflineThreshold = oldThreshold
	})

	account := &model.DnsAccount{
		Name:          "cf",
		Type:          "cloudflare",
		Authorization: `{"api_token":"token"}`,
	}
	if err := account.Insert(); err != nil {
		t.Fatalf("insert account: %v", err)
	}
	now := time.Now()
	if err := (&model.Node{
		NodeID:          "node-jp",
		Name:            "JP",
		IP:              "8.8.4.4",
		PoolName:        "jp",
		PublicIPs:       `["8.8.4.4"]`,
		Weight:          100,
		AgentToken:      "token-jp",
		AgentVersion:    "dev",
		OpenrestyStatus: OpenrestyStatusHealthy,
		Status:          NodeStatusOnline,
		LastSeenAt:      now,
	}).Insert(); err != nil {
		t.Fatalf("insert node: %v", err)
	}
	route := &model.ProxyRoute{
		SiteName:        "edge-site",
		Domain:          "www.example.com",
		Domains:         `["www.example.com"]`,
		OriginURL:       "https://origin.internal",
		Upstreams:       `["https://origin.internal"]`,
		NodePool:        "jp",
		Enabled:         false,
		DNSProviderMode: DNSProviderModeCloudflare,
		DNSRecordType:   "A",
		DNSTargetCount:  20,
		DNSScheduleMode: "weighted",
		DNSTTL:          30,
	}
	if err := route.Insert(); err != nil {
		t.Fatalf("insert route: %v", err)
	}
	policy := defaultGSLBPolicy("jp", 20, "weighted", 30)
	policy.Pools = []ProxyRouteGSLBPoolPolicy{
		{Name: "jp", Weight: 1, Enabled: true},
		{Name: "aliyun hk", Weight: 99, Enabled: true},
	}

	_, err := UpdateProxyRoute(route.ID, ProxyRouteInput{
		SiteName:        "edge-site",
		Domain:          "www.example.com",
		OriginURL:       "https://origin.internal",
		NodePool:        "jp",
		Enabled:         true,
		DNSAutoSync:     true,
		DNSAccountID:    &account.ID,
		DNSZoneID:       "zone-a",
		DNSRecordType:   "A",
		DNSTargetCount:  20,
		DNSScheduleMode: "weighted",
		DNSTTL:          30,
		GSLBEnabled:     true,
		GSLBPolicy:      policy,
	})
	if err == nil || !strings.Contains(err.Error(), "aliyun hk") {
		t.Fatalf("expected missing GSLB pool target error, got %v", err)
	}
}

func TestLatestNodeMetricSnapshotsUsesConfiguredFreshness(t *testing.T) {
	setupServiceTestDB(t)

	oldFreshness := common.GSLBMetricFreshnessSeconds
	common.GSLBMetricFreshnessSeconds = 60
	t.Cleanup(func() {
		common.GSLBMetricFreshnessSeconds = oldFreshness
	})

	now := time.Now()
	fresh := &model.NodeMetricSnapshot{
		NodeID:               "node-fresh",
		CapturedAt:           now.Add(-30 * time.Second),
		OpenrestyConnections: 3,
	}
	stale := &model.NodeMetricSnapshot{
		NodeID:               "node-stale",
		CapturedAt:           now.Add(-2 * time.Minute),
		OpenrestyConnections: 1,
	}
	for _, snapshot := range []*model.NodeMetricSnapshot{fresh, stale} {
		if err := snapshot.Insert(); err != nil {
			t.Fatalf("insert metric snapshot: %v", err)
		}
	}

	metrics := latestNodeMetricSnapshots()
	if _, ok := metrics["node-fresh"]; !ok {
		t.Fatalf("expected fresh metric to be included, got %#v", metrics)
	}
	if _, ok := metrics["node-stale"]; ok {
		t.Fatalf("expected stale metric to be excluded, got %#v", metrics)
	}
}

func TestSelectGSLBDNSTargetsRespectsSelectedPoolNodeIDs(t *testing.T) {
	setupServiceTestDB(t)

	oldThreshold := common.NodeOfflineThreshold
	common.NodeOfflineThreshold = time.Minute
	t.Cleanup(func() {
		common.NodeOfflineThreshold = oldThreshold
	})

	now := time.Now()
	nodes := []*model.Node{
		{
			NodeID:          "node-primary",
			Name:            "primary",
			IP:              "8.8.8.8",
			PoolName:        "hk",
			PublicIPs:       `["8.8.8.8"]`,
			Weight:          1000,
			AgentToken:      "token-primary",
			AgentVersion:    "dev",
			OpenrestyStatus: OpenrestyStatusHealthy,
			Status:          NodeStatusOnline,
			LastSeenAt:      now,
		},
		{
			NodeID:          "node-backup",
			Name:            "backup",
			IP:              "1.1.1.1",
			PoolName:        "hk",
			PublicIPs:       `["1.1.1.1"]`,
			Weight:          1,
			AgentToken:      "token-backup",
			AgentVersion:    "dev",
			OpenrestyStatus: OpenrestyStatusHealthy,
			Status:          NodeStatusOnline,
			LastSeenAt:      now,
		},
	}
	for _, node := range nodes {
		if err := node.Insert(); err != nil {
			t.Fatalf("insert node: %v", err)
		}
	}

	policy := defaultGSLBPolicy("hk", 1, "weighted", 60)
	policy.Pools[0].NodeIDs = []string{"node-backup"}
	rawPolicy, err := json.Marshal(policy)
	if err != nil {
		t.Fatalf("marshal policy: %v", err)
	}

	selection, err := selectGSLBDNSTargets(&model.ProxyRoute{
		ID:              102,
		NodePool:        "hk",
		DNSTargetCount:  1,
		DNSScheduleMode: "weighted",
		DNSTTL:          60,
		GSLBEnabled:     true,
		GSLBPolicy:      string(rawPolicy),
	}, "A")
	if err != nil {
		t.Fatalf("select GSLB targets: %v", err)
	}
	if len(selection.Targets) != 1 || selection.Targets[0] != "1.1.1.1" {
		t.Fatalf("expected selected node target only, got %#v", selection.Targets)
	}
}

func TestLatestNodeMetricSnapshotsIgnoresFutureMetrics(t *testing.T) {
	setupServiceTestDB(t)

	oldFreshness := common.GSLBMetricFreshnessSeconds
	common.GSLBMetricFreshnessSeconds = 120
	t.Cleanup(func() {
		common.GSLBMetricFreshnessSeconds = oldFreshness
	})

	now := time.Now()
	fresh := &model.NodeMetricSnapshot{
		NodeID:               "node-edge",
		CapturedAt:           now.Add(-30 * time.Second),
		OpenrestyConnections: 3,
	}
	future := &model.NodeMetricSnapshot{
		NodeID:               "node-edge",
		CapturedAt:           now.Add(time.Hour),
		OpenrestyConnections: 1,
	}
	for _, snapshot := range []*model.NodeMetricSnapshot{fresh, future} {
		if err := snapshot.Insert(); err != nil {
			t.Fatalf("insert metric snapshot: %v", err)
		}
	}

	metrics := latestNodeMetricSnapshots()
	metric := metrics["node-edge"]
	if metric == nil || metric.CapturedAt.After(now) || metric.OpenrestyConnections != 3 {
		t.Fatalf("expected future metric to be ignored, got %#v", metric)
	}
}

func TestSelectGSLBDNSTargetsKeepsPreviousTargetsDuringCooldown(t *testing.T) {
	setupServiceTestDB(t)

	oldThreshold := common.NodeOfflineThreshold
	common.NodeOfflineThreshold = time.Minute
	t.Cleanup(func() {
		common.NodeOfflineThreshold = oldThreshold
	})

	now := time.Now()
	nodes := []*model.Node{
		{
			NodeID:          "node-previous",
			Name:            "previous",
			IP:              "8.8.8.8",
			PoolName:        "hk",
			PublicIPs:       `["8.8.8.8"]`,
			Weight:          100,
			AgentToken:      "token-previous",
			AgentVersion:    "dev",
			OpenrestyStatus: OpenrestyStatusHealthy,
			Status:          NodeStatusOnline,
			LastSeenAt:      now,
		},
		{
			NodeID:          "node-desired",
			Name:            "desired",
			IP:              "1.1.1.1",
			PoolName:        "hk",
			PublicIPs:       `["1.1.1.1"]`,
			Weight:          900,
			AgentToken:      "token-desired",
			AgentVersion:    "dev",
			OpenrestyStatus: OpenrestyStatusHealthy,
			Status:          NodeStatusOnline,
			LastSeenAt:      now,
		},
	}
	for _, node := range nodes {
		if err := node.Insert(); err != nil {
			t.Fatalf("insert node: %v", err)
		}
	}

	lastChanged := now.Add(-10 * time.Second)
	state := &model.GSLBSchedulingState{
		ProxyRouteID:    101,
		DNSRecordType:   "A",
		SelectedTargets: `["8.8.8.8"]`,
		DesiredTargets:  `["8.8.8.8"]`,
		LastChangedAt:   &lastChanged,
		LastEvaluatedAt: &lastChanged,
	}
	if err := model.DB.Create(state).Error; err != nil {
		t.Fatalf("insert state: %v", err)
	}

	policy := defaultGSLBPolicy("hk", 1, "weighted", 60)
	policy.Debounce.CooldownSeconds = 60
	rawPolicy, err := json.Marshal(policy)
	if err != nil {
		t.Fatalf("marshal policy: %v", err)
	}
	route := &model.ProxyRoute{
		ID:              101,
		NodePool:        "hk",
		DNSTargetCount:  1,
		DNSScheduleMode: "weighted",
		DNSTTL:          60,
		GSLBEnabled:     true,
		GSLBPolicy:      string(rawPolicy),
	}

	selection, err := selectGSLBDNSTargets(route, "A")
	if err != nil {
		t.Fatalf("select gslb targets: %v", err)
	}
	if len(selection.Targets) != 1 || selection.Targets[0] != "8.8.8.8" {
		t.Fatalf("expected cooldown to keep previous target, got %#v", selection.Targets)
	}
	if len(selection.DesiredTargets) != 1 || selection.DesiredTargets[0] != "1.1.1.1" {
		t.Fatalf("expected desired target to prefer higher weight node, got %#v", selection.DesiredTargets)
	}
}
