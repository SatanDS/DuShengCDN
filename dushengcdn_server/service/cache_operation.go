package service

import (
	"crypto/rand"
	"dushengcdn/common"
	"dushengcdn/model"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
)

type CacheOperationInput struct {
	Scope    string   `json:"scope"`
	URLs     []string `json:"urls"`
	Prefixes []string `json:"prefixes"`
	Suffixes []string `json:"suffixes"`
}

type AgentCacheOperation struct {
	OperationID      string   `json:"operation_id"`
	Action           string   `json:"action"`
	Scope            string   `json:"scope"`
	URLs             []string `json:"urls,omitempty"`
	Prefixes         []string `json:"prefixes,omitempty"`
	Suffixes         []string `json:"suffixes,omitempty"`
	CachePath        string   `json:"cache_path,omitempty"`
	CacheLevels      string   `json:"cache_levels,omitempty"`
	CacheKeyTemplate string   `json:"cache_key_template,omitempty"`
	AllowedHosts     []string `json:"allowed_hosts,omitempty"`
}

type CacheOperationResult struct {
	OperationID string   `json:"operation_id"`
	Action      string   `json:"action"`
	TargetNodes []string `json:"target_nodes"`
	FailedNodes []string `json:"failed_nodes"`
}

const maxCacheWarmURLs = 100

func RequestProxyRouteCachePurge(routeID uint, input CacheOperationInput) (*CacheOperationResult, error) {
	scope := normalizeCacheOperationScope(input.Scope)
	if err := validateCachePurgeScopeInput(scope, input); err != nil {
		return nil, err
	}
	cachePath := strings.TrimSpace(common.OpenRestyCachePath)
	if cachePath == "" {
		return nil, errors.New("OpenResty cache path is not configured")
	}
	if err := ValidateAgentCachePath(cachePath); err != nil {
		return nil, err
	}
	route, err := model.GetProxyRouteByID(routeID)
	if err != nil {
		return nil, err
	}
	urls := []string(nil)
	if scope == "url" {
		urls, err = normalizeCacheWarmURLsForRoute(route, input.URLs)
		if err != nil {
			return nil, fmt.Errorf("cache purge URL target is invalid: %w", err)
		}
	}
	operation := &AgentCacheOperation{
		OperationID:      newCacheOperationID(),
		Action:           "purge",
		Scope:            scope,
		URLs:             urls,
		CachePath:        cachePath,
		CacheLevels:      strings.TrimSpace(common.OpenRestyCacheLevels),
		CacheKeyTemplate: strings.TrimSpace(common.OpenRestyCacheKeyTemplate),
	}
	return dispatchRouteCacheOperation(route, operation)
}

func validateCachePurgeScopeInput(scope string, input CacheOperationInput) error {
	switch scope {
	case "all":
		return nil
	case "url":
		if len(input.URLs) == 0 {
			return errors.New("cache purge scope=url requires at least one URL")
		}
		return nil
	case "path_prefix":
		if len(trimNonEmptyStrings(input.Prefixes)) == 0 {
			return errors.New("cache purge scope=path_prefix requires at least one prefix")
		}
		return errors.New("cache purge scope=path_prefix is not supported yet because OpenResty stores cache entries under hashed keys; TODO: maintain a URL-to-cache-key index for safe partial purge")
	case "suffix":
		if len(trimNonEmptyStrings(input.Suffixes)) == 0 {
			return errors.New("cache purge scope=suffix requires at least one suffix")
		}
		return errors.New("cache purge scope=suffix is not supported yet because OpenResty stores cache entries under hashed keys; TODO: maintain a URL-to-cache-key index for safe partial purge")
	default:
		return fmt.Errorf("cache purge scope %q is not supported; supported scopes are all and url", scope)
	}
}

func ValidateAgentCachePath(path string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	cleanPath := filepath.Clean(strings.TrimSpace(path))
	if cleanPath == "" || cleanPath == "." {
		return errors.New("OpenResty cache path is unsafe")
	}
	if !isAgentCacheAbsPath(cleanPath) {
		return errors.New("OpenResty cache path must be absolute")
	}
	components := cachePathComponents(cleanPath)
	if len(components) < 3 {
		return errors.New("OpenResty cache path is too broad")
	}
	if isProtectedAgentCacheDirectory(components) {
		return errors.New("OpenResty cache path points to a protected directory")
	}
	if !looksLikeAgentCacheDirectory(components) {
		return fmt.Errorf("OpenResty cache path %q must clearly be a dushengcdn/openresty cache directory", path)
	}
	return nil
}

func isAgentCacheAbsPath(path string) bool {
	return filepath.IsAbs(path) || strings.HasPrefix(filepath.ToSlash(path), "/")
}

