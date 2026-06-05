package service

import (
	"dushengcdn/common"
	"dushengcdn/model"
	"strings"
	"testing"
)

func TestRequestProxyRouteCachePurgeRejectsUnsafeConfiguredCachePath(t *testing.T) {
	previous := common.OpenRestyCachePath
	common.OpenRestyCachePath = "/home/admin"
	t.Cleanup(func() {
		common.OpenRestyCachePath = previous
	})

	if _, err := RequestProxyRouteCachePurge(1, CacheOperationInput{Scope: "all"}); err == nil {
		t.Fatal("expected unsafe OpenResty cache path to be rejected before route lookup")
	}
}

func TestNormalizeCacheWarmURLsForRouteAllowsOnlyRouteDomains(t *testing.T) {
	route := &model.ProxyRoute{
		Domain:  "www.example.com",
		Domains: `["www.example.com","*.example.net"]`,
	}

	urls, err := normalizeCacheWarmURLsForRoute(route, []string{
		" https://WWW.EXAMPLE.COM/path?a=1#section ",
		"https://static.example.net/app.css",
		"https://www.example.com/path?a=1",
	})
	if err != nil {
		t.Fatalf("normalizeCacheWarmURLsForRoute failed: %v", err)
	}

	expected := []string{
		"https://www.example.com/path?a=1",
		"https://static.example.net/app.css",
	}
	if len(urls) != len(expected) {
		t.Fatalf("unexpected URL count: got %#v want %#v", urls, expected)
	}
	for index := range expected {
		if urls[index] != expected[index] {
			t.Fatalf("unexpected URL at %d: got %q want %q", index, urls[index], expected[index])
		}
	}
}

func TestNormalizeCacheWarmURLsForRouteRejectsUnsafeTargets(t *testing.T) {
	route := &model.ProxyRoute{
		Domain:  "www.example.com",
		Domains: `["www.example.com","*.example.net"]`,
	}
	testCases := []struct {
		name string
		url  string
	}{
		{name: "other host", url: "https://evil.example.com/app.js"},
		{name: "wildcard base", url: "https://example.net/app.js"},
		{name: "wildcard deep", url: "https://deep.static.example.net/app.js"},
		{name: "userinfo", url: "https://user:pass@www.example.com/app.js"},
		{name: "non default port", url: "https://www.example.com:8443/app.js"},
		{name: "private literal", url: "http://127.0.0.1/app.js"},
		{name: "bad scheme", url: "file:///etc/passwd"},
	}

	for _, testCase := range testCases {
		if _, err := normalizeCacheWarmURLsForRoute(route, []string{testCase.url}); err == nil {
			t.Fatalf("%s: expected URL to be rejected", testCase.name)
		}
	}
}

func TestNormalizeCacheWarmURLsForRouteLimitsBatchSize(t *testing.T) {
	route := &model.ProxyRoute{Domain: "www.example.com"}
	urls := make([]string, 0, maxCacheWarmURLs+1)
	for index := 0; index < maxCacheWarmURLs+1; index++ {
		urls = append(urls, "https://www.example.com/assets/"+strings.Repeat("x", index+1))
	}

	if _, err := normalizeCacheWarmURLsForRoute(route, urls); err == nil {
		t.Fatal("expected oversized cache warm batch to be rejected")
	}
}
