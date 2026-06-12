package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

func normalizeProxyRouteRegionRestrictionMode(raw string) string {
	mode := strings.ToLower(strings.TrimSpace(raw))
	switch mode {
	case proxyRouteRegionModeAllow, proxyRouteRegionModeBlock:
		return mode
	default:
		return proxyRouteRegionModeBlock
	}
}

func normalizeProxyRouteRegionRestriction(enabled bool, rawMode string, rawCountries []string) (string, []string, error) {
	mode := normalizeProxyRouteRegionRestrictionMode(rawMode)
	if !enabled {
		return mode, []string{}, nil
	}
	countries := make([]string, 0, len(rawCountries))
	seen := make(map[string]struct{}, len(rawCountries))
	for _, rawCountry := range rawCountries {
		country := strings.ToUpper(strings.TrimSpace(rawCountry))
		if country == "" {
			continue
		}
		if !proxyRouteRegionCountryPattern.MatchString(country) {
			return "", nil, fmt.Errorf("region country code %q is invalid", rawCountry)
		}
		if _, ok := seen[country]; ok {
			continue
		}
		seen[country] = struct{}{}
		countries = append(countries, country)
	}
	if len(countries) == 0 {
		return "", nil, errors.New("region restriction requires at least one country code")
	}
	return mode, countries, nil
}

func decodeStoredRegionRestrictionCountries(raw string) ([]string, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return []string{}, nil
	}
	var countries []string
	if err := json.Unmarshal([]byte(text), &countries); err != nil {
		return nil, errors.New("region_restriction_countries payload is invalid")
	}
	_, normalized, err := normalizeProxyRouteRegionRestriction(len(countries) > 0, proxyRouteRegionModeBlock, countries)
	if err != nil {
		return nil, err
	}
	return normalized, nil
}

func normalizeCachePolicy(enabled bool, raw string) string {
	if !enabled {
		return ""
	}
	policy := strings.TrimSpace(raw)
	if policy == "" {
		return proxyRouteCachePolicyURL
	}
	return policy
}

func normalizeCacheRules(enabled bool, rawPolicy string, rules []string) ([]string, error) {
	if !enabled {
		return []string{}, nil
	}
	policy := normalizeCachePolicy(enabled, rawPolicy)
	switch policy {
	case proxyRouteCachePolicyURL:
		return []string{}, nil
	case proxyRouteCachePolicySuffix:
		return normalizeCacheSuffixRules(rules)
	case proxyRouteCachePolicyPathPrefix:
		return normalizeCachePathRules(rules, true)
	case proxyRouteCachePolicyPathContains:
		return normalizeCachePathRules(rules, true)
	case proxyRouteCachePolicyPathContainsAll:
		return normalizeCachePathRules(rules, true)
	case proxyRouteCachePolicyPathExact:
		return normalizeCachePathRules(rules, false)
	default:
		return nil, errors.New("cache policy is not supported")
	}
}

func normalizeCacheSuffixRules(rules []string) ([]string, error) {
	normalized := make([]string, 0, len(rules))
	seen := make(map[string]struct{}, len(rules))
	for _, rule := range rules {
		item := strings.TrimSpace(strings.TrimPrefix(rule, "."))
		if item == "" {
			continue
		}
		if strings.ContainsAny(item, "/\\ \t\r\n") {
			return nil, errors.New("cache suffix format is invalid")
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		normalized = append(normalized, item)
	}
	if len(normalized) == 0 {
		return nil, errors.New("at least one suffix is required")
	}
	return normalized, nil
}

func normalizeCachePathRules(rules []string, allowPrefix bool) ([]string, error) {
	normalized := make([]string, 0, len(rules))
	seen := make(map[string]struct{}, len(rules))
	for _, rule := range rules {
		item := strings.TrimSpace(rule)
		if item == "" {
			continue
		}
		if !strings.HasPrefix(item, "/") || strings.Contains(item, "://") || strings.ContainsAny(item, " \t\r\n") {
			return nil, errors.New("cache path rule format is invalid")
		}
		if !allowPrefix && strings.HasSuffix(item, "/") && len(item) > 1 {
			item = strings.TrimRight(item, "/")
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		normalized = append(normalized, item)
	}
	if len(normalized) == 0 {
		if allowPrefix {
			return nil, errors.New("at least one path prefix is required")
		}
		return nil, errors.New("at least one exact path is required")
	}
	return normalized, nil
}

func decodeStoredCacheRules(raw string) ([]string, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return []string{}, nil
	}
	var rules []string
	if err := json.Unmarshal([]byte(text), &rules); err != nil {
		return nil, errors.New("cache_rules payload is invalid")
	}
	normalized := make([]string, 0, len(rules))
	for _, rule := range rules {
		item := strings.TrimSpace(rule)
		if item == "" {
			continue
		}
		normalized = append(normalized, item)
	}
	return normalized, nil
}
