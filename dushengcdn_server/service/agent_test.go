package service

import (
	"strings"
	"testing"
	"time"

	"dushengcdn/model"
)

func TestReportApplyLogRedactsSensitiveMessage(t *testing.T) {
	setupServiceTestDB(t)

	node := &model.Node{
		NodeID:       "node-apply-redact",
		Name:         "edge",
		IP:           "127.0.0.1",
		Status:       NodeStatusOnline,
		AgentVersion: "v1.0.0",
	}
	if err := model.DB.Create(node).Error; err != nil {
		t.Fatalf("create node: %v", err)
	}

	log, err := ReportApplyLog(ApplyLogPayload{
		NodeID:  node.NodeID,
		Version: "20260610-001",
		Result:  ApplyResultFailed,
		Message: `openresty -t failed:
local expected_hash = "abcdef123456"
proxy_set_header Authorization "Bearer origin-token";
callback /oauth?code=oauth-code&state=csrf-state`,
	})
	if err != nil {
		t.Fatalf("ReportApplyLog failed: %v", err)
	}
	for _, leaked := range []string{"abcdef123456", "origin-token", "oauth-code", "csrf-state"} {
		if strings.Contains(log.Message, leaked) {
			t.Fatalf("expected %q to be redacted from apply log %q", leaked, log.Message)
		}
	}
	reloaded, err := model.GetNodeByNodeID(node.NodeID)
	if err != nil {
		t.Fatalf("reload node: %v", err)
	}
	if reloaded.LastError != log.Message {
		t.Fatalf("expected node last_error to use redacted apply message, got %q vs %q", reloaded.LastError, log.Message)
	}
}

func TestGetActiveConfigForAgentIncludesPoWConfig(t *testing.T) {
	setupServiceTestDB(t)

	_, err := CreateProxyRoute(ProxyRouteInput{
		Domain:     "pow-agent.example.com",
		OriginURL:  "https://origin.internal",
		Enabled:    true,
		PoWEnabled: true,
		PoWConfig:  `{"difficulty":4,"algorithm":"fast","session_ttl":86400,"challenge_ttl":300,"whitelist":{"paths":["/.well-known/*","/favicon.ico","/robots.txt"],"user_agents":["Googlebot","bingbot","Baiduspider"]},"blacklist":{"ips":[],"ip_cidrs":[],"paths":[],"path_regexes":[],"user_agents":[]}}`,
	})
	if err != nil {
		t.Fatalf("CreateProxyRoute failed: %v", err)
	}

	if _, err := PublishConfigVersion("root", false); err != nil {
		t.Fatalf("PublishConfigVersion failed: %v", err)
	}

	activeConfig, err := GetActiveConfigForAgent()
	if err != nil {
		t.Fatalf("GetActiveConfigForAgent failed: %v", err)
	}

	foundPowConfig := false
	foundRegionConfig := false
	for _, file := range activeConfig.SupportFiles {
		if file.Path == "pow_config.json" {
			foundPowConfig = true
			if file.Content == "" {
				t.Fatal("expected pow_config.json content to be populated")
			}
		}
		if file.Path == "region_config.json" {
			foundRegionConfig = true
		}
	}
	if !foundPowConfig {
		t.Fatal("expected agent config to include pow_config.json support file")
	}
	if !foundRegionConfig {
		t.Fatal("expected agent config to include region_config.json support file")
	}
}

