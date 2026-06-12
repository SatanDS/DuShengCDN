package service

import (
	"errors"
	"fmt"
	"strings"

	"dushengcdn/model"
)

func shouldPrecheckAuthoritativeDNSRoute(
	route *model.ProxyRoute,
	domain string,
	domainsJSON string,
	enabled bool,
	dnsProviderMode string,
	dnsZoneIDRef *uint,
	nodePool string,
	dnsRecordType string,
	dnsRecordName string,
	dnsRecordContent string,
	dnsAutoTarget bool,
	dnsTargetCount int,
	dnsScheduleMode string,
	dnsTTL int,
	gslbEnabled bool,
	gslbPolicyJSON string,
) bool {
	if !enabled || dnsProviderMode != DNSProviderModeAuthoritative || dnsZoneIDRef == nil {
		return false
	}
	if route == nil {
		return true
	}
	if !route.Enabled || normalizeDNSProviderMode(route.DNSProviderMode) != DNSProviderModeAuthoritative {
		return true
	}
	if route.DNSZoneIDRef == nil || *route.DNSZoneIDRef != *dnsZoneIDRef {
		return true
	}
	return route.Domain != domain ||
		route.Domains != domainsJSON ||
		route.NodePool != nodePool ||
		route.DNSRecordType != dnsRecordType ||
		route.DNSRecordName != dnsRecordName ||
		route.DNSRecordContent != dnsRecordContent ||
		route.DNSAutoTarget != dnsAutoTarget ||
		route.DNSTargetCount != dnsTargetCount ||
		route.DNSScheduleMode != dnsScheduleMode ||
		route.DNSTTL != dnsTTL ||
		route.GSLBEnabled != gslbEnabled ||
		route.GSLBPolicy != gslbPolicyJSON
}

func buildAuthoritativeDNSPrecheckProxyRoute(
	route *model.ProxyRoute,
	nodePool string,
	dnsRecordType string,
	dnsRecordName string,
	dnsTargetCount int,
	dnsScheduleMode string,
	dnsTTL int,
	gslbEnabled bool,
	gslbPolicyJSON string,
	dnsZoneIDRef *uint,
) *model.ProxyRoute {
	precheckRoute := &model.ProxyRoute{}
	if route != nil {
		copied := *route
		precheckRoute = &copied
	}
	precheckRoute.NodePool = nodePool
	precheckRoute.Enabled = true
	precheckRoute.DNSProviderMode = DNSProviderModeAuthoritative
	precheckRoute.DNSZoneIDRef = dnsZoneIDRef
	precheckRoute.DNSRecordType = dnsRecordType
	precheckRoute.DNSRecordName = dnsRecordName
	precheckRoute.DNSRecordContent = ""
	precheckRoute.DNSAutoTarget = true
	precheckRoute.DNSTargetCount = dnsTargetCount
	precheckRoute.DNSScheduleMode = dnsScheduleMode
	precheckRoute.DNSTTL = dnsTTL
	precheckRoute.GSLBEnabled = gslbEnabled
	precheckRoute.GSLBPolicy = gslbPolicyJSON
	return precheckRoute
}

