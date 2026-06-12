package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"dushengcdn/model"

	"gorm.io/gorm"
)

// replaceProxyRouteRuleInputsWithDB expects the caller to have synced the
// normalized tables for the route in the same transaction (InsertWithDB and
// UpdateWithDB both do), so the default site and origin group already exist.
func replaceProxyRouteRuleInputsWithDB(db *gorm.DB, route *model.ProxyRoute, inputs []ProxyRouteRuleInput) error {
	if db == nil {
		db = model.DB
	}
	if db == nil || route == nil || route.ID == 0 {
		return nil
	}
	previousRuleSecurityPolicies, err := loadProxyRouteRuleSecurityPoliciesByMatch(db, route.ID)
	if err != nil {
		return err
	}
	if err := db.Where("proxy_route_id = ? AND match_type <> ?", route.ID, proxyRouteRuleMatchDefault).Delete(&model.ProxyRouteRule{}).Error; err != nil {
		return err
	}
	var staleGroups []model.OriginGroup
	if err := db.Where("proxy_route_id = ? AND is_default = ?", route.ID, false).Find(&staleGroups).Error; err != nil {
		return err
	}
	if len(staleGroups) > 0 {
		groupIDs := make([]uint, 0, len(staleGroups))
		for _, group := range staleGroups {
			groupIDs = append(groupIDs, group.ID)
		}
		if err := db.Where("origin_group_id IN ?", groupIDs).Delete(&model.OriginServer{}).Error; err != nil {
			return err
		}
		if err := db.Where("id IN ?", groupIDs).Delete(&model.OriginGroup{}).Error; err != nil {
			return err
		}
	}
	if err := db.Where("proxy_route_id = ? AND is_default = ?", route.ID, false).Delete(&model.CachePolicy{}).Error; err != nil {
		return err
	}
	if err := db.Where("proxy_route_id = ? AND is_default = ?", route.ID, false).Delete(&model.SecurityPolicy{}).Error; err != nil {
		return err
	}
	var defaultOriginGroup model.OriginGroup
	if err := db.Where("proxy_route_id = ? AND is_default = ?", route.ID, true).First(&defaultOriginGroup).Error; err != nil {
		return err
	}
	site, err := model.GetProxySiteByRouteIDWithDB(db, route.ID)
	if err != nil {
		return err
	}
	seenRuleMatches := make(map[string]struct{}, len(inputs))
	for index, input := range inputs {
		rule, err := normalizeProxyRouteRuleInput(route, site.ID, defaultOriginGroup.ID, input, index)
		if err != nil {
			return err
		}
		if rule.MatchType == proxyRouteRuleMatchDefault {
			// The default rule mirrors the route-level settings and is managed
			// by SyncProxyRouteNormalizedTablesWithDB; storing another row here
			// would render a duplicate root location and accumulate across
			// saves (the delete above only removes non-default rules).
			continue
		}
		matchKey := rule.MatchType + " " + rule.Path
		if _, ok := seenRuleMatches[matchKey]; ok {
			return fmt.Errorf("route_rules contains duplicated match %s %s", rule.MatchType, rule.Path)
		}
		seenRuleMatches[matchKey] = struct{}{}
		if strings.TrimSpace(input.OriginURL) != "" || len(input.Upstreams) > 0 {
			group := &model.OriginGroup{
				ProxyRouteID: route.ID,
				OriginID:     route.OriginID,
				Name:         rule.Name,
				IsDefault:    false,
			}
			if strings.TrimSpace(group.Name) == "" {
				group.Name = fmt.Sprintf("rule-%d", index+1)
			}
			if err := db.Create(group).Error; err != nil {
				return err
			}
			originURL := strings.TrimSpace(input.OriginURL)
			if originURL == "" {
				originURL = route.OriginURL
			}
			upstreams, err := normalizeUpstreams(originURL, input.Upstreams)
			if err != nil {
				return err
			}
			tempRoute := *route
			upstreamsJSON, err := json.Marshal(upstreams)
			if err != nil {
				return err
			}
			tempRoute.OriginURL = upstreams[0]
			tempRoute.Upstreams = string(upstreamsJSON)
			if err := model.ReplaceOriginServersForProxyRouteRule(db, group.ID, &tempRoute, route.OriginID); err != nil {
				return err
			}
			rule.OriginGroupID = group.ID
		}
		if input.CacheEnabled != nil {
			cacheRules, err := normalizeCacheRules(*input.CacheEnabled, input.CachePolicy, input.CacheRules)
			if err != nil {
				return err
			}
			cacheRulesJSON, err := json.Marshal(cacheRules)
			if err != nil {
				return err
			}
			cachePolicy := &model.CachePolicy{
				ProxyRouteID: route.ID,
				Name:         rule.Name,
				IsDefault:    false,
				Enabled:      *input.CacheEnabled,
				Policy:       normalizeCachePolicy(*input.CacheEnabled, input.CachePolicy),
				RulesJSON:    string(cacheRulesJSON),
			}
			if strings.TrimSpace(cachePolicy.Name) == "" {
				cachePolicy.Name = fmt.Sprintf("rule-%d-cache", index+1)
			}
			if err := db.Create(cachePolicy).Error; err != nil {
				return err
			}
			rule.CachePolicyID = &cachePolicy.ID
		}
		if input.BasicAuthEnabled != nil {
			securityPolicy, err := buildProxyRouteRuleSecurityPolicy(route.ID, rule.Name, input, previousRuleSecurityPolicies[matchKey])
			if err != nil {
				return err
			}
			if strings.TrimSpace(securityPolicy.Name) == "" {
				securityPolicy.Name = fmt.Sprintf("rule-%d-security", index+1)
			}
			if err := db.Create(securityPolicy).Error; err != nil {
				return err
			}
			rule.SecurityPolicyID = &securityPolicy.ID
		}
		if err := db.Create(rule).Error; err != nil {
			return err
		}
	}
	return nil
}