func TestGetActiveConfigForAgentIncludesWAFConfig(t *testing.T) {
	setupServiceTestDB(t)

	_, err := CreateProxyRoute(ProxyRouteInput{
		Domain:     "waf-agent.example.com",
		OriginURL:  "https://origin.internal",
		Enabled:    true,
		WAFEnabled: true,
		WAFMode:    "log",
		WAFConfig:  `{"builtin_rules":["sqli","sensitive_paths"],"whitelist":{"ips":["1.2.3.4"],"ip_cidrs":["10.0.0.0/8"],"paths":["/api/public/*"]},"block_rules":{"path_contains":["/debug"],"path_regexes":["^/private/"],"query_contains":["debug=true"],"header_contains":["X-Scanner"],"user_agents":["sqlmap"]}}`,
	})
	if err != nil {
		t.Fatalf("CreateProxyRoute failed: %v", err)
	}

	release, err := PublishConfigVersion("root", false)
	if err != nil {
		t.Fatalf("PublishConfigVersion failed: %v", err)
	}
	if !strings.Contains(release.Version.RenderedConfig, "access_by_lua_file __DUSHENGCDN_LUA_DIR__/access.lua;") {
		t.Fatalf("expected rendered config to include unified access lua for WAF, got %s", release.Version.RenderedConfig)
	}
	if !strings.Contains(release.Version.RenderedConfig, `set $dushengcdn_request_reason "";`) {
		t.Fatalf("expected rendered config to initialize access log reason variable, got %s", release.Version.RenderedConfig)
	}
	if !strings.Contains(release.Version.MainConfig, "lua_shared_dict dushengcdn_waf_config 1m;") {
		t.Fatalf("expected main config to include WAF shared dict, got %s", release.Version.MainConfig)
	}

	activeConfig, err := GetActiveConfigForAgent()
	if err != nil {
		t.Fatalf("GetActiveConfigForAgent failed: %v", err)
	}

	for _, file := range activeConfig.SupportFiles {
		if file.Path == "waf_config.json" {
			if !strings.Contains(file.Content, `"mode":"log"`) {
				t.Fatalf("expected waf_config.json to preserve mode, got %s", file.Content)
			}
			if !strings.Contains(file.Content, `"sensitive_paths"`) {
				t.Fatalf("expected waf_config.json to include builtin rules, got %s", file.Content)
			}
			return
		}
	}
	t.Fatal("expected agent config to include waf_config.json support file")
}

func TestGetActiveConfigForAgentIncludesCCConfigAndForceOnlyPoW(t *testing.T) {
	setupServiceTestDB(t)

	_, err := CreateProxyRoute(ProxyRouteInput{
		Domain:     "cc-agent.example.com",
		OriginURL:  "https://origin.internal",
		Enabled:    true,
		CCEnabled:  true,
		CCMode:     proxyRouteCCModePoW,
		CCConfig:   `{"window_seconds":5,"max_requests":10,"path_window_seconds":7,"path_max_requests":3,"block_duration_seconds":120,"whitelist":{"ips":["1.2.3.4"],"ip_cidrs":["10.0.0.0/8"],"paths":["/api/health"],"user_agents":["monitor"]},"exclude":{"ips":["5.6.7.8"],"ip_cidrs":["172.16.0.0/12"],"paths":["/assets/*"],"user_agents":["uptime"]}}`,
		PoWEnabled: false,
		PoWConfig:  `{"difficulty":5,"algorithm":"fast","session_ttl":600,"challenge_ttl":300,"whitelist":{"paths":["/public/*"]},"blacklist":{"user_agents":["badbot"]}}`,
	})
	if err != nil {
		t.Fatalf("CreateProxyRoute failed: %v", err)
	}

	release, err := PublishConfigVersion("root", false)
	if err != nil {
		t.Fatalf("PublishConfigVersion failed: %v", err)
	}
	for _, expected := range []string{
		"access_by_lua_file __DUSHENGCDN_LUA_DIR__/access.lua;",
		"location /.within.website/x/cmd/anubis/static/ {",
		"set $dushengcdn_request_reason \"\";",
	} {
		if !strings.Contains(release.Version.RenderedConfig, expected) {
			t.Fatalf("expected rendered config to contain %q, got %s", expected, release.Version.RenderedConfig)
		}
	}
	for _, expected := range []string{
		"lua_shared_dict dushengcdn_cc_config 1m;",
		"lua_shared_dict dushengcdn_cc_counters 20m;",
		"lua_shared_dict dushengcdn_pow_config 1m;",
	} {
		if !strings.Contains(release.Version.MainConfig, expected) {
			t.Fatalf("expected main config to contain %q, got %s", expected, release.Version.MainConfig)
		}
	}

	activeConfig, err := GetActiveConfigForAgent()
	if err != nil {
		t.Fatalf("GetActiveConfigForAgent failed: %v", err)
	}
	files := supportFilesByPath(activeConfig.SupportFiles)
	ccConfig := files["cc_config.json"]
	if ccConfig == "" {
		t.Fatal("expected agent config to include cc_config.json support file")
	}
	for _, expected := range []string{
		`"mode":"pow"`,
		`"max_requests":10`,
		`"path_max_requests":3`,
		`"/assets/*"`,
	} {
		if !strings.Contains(ccConfig, expected) {
			t.Fatalf("expected cc_config.json to contain %q, got %s", expected, ccConfig)
		}
	}
	powConfig := files["pow_config.json"]
	if powConfig == "" {
		t.Fatal("expected CC pow mode to include pow_config.json support file")
	}
	for _, expected := range []string{
		`"force_only":true`,
		`"difficulty":5`,
		`"/public/*"`,
		`"badbot"`,
	} {
		if !strings.Contains(powConfig, expected) {
			t.Fatalf("expected pow_config.json to contain %q, got %s", expected, powConfig)
		}
	}
}