func cachePathComponents(path string) []string {
	volume := filepath.VolumeName(path)
	remainder := strings.TrimPrefix(path, volume)
	parts := strings.FieldsFunc(strings.Trim(remainder, `/\`), func(r rune) bool {
		return r == '/' || r == '\\'
	})
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		normalized := strings.ToLower(strings.TrimSpace(part))
		if normalized != "" {
			result = append(result, normalized)
		}
	}
	return result
}

func isProtectedAgentCacheDirectory(components []string) bool {
	if len(components) == 0 {
		return true
	}
	first := components[0]
	if len(components) <= 2 {
		switch first {
		case "bin", "boot", "dev", "etc", "home", "lib", "lib64", "opt", "proc", "root", "run", "sbin", "sys", "tmp", "usr", "var", "windows":
			return true
		}
	}
	return false
}

func looksLikeAgentCacheDirectory(components []string) bool {
	hasCache := false
	hasDuShengCDN := false
	hasRuntimeMarker := false
	for _, component := range components {
		compacted := strings.NewReplacer("-", "", "_", "", ".", "", " ", "").Replace(component)
		if strings.Contains(compacted, "cache") {
			hasCache = true
		}
		if strings.Contains(compacted, "dushengcdn") {
			hasDuShengCDN = true
		}
		if strings.Contains(compacted, "openresty") || strings.Contains(compacted, "nginx") {
			hasRuntimeMarker = true
		}
	}
	return hasDuShengCDN && (hasCache || hasRuntimeMarker)
}

func RequestProxyRouteCacheWarm(routeID uint, input CacheOperationInput) (*CacheOperationResult, error) {
	route, err := model.GetProxyRouteByID(routeID)
	if err != nil {
		return nil, err
	}
	urls, err := normalizeCacheWarmURLsForRoute(route, input.URLs)
	if err != nil {
		return nil, err
	}
	operation := &AgentCacheOperation{
		OperationID:  newCacheOperationID(),
		Action:       "warm",
		Scope:        "url",
		URLs:         urls,
		AllowedHosts: allowedCacheWarmHosts(route),
	}
	return dispatchRouteCacheOperation(route, operation)
}

func dispatchRouteCacheOperation(route *model.ProxyRoute, operation *AgentCacheOperation) (*CacheOperationResult, error) {
	if route == nil || operation == nil {
		return nil, errors.New("cache operation target is invalid")
	}
	nodes, err := model.ListNodes()
	if err != nil {
		return nil, err
	}
	poolName := normalizeNodePoolName(route.NodePool)
	result := &CacheOperationResult{
		OperationID: operation.OperationID,
		Action:      operation.Action,
	}
	for _, node := range nodes {
		if normalizeNodePoolName(node.PoolName) != poolName {
			continue
		}
		if !isNodeEligibleForCacheOperation(node) {
			continue
		}
		if SendAgentWSCacheOperation(node.NodeID, operation) {
			result.TargetNodes = append(result.TargetNodes, node.NodeID)
			continue
		}
		result.FailedNodes = append(result.FailedNodes, node.NodeID)
	}
	if len(result.TargetNodes) == 0 {
		return result, errors.New("no online node is available for this cache operation")
	}
	return result, nil
}

func normalizeCacheOperationScope(raw string) string {
	scope := strings.ToLower(strings.TrimSpace(raw))
	if scope == "" {
		return "all"
	}
	return scope
}

func trimNonEmptyStrings(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func normalizeCacheWarmURLsForRoute(route *model.ProxyRoute, values []string) ([]string, error) {
	if route == nil {
		return nil, errors.New("cache warm route is invalid")
	}
	allowedDomains, err := decodeStoredDomains(route.Domains, route.Domain)
	if err != nil {
		return nil, err
	}
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		raw := strings.TrimSpace(value)
		if raw == "" {
			continue
		}
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return nil, errors.New("cache warm URL format is invalid")
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" || parsed.User != nil {
			return nil, errors.New("cache warm URL must be http/https and must not contain user info")
		}
		if !cacheWarmPortAllowed(parsed) {
			return nil, errors.New("cache warm URL must use the default http/https port")
		}
		host := normalizeCacheWarmHost(parsed.Hostname())
		if host == "" {
			return nil, errors.New("cache warm URL host is invalid")
		}
		if !cacheWarmHostAllowed(host, allowedDomains) {
			return nil, fmt.Errorf("cache warm URL host %q is not part of the route domains", host)
		}
		parsed.Fragment = ""
		parsed.Host = strings.ToLower(parsed.Host)
		normalized := parsed.String()
		if _, ok := seen[normalized]; ok {
			continue
		}
		if len(result) >= maxCacheWarmURLs {
			return nil, fmt.Errorf("cache warm accepts at most %d URLs", maxCacheWarmURLs)
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}
	if len(result) == 0 {
		return nil, errors.New("cache warm requires at least one http/https URL")
	}
	return result, nil
}

func normalizeCacheWarmHost(host string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
}

func cacheWarmPortAllowed(parsed *url.URL) bool {
	if parsed == nil {
		return false
	}
	port := parsed.Port()
	return port == "" ||
		(parsed.Scheme == "http" && port == "80") ||
		(parsed.Scheme == "https" && port == "443")
}

func cacheWarmHostAllowed(host string, routeDomains []string) bool {
	host = normalizeCacheWarmHost(host)
	if host == "" {
		return false
	}
	for _, domain := range routeDomains {
		normalized := normalizeCacheWarmHost(domain)
		if normalized == "" {
			continue
		}
		if normalized == host {
			return true
		}
		if !strings.HasPrefix(normalized, "*.") {
			continue
		}
		suffix := strings.TrimPrefix(normalized, "*.")
		if suffix == "" || !strings.HasSuffix(host, "."+suffix) {
			continue
		}
		prefix := strings.TrimSuffix(host, "."+suffix)
		if prefix != "" && !strings.Contains(prefix, ".") {
			return true
		}
	}
	return false
}

func allowedCacheWarmHosts(route *model.ProxyRoute) []string {
	if route == nil {
		return nil
	}
	domains, err := decodeStoredDomains(route.Domains, route.Domain)
	if err != nil {
		return nil
	}
	result := make([]string, 0, len(domains))
	seen := make(map[string]struct{}, len(domains))
	for _, domain := range domains {
		normalized := normalizeCacheWarmHost(domain)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}
	return result
}

func newCacheOperationID() string {
	buffer := make([]byte, 8)
	if _, err := rand.Read(buffer); err != nil {
		return "cache-op"
	}
	return "cache-op-" + hex.EncodeToString(buffer)
}
