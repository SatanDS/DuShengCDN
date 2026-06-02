package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"

	"dushengcdn/model"
	"dushengcdn/utils/geoip/iputil"
)

const (
	gslbModeCloudflareDNS      = "cloudflare_dns"
	gslbSourceProviderNone     = "none"
	gslbSourceProviderHTTP     = "http"
	defaultGSLBCooldownSeconds = 60
)

type ProxyRouteGSLBPoolPolicy struct {
	Name        string   `json:"name"`
	Weight      int      `json:"weight"`
	Countries   []string `json:"countries"`
	SourceCIDRs []string `json:"source_cidrs"`
	Enabled     bool     `json:"enabled"`
}

type ProxyRouteGSLBSourceIPProvider struct {
	Provider string `json:"provider"`
	APIURL   string `json:"api_url"`
	APIToken string `json:"api_token"`
}

type ProxyRouteGSLBLoadThresholds struct {
	MaxOpenrestyConnections int64   `json:"max_openresty_connections"`
	MaxCPUPercent           float64 `json:"max_cpu_percent"`
	MaxMemoryPercent        float64 `json:"max_memory_percent"`
}

type ProxyRouteGSLBDebounce struct {
	CooldownSeconds    int `json:"cooldown_seconds"`
	UnhealthyThreshold int `json:"unhealthy_threshold"`
	RecoveryThreshold  int `json:"recovery_threshold"`
}

type ProxyRouteGSLBPolicy struct {
	Mode           string                         `json:"mode"`
	Strategy       string                         `json:"strategy"`
	Pools          []ProxyRouteGSLBPoolPolicy     `json:"pools"`
	TargetCount    int                            `json:"target_count"`
	TTL            int                            `json:"ttl"`
	SourceIP       ProxyRouteGSLBSourceIPProvider `json:"source_ip"`
	LoadThresholds ProxyRouteGSLBLoadThresholds   `json:"load_thresholds"`
	Debounce       ProxyRouteGSLBDebounce         `json:"debounce"`
}

func defaultGSLBPolicy(nodePool string, targetCount int, scheduleMode string, ttl int) ProxyRouteGSLBPolicy {
	return ProxyRouteGSLBPolicy{
		Mode:        gslbModeCloudflareDNS,
		Strategy:    normalizeDNSScheduleMode(scheduleMode),
		TargetCount: normalizeDNSTargetCount(targetCount),
		TTL:         normalizeDNSTTL(ttl),
		Pools: []ProxyRouteGSLBPoolPolicy{
			{
				Name:        normalizeNodePoolName(nodePool),
				Weight:      100,
				Countries:   []string{},
				SourceCIDRs: []string{},
				Enabled:     true,
			},
		},
		SourceIP: ProxyRouteGSLBSourceIPProvider{
			Provider: gslbSourceProviderNone,
		},
		Debounce: ProxyRouteGSLBDebounce{
			CooldownSeconds:    defaultGSLBCooldownSeconds,
			UnhealthyThreshold: 1,
			RecoveryThreshold:  1,
		},
	}
}

func normalizeGSLBPolicy(input ProxyRouteGSLBPolicy, nodePool string, targetCount int, scheduleMode string, ttl int) (ProxyRouteGSLBPolicy, error) {
	defaultPolicy := defaultGSLBPolicy(nodePool, targetCount, scheduleMode, ttl)
	if len(input.Pools) == 0 &&
		strings.TrimSpace(input.Mode) == "" &&
		strings.TrimSpace(input.Strategy) == "" &&
		input.TargetCount == 0 &&
		input.TTL == 0 {
		return defaultPolicy, nil
	}

	policy := defaultPolicy
	if strings.TrimSpace(input.Mode) != "" {
		mode := strings.ToLower(strings.TrimSpace(input.Mode))
		switch mode {
		case gslbModeCloudflareDNS:
			policy.Mode = mode
		default:
			return policy, errors.New("gslb_policy.mode currently only supports cloudflare_dns")
		}
	}
	if strings.TrimSpace(input.Strategy) != "" {
		policy.Strategy = normalizeDNSScheduleMode(input.Strategy)
	}
	if input.TargetCount > 0 {
		policy.TargetCount = normalizeDNSTargetCount(input.TargetCount)
	}
	if input.TTL > 0 {
		policy.TTL = normalizeDNSTTL(input.TTL)
	}

	if strings.TrimSpace(input.SourceIP.Provider) != "" {
		provider := strings.ToLower(strings.TrimSpace(input.SourceIP.Provider))
		switch provider {
		case gslbSourceProviderNone, gslbSourceProviderHTTP:
			policy.SourceIP.Provider = provider
		default:
			return policy, errors.New("gslb_policy.source_ip.provider is not supported")
		}
		policy.SourceIP.APIURL = strings.TrimSpace(input.SourceIP.APIURL)
		policy.SourceIP.APIToken = strings.TrimSpace(input.SourceIP.APIToken)
	}

	policy.LoadThresholds = normalizeGSLBLoadThresholds(input.LoadThresholds)
	policy.Debounce = normalizeGSLBDebounce(input.Debounce)

	if len(input.Pools) > 0 {
		pools, err := normalizeGSLBPools(input.Pools)
		if err != nil {
			return policy, err
		}
		policy.Pools = pools
	}
	return policy, nil
}