func TestCreateProxyRouteAcceptsIPv6CCCIDR(t *testing.T) {
	setupServiceTestDB(t)

	_, err := CreateProxyRoute(ProxyRouteInput{
		Domain:    "cc-ipv6.example.com",
		OriginURL: "https://origin.internal",
		Enabled:   true,
		CCEnabled: true,
		CCConfig:  `{"window_seconds":10,"max_requests":120,"path_window_seconds":10,"path_max_requests":60,"block_duration_seconds":300,"whitelist":{"ip_cidrs":["2001:db8::/32"]}}`,
	})
	if err != nil {
		t.Fatalf("CreateProxyRoute failed: %v", err)
	}
	if _, err := PublishConfigVersion("root", false); err != nil {
		t.Fatalf("PublishConfigVersion failed: %v", err)
	}

	config, err := GetActiveConfigForAgent()
	if err != nil {
		t.Fatalf("GetActiveConfigForAgent failed: %v", err)
	}
	files := supportFilesByPath(config.SupportFiles)
	ccConfig := files["cc_config.json"]
	if !strings.Contains(ccConfig, `"2001:db8::/32"`) {
		t.Fatalf("expected cc_config.json to contain IPv6 CIDR, got %s", ccConfig)
	}
}

func TestGetActiveConfigForAgentUsesTenMinutePoWSessionDefault(t *testing.T) {
	setupServiceTestDB(t)

	_, err := CreateProxyRoute(ProxyRouteInput{
		Domain:     "pow-default.example.com",
		OriginURL:  "https://origin.internal",
		Enabled:    true,
		PoWEnabled: true,
		PoWConfig:  `{}`,
	})
	if err != nil {
		t.Fatalf("CreateProxyRoute failed: %v", err)
	}

	if _, err := PublishConfigVersion("root", false); err != nil {
		t.Fatalf("PublishConfigVersion failed: %v", err)
	}

	activeConfig, err := GetActiveConfigForAgent()
	if err != nil {
		t.Fatalf("GetActiveConfigForAgent failed: %v", err)
	}

	for _, file := range activeConfig.SupportFiles {
		if file.Path == "pow_config.json" {
			if !strings.Contains(file.Content, `"session_ttl":600`) {
				t.Fatalf("expected default PoW session TTL to be 600 seconds, got %s", file.Content)
			}
			return
		}
	}
	t.Fatal("expected agent config to include pow_config.json support file")
}

