package service

import (
	"strings"
	"testing"

	"dushengcdn/model"
)

func TestProxyRouteLifecycleSyncsNormalizedTables(t *testing.T) {
	setupServiceTestDB(t)

	created, err := CreateProxyRoute(ProxyRouteInput{
		SiteName:     "example-site",
		Domain:       "www.example.com",
		Domains:      []string{"www.example.com", "api.example.com"},
		OriginURL:    "https://origin-a.internal",
		Upstreams:    []string{"https://origin-b.internal"},
		NodePool:     "edge",
		Enabled:      true,
		CacheEnabled: true,
		CachePolicy:  proxyRouteCachePolicySuffix,
		CacheRules:   []string{"jpg"},
		CustomHeaders: []ProxyRouteCustomHeaderInput{
			{Key: "X-Test", Value: "yes"},
		},
		BasicAuthEnabled:  true,
		BasicAuthUsername: "admin",
		BasicAuthPassword: "secret",
	})
	if err != nil {
		t.Fatalf("CreateProxyRoute failed: %v", err)
	}

	site, err := model.GetProxySiteByRouteID(created.ID)
	if err != nil {
		t.Fatalf("GetProxySiteByRouteID failed: %v", err)
	}
	if site.Name != "example-site" || site.NodePool != "edge" || !site.Enabled {
		t.Fatalf("unexpected proxy site: %+v", site)
	}
	domains, err := model.ListProxySiteDomainsByRouteID(created.ID)
	if err != nil {
		t.Fatalf("ListProxySiteDomainsByRouteID failed: %v", err)
	}
	if len(domains) != 2 || domains[0].Domain != "www.example.com" || !domains[0].IsPrimary || domains[1].Domain != "api.example.com" {
		t.Fatalf("unexpected domains: %+v", domains)
	}
	servers, err := model.ListOriginServersByRouteID(created.ID)
	if err != nil {
		t.Fatalf("ListOriginServersByRouteID failed: %v", err)
	}
	if len(servers) != 2 || servers[0].Host != "origin-a.internal" || servers[1].Host != "origin-b.internal" {
		t.Fatalf("unexpected origin servers: %+v", servers)
	}
	cachePolicy, err := model.GetCachePolicyByRouteID(created.ID)
	if err != nil {
		t.Fatalf("GetCachePolicyByRouteID failed: %v", err)
	}
	if !cachePolicy.Enabled || cachePolicy.Policy != proxyRouteCachePolicySuffix || cachePolicy.RulesJSON != `["jpg"]` {
		t.Fatalf("unexpected cache policy: %+v", cachePolicy)
	}
	securityPolicy, err := model.GetSecurityPolicyByRouteID(created.ID)
	if err != nil {
		t.Fatalf("GetSecurityPolicyByRouteID failed: %v", err)
	}
	if !securityPolicy.BasicAuthEnabled || securityPolicy.BasicAuthUsername != "admin" || securityPolicy.BasicAuthPasswordHash == "" {
		t.Fatalf("unexpected security policy: %+v", securityPolicy)
	}
	var stored model.ProxyRoute
	if err := model.DB.First(&stored, created.ID).Error; err != nil {
		t.Fatalf("load stored proxy route: %v", err)
	}
	if stored.BasicAuthPassword != "" || stored.BasicAuthPasswordHash == "" || stored.BasicAuthPasswordUpdatedAt == nil {
		t.Fatalf("expected basic auth password to be stored only as hash, got password=%q hash=%q updated_at=%v", stored.BasicAuthPassword, stored.BasicAuthPasswordHash, stored.BasicAuthPasswordUpdatedAt)
	}
	preservedHash := stored.BasicAuthPasswordHash

	if _, err = UpdateProxyRoute(created.ID, ProxyRouteInput{
		SiteName:  "example-site",
		Domain:    "app.example.com",
		Domains:   []string{"app.example.com"},
		OriginURL: "http://origin-c.internal:8080",
		NodePool:  "default",
		Enabled:   true,
	}); err != nil {
		t.Fatalf("UpdateProxyRoute failed: %v", err)
	}
	domains, err = model.ListProxySiteDomainsByRouteID(created.ID)
	if err != nil {
		t.Fatalf("ListProxySiteDomainsByRouteID after update failed: %v", err)
	}
	if len(domains) != 1 || domains[0].Domain != "app.example.com" || !domains[0].IsPrimary {
		t.Fatalf("unexpected domains after update: %+v", domains)
	}
	servers, err = model.ListOriginServersByRouteID(created.ID)
	if err != nil {
		t.Fatalf("ListOriginServersByRouteID after update failed: %v", err)
	}
	if len(servers) != 1 || servers[0].Scheme != "http" || servers[0].Host != "origin-c.internal" || servers[0].Port != "8080" {
		t.Fatalf("unexpected origin servers after update: %+v", servers)
	}
	stored = model.ProxyRoute{}
	if err := model.DB.First(&stored, created.ID).Error; err != nil {
		t.Fatalf("reload stored proxy route after update: %v", err)
	}
	if stored.BasicAuthEnabled || stored.BasicAuthPasswordHash != "" || stored.BasicAuthPasswordUpdatedAt != nil {
		t.Fatalf("expected basic auth hash to be cleared when disabled, got %+v", stored)
	}

	updated, err := UpdateProxyRoute(created.ID, ProxyRouteInput{
		SiteName:          "example-site",
		Domain:            "app.example.com",
		Domains:           []string{"app.example.com"},
		OriginURL:         "http://origin-c.internal:8080",
		NodePool:          "default",
		Enabled:           true,
		BasicAuthEnabled:  true,
		BasicAuthUsername: "admin",
		BasicAuthPassword: "secret",
	})
	if err != nil {
		t.Fatalf("UpdateProxyRoute re-enable basic auth failed: %v", err)
	}
	if !updated.BasicAuthPasswordConfigured || updated.BasicAuthPasswordUpdatedAt == nil {
		t.Fatalf("expected re-enabled basic auth to be configured, got %+v", updated)
	}
	stored = model.ProxyRoute{}
	if err := model.DB.First(&stored, created.ID).Error; err != nil {
		t.Fatalf("reload stored proxy route after re-enable: %v", err)
	}
	if stored.BasicAuthPasswordHash != preservedHash {
		t.Fatalf("expected same credentials to produce same hash, got %q want %q", stored.BasicAuthPasswordHash, preservedHash)
	}

	_, err = UpdateProxyRoute(created.ID, ProxyRouteInput{
		SiteName:          "example-site",
		Domain:            "app.example.com",
		Domains:           []string{"app.example.com"},
		OriginURL:         "http://origin-c.internal:8080",
		NodePool:          "default",
		Enabled:           true,
		BasicAuthEnabled:  true,
		BasicAuthUsername: "renamed-admin",
		BasicAuthPassword: "",
	})
	if err == nil {
		t.Fatal("expected username change without password to be rejected")
	}

	if err = DeleteProxyRoute(created.ID); err != nil {
		t.Fatalf("DeleteProxyRoute failed: %v", err)
	}
	domains, err = model.ListProxySiteDomainsByRouteID(created.ID)
	if err != nil {
		t.Fatalf("ListProxySiteDomainsByRouteID after delete failed: %v", err)
	}
	if len(domains) != 0 {
		t.Fatalf("expected normalized domains to be deleted, got %+v", domains)
	}
	servers, err = model.ListOriginServersByRouteID(created.ID)
	if err != nil {
		t.Fatalf("ListOriginServersByRouteID after delete failed: %v", err)
	}
	if len(servers) != 0 {
		t.Fatalf("expected normalized origin servers to be deleted, got %+v", servers)
	}
}

