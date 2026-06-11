package openresty

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
)

func TestRenderRouteConfigRuntimeDNSUsesVariableProxyPass(t *testing.T) {
	lookupCalled := false
	config, _, err := RenderRouteConfig([]Route{{
		ID:                7,
		Domain:            "cdn.example.com",
		OriginURL:         "https://origin.example.com/app?version=1",
		OriginResolveMode: OriginResolveModeRuntimeDNS,
		Upstreams:         []string{"https://origin.example.com:8443/app?version=1"},
	}}, ConfigSnapshot{}, RenderOptions{
		LookupIPAddr: func(context.Context, string) ([]net.IPAddr, error) {
			lookupCalled = true
			return nil, errors.New("unexpected lookup")
		},
	})
	if err != nil {
		t.Fatalf("RenderRouteConfig returned error: %v", err)
	}
	if lookupCalled {
		t.Fatal("runtime_dns should not resolve origin during rendering")
	}
	if strings.Contains(config, "upstream backend_") {
		t.Fatalf("runtime_dns should not render a publish-time upstream block:\n%s", config)
	}
	if !strings.Contains(config, `set $dushengcdn_origin "https://origin.example.com:8443";`) {
		t.Fatalf("runtime_dns should set a variable origin for nginx resolver:\n%s", config)
	}
	if !strings.Contains(config, "proxy_pass $dushengcdn_origin/app?version=1;") {
		t.Fatalf("runtime_dns should proxy through the variable origin and keep URI:\n%s", config)
	}
}

func TestRenderRouteConfigRuntimeDNSMultiUpstreamUsesResolve(t *testing.T) {
	lookupCalled := false
	config, _, err := RenderRouteConfig([]Route{{
		ID:                11,
		Domain:            "cdn.example.com",
		OriginURL:         "https://origin-a.example.com",
		OriginResolveMode: OriginResolveModeRuntimeDNS,
		Upstreams: []string{
			"https://origin-a.example.com:8443",
			"https://origin-b.example.com:8443",
		},
	}}, ConfigSnapshot{}, RenderOptions{
		LookupIPAddr: func(context.Context, string) ([]net.IPAddr, error) {
			lookupCalled = true
			return nil, errors.New("unexpected lookup")
		},
	})
	if err != nil {
		t.Fatalf("RenderRouteConfig returned error: %v", err)
	}
	if lookupCalled {
		t.Fatal("runtime_dns multi-upstream should not resolve origins during rendering")
	}
	for _, want := range []string{
		"server origin-a.example.com:8443 resolve max_fails=3 fail_timeout=10s;",
		"server origin-b.example.com:8443 resolve max_fails=3 fail_timeout=10s;",
		"proxy_pass https://backend_cdn_example_com_11;",
	} {
		if !strings.Contains(config, want) {
			t.Fatalf("rendered config missing %q:\n%s", want, config)
		}
	}
}

func TestRenderRouteConfigPublishResolveUsesResolvedUpstream(t *testing.T) {
	config, _, err := RenderRouteConfig([]Route{{
		ID:        8,
		Domain:    "cdn.example.com",
		OriginURL: "https://origin.example.com",
		Upstreams: []string{"https://origin.example.com:8443"},
	}}, ConfigSnapshot{}, RenderOptions{
		LookupIPAddr: func(context.Context, string) ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
		},
	})
	if err != nil {
		t.Fatalf("RenderRouteConfig returned error: %v", err)
	}
	if !strings.Contains(config, "server 93.184.216.34:8443 max_fails=3 fail_timeout=10s;") {
		t.Fatalf("publish_resolve should render resolved IP server:\n%s", config)
	}
	if !strings.Contains(config, "proxy_pass https://backend_cdn_example_com_8;") {
		t.Fatalf("publish_resolve should proxy to named upstream:\n%s", config)
	}
}

