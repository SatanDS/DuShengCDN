package service

import (
	"context"
	"dushengcdn/common"
	"dushengcdn/model"
	"dushengcdn/utils/geoip/iputil"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sort"
	"strings"
	"time"
)

func SyncProxyRouteDNS(route *model.ProxyRoute) error {
	if route == nil || !route.DNSAutoSync {
		return nil
	}
	account, err := proxyRouteDNSAccount(route)
	if err != nil {
		recordProxyRouteDNSSyncFailure(route, err)
		return err
	}
	client, err := newCloudflareClientFromAccount(account)
	if err != nil {
		recordProxyRouteDNSSyncFailure(route, err)
		return err
	}

	domains, err := decodeStoredDomains(route.Domains, route.Domain)
	if err != nil {
		recordProxyRouteDNSSyncFailure(route, err)
		return err
	}
	recordType := normalizeDNSRecordType(route.DNSRecordType)
	content := strings.TrimSpace(route.DNSRecordContent)
	targets := splitDNSRecordContent(content)
	if route.DNSAutoTarget || content == "" {
		targets, err = selectHealthyNodeDNSTargets(recordType, route.NodePool, route.DNSTargetCount, route.DNSScheduleMode)
		if err != nil {
			recordProxyRouteDNSSyncFailure(route, err)
			return err
		}
	}
	targets, err = normalizeDNSRecordContents(recordType, targets)
	if err != nil {
		recordProxyRouteDNSSyncFailure(route, err)
		return err
	}
	content = strings.Join(targets, ",")

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	zoneID := strings.TrimSpace(route.DNSZoneID)
	if zoneID == "" {
		zone, err := client.FindBestZoneForDomain(ctx, domains[0])
		if err != nil {
			recordProxyRouteDNSSyncFailure(route, err)
			return err
		}
		zoneID = zone.ID
	}

	recordIDs := decodeDNSRecordIDs(route.DNSRecordIDs)
	nextRecordIDs := make(map[string]string, len(domains))
	for _, domain := range domains {
		recordName := normalizeDNSRecordName(domain)
		if strings.TrimSpace(route.DNSRecordName) != "" && len(domains) == 1 {
			recordName = normalizeDNSRecordName(route.DNSRecordName)
		}
		records, err := client.SyncDNSRecords(ctx, CloudflareDNSSyncInput{
			ZoneID:           zoneID,
			Type:             recordType,
			Name:             recordName,
			Contents:         targets,
			Proxied:          route.CloudflareProxied,
			ManagedRecordIDs: filterDNSRecordIDsForName(recordIDs, recordName),
		})
		if err != nil {
			recordProxyRouteDNSSyncFailure(route, fmt.Errorf("同步 DNS 记录 %s 失败：%w", recordName, err))
			return err
		}
		for _, record := range records {
			nextRecordIDs[dnsRecordStorageKey(recordName, record.Content)] = record.ID
		}
	}

	nextRecordIDSet := make(map[string]struct{}, len(nextRecordIDs))
	for _, recordID := range nextRecordIDs {
		if strings.TrimSpace(recordID) != "" {
			nextRecordIDSet[recordID] = struct{}{}
		}
	}
	for recordName, recordID := range recordIDs {
		if _, ok := nextRecordIDs[recordName]; ok {
			continue
		}
		if _, ok := nextRecordIDSet[recordID]; ok {
			continue
		}
		if err := client.DeleteDNSRecord(ctx, zoneID, recordID); err != nil {
			slog.Warn("delete stale cloudflare dns record failed", "route_id", route.ID, "record_name", recordName, "error", err)
		}
	}

	now := time.Now()
	route.DNSZoneID = zoneID
	route.DNSRecordType = recordType
	route.DNSRecordContent = content
	route.DNSRecordIDs = encodeDNSRecordIDs(nextRecordIDs)
	route.DNSLastSyncStatus = DNSRecordSyncStatusSuccess
	route.DNSLastSyncMessage = fmt.Sprintf("已同步 %d 条 Cloudflare DNS 记录到 %s", len(nextRecordIDs), content)
	route.DNSLastSyncedAt = &now
	if err := model.DB.Model(route).Select(
		"dns_zone_id",
		"dns_record_type",
		"dns_record_content",
		"dns_auto_target",
		"dns_record_ids",
		"cloudflare_proxied",
		"dns_last_sync_status",
		"dns_last_sync_message",
		"dns_last_synced_at",
	).Updates(route).Error; err != nil {
		return err
	}
	slog.Info("cloudflare dns synced", "route_id", route.ID, "site_name", route.SiteName, "records", len(nextRecordIDs), "content", content, "proxied", route.CloudflareProxied)
	return nil
}