func TestAgentTokenPoolUsesPoolArtifactChecksum(t *testing.T) {
	setupServiceTestDB(t)

	version := seedAgentTestActiveConfigVersionWithArtifacts(t, "20260609-001", "checksum-global", map[string]string{
		"default": "checksum-default",
		"edge-a":  "checksum-edge-a",
		"edge-b":  "checksum-edge-b",
	})

	nodes := []*model.Node{
		{
			NodeID:       "node-pool-edge-a",
			Name:         "edge-a",
			IP:           "10.0.0.31",
			PoolName:     "edge-a",
			AgentToken:   "token-edge-a",
			AgentVersion: "v0.6.0",
			NginxVersion: "1.27.1.2",
			Status:       NodeStatusOnline,
		},
		{
			NodeID:       "node-pool-edge-b",
			Name:         "edge-b",
			IP:           "10.0.0.32",
			PoolName:     "edge-b",
			AgentToken:   "token-edge-b",
			AgentVersion: "v0.6.0",
			NginxVersion: "1.27.1.2",
			Status:       NodeStatusOnline,
		},
	}
	for _, node := range nodes {
		if err := node.Insert(); err != nil {
			t.Fatalf("failed to insert node %s: %v", node.NodeID, err)
		}
	}

	authNode, err := AuthenticateAgentToken(" token-edge-a ")
	if err != nil {
		t.Fatalf("AuthenticateAgentToken failed: %v", err)
	}
	if authNode.PoolName != "edge-a" {
		t.Fatalf("expected token to authenticate edge-a pool node, got %+v", authNode)
	}

	config, err := GetActiveConfigForAgentNode(authNode)
	if err != nil {
		t.Fatalf("GetActiveConfigForAgentNode failed: %v", err)
	}
	if config.Version != version.Version {
		t.Fatalf("expected active version %s, got %s", version.Version, config.Version)
	}
	if config.Checksum != "checksum-edge-a" {
		t.Fatalf("expected pool artifact checksum, got %s", config.Checksum)
	}
	if config.Checksum == version.Checksum || config.Checksum == "checksum-edge-b" {
		t.Fatalf("expected edge-a checksum, got global/other pool checksum %s", config.Checksum)
	}

	resp, err := HeartbeatNode(authNode, AgentNodePayload{
		NodeID:          authNode.NodeID,
		Name:            authNode.Name,
		IP:              authNode.IP,
		AgentVersion:    authNode.AgentVersion,
		NginxVersion:    authNode.NginxVersion,
		CurrentVersion:  version.Version,
		CurrentChecksum: "checksum-edge-a",
	})
	if err != nil {
		t.Fatalf("HeartbeatNode failed: %v", err)
	}
	if resp.ActiveConfig == nil {
		t.Fatal("expected active config summary in heartbeat response")
	}
	if resp.ActiveConfig.Checksum != "checksum-edge-a" {
		t.Fatalf("expected heartbeat to return pool artifact checksum, got %+v", resp.ActiveConfig)
	}
}

func seedAgentTestActiveConfigVersionWithArtifacts(t *testing.T, versionName string, globalChecksum string, artifacts map[string]string) *model.ConfigVersion {
	t.Helper()
	version := &model.ConfigVersion{
		Version:          versionName,
		SnapshotJSON:     "{}",
		MainConfig:       "worker_processes auto;",
		RenderedConfig:   "global rendered config",
		SupportFilesJSON: "[]",
		Checksum:         globalChecksum,
		IsActive:         true,
		CreatedBy:        "root",
	}
	if err := model.DB.Create(version).Error; err != nil {
		t.Fatalf("failed to seed active config version: %v", err)
	}
	for poolName, checksum := range artifacts {
		normalizedPoolName := normalizeNodePoolName(poolName)
		artifact := &model.ConfigVersionArtifact{
			ConfigVersionID:     version.ID,
			PoolName:            normalizedPoolName,
			Checksum:            checksum,
			MainConfigChecksum:  "main-checksum",
			RouteConfigChecksum: "route-checksum",
			RenderedConfig:      "rendered config for " + normalizedPoolName,
			SupportFilesJSON:    "[]",
			RouteCount:          1,
		}
		if err := model.DB.Create(artifact).Error; err != nil {
			t.Fatalf("failed to seed config artifact for pool %s: %v", normalizedPoolName, err)
		}
	}
	return version
}

