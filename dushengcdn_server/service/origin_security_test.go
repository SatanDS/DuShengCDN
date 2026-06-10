package service

import (
	"context"
	"net"
	"strings"
	"testing"
)

func TestValidateOriginAddressRejectsUnsafeIPRanges(t *testing.T) {
	unsafeAddresses := []string{
		"127.0.0.1",
		"10.0.0.8",
		"172.16.0.8",
		"192.168.1.8",
		"100.64.0.1",
		"198.18.0.1",
		"169.254.169.254",
		"203.0.113.10",
		"::1",
	}
	for _, address := range unsafeAddresses {
		t.Run(address, func(t *testing.T) {
			if err := validateOriginAddress(address); err == nil {
				t.Fatalf("expected unsafe origin address %s to be rejected", address)
			}
		})
	}
}

func TestValidateOriginURLRejectsUnsafeIPRanges(t *testing.T) {
	for _, rawURL := range []string{
		"http://127.0.0.1:8080",
		"http://100.64.0.1:8080",
		"http://198.18.0.1:8080",
		"http://203.0.113.10:8080",
	} {
		t.Run(rawURL, func(t *testing.T) {
			if err := validateOriginURL(rawURL); err == nil {
				t.Fatal("expected unsafe origin URL to be rejected")
			}
		})
	}
}

func TestValidateOriginURLRejectsUserinfo(t *testing.T) {
	if err := validateOriginURL("https://user:pass@origin.example.net:8443"); err == nil {
		t.Fatal("expected origin URL userinfo to be rejected")
	}
	if _, _, _, _, err := splitOriginURL("https://user:pass@origin.example.net:8443/app"); err == nil {
		t.Fatal("expected splitOriginURL to reject userinfo")
	}
}

func TestPublishConfigVersionPinsHostnameUpstreamToResolvedPublicIP(t *testing.T) {
	setupServiceTestDB(t)
	routeUpstreamLookupIPAddr = func(ctx context.Context, host string) ([]net.IPAddr, error) {
		return []net.IPAddr{
			{IP: net.ParseIP("8.8.4.4")},
			{IP: net.ParseIP("1.1.1.1")},
		}, nil
	}

	_, err := CreateProxyRoute(ProxyRouteInput{
		Domain:     "pin-origin.example.com",
		OriginURL:  "https://origin.example.net:8443/api/",
		OriginHost: "origin.example.net",
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("CreateProxyRoute failed: %v", err)
	}

	result, err := PublishConfigVersion("root", false)
	if err != nil {
		t.Fatalf("PublishConfigVersion failed: %v", err)
	}
	if !strings.Contains(result.Version.RenderedConfig, "server 8.8.4.4:8443 max_fails=3 fail_timeout=10s;") ||
		!strings.Contains(result.Version.RenderedConfig, "server 1.1.1.1:8443 max_fails=3 fail_timeout=10s;") {
		t.Fatalf("expected upstream servers to be pinned to resolved public IPs, got:\n%s", result.Version.RenderedConfig)
	}
	if strings.Contains(result.Version.RenderedConfig, "server origin.example.net:8443") {
		t.Fatalf("expected rendered upstream to avoid runtime hostname resolution, got:\n%s", result.Version.RenderedConfig)
	}
	if !strings.Contains(result.Version.RenderedConfig, `proxy_ssl_name "origin.example.net";`) {
		t.Fatalf("expected upstream TLS SNI to keep the origin hostname, got:\n%s", result.Version.RenderedConfig)
	}
}

func TestCreateProxyRouteRejectsUnsafeOriginPathAndHostHeaders(t *testing.T) {
	setupServiceTestDB(t)
	tests := []struct {
		name  string
		input ProxyRouteInput
	}{
		{
			name: "url semicolon path",
			input: ProxyRouteInput{
				Domain:    "semicolon-url.example.com",
				OriginURL: "http://8.8.8.8:8080/app;internal;",
				Enabled:   true,
			},
		},
		{
			name: "structured semicolon path",
			input: ProxyRouteInput{
				Domain:        "semicolon-structured.example.com",
				OriginScheme:  "http",
				OriginAddress: "8.8.8.8",
				OriginPort:    "8080",
				OriginURI:     "/app;internal;",
				Enabled:       true,
			},
		},
		{
			name: "invalid origin url port",
			input: ProxyRouteInput{
				Domain:    "bad-port.example.com",
				OriginURL: "http://8.8.8.8:65536",
				Enabled:   true,
			},
		},
		{
			name: "origin host variable",
			input: ProxyRouteInput{
				Domain:     "origin-host-variable.example.com",
				OriginURL:  "https://8.8.8.8:8443",
				OriginHost: "$http_x_origin",
				Enabled:    true,
			},
		},
		{
			name: "custom host header",
			input: ProxyRouteInput{
				Domain:    "custom-host.example.com",
				OriginURL: "https://8.8.8.8:8443",
				Enabled:   true,
				CustomHeaders: []ProxyRouteCustomHeaderInput{
					{Key: "Host", Value: "attacker.example.com"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := CreateProxyRoute(tt.input); err == nil {
				t.Fatal("expected unsafe origin route input to be rejected")
			}
		})
	}
}

func TestPublishConfigVersionRejectsHostnameUpstreamResolvingToUnsafeIP(t *testing.T) {
	setupServiceTestDB(t)
	routeUpstreamLookupIPAddr = func(ctx context.Context, host string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
	}

	if _, err := CreateProxyRoute(ProxyRouteInput{
		Domain:    "unsafe-origin.example.com",
		OriginURL: "http://origin.example.net:8080",
		Enabled:   true,
	}); err != nil {
		t.Fatalf("CreateProxyRoute failed: %v", err)
	}

	if _, err := PublishConfigVersion("root", false); err == nil || !strings.Contains(err.Error(), "resolved to unsafe ip") {
		t.Fatalf("expected unsafe resolved origin to block publish, got %v", err)
	}
}
