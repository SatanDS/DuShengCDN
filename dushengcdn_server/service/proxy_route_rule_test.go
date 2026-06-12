package service

import (
	"reflect"
	"strings"
	"testing"

	"dushengcdn/model"
)

func TestProxyRouteRulesRenderPathLevelLocations(t *testing.T) {
	setupServiceTestDB(t)
	if err := model.UpdateOption("OpenRestyCacheEnabled", "true"); err != nil {
		t.Fatalf("UpdateOption OpenRestyCacheEnabled failed: %v", err)
	}
	if err := model.UpdateOption("OpenRestyCachePath", "/var/cache/openresty/dushengcdn"); err != nil {
		t.Fatalf("UpdateOption OpenRestyCachePath failed: %v", err)
	}

	enabled := true
	disabled := false
	route, err := CreateProxyRoute(ProxyRouteInput{
		SiteName:     "path-site",
		Domain:       "paths.example.com",
		OriginURL:    "http://8.8.8.8",
		Enabled:      true,
		CacheEnabled: false,
		RouteRules: []ProxyRouteRuleInput{
			{
				Name:              "admin",
				MatchType:         proxyRouteRuleMatchExact,
				Path:              "/admin",
				Priority:          1,
				Enabled:           &enabled,
				OriginURL:         "https://93.184.216.34:443",
				CacheEnabled:      &disabled,
				BasicAuthEnabled:  &enabled,
				BasicAuthUsername: "ops",
				BasicAuthPassword: "secret",
			},
			{
				Name:         "static",
				MatchType:    proxyRouteRuleMatchPrefix,
				Path:         "/static",
				Priority:     2,
				Enabled:      &enabled,
				OriginURL:    "http://9.9.9.9",
				CacheEnabled: &enabled,
				CachePolicy:  proxyRouteCachePolicyPathPrefix,
				CacheRules:   []string{"/static/"},
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateProxyRoute failed: %v", err)
	}
	if len(route.RouteRules) != 3 {
		t.Fatalf("expected default rule plus two explicit route rules in view, got %+v", route.RouteRules)
	}

	release, err := PublishConfigVersion("root", false)
	if err != nil {
		t.Fatalf("PublishConfigVersion failed: %v", err)
	}
	rendered := release.Version.RenderedConfig
	adminIndex := strings.Index(rendered, "location = /admin {")
	staticIndex := strings.Index(rendered, "location ^~ /static/ {")
	rootIndex := strings.LastIndex(rendered, "location / {")
	if adminIndex < 0 || staticIndex < 0 || rootIndex < 0 {
		t.Fatalf("expected exact, prefix, and root fallback locations, got %s", rendered)
	}
	if !(adminIndex < staticIndex && staticIndex < rootIndex) {
		t.Fatalf("expected route rule locations before root fallback, got indexes admin=%d static=%d root=%d", adminIndex, staticIndex, rootIndex)
	}
	if strings.Count(rendered, "proxy_cache dushengcdn_cache;") != 1 {
		t.Fatalf("expected cache block only for static path rule, got %s", rendered)
	}
	if !strings.Contains(rendered, "rewrite_by_lua_block") {
		t.Fatalf("expected Basic Auth block on admin rule, got %s", rendered)
	}

	rules, err := model.ListProxyRouteRulesByRouteID(route.ID)
	if err != nil {
		t.Fatalf("ListProxyRouteRulesByRouteID failed: %v", err)
	}
	if len(rules) != 3 {
		t.Fatalf("expected default rule plus two stored path rules after replacement, got %+v", rules)
	}
	if rules[len(rules)-1].MatchType != proxyRouteRuleMatchDefault || rules[len(rules)-1].Path != "/" {
		t.Fatalf("expected default rule to be preserved after replacement, got %+v", rules)
	}

	updated, err := UpdateProxyRoute(route.ID, ProxyRouteInput{
		SiteName:   "path-site",
		Domain:     "paths.example.com",
		OriginURL:  "http://8.8.8.8",
		Enabled:    true,
		RouteRules: []ProxyRouteRuleInput{},
	})
	if err != nil {
		t.Fatalf("UpdateProxyRoute clear route rules failed: %v", err)
	}
	if len(updated.RouteRules) != 1 || updated.RouteRules[0].MatchType != proxyRouteRuleMatchDefault {
		t.Fatalf("expected empty route_rules to preserve default normalized rule, got %+v", updated.RouteRules)
	}
}

func TestProxyRouteRuleUpsertPreservesIDsAndRenderedChecksum(t *testing.T) {
	setupServiceTestDB(t)
	if err := model.UpdateOption("OpenRestyCacheEnabled", "true"); err != nil {
		t.Fatalf("UpdateOption OpenRestyCacheEnabled failed: %v", err)
	}
	if err := model.UpdateOption("OpenRestyCachePath", "/var/cache/openresty/dushengcdn"); err != nil {
		t.Fatalf("UpdateOption OpenRestyCachePath failed: %v", err)
	}

	enabled := true
	routeRule := ProxyRouteRuleInput{
		Name:              "api",
		MatchType:         proxyRouteRuleMatchPrefix,
		Path:              "/api",
		Priority:          1,
		Enabled:           &enabled,
		OriginURL:         "http://9.9.9.9",
		CacheEnabled:      &enabled,
		CachePolicy:       proxyRouteCachePolicyPathPrefix,
		CacheRules:        []string{"/api/"},
		BasicAuthEnabled:  &enabled,
		BasicAuthUsername: "ops",
		BasicAuthPassword: "secret",
	}
	route, err := CreateProxyRoute(ProxyRouteInput{
		SiteName:     "stable-rule-site",
		Domain:       "stable-rule.example.com",
		OriginURL:    "http://8.8.8.8",
		Enabled:      true,
		CacheEnabled: false,
		RouteRules:   []ProxyRouteRuleInput{routeRule},
	})
	if err != nil {
		t.Fatalf("CreateProxyRoute failed: %v", err)
	}
	initialState := mustProxyRouteRuleState(t, route.ID, proxyRouteRuleMatchPrefix, "/api/")
	initialBundle, err := buildCurrentConfigBundle(true)
	if err != nil {
		t.Fatalf("buildCurrentConfigBundle initial failed: %v", err)
	}

	routeRule.BasicAuthPassword = ""
	updated, err := UpdateProxyRoute(route.ID, ProxyRouteInput{
		SiteName:     "stable-rule-site",
		Domain:       "stable-rule.example.com",
		OriginURL:    "http://8.8.8.8",
		Enabled:      true,
		CacheEnabled: false,
		RouteRules:   []ProxyRouteRuleInput{routeRule},
	})
	if err != nil {
		t.Fatalf("UpdateProxyRoute unchanged save failed: %v", err)
	}
	if len(updated.RouteRules) != 2 {
		t.Fatalf("expected default plus explicit rule after unchanged save, got %+v", updated.RouteRules)
	}
	nextState := mustProxyRouteRuleState(t, route.ID, proxyRouteRuleMatchPrefix, "/api/")
	if !reflect.DeepEqual(initialState, nextState) {
		t.Fatalf("expected unchanged save to preserve rule child IDs, before=%+v after=%+v", initialState, nextState)
	}
	nextBundle, err := buildCurrentConfigBundle(true)
	if err != nil {
		t.Fatalf("buildCurrentConfigBundle after unchanged save failed: %v", err)
	}
	if initialBundle.Checksum != nextBundle.Checksum {
		t.Fatalf("expected unchanged route rule save to keep rendered checksum stable, before=%s after=%s", initialBundle.Checksum, nextBundle.Checksum)
	}
	if initialBundle.RouteConfig != nextBundle.RouteConfig {
		t.Fatal("expected unchanged route rule save to keep rendered route config stable")
	}

	if _, err := UpdateProxyRoute(route.ID, ProxyRouteInput{
		SiteName:     "stable-rule-site",
		Domain:       "stable-rule.example.com",
		OriginURL:    "http://8.8.8.8",
		Enabled:      true,
		CacheEnabled: false,
		RouteRules:   []ProxyRouteRuleInput{},
	}); err != nil {
		t.Fatalf("UpdateProxyRoute delete rule failed: %v", err)
	}
	assertNoNonDefaultProxyRouteRuleObjects(t, route.ID)
}

func TestProxyRouteCustomHeadersRejectDollarExpansion(t *testing.T) {
	setupServiceTestDB(t)

	_, err := CreateProxyRoute(ProxyRouteInput{
		Domain:    "header-dollar.example.com",
		OriginURL: "http://8.8.8.8",
		Enabled:   true,
		CustomHeaders: []ProxyRouteCustomHeaderInput{
			{Key: "X-Test", Value: "abc$def"},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "custom header value cannot contain $") {
		t.Fatalf("expected route custom header dollar rejection, got %v", err)
	}

	enabled := true
	_, err = CreateProxyRoute(ProxyRouteInput{
		Domain:    "rule-header-dollar.example.com",
		OriginURL: "http://8.8.8.8",
		Enabled:   true,
		RouteRules: []ProxyRouteRuleInput{{
			MatchType: proxyRouteRuleMatchExact,
			Path:      "/api",
			Enabled:   &enabled,
			CustomHeaders: []ProxyRouteCustomHeaderInput{
				{Key: "X-Test", Value: "abc$def"},
			},
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "custom header value cannot contain $") {
		t.Fatalf("expected route rule custom header dollar rejection, got %v", err)
	}
}

func TestProxyRouteRegexRulePublishesQuotedLocation(t *testing.T) {
	setupServiceTestDB(t)

	enabled := true
	_, err := CreateProxyRoute(ProxyRouteInput{
		SiteName:  "regex-site",
		Domain:    "regex.example.com",
		OriginURL: "http://8.8.8.8",
		Enabled:   true,
		RouteRules: []ProxyRouteRuleInput{{
			Name:      "item-regex",
			MatchType: proxyRouteRuleMatchRegex,
			Path:      `^/items/[0-9]+$`,
			Priority:  1,
			Enabled:   &enabled,
			OriginURL: "http://9.9.9.9",
		}},
	})
	if err != nil {
		t.Fatalf("CreateProxyRoute regex rule failed: %v", err)
	}
	release, err := PublishConfigVersion("root", false)
	if err != nil {
		t.Fatalf("PublishConfigVersion regex rule failed: %v", err)
	}
	if !strings.Contains(release.Version.RenderedConfig, `location ~ "^/items/[0-9]+$" {`) {
		t.Fatalf("expected quoted regex location in rendered config, got:\n%s", release.Version.RenderedConfig)
	}
}

type proxyRouteRuleIDState struct {
	RuleID           uint
	OriginGroupID    uint
	OriginServerIDs  []uint
	CachePolicyID    uint
	SecurityPolicyID uint
}

func mustProxyRouteRuleState(t *testing.T, routeID uint, matchType string, path string) proxyRouteRuleIDState {
	t.Helper()
	var rule model.ProxyRouteRule
	if err := model.DB.Where("proxy_route_id = ? AND match_type = ? AND path = ?", routeID, matchType, path).First(&rule).Error; err != nil {
		t.Fatalf("load proxy route rule state: %v", err)
	}
	state := proxyRouteRuleIDState{
		RuleID:        rule.ID,
		OriginGroupID: rule.OriginGroupID,
	}
	if rule.CachePolicyID != nil {
		state.CachePolicyID = *rule.CachePolicyID
	}
	if rule.SecurityPolicyID != nil {
		state.SecurityPolicyID = *rule.SecurityPolicyID
	}
	var servers []model.OriginServer
	if err := model.DB.Where("origin_group_id = ?", rule.OriginGroupID).Order("sort_order asc, id asc").Find(&servers).Error; err != nil {
		t.Fatalf("load proxy route rule origin servers: %v", err)
	}
	for _, server := range servers {
		state.OriginServerIDs = append(state.OriginServerIDs, server.ID)
	}
	return state
}

func assertNoNonDefaultProxyRouteRuleObjects(t *testing.T, routeID uint) {
	t.Helper()
	for _, item := range []struct {
		name  string
		model any
	}{
		{name: "route rules", model: &model.ProxyRouteRule{}},
		{name: "origin groups", model: &model.OriginGroup{}},
		{name: "cache policies", model: &model.CachePolicy{}},
		{name: "security policies", model: &model.SecurityPolicy{}},
	} {
		var count int64
		if err := model.DB.Model(item.model).Where("proxy_route_id = ? AND is_default = ?", routeID, false).Count(&count).Error; err != nil {
			if item.name == "route rules" {
				err = model.DB.Model(item.model).Where("proxy_route_id = ? AND match_type <> ?", routeID, proxyRouteRuleMatchDefault).Count(&count).Error
			}
			if err != nil {
				t.Fatalf("count non-default %s: %v", item.name, err)
			}
		}
		if count != 0 {
			t.Fatalf("expected no non-default %s after deleting rules, got %d", item.name, count)
		}
	}
	var serverCount int64
	if err := model.DB.Model(&model.OriginServer{}).
		Joins("JOIN origin_groups ON origin_groups.id = origin_servers.origin_group_id").
		Where("origin_groups.proxy_route_id = ? AND origin_groups.is_default = ?", routeID, false).
		Count(&serverCount).Error; err != nil {
		t.Fatalf("count non-default origin servers: %v", err)
	}
	if serverCount != 0 {
		t.Fatalf("expected no non-default origin servers after deleting rules, got %d", serverCount)
	}
}