func supportFilesByPath(files []SupportFile) map[string]string {
	result := make(map[string]string, len(files))
	for _, file := range files {
		result[file.Path] = file.Content
	}
	return result
}

func TestBuildAgentDNSProbeTargetsLimitsOnlineWorkers(t *testing.T) {
	setupServiceTestDB(t)

	if _, err := CreateAuthoritativeDNSZone(DNSZoneInput{Name: "example.com"}); err != nil {
		t.Fatalf("CreateAuthoritativeDNSZone: %v", err)
	}
	for i := 0; i < maxAgentDNSProbeTargets+2; i++ {
		worker, err := CreateAuthoritativeDNSWorker(DNSWorkerInput{
			Name:          strings.Repeat("n", i+1),
			PublicAddress: "ns.example.net",
		})
		if err != nil {
			t.Fatalf("CreateAuthoritativeDNSWorker: %v", err)
		}
		workerModel, err := model.GetDNSWorkerByID(worker.ID)
		if err != nil {
			t.Fatalf("GetDNSWorkerByID: %v", err)
		}
		if _, err := RecordDNSWorkerHeartbeat(workerModel, DNSWorkerHeartbeatInput{Status: dnsWorkerStatusOnline}); err != nil {
			t.Fatalf("RecordDNSWorkerHeartbeat: %v", err)
		}
	}

	targets := buildAgentDNSProbeTargets()
	if len(targets) != maxAgentDNSProbeTargets {
		t.Fatalf("expected probe target limit %d, got %+v", maxAgentDNSProbeTargets, targets)
	}
	for _, target := range targets {
		if target.WorkerID == "" || target.PublicAddress != "ns.example.net:53" || target.QueryName != "example.com." || target.QueryType != "SOA" {
			t.Fatalf("unexpected target: %+v", target)
		}
	}
}

func TestBuildAgentDNSProbeTargetsSkipsUnsafeStoredAddress(t *testing.T) {
	setupServiceTestDB(t)

	if _, err := CreateAuthoritativeDNSZone(DNSZoneInput{Name: "example.com"}); err != nil {
		t.Fatalf("CreateAuthoritativeDNSZone: %v", err)
	}
	worker, err := CreateAuthoritativeDNSWorker(DNSWorkerInput{
		Name:          "ns-safe",
		PublicAddress: "8.8.8.8",
	})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSWorker: %v", err)
	}
	workerModel, err := model.GetDNSWorkerByID(worker.ID)
	if err != nil {
		t.Fatalf("GetDNSWorkerByID: %v", err)
	}
	if _, err := RecordDNSWorkerHeartbeat(workerModel, DNSWorkerHeartbeatInput{Status: dnsWorkerStatusOnline}); err != nil {
		t.Fatalf("RecordDNSWorkerHeartbeat: %v", err)
	}
	if err := model.DB.Model(workerModel).Update("public_address", "169.254.169.254:53").Error; err != nil {
		t.Fatalf("seed unsafe public address: %v", err)
	}

	if targets := buildAgentDNSProbeTargets(); len(targets) != 0 {
		t.Fatalf("expected unsafe stored address to be skipped, got %+v", targets)
	}
}

