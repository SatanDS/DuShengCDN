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