func TestRenderRouteConfigRouteRulesLocationOrder(t *testing.T) {
	config, _, err := RenderRouteConfig([]Route{{
		ID:        21,
		Domain:    "cdn.example.com",
		OriginURL: "https://site-origin.example.com",
		Rules: []RouteRule{
			{
				ID:        4,
				MatchType: RouteRuleMatchTypeDefault,
				Path:      "/",
				Priority:  1,
				Enabled:   true,
				OriginURL: "https://ignored-default.example.com",
			},
			{
				ID:        3,
				MatchType: RouteRuleMatchTypeRegex,
				Path:      `^/items/[0-9]+$`,
				Priority:  30,
				Enabled:   true,
			},
			{
				ID:        2,
				MatchType: RouteRuleMatchTypePrefix,
				Path:      "/prefix",
				Priority:  20,
				Enabled:   true,
			},
			{
				ID:        1,
				MatchType: RouteRuleMatchTypeExact,
				Path:      "/exact",
				Priority:  10,
				Enabled:   true,
			},
			{
				ID:        5,
				MatchType: RouteRuleMatchTypeExact,
				Path:      "/disabled",
				Priority:  0,
				Enabled:   false,
			},
		},
	}}, ConfigSnapshot{}, RenderOptions{})
	if err != nil {
		t.Fatalf("RenderRouteConfig returned error: %v", err)
	}
	assertInOrder(t, config, []string{
		"location = /exact {",
		"location ^~ /prefix/ {",
		"location ~ ^/items/[0-9]+$ {",
		"location / {",
	})
	if strings.Contains(config, "/disabled") {
		t.Fatalf("disabled rule should not render:\n%s", config)
	}
	if strings.Contains(config, "ignored-default.example.com") {
		t.Fatalf("default rule overrides should not replace the site fallback:\n%s", config)
	}
}

func TestRenderRouteConfigWithoutEnabledRulesKeepsRootLocation(t *testing.T) {
	config, _, err := RenderRouteConfig([]Route{{
		ID:        22,
		Domain:    "cdn.example.com",
		OriginURL: "https://site-origin.example.com",
		Rules: []RouteRule{{
			ID:        1,
			MatchType: RouteRuleMatchTypePrefix,
			Path:      "/disabled",
			Priority:  1,
			Enabled:   false,
		}},
	}}, ConfigSnapshot{}, RenderOptions{})
	if err != nil {
		t.Fatalf("RenderRouteConfig returned error: %v", err)
	}
	if count := strings.Count(config, "location / {"); count != 1 {
		t.Fatalf("expected exactly one root location, got %d:\n%s", count, config)
	}
	for _, unexpected := range []string{"location =", "location ^~", "location ~", "/disabled"} {
		if strings.Contains(config, unexpected) {
			t.Fatalf("unexpected rule location %q rendered:\n%s", unexpected, config)
		}
	}
}

func TestRenderRouteConfigRouteRuleOriginAndUpstreamOverride(t *testing.T) {
	config, _, err := RenderRouteConfig([]Route{{
		ID:        31,
		Domain:    "cdn.example.com",
		OriginURL: "https://site-origin.example.com",
		Rules: []RouteRule{
			{
				ID:        101,
				MatchType: RouteRuleMatchTypeExact,
				Path:      "/direct",
				Priority:  10,
				Enabled:   true,
				OriginURL: "https://direct-origin.example.com/app",
			},
			{
				ID:        102,
				MatchType: RouteRuleMatchTypePrefix,
				Path:      "/balanced",
				Priority:  20,
				Enabled:   true,
				OriginURL: "https://balanced-origin.example.com",
				Upstreams: []string{"https://93.184.216.34:8443"},
			},
		},
	}}, ConfigSnapshot{}, RenderOptions{})
	if err != nil {
		t.Fatalf("RenderRouteConfig returned error: %v", err)
	}
	for _, want := range []string{
		"location = /direct {",
		"proxy_pass https://direct-origin.example.com/app;",
		"upstream backend_cdn_example_com_rule_102_31 {",
		"server 93.184.216.34:8443 max_fails=3 fail_timeout=10s;",
		"proxy_pass https://backend_cdn_example_com_rule_102_31;",
		"location / {",
		"proxy_pass https://site-origin.example.com;",
	} {
		if !strings.Contains(config, want) {
			t.Fatalf("rendered config missing %q:\n%s", want, config)
		}
	}
}