func TestHeartbeatNodeIgnoresUnassignedDNSProbeReports(t *testing.T) {
	setupServiceTestDB(t)

	if _, err := CreateAuthoritativeDNSZone(DNSZoneInput{Name: "example.com"}); err != nil {
		t.Fatalf("CreateAuthoritativeDNSZone: %v", err)
	}
	assigned, err := CreateAuthoritativeDNSWorker(DNSWorkerInput{Name: "ns-assigned", PublicAddress: "ns1.example.net"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSWorker assigned: %v", err)
	}
	worker, err := model.GetDNSWorkerByID(assigned.ID)
	if err != nil {
		t.Fatalf("GetDNSWorkerByID: %v", err)
	}
	if _, err := RecordDNSWorkerHeartbeat(worker, DNSWorkerHeartbeatInput{Status: dnsWorkerStatusOnline}); err != nil {
		t.Fatalf("RecordDNSWorkerHeartbeat: %v", err)
	}

	node := &model.Node{
		NodeID:       "node-dnsprobe-authz",
		Name:         "edge-dnsprobe-authz",
		IP:           "203.0.113.10",
		AgentToken:   "agent-token",
		AgentVersion: "v1.0.0",
		Status:       NodeStatusOnline,
	}
	if err := node.Insert(); err != nil {
		t.Fatalf("insert node: %v", err)
	}
	now := time.Now().UTC()
	results := []AgentDNSProbeResult{
		{Network: "UDP", Reachable: true, DurationMs: 10, RCode: "NOERROR", AnswerCount: 1},
		{Network: "TCP", Reachable: true, DurationMs: 15, RCode: "NOERROR", AnswerCount: 1},
	}
	_, err = HeartbeatNode(node, AgentNodePayload{
		IP:              "203.0.113.10",
		AgentVersion:    "v1.0.1",
		OpenrestyStatus: OpenrestyStatusHealthy,
		DNSProbeResults: []AgentDNSProbeReport{
			{
				WorkerID:      assigned.WorkerID,
				PublicAddress: "ns1.example.net:53",
				QueryName:     "example.com.",
				QueryType:     "SOA",
				CheckedAtUnix: now.Unix(),
				Results:       results,
			},
			{
				WorkerID:      assigned.WorkerID,
				PublicAddress: "127.0.0.1:53",
				QueryName:     "example.com.",
				QueryType:     "SOA",
				CheckedAtUnix: now.Unix(),
				Results:       results,
			},
			{
				WorkerID:      "forged-worker",
				PublicAddress: "ns2.example.net",
				QueryName:     "example.com.",
				QueryType:     "SOA",
				CheckedAtUnix: now.Unix(),
				Results:       results,
			},
		},
	})
	if err != nil {
		t.Fatalf("HeartbeatNode: %v", err)
	}

	probes, err := model.ListDNSWorkerNodeProbes()
	if err != nil {
		t.Fatalf("ListDNSWorkerNodeProbes: %v", err)
	}
	if len(probes) != 1 {
		t.Fatalf("expected only the assigned matching probe to persist, got %+v", probes)
	}
	if probes[0].WorkerID != assigned.WorkerID || probes[0].PublicAddress != "ns1.example.net:53" || probes[0].QueryName != "example.com." {
		t.Fatalf("unexpected persisted probe: %+v", probes[0])
	}
}

func TestHeartbeatNodeReturnsErrorWhenObservabilityPersistenceFails(t *testing.T) {
	setupServiceTestDB(t)

	node := &model.Node{
		NodeID:       "node-observe-error",
		Name:         "observe-error-edge",
		IP:           "10.0.0.32",
		AgentToken:   "token-observe-error",
		AgentVersion: "v0.6.0",
		NginxVersion: "1.27.1.2",
		Status:       NodeStatusOnline,
	}
	if err := node.Insert(); err != nil {
		t.Fatalf("failed to seed node: %v", err)
	}

	_, err := HeartbeatNode(node, AgentNodePayload{
		NodeID:       node.NodeID,
		Name:         node.Name,
		IP:           node.IP,
		AgentVersion: node.AgentVersion,
		NginxVersion: node.NginxVersion,
		TrafficReport: &AgentNodeTrafficReport{
			WindowStartedAtUnix: time.Now().Unix(),
			WindowEndedAtUnix:   time.Now().Add(-time.Minute).Unix(),
			RequestCount:        1,
		},
	})
	if err == nil {
		t.Fatal("expected heartbeat to fail when observability persistence fails")
	}
	if !strings.Contains(err.Error(), "window_started_at_unix") {
		t.Fatalf("expected observability validation error, got %v", err)
	}
}