func DeleteProxyRouteDNSRecords(route *model.ProxyRoute) error {
	if route == nil || !route.DNSAutoSync || route.DNSAccountID == nil {
		return nil
	}
	recordIDs := decodeDNSRecordIDs(route.DNSRecordIDs)
	if len(recordIDs) == 0 || strings.TrimSpace(route.DNSZoneID) == "" {
		return nil
	}
	account, err := proxyRouteDNSAccount(route)
	if err != nil {
		return err
	}
	client, err := newCloudflareClientFromAccount(account)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for recordName, recordID := range recordIDs {
		if err := client.DeleteDNSRecord(ctx, route.DNSZoneID, recordID); err != nil {
			return fmt.Errorf("删除 Cloudflare DNS 记录 %s 失败：%w", recordName, err)
		}
	}
	return nil
}

func ReconcileCloudflareDNSAutomation() error {
	routes, err := model.ListProxyRoutes()
	if err != nil {
		return err
	}
	for _, route := range routes {
		if route == nil || !route.DNSAutoSync {
			continue
		}
		if route.DNSAutoTarget || strings.TrimSpace(route.DNSRecordContent) == "" {
			previousContent := strings.TrimSpace(route.DNSRecordContent)
			targets, selectErr := selectHealthyNodeDNSTargets(normalizeDNSRecordType(route.DNSRecordType), route.NodePool, route.DNSTargetCount, route.DNSScheduleMode)
			desiredContent := strings.Join(targets, ",")
			if selectErr == nil && desiredContent != "" && desiredContent != previousContent {
				route.DNSRecordContent = desiredContent
				route.DNSAutoTarget = true
			}
		}
		if route.DDOSProtectionMode == DDOSProtectionModeAuto && shouldEnableCloudflareProxyForDDOS() {
			route.CloudflareProxied = true
		}
		if err := SyncProxyRouteDNS(route); err != nil {
			slog.Warn("cloudflare dns reconcile failed", "route_id", route.ID, "site_name", route.SiteName, "error", err)
			continue
		}
	}
	return nil
}

func proxyRouteDNSAccount(route *model.ProxyRoute) (*model.DnsAccount, error) {
	if route == nil || route.DNSAccountID == nil || *route.DNSAccountID == 0 {
		return nil, errors.New("规则未绑定 DNS 账号")
	}
	return model.GetDnsAccountByID(*route.DNSAccountID)
}

func recordProxyRouteDNSSyncFailure(route *model.ProxyRoute, syncErr error) {
	if route == nil || route.ID == 0 || syncErr == nil {
		return
	}
	now := time.Now()
	route.DNSLastSyncStatus = DNSRecordSyncStatusFailed
	route.DNSLastSyncMessage = syncErr.Error()
	route.DNSLastSyncedAt = &now
	if err := model.DB.Model(route).Select("dns_last_sync_status", "dns_last_sync_message", "dns_last_synced_at").Updates(route).Error; err != nil {
		slog.Warn("record dns sync failure failed", "route_id", route.ID, "error", err)
	}
}

func filterDNSRecordIDsForName(recordIDs map[string]string, recordName string) map[string]string {
	recordName = normalizeDNSRecordName(recordName)
	filtered := make(map[string]string)
	for key, recordID := range recordIDs {
		name, _, ok := strings.Cut(key, "|")
		if !ok {
			name = key
		}
		if normalizeDNSRecordName(name) != recordName {
			continue
		}
		filtered[key] = recordID
	}
	return filtered
}

func selectHealthyNodeDNSContent(recordType string) (string, error) {
	targets, err := selectHealthyNodeDNSTargets(recordType, "default", 1, "healthy")
	if err != nil {
		return "", err
	}
	return targets[0], nil
}

type nodeDNSTargetCandidate struct {
	NodeID     string
	Content    string
	Weight     int
	LastSeenAt time.Time
}

func selectHealthyNodeDNSTargets(recordType string, poolName string, count int, scheduleMode string) ([]string, error) {
	nodes, err := model.ListNodes()
	if err != nil {
		return nil, err
	}
	recordType = normalizeDNSRecordType(recordType)
	if recordType != "A" && recordType != "AAAA" {
		return nil, errors.New("automatic DNS target selection only supports A/AAAA records")
	}
	poolName = normalizeNodePoolName(poolName)
	count = normalizeDNSTargetCount(count)
	scheduleMode = normalizeDNSScheduleMode(scheduleMode)

	candidates := make([]nodeDNSTargetCandidate, 0, len(nodes))
	seen := make(map[string]struct{}, len(nodes))
	for _, node := range nodes {
		if normalizeNodePoolName(node.PoolName) != poolName {
			continue
		}
		if !isNodeSchedulableForDNS(node) || !isNodeOnlineAndOpenRestyHealthy(node) {
			continue
		}
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
			content := parsed.String()
			if _, ok := seen[content]; ok {
				continue
			}
			seen[content] = struct{}{}
			candidates = append(candidates, nodeDNSTargetCandidate{
				NodeID:     node.NodeID,
				Content:    content,
				Weight:     normalizeNodeWeight(node.Weight),
				LastSeenAt: node.LastSeenAt,
			})
		}
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no online public node IP is available for %s records in pool %s", recordType, poolName)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		left := candidates[i]
		right := candidates[j]
		if scheduleMode == "weighted" && left.Weight != right.Weight {
			return left.Weight > right.Weight
		}
		if !left.LastSeenAt.Equal(right.LastSeenAt) {
			return left.LastSeenAt.After(right.LastSeenAt)
		}
		if left.NodeID != right.NodeID {
			return left.NodeID < right.NodeID
		}
		return left.Content < right.Content
	})
	if len(candidates) > count {
		candidates = candidates[:count]
	}
	targets := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		targets = append(targets, candidate.Content)
	}
	return targets, nil
}