func normalizeProxyRouteDNSSettings(input ProxyRouteInput) (*uint, string, string, string, string, bool, string, error) {
	ddosMode := normalizeDDOSProtectionMode(input.DDOSProtectionMode)
	if !input.DNSAutoSync {
		return nil, "", normalizeDNSRecordType(input.DNSRecordType), "", "", false, ddosMode, nil
	}

	if input.DNSAccountID == nil || *input.DNSAccountID == 0 {
		return nil, "", "", "", "", false, "", errors.New("启用自动 DNS 时必须选择 DNS 账号")
	}
	account, err := model.GetDnsAccountByID(*input.DNSAccountID)
	if err != nil {
		return nil, "", "", "", "", false, "", errors.New("选择的 DNS 账号不存在")
	}
	if strings.ToLower(strings.TrimSpace(account.Type)) != cloudflareDNSProviderType {
		return nil, "", "", "", "", false, "", errors.New("自动 DNS 目前仅支持 Cloudflare DNS 账号")
	}

	recordType := normalizeDNSRecordType(input.DNSRecordType)
	recordName := normalizeDNSRecordName(input.DNSRecordName)
	recordContent := strings.TrimSpace(input.DNSRecordContent)
	if recordName != "" && !isValidProxyRouteDomain(recordName) {
		return nil, "", "", "", "", false, "", errors.New("DNS 记录名格式无效")
	}
	if recordContent != "" {
		if err := validateDNSRecordContent(recordType, recordContent); err != nil {
			return nil, "", "", "", "", false, "", err
		}
	}
	dnsAutoTarget := input.DNSAutoTarget || recordContent == ""
	if recordType == "CNAME" && dnsAutoTarget {
		return nil, "", "", "", "", false, "", errors.New("CNAME 记录必须手动填写记录内容")
	}

	dnsAccountID := *input.DNSAccountID
	return &dnsAccountID, strings.TrimSpace(input.DNSZoneID), recordType, recordName, recordContent, dnsAutoTarget, ddosMode, nil
}

func normalizeProxyRouteDNSSettingsV3(input ProxyRouteInput) (*uint, string, string, string, string, bool, int, string, int, bool, ProxyRouteGSLBPolicy, string, string, string, error) {
	ddosMode := normalizeDDOSProtectionMode(input.DDOSProtectionMode)
	dnsTargetCount := normalizeDNSTargetCount(input.DNSTargetCount)
	dnsScheduleMode := normalizeDNSScheduleMode(input.DNSScheduleMode)
	dnsTTL := normalizeDNSTTL(input.DNSTTL)
	gslbPolicy, err := normalizeGSLBPolicy(input.GSLBPolicy, input.NodePool, dnsTargetCount, dnsScheduleMode, dnsTTL)
	if err != nil {
		return nil, "", "", "", "", false, 0, "", 0, false, ProxyRouteGSLBPolicy{}, "", "", "", err
	}
	gslbEnabled := input.GSLBEnabled
	dnsProviderMode := normalizeDNSProviderMode(input.DNSProviderMode)
	if dnsProviderMode == DNSProviderModeAuthoritative {
		ddosProvider, ddosTarget, err := normalizeDDOSProtectionTarget(input, dnsProviderMode)
		if err != nil {
			return nil, "", "", "", "", false, 0, "", 0, false, ProxyRouteGSLBPolicy{}, "", "", "", err
		}
		dnsRecordType := normalizeDNSRecordType(input.DNSRecordType)
		if gslbEnabled && dnsRecordType != "A" && dnsRecordType != "AAAA" {
			return nil, "", "", "", "", false, 0, "", 0, false, ProxyRouteGSLBPolicy{}, "", "", "", errors.New("GSLB scheduling only supports A/AAAA records")
		}
		return nil,
			"",
			dnsRecordType,
			normalizeDNSRecordName(input.DNSRecordName),
			"",
			true,
			dnsTargetCount,
			dnsScheduleMode,
			dnsTTL,
			gslbEnabled,
			gslbPolicy,
			ddosMode,
			ddosProvider,
			ddosTarget,
			nil
	}
	if !input.DNSAutoSync {
		ddosProvider, ddosTarget, err := normalizeDDOSProtectionTarget(input, dnsProviderMode)
		if err != nil {
			return nil, "", "", "", "", false, 0, "", 0, false, ProxyRouteGSLBPolicy{}, "", "", "", err
		}
		return nil, "", normalizeDNSRecordType(input.DNSRecordType), "", "", false, dnsTargetCount, dnsScheduleMode, dnsTTL, false, gslbPolicy, ddosMode, ddosProvider, ddosTarget, nil
	}
	if input.DNSAccountID == nil || *input.DNSAccountID == 0 {
		return nil, "", "", "", "", false, 0, "", 0, false, ProxyRouteGSLBPolicy{}, "", "", "", errors.New("automatic DNS requires a DNS account")
	}
	account, err := model.GetDnsAccountByID(*input.DNSAccountID)
	if err != nil {
		return nil, "", "", "", "", false, 0, "", 0, false, ProxyRouteGSLBPolicy{}, "", "", "", errors.New("selected DNS account does not exist")
	}
	if strings.ToLower(strings.TrimSpace(account.Type)) != cloudflareDNSProviderType {
		return nil, "", "", "", "", false, 0, "", 0, false, ProxyRouteGSLBPolicy{}, "", "", "", errors.New("automatic DNS currently only supports Cloudflare DNS accounts")
	}
	dnsRecordType := normalizeDNSRecordType(input.DNSRecordType)
	dnsRecordName := normalizeDNSRecordName(input.DNSRecordName)
	if dnsRecordName != "" && !isValidProxyRouteDomain(dnsRecordName) {
		return nil, "", "", "", "", false, 0, "", 0, false, ProxyRouteGSLBPolicy{}, "", "", "", errors.New("DNS record name format is invalid")
	}
	dnsRecordContent := strings.TrimSpace(input.DNSRecordContent)
	if dnsRecordContent != "" {
		contents, err := normalizeDNSRecordContents(dnsRecordType, splitDNSRecordContent(dnsRecordContent))
		if err != nil {
			return nil, "", "", "", "", false, 0, "", 0, false, ProxyRouteGSLBPolicy{}, "", "", "", err
		}
		if dnsRecordType == "CNAME" && len(contents) > 1 {
			return nil, "", "", "", "", false, 0, "", 0, false, ProxyRouteGSLBPolicy{}, "", "", "", errors.New("CNAME record only supports one target")
		}
		dnsRecordContent = strings.Join(contents, ",")
	}
	dnsAutoTarget := input.DNSAutoTarget || dnsRecordContent == "" || gslbEnabled
	if dnsRecordType == "CNAME" && dnsAutoTarget {
		return nil, "", "", "", "", false, 0, "", 0, false, ProxyRouteGSLBPolicy{}, "", "", "", errors.New("CNAME record requires manual content")
	}
	if gslbEnabled && dnsRecordType != "A" && dnsRecordType != "AAAA" {
		return nil, "", "", "", "", false, 0, "", 0, false, ProxyRouteGSLBPolicy{}, "", "", "", errors.New("GSLB scheduling only supports A/AAAA records")
	}
	ddosProvider, ddosTarget, err := normalizeDDOSProtectionTarget(input, dnsProviderMode)
	if err != nil {
		return nil, "", "", "", "", false, 0, "", 0, false, ProxyRouteGSLBPolicy{}, "", "", "", err
	}
	dnsAccountID := *input.DNSAccountID
	return &dnsAccountID,
		strings.TrimSpace(input.DNSZoneID),
		dnsRecordType,
		dnsRecordName,
		dnsRecordContent,
		dnsAutoTarget,
		dnsTargetCount,
		dnsScheduleMode,
		dnsTTL,
		gslbEnabled,
		gslbPolicy,
		ddosMode,
		ddosProvider,
		ddosTarget,
		nil
}