func normalizeProxyRouteRuleInput(route *model.ProxyRoute, siteID uint, defaultOriginGroupID uint, input ProxyRouteRuleInput, index int) (*model.ProxyRouteRule, error) {
	matchType, path, err := normalizeProxyRouteRuleMatch(input.MatchType, input.Path)
	if err != nil {
		return nil, err
	}
	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	priority := input.Priority
	if priority == 0 {
		priority = index + 1
	}
	originHostHeader := strings.TrimSpace(input.OriginHostHeader)
	if originHostHeader == "" {
		originHostHeader = normalizeStoredOriginHostHeader(route)
	}
	if err := validateOriginHostHeader(originHostHeader); err != nil {
		return nil, err
	}
	originSNI := strings.TrimSpace(input.OriginSNI)
	if err := validateOriginSNI(originSNI); err != nil {
		return nil, err
	}
	originTLSVerify := normalizeOriginTLSVerify(input.OriginTLSVerify)
	if input.OriginTLSVerify == nil {
		originTLSVerify = normalizeStoredOriginTLSVerify(route)
	}
	originResolveMode := strings.TrimSpace(input.OriginResolveMode)
	if originResolveMode == "" {
		originResolveMode = normalizeStoredOriginResolveMode(route.OriginResolveMode)
	}
	originResolveMode, err = normalizeOriginResolveMode(originResolveMode)
	if err != nil {
		return nil, err
	}
	customHeaders, err := normalizeCustomHeaders(input.CustomHeaders)
	if err != nil {
		return nil, err
	}
	customHeadersJSON, err := json.Marshal(customHeaders)
	if err != nil {
		return nil, err
	}
	limitConnPerServer, err := normalizeProxyRouteLimitConnValue(input.LimitConnPerServer, "route_rules.limit_conn_per_server")
	if err != nil {
		return nil, err
	}
	limitConnPerIP, err := normalizeProxyRouteLimitConnValue(input.LimitConnPerIP, "route_rules.limit_conn_per_ip")
	if err != nil {
		return nil, err
	}
	limitRate, err := normalizeProxyRouteLimitRate(input.LimitRate)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = fmt.Sprintf("%s %s", matchType, path)
	}
	return &model.ProxyRouteRule{
		ProxyRouteID:       route.ID,
		ProxySiteID:        siteID,
		OriginGroupID:      defaultOriginGroupID,
		Name:               name,
		MatchType:          matchType,
		Path:               path,
		Priority:           priority,
		Enabled:            enabled,
		OriginHostHeader:   originHostHeader,
		OriginSNI:          originSNI,
		OriginTLSVerify:    originTLSVerify,
		OriginCABundle:     strings.TrimSpace(input.OriginCABundle),
		OriginResolveMode:  originResolveMode,
		LimitConnPerServer: limitConnPerServer,
		LimitConnPerIP:     limitConnPerIP,
		LimitRate:          limitRate,
		ProxyBufferingMode: normalizeProxyRouteProxyBufferingMode(input.ProxyBufferingMode),
		CustomHeadersJSON:  string(customHeadersJSON),
	}, nil
}

