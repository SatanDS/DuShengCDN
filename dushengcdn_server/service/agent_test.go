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
		if target.WorkerID == "" || target.PublicAddress == "" || target.QueryName != "example.com." || target.QueryType != "SOA" {
			t.Fatalf("unexpected target: %+v", target)
		}
	}
}