func selectHealthyNodeDNSContentLegacy(recordType string) (string, error) {
	nodes, err := model.ListNodes()
	if err != nil {
		return "", err
	}
	recordType = normalizeDNSRecordType(recordType)
	for _, node := range nodes {
		if !isNodeHealthyForDNS(node) {
			continue
		}
		ip := iputil.NormalizeIP(node.IP)
		if ip == "" {
			continue
		}
		parsed := net.ParseIP(ip)
		if parsed == nil {
			continue
		}
		switch recordType {
		case "A":
			if parsed.To4() != nil {
				return parsed.String(), nil
			}
		case "AAAA":
			if parsed.To4() == nil {
				return parsed.String(), nil
			}
		default:
			return "", errors.New("自动选择节点仅支持 A/AAAA 记录")
		}
	}
	return "", fmt.Errorf("没有可用于 %s 记录的在线节点公网 IP，请先部署 Agent 或手动填写 DNS 记录内容", recordType)
}

func isNodeHealthyForDNS(node *model.Node) bool {
	if node == nil {
		return false
	}
	if iputil.NormalizeIP(node.IP) == "" || !iputil.IsPublicString(node.IP) {
		return false
	}
	return isNodeOnlineAndOpenRestyHealthy(node)
}

func isNodeOnlineAndOpenRestyHealthy(node *model.Node) bool {
	if node == nil {
		return false
	}
	if node.OpenrestyStatus != "" && normalizeOpenrestyStatus(node.OpenrestyStatus) == OpenrestyStatusUnhealthy {
		return false
	}
	if IsAgentWSConnected(node.NodeID) {
		return true
	}
	if node.LastSeenAt.IsZero() {
		return false
	}
	return time.Since(node.LastSeenAt) <= common.NodeOfflineThreshold
}

func isNodeEligibleForCacheOperation(node *model.Node) bool {
	if node == nil || node.DrainMode {
		return false
	}
	return isNodeOnlineAndOpenRestyHealthy(node)
}

func isNodeSchedulableForDNS(node *model.Node) bool {
	if node == nil {
		return false
	}
	if node.DrainMode {
		return false
	}
	if !node.SchedulingEnabled && (strings.TrimSpace(node.PoolName) != "" || strings.TrimSpace(node.PublicIPs) != "" || node.Weight != 0) {
		return false
	}
	// Old rows and tests may not have scheduling_enabled populated. Treat zero-value
	// nodes as schedulable for backward compatibility unless drain mode is enabled.
	return true
}

func shouldEnableCloudflareProxyForDDOS() bool {
	common.OptionMapRWMutex.RLock()
	requestThreshold := strings.TrimSpace(common.OptionMap["CloudflareDDoSRequestThreshold"])
	errorRateThreshold := strings.TrimSpace(common.OptionMap["CloudflareDDoSErrorRateThreshold"])
	common.OptionMapRWMutex.RUnlock()
	maxRequests := int64(0)
	maxErrorRate := float64(0)
	if requestThreshold != "" {
		_, _ = fmt.Sscan(requestThreshold, &maxRequests)
	}
	if errorRateThreshold != "" {
		_, _ = fmt.Sscan(errorRateThreshold, &maxErrorRate)
	}
	if maxRequests <= 0 {
		maxRequests = 20000
	}
	if maxErrorRate <= 0 {
		maxErrorRate = 30
	}
	reports, err := model.ListRequestReportsSince(time.Now().Add(-5 * time.Minute))
	if err != nil {
		slog.Warn("load request reports for ddos protection failed", "error", err)
		return false
	}
	var requestCount int64
	var errorCount int64
	for _, report := range reports {
		requestCount += report.RequestCount
		errorCount += report.ErrorCount
	}
	if requestCount >= maxRequests {
		return true
	}
	if requestCount <= 0 {
		return false
	}
	errorRate := float64(errorCount) / float64(requestCount) * 100
	return errorRate >= maxErrorRate
}