func normalizeProxyRouteRuleMatch(rawMatchType string, rawPath string) (string, string, error) {
	matchType := strings.ToLower(strings.TrimSpace(rawMatchType))
	if matchType == "" {
		matchType = proxyRouteRuleMatchPrefix
	}
	path := strings.TrimSpace(rawPath)
	switch matchType {
	case proxyRouteRuleMatchDefault:
		return proxyRouteRuleMatchDefault, "/", nil
	case proxyRouteRuleMatchPrefix, proxyRouteRuleMatchExact:
		if path == "" {
			return "", "", errors.New("route_rules.path cannot be empty")
		}
		if !strings.HasPrefix(path, "/") || strings.Contains(path, "://") || strings.ContainsAny(path, " \t\r\n;{}") {
			return "", "", errors.New("route_rules.path format is invalid")
		}
		if matchType == proxyRouteRuleMatchPrefix && path != "/" && !strings.HasSuffix(path, "/") {
			path += "/"
		}
		return matchType, path, nil
	case proxyRouteRuleMatchRegex:
		if path == "" || strings.ContainsAny(path, "\r\n;") {
			return "", "", errors.New("route_rules regex path format is invalid")
		}
		if _, err := regexp.Compile(path); err != nil {
			return "", "", errors.New("route_rules regex path is invalid")
		}
		return matchType, path, nil
	default:
		return "", "", errors.New("route_rules.match_type must be one of prefix, exact, regex, default")
	}
}

// loadProxyRouteRuleSecurityPoliciesByMatch indexes the existing non-default
// security policies by their rule match key so a save without a password can
// keep the previous credential hash (mirroring the route-level behaviour in
// normalizeProxyRouteBasicAuth). Must run before the old rules are deleted.
func loadProxyRouteRuleSecurityPoliciesByMatch(db *gorm.DB, routeID uint) (map[string]*model.SecurityPolicy, error) {
	var rules []model.ProxyRouteRule
	if err := db.Where("proxy_route_id = ? AND match_type <> ?", routeID, proxyRouteRuleMatchDefault).Find(&rules).Error; err != nil {
		return nil, err
	}
	policyIDs := make([]uint, 0, len(rules))
	for _, rule := range rules {
		if rule.SecurityPolicyID != nil && *rule.SecurityPolicyID != 0 {
			policyIDs = append(policyIDs, *rule.SecurityPolicyID)
		}
	}
	result := make(map[string]*model.SecurityPolicy, len(policyIDs))
	if len(policyIDs) == 0 {
		return result, nil
	}
	var policies []model.SecurityPolicy
	if err := db.Where("id IN ?", policyIDs).Find(&policies).Error; err != nil {
		return nil, err
	}
	policiesByID := make(map[uint]*model.SecurityPolicy, len(policies))
	for index := range policies {
		policiesByID[policies[index].ID] = &policies[index]
	}
	for _, rule := range rules {
		if rule.SecurityPolicyID == nil {
			continue
		}
		if policy, ok := policiesByID[*rule.SecurityPolicyID]; ok {
			result[rule.MatchType+" "+rule.Path] = policy
		}
	}
	return result, nil
}