func normalizeGSLBPools(input []ProxyRouteGSLBPoolPolicy) ([]ProxyRouteGSLBPoolPolicy, error) {
	result := make([]ProxyRouteGSLBPoolPolicy, 0, len(input))
	seen := make(map[string]int, len(input))
	hasExplicitEnabledPool := false
	for _, pool := range input {
		if pool.Enabled {
			hasExplicitEnabledPool = true
			break
		}
	}
	for _, pool := range input {
		if hasExplicitEnabledPool && !pool.Enabled {
			continue
		}
		name := normalizeNodePoolName(pool.Name)
		if name == "" {
			continue
		}
		weight := pool.Weight
		if weight <= 0 {
			weight = 100
		}
		if weight > 1000 {
			weight = 1000
		}
		countries := normalizeGSLBCountryList(pool.Countries)
		sourceCIDRs, err := normalizeGSLBCIDRList(pool.SourceCIDRs)
		if err != nil {
			return nil, err
		}
		if existingIndex, ok := seen[name]; ok {
			result[existingIndex].Weight = weight
			result[existingIndex].Countries = mergeGSLBStringLists(result[existingIndex].Countries, countries)
			result[existingIndex].SourceCIDRs = mergeGSLBStringLists(result[existingIndex].SourceCIDRs, sourceCIDRs)
			continue
		}
		seen[name] = len(result)
		result = append(result, ProxyRouteGSLBPoolPolicy{
			Name:        name,
			Weight:      weight,
			Countries:   countries,
			SourceCIDRs: sourceCIDRs,
			Enabled:     true,
		})
	}
	if len(result) == 0 {
		return nil, errors.New("gslb_policy.pools requires at least one enabled node pool")
	}
	return result, nil
}

func validateGSLBPolicyPoolTargets(policy ProxyRouteGSLBPolicy, recordType string) error {
	recordType = normalizeDNSRecordType(recordType)
	if recordType != "A" && recordType != "AAAA" {
		return errors.New("GSLB scheduling only supports A/AAAA records")
	}
	if len(policy.Pools) == 0 {
		return errors.New("gslb_policy.pools requires at least one enabled node pool")
	}
	nodes, err := model.ListNodes()
	if err != nil {
		return err
	}
	availablePools := make(map[string]int, len(policy.Pools))
	for _, node := range nodes {
		if !isNodeSchedulableForDNS(node) || !isNodeOnlineAndOpenRestyHealthy(node) {
			continue
		}
		poolName := normalizeNodePoolName(node.PoolName)
		for _, value := range resolveNodePublicIPs(node) {
			ip := iputil.NormalizeIP(value)
			parsed := net.ParseIP(ip)
			if parsed == nil || !iputil.IsPublicString(ip) {
				continue
			}
			if recordType == "A" && parsed.To4() == nil {
				continue
			}
			if recordType == "AAAA" && parsed.To4() != nil {
				continue
			}
			availablePools[poolName]++
		}
	}
	missingPools := make([]string, 0)
	seen := map[string]struct{}{}
	for _, pool := range policy.Pools {
		if !pool.Enabled {
			continue
		}
		name := normalizeNodePoolName(pool.Name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		if availablePools[name] == 0 {
			missingPools = append(missingPools, name)
		}
	}
	if len(missingPools) > 0 {
		return fmt.Errorf("多节点智能解析节点池没有可用于 %s 记录的在线公网 IP：%s。请确认填写的是节点池名称，不是节点名称", recordType, strings.Join(missingPools, "、"))
	}
	return nil
}

func normalizeGSLBCountryList(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		country := strings.ToUpper(strings.TrimSpace(value))
		if country == "" || !proxyRouteRegionCountryPattern.MatchString(country) {
			continue
		}
		if _, ok := seen[country]; ok {
			continue
		}
		seen[country] = struct{}{}
		result = append(result, country)
	}
	return result
}

