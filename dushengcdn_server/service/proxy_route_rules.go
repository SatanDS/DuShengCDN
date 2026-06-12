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
	existingRules, err := loadProxyRouteRulesByMatch(db, route.ID)
	if err != nil {
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
	keptRuleIDs := make(map[uint]struct{}, len(inputs))
	keptOriginGroupIDs := map[uint]struct{}{defaultOriginGroup.ID: {}}
	keptCachePolicyIDs := make(map[uint]struct{}, len(inputs))
	keptSecurityPolicyIDs := make(map[uint]struct{}, len(inputs))
	for index, input := range inputs {
		rule, err := normalizeProxyRouteRuleInput(route, site.ID, defaultOriginGroup.ID, input, index)
		if err != nil {
			return err
		}
		if rule.MatchType == proxyRouteRuleMatchDefault {
			// The default rule mirrors the route-level settings and is managed
			// by SyncProxyRouteNormalizedTablesWithDB; storing another row here
			// would render a duplicate root location and accumulate across
			// saves.
			continue
		}
		matchKey := proxyRouteRuleMatchKey(rule.MatchType, rule.Path)
		if _, ok := seenRuleMatches[matchKey]; ok {
			return fmt.Errorf("route_rules contains duplicated match %s %s", rule.MatchType, rule.Path)
		}
		seenRuleMatches[matchKey] = struct{}{}
		existingRule := existingRules[matchKey]
		if strings.TrimSpace(input.OriginURL) != "" || len(input.Upstreams) > 0 {
			group, err := upsertProxyRouteRuleOriginGroup(db, route, rule.Name, index, existingRule)
			if err != nil {
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
			keptOriginGroupIDs[group.ID] = struct{}{}
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
			cachePolicy, err := upsertProxyRouteRuleCachePolicy(db, existingRule, &model.CachePolicy{
				ProxyRouteID: route.ID,
				Name:         rule.Name,
				IsDefault:    false,
				Enabled:      *input.CacheEnabled,
				Policy:       normalizeCachePolicy(*input.CacheEnabled, input.CachePolicy),
				RulesJSON:    string(cacheRulesJSON),
			}, fmt.Sprintf("rule-%d-cache", index+1))
			if err != nil {
				return err
			}
			rule.CachePolicyID = &cachePolicy.ID
			keptCachePolicyIDs[cachePolicy.ID] = struct{}{}
		}
		if input.BasicAuthEnabled != nil {
			securityPolicy, err := buildProxyRouteRuleSecurityPolicy(route.ID, rule.Name, input, previousRuleSecurityPolicies[matchKey])
			if err != nil {
				return err
			}
			securityPolicy, err = upsertProxyRouteRuleSecurityPolicy(db, existingRule, securityPolicy, fmt.Sprintf("rule-%d-security", index+1))
			if err != nil {
				return err
			}
			rule.SecurityPolicyID = &securityPolicy.ID
			keptSecurityPolicyIDs[securityPolicy.ID] = struct{}{}
		}
		savedRule, err := upsertProxyRouteRuleWithDB(db, existingRule, rule)
		if err != nil {
			return err
		}
		keptRuleIDs[savedRule.ID] = struct{}{}
	}
	if err := deleteStaleProxyRouteRules(db, route.ID, keptRuleIDs); err != nil {
		return err
	}
	if err := deleteStaleProxyRouteRuleOriginGroups(db, route.ID, keptOriginGroupIDs); err != nil {
		return err
	}
	if err := deleteStaleProxyRouteRuleCachePolicies(db, route.ID, keptCachePolicyIDs); err != nil {
		return err
	}
	if err := deleteStaleProxyRouteRuleSecurityPolicies(db, route.ID, keptSecurityPolicyIDs); err != nil {
		return err
	}
	return nil
}

func loadProxyRouteRulesByMatch(db *gorm.DB, routeID uint) (map[string]*model.ProxyRouteRule, error) {
	var rules []model.ProxyRouteRule
	if err := db.Where("proxy_route_id = ? AND match_type <> ?", routeID, proxyRouteRuleMatchDefault).Find(&rules).Error; err != nil {
		return nil, err
	}
	result := make(map[string]*model.ProxyRouteRule, len(rules))
	for index := range rules {
		result[proxyRouteRuleMatchKey(rules[index].MatchType, rules[index].Path)] = &rules[index]
	}
	return result, nil
}

func proxyRouteRuleMatchKey(matchType string, path string) string {
	return strings.TrimSpace(matchType) + " " + strings.TrimSpace(path)
}

func upsertProxyRouteRuleOriginGroup(db *gorm.DB, route *model.ProxyRoute, name string, index int, existingRule *model.ProxyRouteRule) (*model.OriginGroup, error) {
	group := &model.OriginGroup{
		ProxyRouteID: route.ID,
		OriginID:     route.OriginID,
		Name:         strings.TrimSpace(name),
		IsDefault:    false,
	}
	if group.Name == "" {
		group.Name = fmt.Sprintf("rule-%d", index+1)
	}
	if existingRule != nil && existingRule.OriginGroupID != 0 {
		var current model.OriginGroup
		err := db.Where("id = ? AND proxy_route_id = ? AND is_default = ?", existingRule.OriginGroupID, route.ID, false).First(&current).Error
		if err == nil {
			current.OriginID = group.OriginID
			current.Name = group.Name
			current.IsDefault = false
			if err := db.Save(&current).Error; err != nil {
				return nil, err
			}
			return &current, nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}
	if err := db.Create(group).Error; err != nil {
		return nil, err
	}
	return group, nil
}

func upsertProxyRouteRuleCachePolicy(db *gorm.DB, existingRule *model.ProxyRouteRule, next *model.CachePolicy, fallbackName string) (*model.CachePolicy, error) {
	if strings.TrimSpace(next.Name) == "" {
		next.Name = fallbackName
	}
	if existingRule != nil && existingRule.CachePolicyID != nil && *existingRule.CachePolicyID != 0 {
		var current model.CachePolicy
		err := db.Where("id = ? AND proxy_route_id = ? AND is_default = ?", *existingRule.CachePolicyID, next.ProxyRouteID, false).First(&current).Error
		if err == nil {
			current.Name = next.Name
			current.IsDefault = false
			current.Enabled = next.Enabled
			current.DefaultTTL = next.DefaultTTL
			current.StatusTTLsJSON = next.StatusTTLsJSON
			current.CacheKey = next.CacheKey
			current.BypassCookiesJSON = next.BypassCookiesJSON
			current.BypassHeadersJSON = next.BypassHeadersJSON
			current.IncludeQuery = next.IncludeQuery
			current.IgnoreQueryParams = next.IgnoreQueryParams
			current.CacheMethodsJSON = next.CacheMethodsJSON
			current.Policy = next.Policy
			current.RulesJSON = next.RulesJSON
			if err := db.Save(&current).Error; err != nil {
				return nil, err
			}
			return &current, nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}
	if err := db.Create(next).Error; err != nil {
		return nil, err
	}
	return next, nil
}

func upsertProxyRouteRuleSecurityPolicy(db *gorm.DB, existingRule *model.ProxyRouteRule, next *model.SecurityPolicy, fallbackName string) (*model.SecurityPolicy, error) {
	if strings.TrimSpace(next.Name) == "" {
		next.Name = fallbackName
	}
	if existingRule != nil && existingRule.SecurityPolicyID != nil && *existingRule.SecurityPolicyID != 0 {
		var current model.SecurityPolicy
		err := db.Where("id = ? AND proxy_route_id = ? AND is_default = ?", *existingRule.SecurityPolicyID, next.ProxyRouteID, false).First(&current).Error
		if err == nil {
			current.Name = next.Name
			current.IsDefault = false
			current.BasicAuthEnabled = next.BasicAuthEnabled
			current.BasicAuthUsername = next.BasicAuthUsername
			current.BasicAuthPasswordHash = next.BasicAuthPasswordHash
			current.BasicAuthPasswordUpdatedAt = next.BasicAuthPasswordUpdatedAt
			current.RegionRestrictionMode = next.RegionRestrictionMode
			current.WAFMode = next.WAFMode
			current.CCMode = next.CCMode
			current.DDOSProtectionMode = next.DDOSProtectionMode
			current.DDOSProtectionProvider = next.DDOSProtectionProvider
			current.DDOSProtectionTarget = next.DDOSProtectionTarget
			if err := db.Save(&current).Error; err != nil {
				return nil, err
			}
			return &current, nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}
	if err := db.Create(next).Error; err != nil {
		return nil, err
	}
	return next, nil
}

func upsertProxyRouteRuleWithDB(db *gorm.DB, existingRule *model.ProxyRouteRule, next *model.ProxyRouteRule) (*model.ProxyRouteRule, error) {
	if existingRule == nil || existingRule.ID == 0 {
		if err := db.Create(next).Error; err != nil {
			return nil, err
		}
		return next, nil
	}
	current := *existingRule
	current.ProxySiteID = next.ProxySiteID
	current.OriginGroupID = next.OriginGroupID
	current.CachePolicyID = next.CachePolicyID
	current.SecurityPolicyID = next.SecurityPolicyID
	current.Name = next.Name
	current.MatchType = next.MatchType
	current.Path = next.Path
	current.Priority = next.Priority
	current.Enabled = next.Enabled
	current.OriginHostHeader = next.OriginHostHeader
	current.OriginSNI = next.OriginSNI
	current.OriginTLSVerify = next.OriginTLSVerify
	current.OriginCABundle = next.OriginCABundle
	current.OriginResolveMode = next.OriginResolveMode
	current.LimitConnPerServer = next.LimitConnPerServer
	current.LimitConnPerIP = next.LimitConnPerIP
	current.LimitRate = next.LimitRate
	current.ProxyBufferingMode = next.ProxyBufferingMode
	current.CustomHeadersJSON = next.CustomHeadersJSON
	if err := db.Save(&current).Error; err != nil {
		return nil, err
	}
	return &current, nil
}

func deleteStaleProxyRouteRules(db *gorm.DB, routeID uint, keptRuleIDs map[uint]struct{}) error {
	query := db.Where("proxy_route_id = ? AND match_type <> ?", routeID, proxyRouteRuleMatchDefault)
	if ids := mapKeysUint(keptRuleIDs); len(ids) > 0 {
		query = query.Where("id NOT IN ?", ids)
	}
	return query.Delete(&model.ProxyRouteRule{}).Error
}

func deleteStaleProxyRouteRuleOriginGroups(db *gorm.DB, routeID uint, keptGroupIDs map[uint]struct{}) error {
	var staleGroups []model.OriginGroup
	query := db.Where("proxy_route_id = ? AND is_default = ?", routeID, false)
	if ids := mapKeysUint(keptGroupIDs); len(ids) > 0 {
		query = query.Where("id NOT IN ?", ids)
	}
	if err := query.Find(&staleGroups).Error; err != nil {
		return err
	}
	if len(staleGroups) == 0 {
		return nil
	}
	groupIDs := make([]uint, 0, len(staleGroups))
	for _, group := range staleGroups {
		groupIDs = append(groupIDs, group.ID)
	}
	if err := db.Where("origin_group_id IN ?", groupIDs).Delete(&model.OriginServer{}).Error; err != nil {
		return err
	}
	return db.Where("id IN ?", groupIDs).Delete(&model.OriginGroup{}).Error
}

func deleteStaleProxyRouteRuleCachePolicies(db *gorm.DB, routeID uint, keptPolicyIDs map[uint]struct{}) error {
	query := db.Where("proxy_route_id = ? AND is_default = ?", routeID, false)
	if ids := mapKeysUint(keptPolicyIDs); len(ids) > 0 {
		query = query.Where("id NOT IN ?", ids)
	}
	return query.Delete(&model.CachePolicy{}).Error
}

func deleteStaleProxyRouteRuleSecurityPolicies(db *gorm.DB, routeID uint, keptPolicyIDs map[uint]struct{}) error {
	query := db.Where("proxy_route_id = ? AND is_default = ?", routeID, false)
	if ids := mapKeysUint(keptPolicyIDs); len(ids) > 0 {
		query = query.Where("id NOT IN ?", ids)
	}
	return query.Delete(&model.SecurityPolicy{}).Error
}

func mapKeysUint(values map[uint]struct{}) []uint {
	if len(values) == 0 {
		return nil
	}
	result := make([]uint, 0, len(values))
	for value := range values {
		if value != 0 {
			result = append(result, value)
		}
	}
	return result
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
