package service

import (
	"crypto/rand"
	"dushengcdn/common"
	"dushengcdn/model"
	"encoding/hex"
	"errors"
	"net/url"
	"strings"
)

type CacheOperationInput struct {
	Scope    string   `json:"scope"`
	URLs     []string `json:"urls"`
	Prefixes []string `json:"prefixes"`
}

type AgentCacheOperation struct {
	OperationID string   `json:"operation_id"`
	Action      string   `json:"action"`
	Scope       string   `json:"scope"`
	URLs        []string `json:"urls,omitempty"`
	Prefixes    []string `json:"prefixes,omitempty"`
	CachePath   string   `json:"cache_path,omitempty"`
}

type CacheOperationResult struct {
	OperationID string   `json:"operation_id"`
	Action      string   `json:"action"`
	TargetNodes []string `json:"target_nodes"`
	FailedNodes []string `json:"failed_nodes"`
}

func RequestProxyRouteCachePurge(routeID uint, input CacheOperationInput) (*CacheOperationResult, error) {
	route, err := model.GetProxyRouteByID(routeID)
	if err != nil {
		return nil, err
	}
	scope := normalizeCacheOperationScope(input.Scope)
	if scope != "all" {
		return nil, errors.New("cache purge currently supports scope=all only")
	}
	cachePath := strings.TrimSpace(common.OpenRestyCachePath)
	if cachePath == "" {
		return nil, errors.New("OpenResty cache path is not configured")
	}
	operation := &AgentCacheOperation{
		OperationID: newCacheOperationID(),
		Action:      "purge",
		Scope:       scope,
		CachePath:   cachePath,
	}
	return dispatchRouteCacheOperation(route, operation)
}

func RequestProxyRouteCacheWarm(routeID uint, input CacheOperationInput) (*CacheOperationResult, error) {
	route, err := model.GetProxyRouteByID(routeID)
	if err != nil {
		return nil, err
	}
	urls := normalizeCacheWarmURLs(input.URLs)
	if len(urls) == 0 {
		return nil, errors.New("cache warm requires at least one http/https URL")
	}
	operation := &AgentCacheOperation{
		OperationID: newCacheOperationID(),
		Action:      "warm",
		Scope:       "url",
		URLs:        urls,
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

func normalizeCacheWarmURLs(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		raw := strings.TrimSpace(value)
		if raw == "" {
			continue
		}
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			continue
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			continue
		}
		normalized := parsed.String()
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
