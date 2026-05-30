package service

import (
	"testing"
	"time"

	"openflare/common"
	"openflare/model"
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