func buildProxyRouteRuleSecurityPolicy(routeID uint, name string, input ProxyRouteRuleInput, previous *model.SecurityPolicy) (*model.SecurityPolicy, error) {
	enabled := input.BasicAuthEnabled != nil && *input.BasicAuthEnabled
	username := strings.TrimSpace(input.BasicAuthUsername)
	password := strings.TrimSpace(input.BasicAuthPassword)
	passwordHash := ""
	var updatedAt *time.Time
	if enabled {
		if username == "" {
			return nil, errors.New("route_rules basic_auth_username and basic_auth_password cannot be empty when basic auth is enabled")
		}
		now := time.Now()
		switch {
		case password != "":
			passwordHash = model.BasicAuthCredentialHash(username, password)
			updatedAt = &now
		case previous != nil && previous.BasicAuthEnabled &&
			strings.TrimSpace(previous.BasicAuthUsername) == username &&
			strings.TrimSpace(previous.BasicAuthPasswordHash) != "":
			// The view never returns the password, so an unchanged save keeps
			// the previous credential hash.
			passwordHash = previous.BasicAuthPasswordHash
			updatedAt = previous.BasicAuthPasswordUpdatedAt
			if updatedAt == nil {
				updatedAt = &now
			}
		default:
			return nil, errors.New("route_rules basic_auth_username and basic_auth_password cannot be empty when basic auth is enabled")
		}
	}
	return &model.SecurityPolicy{
		ProxyRouteID:               routeID,
		Name:                       strings.TrimSpace(name),
		IsDefault:                  false,
		BasicAuthEnabled:           enabled,
		BasicAuthUsername:          username,
		BasicAuthPasswordHash:      passwordHash,
		BasicAuthPasswordUpdatedAt: updatedAt,
		RegionRestrictionMode:      proxyRouteRegionModeBlock,
		WAFMode:                    proxyRouteWAFModeBlock,
		CCMode:                     proxyRouteCCModeBlock,
		DDOSProtectionMode:         "off",
		DDOSProtectionProvider:     DNSProviderModeCloudflare,
	}, nil
}

func buildProxyRouteRuleViews(route *model.ProxyRoute, context *proxyRouteViewBuildContext) ([]ProxyRouteRuleView, error) {
	if route == nil || route.ID == 0 {
		return []ProxyRouteRuleView{}, nil
	}
	configs, err := context.loadProxyRouteRuleConfigs(route)
	if err != nil {
		return nil, err
	}
	views := make([]ProxyRouteRuleView, 0, len(configs))
	for _, config := range configs {
		rule := config.Rule
		view := ProxyRouteRuleView{
			ID:                 rule.ID,
			Name:               rule.Name,
			MatchType:          rule.MatchType,
			Path:               rule.Path,
			Priority:           rule.Priority,
			Enabled:            rule.Enabled,
			OriginGroupID:      rule.OriginGroupID,
			OriginURL:          firstString(config.Upstreams),
			Upstreams:          config.Upstreams,
			OriginHostHeader:   strings.TrimSpace(rule.OriginHostHeader),
			OriginSNI:          strings.TrimSpace(rule.OriginSNI),
			OriginTLSVerify:    rule.OriginTLSVerify,
			OriginCABundle:     strings.TrimSpace(rule.OriginCABundle),
			OriginResolveMode:  normalizeStoredOriginResolveMode(rule.OriginResolveMode),
			LimitConnPerServer: rule.LimitConnPerServer,
			LimitConnPerIP:     rule.LimitConnPerIP,
			LimitRate:          rule.LimitRate,
			ProxyBufferingMode: normalizeProxyRouteProxyBufferingMode(rule.ProxyBufferingMode),
			CustomHeaders:      redactSensitiveCustomHeaders(config.CustomHeaders),
			CachePolicyID:      rule.CachePolicyID,
			SecurityPolicyID:   rule.SecurityPolicyID,
			CreatedAt:          rule.CreatedAt,
			UpdatedAt:          rule.UpdatedAt,
		}
		if config.CachePolicy != nil {
			view.CacheEnabled = config.CachePolicy.Enabled
			view.CachePolicy = config.CachePolicy.Policy
			view.CacheRules = config.CacheRules
		}
		if config.SecurityPolicy != nil {
			view.BasicAuthEnabled = config.SecurityPolicy.BasicAuthEnabled
			view.BasicAuthUsername = config.SecurityPolicy.BasicAuthUsername
			view.BasicAuthPasswordConfigured = config.SecurityPolicy.BasicAuthEnabled && strings.TrimSpace(config.SecurityPolicy.BasicAuthPasswordHash) != ""
		}
		views = append(views, view)
	}
	return views, nil
}
