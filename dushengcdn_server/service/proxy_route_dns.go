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
	if route.DNSAutoTarget || content == "" {
		content, err = selectHealthyNodeDNSContent(recordType)
		if err != nil {
			recordProxyRouteDNSSyncFailure(route, err)
			return err
		}
	}
	if err := validateDNSRecordContent(recordType, content); err != nil {
		recordProxyRouteDNSSyncFailure(route, err)
		return err
	}

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
		record, err := client.UpsertDNSRecord(ctx, CloudflareDNSUpsertInput{
			ZoneID:  zoneID,
			Type:    recordType,
			Name:    recordName,
			Content: content,
			Proxied: route.CloudflareProxied,
		})
		if err != nil {
			recordProxyRouteDNSSyncFailure(route, fmt.Errorf("同步 DNS 记录 %s 失败：%w", recordName, err))
			return err
		}
		if record != nil {
			nextRecordIDs[recordName] = record.ID
		}
	}

	for recordName, recordID := range recordIDs {
		if _, ok := nextRecordIDs[recordName]; ok {
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
			desiredContent, selectErr := selectHealthyNodeDNSContent(normalizeDNSRecordType(route.DNSRecordType))
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

func selectHealthyNodeDNSContent(recordType string) (string, error) {
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
	if node.OpenrestyStatus != "" && normalizeOpenrestyStatus(node.OpenrestyStatus) == OpenrestyStatusUnhealthy {
		return false
	}
	if iputil.NormalizeIP(node.IP) == "" || !iputil.IsPublicString(node.IP) {
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
