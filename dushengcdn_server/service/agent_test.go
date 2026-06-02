package service

import (
	"strings"
	"testing"

	"dushengcdn/model"
)

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
		if target.WorkerID == "" || target.PublicAddress == "" || target.QueryName != "example.com." || target.QueryType != "SOA" {
			t.Fatalf("unexpected target: %+v", target)
		}
	}
}
