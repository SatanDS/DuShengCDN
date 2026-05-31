package service

import (
	"encoding/json"
	"testing"
	"time"

	"dushengcdn/common"
	"dushengcdn/model"
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
