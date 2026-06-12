package dnsworker

import (
	"net"
	"strings"
)

func normalizeDomain(raw string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(raw)), ".")
}

func dnsName(raw string) string {
	name := normalizeDomain(raw)
	if name == "" {
		return "."
	}
	return name + "."
}

func normalizeRecordName(zoneName string, raw string) string {
	name := normalizeDomain(raw)
	zoneName = normalizeDomain(zoneName)
	if name == "" || name == "@" {
		return zoneName
	}
	if !strings.Contains(name, ".") && zoneName != "" {
		return name + "." + zoneName
	}
	return name
}

func normalizeDomainList(values []string) []string {
	result := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		item := normalizeDomain(value)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	return result
}

func normalizeRecordType(raw string) string {
	recordType := strings.ToUpper(strings.TrimSpace(raw))
	switch recordType {
	case "A", "AAAA", "CNAME", "TXT", "MX", "NS", "SOA", "CAA", "SRV", "HTTPS", "SVCB", "TLSA", "DNSKEY", "RRSIG", "NSEC", "NSEC3", "NSEC3PARAM":
		return recordType
	default:
		return "A"
	}
}

func normalizeAddressRecordType(raw string) string {
	recordType := strings.ToUpper(strings.TrimSpace(raw))
	if recordType == "AAAA" {
		return "AAAA"
	}
	return "A"
}

func normalizeNodePoolName(raw string) string {
	name := strings.ToLower(strings.TrimSpace(raw))
	if name == "" {
		return "default"
	}
	if len(name) > 64 {
		return name[:64]
	}
	return name
}

func normalizeTargetCount(value int) int {
	if value <= 0 {
		return 1
	}
	if value > 20 {
		return 20
	}
	return value
}

func normalizeStrategy(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "weighted":
		return "weighted"
	case "load_aware":
		return "load_aware"
	default:
		return "healthy"
	}
}

func normalizeStaticTTL(value int, fallback int) int {
	if fallback <= 0 {
		fallback = DefaultZoneTTL
	}
	if value <= 0 {
		return fallback
	}
	if value > 86400 {
		return 86400
	}
	return value
}

func normalizeAuthoritativeTTL(value int) int {
	if value <= 1 {
		return DefaultAuthoritativeTTL
	}
	if value < DefaultAuthoritativeTTL {
		return DefaultAuthoritativeTTL
	}
	if value > 86400 {
		return 86400
	}
	return value
}

func normalizeWeight(weight int) int {
	if weight <= 0 {
		return 100
	}
	if weight > 1000 {
		return 1000
	}
	return weight
}

func normalizePolicy(input GSLBPolicy, route SnapshotRoute) GSLBPolicy {
	policy := input
	policy.Pools = append([]GSLBPoolPolicy(nil), input.Pools...)
	policy.Strategy = normalizeStrategy(firstNonEmpty(policy.Strategy, route.ScheduleMode))
	policy.PoolMatchMode = normalizePoolMatchMode(policy.PoolMatchMode)
	policy.TargetCount = normalizeTargetCount(firstPositive(policy.TargetCount, route.TargetCount))
	policy.TTL = normalizeAuthoritativeTTL(firstPositive(policy.TTL, route.TTL))
	policy.SourcePoolFallbackMode = normalizeSourcePoolFallbackMode(policy.SourcePoolFallbackMode)
	policy.LoadThresholds = normalizeLoadThresholds(policy.LoadThresholds)
	policy.Debounce = normalizeDebounce(policy.Debounce)
	if len(policy.Pools) == 0 {
		policy.Pools = []GSLBPoolPolicy{
			{
				Name:    normalizeNodePoolName(route.NodePool),
				Weight:  100,
				Enabled: true,
			},
		}
	}
	hasExplicitEnabledPool := false
	for _, pool := range policy.Pools {
		if pool.Enabled {
			hasExplicitEnabledPool = true
			break
		}
	}
	for i := range policy.Pools {
		policy.Pools[i].Name = normalizeNodePoolName(policy.Pools[i].Name)
		policy.Pools[i].Weight = normalizeWeight(policy.Pools[i].Weight)
		if len(policy.Pools[i].Countries) > 0 {
			policy.Pools[i].Countries = normalizeCountryList(policy.Pools[i].Countries)
		}
		if len(policy.Pools[i].SourceCIDRs) > 0 {
			policy.Pools[i].SourceCIDRs = normalizeCIDRList(policy.Pools[i].SourceCIDRs)
		}
		policy.Pools[i].compiledSourceCIDRs = compileSchedulerCIDRList(policy.Pools[i].SourceCIDRs)
		if len(policy.Pools[i].Operators) > 0 {
			policy.Pools[i].Operators = normalizeOperatorList(policy.Pools[i].Operators)
		}
		if len(policy.Pools[i].ASNs) > 0 {
			policy.Pools[i].ASNs = normalizeASNList(policy.Pools[i].ASNs)
		}
		if len(policy.Pools[i].NodeIDs) > 0 {
			policy.Pools[i].NodeIDs = normalizeNodeIDList(policy.Pools[i].NodeIDs)
		}
		if len(policy.Pools[i].ExcludeCountries) > 0 {
			policy.Pools[i].ExcludeCountries = normalizeCountryList(policy.Pools[i].ExcludeCountries)
		}
		if len(policy.Pools[i].ExcludeSourceCIDRs) > 0 {
			policy.Pools[i].ExcludeSourceCIDRs = normalizeCIDRList(policy.Pools[i].ExcludeSourceCIDRs)
		}
		policy.Pools[i].compiledExcludeSourceCIDRs = compileSchedulerCIDRList(policy.Pools[i].ExcludeSourceCIDRs)
		if len(policy.Pools[i].ExcludeOperators) > 0 {
			policy.Pools[i].ExcludeOperators = normalizeOperatorList(policy.Pools[i].ExcludeOperators)
		}
		if len(policy.Pools[i].ExcludeASNs) > 0 {
			policy.Pools[i].ExcludeASNs = normalizeASNList(policy.Pools[i].ExcludeASNs)
		}
		if !hasExplicitEnabledPool {
			policy.Pools[i].Enabled = true
		}
	}
	return policy
}

func normalizePoolMatchMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "mixed_weighted":
		return "mixed_weighted"
	default:
		return "priority"
	}
}

func normalizeSourcePoolFallbackMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "fallback_to_global":
		return "fallback_to_global"
	default:
		return "strict"
	}
}

func normalizeNodeIDList(values []string) []string {
	result := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		nodeID := strings.TrimSpace(value)
		if nodeID == "" {
			continue
		}
		if _, ok := seen[nodeID]; ok {
			continue
		}
		seen[nodeID] = struct{}{}
		result = append(result, nodeID)
	}
	return result
}

func normalizeLoadThresholds(input GSLBLoadThresholds) GSLBLoadThresholds {
	if input.MaxOpenrestyConnections < 0 {
		input.MaxOpenrestyConnections = 0
	}
	if input.MaxCPUPercent < 0 {
		input.MaxCPUPercent = 0
	}
	if input.MaxCPUPercent > 100 {
		input.MaxCPUPercent = 100
	}
	if input.MaxMemoryPercent < 0 {
		input.MaxMemoryPercent = 0
	}
	if input.MaxMemoryPercent > 100 {
		input.MaxMemoryPercent = 100
	}
	return input
}

func normalizeDebounce(input GSLBDebounce) GSLBDebounce {
	if input.CooldownSeconds <= 0 {
		input.CooldownSeconds = 60
	}
	if input.CooldownSeconds > 3600 {
		input.CooldownSeconds = 3600
	}
	if input.UnhealthyThreshold <= 0 {
		input.UnhealthyThreshold = 1
	}
	if input.RecoveryThreshold <= 0 {
		input.RecoveryThreshold = 1
	}
	return input
}

func normalizeDNSSECDenialMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "nsec3":
		return "nsec3"
	default:
		return "nsec"
	}
}

func normalizeDNSSECSignatureValidity(value int) int {
	if value <= 0 {
		return 7 * 24 * 3600
	}
	if value < 3600 {
		return 3600
	}
	return value
}

func normalizeDNSSECNSEC3Iterations(value int) int {
	if value < 0 {
		return 0
	}
	if value > 50 {
		return 50
	}
	return value
}

func normalizeCountryList(values []string) []string {
	result := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		country := strings.ToUpper(strings.TrimSpace(value))
		if len(country) != 2 {
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

func normalizeOperatorList(values []string) []string {
	result := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		operator := normalizeOperator(value)
		if operator == "" {
			continue
		}
		if _, ok := seen[operator]; ok {
			continue
		}
		seen[operator] = struct{}{}
		result = append(result, operator)
	}
	return result
}

func normalizeOperator(value string) string {
	operator := strings.ToLower(strings.TrimSpace(value))
	operator = strings.ReplaceAll(operator, "_", "-")
	operator = strings.ReplaceAll(operator, " ", "-")
	switch operator {
	case "telecom", "china-telecom", "ct", "cn-telecom", "chinatelecom", "中国电信", "电信":
		return "cn-telecom"
	case "unicom", "china-unicom", "cu", "cn-unicom", "chinaunicom", "中国联通", "联通":
		return "cn-unicom"
	case "mobile", "china-mobile", "cmcc", "cn-mobile", "chinamobile", "中国移动", "移动":
		return "cn-mobile"
	case "broadcast", "cbn", "china-broadcast", "cn-broadcast", "广电", "中国广电":
		return "cn-broadcast"
	case "cernet", "edu", "education", "教育网", "中国教育网":
		return "cernet"
	default:
		if len(operator) > 64 {
			operator = operator[:64]
		}
		return operator
	}
}

func normalizeASNList(values []uint32) []uint32 {
	result := make([]uint32, 0, len(values))
	seen := map[uint32]struct{}{}
	for _, value := range values {
		if value == 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func normalizeCIDRList(values []string) []string {
	result := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		cidr, ok := normalizeCIDR(value)
		if !ok {
			continue
		}
		if _, exists := seen[cidr]; exists {
			continue
		}
		seen[cidr] = struct{}{}
		result = append(result, cidr)
	}
	return result
}

func normalizeCIDR(value string) (string, bool) {
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

func normalizeIPList(values []string, recordType string) []string {
	result := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		ip := net.ParseIP(strings.TrimSpace(value))
		if ip == nil {
			continue
		}
		if ipv4 := ip.To4(); ipv4 != nil {
			ip = ipv4
		}
		if recordType == "A" && ip.To4() == nil {
			continue
		}
		if recordType == "AAAA" && ip.To4() != nil {
			continue
		}
		key := ip.String()
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, key)
	}
	return result
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func firstPositive(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}