func TestRenderRouteConfigOriginTLSOptions(t *testing.T) {
	verify := false
	config, files, err := RenderRouteConfig([]Route{{
		ID:               12,
		Domain:           "cdn.example.com",
		OriginURL:        "https://203.0.113.10:443",
		OriginHostHeader: "bucket.storage.example.com",
		OriginSNI:        "cert.storage.example.com",
		OriginTLSVerify:  &verify,
		OriginCABundle:   "-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----",
		Upstreams:        []string{"https://8.8.8.8:443"},
	}}, ConfigSnapshot{}, RenderOptions{})
	if err != nil {
		t.Fatalf("RenderRouteConfig returned error: %v", err)
	}
	for _, want := range []string{
		`proxy_set_header Host "bucket.storage.example.com";`,
		`proxy_ssl_name "cert.storage.example.com";`,
		"proxy_ssl_verify off;",
	} {
		if !strings.Contains(config, want) {
			t.Fatalf("rendered config missing %q:\n%s", want, config)
		}
	}
	if strings.Contains(config, "proxy_ssl_trusted_certificate") {
		t.Fatalf("disabled TLS verify should not render a trusted certificate path:\n%s", config)
	}
	if len(files) != 1 || files[0].Path != OriginCABundleFileName(12) {
		t.Fatalf("expected origin CA support file, got %#v", files)
	}
}

func TestRenderRouteConfigStaticIPRejectsHostname(t *testing.T) {
	_, _, err := RenderRouteConfig([]Route{{
		ID:                9,
		Domain:            "cdn.example.com",
		OriginURL:         "https://origin.example.com",
		OriginResolveMode: OriginResolveModeStaticIP,
		Upstreams:         []string{"https://origin.example.com:8443"},
	}}, ConfigSnapshot{}, RenderOptions{})
	if err == nil {
		t.Fatal("expected static_ip hostname to fail")
	}
	if !strings.Contains(err.Error(), "static_ip origin host origin.example.com must be a public IP address") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRenderRouteConfigMultiUpstreamRejectsPathQueryFragment(t *testing.T) {
	for _, upstreams := range [][]string{
		{"https://origin-a.example.com:8443/api", "https://origin-b.example.com:8443"},
		{"https://origin-a.example.com:8443?debug=1", "https://origin-b.example.com:8443"},
		{"https://origin-a.example.com:8443#frag", "https://origin-b.example.com:8443"},
	} {
		_, _, err := RenderRouteConfig([]Route{{
			ID:        10,
			Domain:    "cdn.example.com",
			OriginURL: "https://origin-a.example.com",
			Upstreams: upstreams,
		}}, ConfigSnapshot{}, RenderOptions{})
		if err == nil {
			t.Fatalf("expected invalid multi-origin upstreams to fail: %#v", upstreams)
		}
		if !strings.Contains(err.Error(), multiOriginShapeError) {
			t.Fatalf("error should mention multi-origin shape, got: %v", err)
		}
	}
}

func TestBasicAuthCredentialHashIsStable(t *testing.T) {
	got := BasicAuthCredentialHash("admin", "secret")
	const want = "7f4db006c4751e37a685071c95013d191b9fdce05096ab52b22b36b0b7b4c251"
	if got != want {
		t.Fatalf("BasicAuthCredentialHash changed: got %s want %s", got, want)
	}
}

func assertInOrder(t *testing.T, text string, needles []string) {
	t.Helper()
	lastIndex := -1
	for _, needle := range needles {
		index := strings.Index(text, needle)
		if index < 0 {
			t.Fatalf("rendered config missing %q:\n%s", needle, text)
		}
		if index <= lastIndex {
			t.Fatalf("rendered config has %q out of order:\n%s", needle, text)
		}
		lastIndex = index
	}
}