func TestPublishConfigVersionUsesReusableOriginGroupAndCachePolicy(t *testing.T) {
	setupServiceTestDB(t)
	if err := model.UpdateOption("OpenRestyCacheEnabled", "true"); err != nil {
		t.Fatalf("UpdateOption OpenRestyCacheEnabled failed: %v", err)
	}
	if err := model.UpdateOption("OpenRestyCachePath", "/var/cache/openresty/dushengcdn"); err != nil {
		t.Fatalf("UpdateOption OpenRestyCachePath failed: %v", err)
	}

	group := model.OriginGroup{
		ProxyRouteID: 0,
		Name:         "shared-origin",
		ResolveMode:  "publish_resolve",
	}
	if err := model.DB.Create(&group).Error; err != nil {
		t.Fatalf("create shared origin group: %v", err)
	}
	if err := model.DB.Create(&model.OriginServer{
		OriginGroupID: group.ID,
		ProxyRouteID:  0,
		URL:           "http://8.8.8.8:39010",
		Scheme:        "http",
		Host:          "8.8.8.8",
		Port:          "39010",
		Weight:        1,
		Enabled:       true,
		URI:           "/",
	}).Error; err != nil {
		t.Fatalf("create shared origin server: %v", err)
	}
	policy := model.CachePolicy{
		ProxyRouteID:      0,
		Name:              "shared-cache",
		Enabled:           true,
		Policy:            proxyRouteCachePolicySuffix,
		RulesJSON:         `["png"]`,
		StatusTTLsJSON:    "{}",
		BypassCookiesJSON: "[]",
		BypassHeadersJSON: "[]",
		IncludeQuery:      true,
		IgnoreQueryParams: "[]",
		CacheMethodsJSON:  `["GET","HEAD"]`,
	}
	if err := model.DB.Create(&policy).Error; err != nil {
		t.Fatalf("create shared cache policy: %v", err)
	}

	route, err := CreateProxyRoute(ProxyRouteInput{
		Domain:    "reusable.example.com",
		OriginURL: "http://8.8.4.4:39010",
		Enabled:   true,
	})
	if err != nil {
		t.Fatalf("CreateProxyRoute failed: %v", err)
	}
	if err := model.DB.Model(&model.ProxyRoute{}).Where("id = ?", route.ID).Updates(map[string]any{
		"origin_group_id": group.ID,
		"cache_policy_id": policy.ID,
	}).Error; err != nil {
		t.Fatalf("bind reusable origin/cache policy: %v", err)
	}

	result, err := PublishConfigVersion("root", false)
	if err != nil {
		t.Fatalf("PublishConfigVersion failed: %v", err)
	}
	if !strings.Contains(result.Version.RenderedConfig, "server 8.8.8.8:39010 max_fails=3 fail_timeout=10s;") {
		t.Fatalf("expected reusable origin server to render, got %s", result.Version.RenderedConfig)
	}
	if strings.Contains(result.Version.RenderedConfig, "server 8.8.4.4:39010 max_fails=3 fail_timeout=10s;") {
		t.Fatal("expected legacy route origin to be overridden by reusable origin group")
	}
	if !strings.Contains(result.Version.RenderedConfig, `if ($uri !~* "\\.(?:png)$")`) {
		t.Fatal("expected reusable cache policy suffix rule to render")
	}
	if !strings.Contains(result.Version.SnapshotJSON, `"upstreams":["http://8.8.8.8:39010"]`) {
		t.Fatalf("expected snapshot to use reusable origin upstreams, got %s", result.Version.SnapshotJSON)
	}
	if !strings.Contains(result.Version.SnapshotJSON, `"cache_policy":"suffix"`) {
		t.Fatalf("expected snapshot to use reusable cache policy, got %s", result.Version.SnapshotJSON)
	}
}

func TestProxyRouteDomainUniquenessUsesNormalizedDomainTable(t *testing.T) {
	setupServiceTestDB(t)

	route, err := CreateProxyRoute(ProxyRouteInput{
		Domain:    "www.example.com",
		Domains:   []string{"www.example.com", "api.example.com"},
		OriginURL: "https://origin-a.internal",
		Enabled:   true,
	})
	if err != nil {
		t.Fatalf("CreateProxyRoute failed: %v", err)
	}
	if _, err = model.GetProxySiteDomainByDomain("api.example.com"); err != nil {
		t.Fatalf("expected normalized domain binding for route %d: %v", route.ID, err)
	}

	_, err = CreateProxyRoute(ProxyRouteInput{
		Domain:    "api.example.com",
		OriginURL: "https://origin-b.internal",
		Enabled:   true,
	})
	if err == nil {
		t.Fatal("expected duplicate normalized domain to be rejected")
	}
}