func normalizeDNSProviderMode(raw string) string {
	mode := strings.ToLower(strings.TrimSpace(raw))
	switch mode {
	case DNSProviderModeAuthoritative:
		return DNSProviderModeAuthoritative
	default:
		return DNSProviderModeCloudflare
	}
}

func normalizeProxyRouteAuthoritativeZone(input ProxyRouteInput, providerMode string, domains []string) (*uint, error) {
	if providerMode != DNSProviderModeAuthoritative {
		return nil, nil
	}
	if input.DNSZoneIDRef == nil || *input.DNSZoneIDRef == 0 {
		return nil, errors.New("authoritative DNS mode requires a DNS zone")
	}
	zone, err := model.GetDNSZoneByID(*input.DNSZoneIDRef)
	if err != nil {
		return nil, errors.New("selected DNS zone does not exist")
	}
	if !zone.Enabled {
		return nil, errors.New("selected DNS zone is disabled")
	}
	for _, domain := range domains {
		if !domainBelongsToZone(domain, zone.Name) {
			return nil, fmt.Errorf("domain %s is not under DNS zone %s", domain, zone.Name)
		}
	}
	zoneID := *input.DNSZoneIDRef
	return &zoneID, nil
}

func domainBelongsToZone(domain string, zoneName string) bool {
	domain = normalizeDNSRecordName(domain)
	zoneName = normalizeDNSRecordName(zoneName)
	return domain == zoneName || strings.HasSuffix(domain, "."+zoneName)
}