func normalizeGSLBCIDRList(values []string) ([]string, error) {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		cidr, ok := normalizeGSLBCIDR(value)
		if !ok {
			if strings.TrimSpace(value) == "" {
				continue
			}
			return nil, errors.New("gslb_policy.pools.source_cidrs contains invalid CIDR")
		}
		if _, exists := seen[cidr]; exists {
			continue
		}
		seen[cidr] = struct{}{}
		result = append(result, cidr)
	}
	return result, nil
}

func normalizeGSLBCIDR(value string) (string, bool) {
	text := strings.TrimSpace(value)
	if text == "" {
		return "", false
	}
	if strings.Contains(text, "/") {
		ip, network, err := net.ParseCIDR(text)
		if err != nil {
			return "", false
		}
		if ipv4 := ip.To4(); ipv4 != nil {
			ip = ipv4
		}
		network.IP = ip.Mask(network.Mask)
		return network.String(), true
	}
	ip := net.ParseIP(text)
	if ip == nil {
		return "", false
	}
	if ipv4 := ip.To4(); ipv4 != nil {
		network := net.IPNet{IP: ipv4, Mask: net.CIDRMask(32, 32)}
		return network.String(), true
	}
	network := net.IPNet{IP: ip, Mask: net.CIDRMask(128, 128)}
	return network.String(), true
}

func sourceIPMatchesCIDRList(sourceIP string, cidrs []string) (string, bool) {
	ip := net.ParseIP(strings.TrimSpace(sourceIP))
	if ip == nil {
		return "", false
	}
	if ipv4 := ip.To4(); ipv4 != nil {
		ip = ipv4
	}
	for _, value := range cidrs {
		cidr, ok := normalizeGSLBCIDR(value)
		if !ok {
			continue
		}
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		if network.Contains(ip) {
			return cidr, true
		}
	}
	return "", false
}

func mergeGSLBStringLists(left []string, right []string) []string {
	result := append([]string(nil), left...)
	seen := make(map[string]struct{}, len(left)+len(right))
	for _, value := range result {
		seen[value] = struct{}{}
	}
	for _, value := range right {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func normalizeGSLBLoadThresholds(input ProxyRouteGSLBLoadThresholds) ProxyRouteGSLBLoadThresholds {
	thresholds := input
	if thresholds.MaxOpenrestyConnections < 0 {
		thresholds.MaxOpenrestyConnections = 0
	}
	if thresholds.MaxCPUPercent < 0 {
		thresholds.MaxCPUPercent = 0
	}
	if thresholds.MaxCPUPercent > 100 {
		thresholds.MaxCPUPercent = 100
	}
	if thresholds.MaxMemoryPercent < 0 {
		thresholds.MaxMemoryPercent = 0
	}
	if thresholds.MaxMemoryPercent > 100 {
		thresholds.MaxMemoryPercent = 100
	}
	return thresholds
}

func normalizeGSLBDebounce(input ProxyRouteGSLBDebounce) ProxyRouteGSLBDebounce {
	debounce := input
	if debounce.CooldownSeconds <= 0 {
		debounce.CooldownSeconds = defaultGSLBCooldownSeconds
	}
	if debounce.CooldownSeconds > 3600 {
		debounce.CooldownSeconds = 3600
	}
	if debounce.UnhealthyThreshold <= 0 {
		debounce.UnhealthyThreshold = 1
	}
	if debounce.RecoveryThreshold <= 0 {
		debounce.RecoveryThreshold = 1
	}
	return debounce
}

func decodeStoredGSLBPolicy(raw string) (ProxyRouteGSLBPolicy, error) {
	text := strings.TrimSpace(raw)
	if text == "" || text == "{}" {
		return defaultGSLBPolicy("default", 1, "healthy", cloudflareDefaultRecordTTL), nil
	}
	var policy ProxyRouteGSLBPolicy
	if err := json.Unmarshal([]byte(text), &policy); err != nil {
		return policy, errors.New("gslb_policy payload is invalid")
	}
	return normalizeGSLBPolicy(policy, "default", policy.TargetCount, policy.Strategy, policy.TTL)
}