func shouldSyncProxyRouteCloudflareDNS(route *model.ProxyRoute) bool {
	if route == nil || !route.DNSAutoSync {
		return false
	}
	return normalizeDNSProviderMode(route.DNSProviderMode) == DNSProviderModeCloudflare
}

func normalizeDNSTargetCount(value int) int {
	if value <= 0 {
		return 1
	}
	if value > 20 {
		return 20
	}
	return value
}

func normalizeDNSScheduleMode(raw string) string {
	mode := strings.ToLower(strings.TrimSpace(raw))
	switch mode {
	case "load_aware":
		return "load_aware"
	case "weighted":
		return "weighted"
	default:
		return "healthy"
	}
}

func normalizeDNSTTL(value int) int {
	if value <= 0 {
		return cloudflareDefaultRecordTTL
	}
	if value == cloudflareDefaultRecordTTL {
		return cloudflareDefaultRecordTTL
	}
	if value < 30 {
		return 30
	}
	if value > 86400 {
		return 86400
	}
	return value
}

func normalizeDDOSProtectionMode(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case DDOSProtectionModeAuto:
		return DDOSProtectionModeAuto
	default:
		return DDOSProtectionModeOff
	}
}

func normalizeDDOSProtectionProvider(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case DDOSProtectionProviderCustom:
		return DDOSProtectionProviderCustom
	default:
		return DDOSProtectionProviderCloudflare
	}
}

func normalizeDDOSProtectionTarget(input ProxyRouteInput, dnsProviderMode string) (string, string, error) {
	mode := normalizeDDOSProtectionMode(input.DDOSProtectionMode)
	provider := normalizeDDOSProtectionProvider(input.DDOSProtectionProvider)
	rawTarget := strings.TrimSpace(input.DDOSProtectionTarget)
	if mode != DDOSProtectionModeAuto {
		return provider, "", nil
	}
	if dnsProviderMode == DNSProviderModeAuthoritative && provider == DDOSProtectionProviderCloudflare {
		return provider, "", errors.New("authoritative DNS mode cannot enable Cloudflare DDoS protection provider")
	}
	if provider == DDOSProtectionProviderCloudflare {
		if dnsProviderMode != DNSProviderModeCloudflare {
			return provider, "", nil
		}
		targetID := uint(0)
		if rawTarget != "" {
			var parsed uint64
			if _, err := fmt.Sscan(rawTarget, &parsed); err != nil || parsed == 0 {
				return provider, "", errors.New("Cloudflare DDoS protection account is invalid")
			}
			targetID = uint(parsed)
		} else if input.DNSAccountID != nil {
			targetID = *input.DNSAccountID
		}
		if targetID > 0 {
			account, err := model.GetDnsAccountByID(targetID)
			if err != nil {
				return provider, "", errors.New("selected Cloudflare DDoS protection account does not exist")
			}
			if strings.ToLower(strings.TrimSpace(account.Type)) != cloudflareDNSProviderType {
				return provider, "", errors.New("Cloudflare DDoS protection account must be a Cloudflare DNS account")
			}
			return provider, fmt.Sprint(targetID), nil
		}
		return provider, "", nil
	}
	if provider == DDOSProtectionProviderCustom {
		target := normalizeNodePoolName(rawTarget)
		if target == "" {
			return provider, "", errors.New("custom DDoS protection requires a node/IP pool")
		}
		if normalizeDNSRecordType(input.DNSRecordType) != "A" && normalizeDNSRecordType(input.DNSRecordType) != "AAAA" {
			return provider, "", errors.New("custom DDoS protection only supports A/AAAA records")
		}
		return provider, target, nil
	}
	return provider, "", nil
}
