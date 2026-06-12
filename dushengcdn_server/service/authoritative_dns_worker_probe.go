package service

import (
	"context"
	"dushengcdn/model"
	"dushengcdn/utils/security"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/miekg/dns"
	"gorm.io/gorm"
	"math"
	"net"
	"net/url"
	"sort"
	"strings"
	"time"
)

var dnsWorkerProbeExchange = exchangeDNSWorkerProbe

var dnsWorkerAddressLookupIP = net.DefaultResolver.LookupIPAddr

type DNSWorkerProbeInput struct {
	ZoneID uint `json:"zone_id"`
}

type DNSWorkerProbeView struct {
	WorkerID      string                     `json:"worker_id"`
	Name          string                     `json:"name"`
	PublicAddress string                     `json:"public_address"`
	QueryName     string                     `json:"query_name"`
	QueryType     string                     `json:"query_type"`
	CheckedAt     time.Time                  `json:"checked_at"`
	Results       []DNSWorkerProbeResultView `json:"results"`
}

type DNSWorkerProbeResultView struct {
	Network     string `json:"network"`
	Reachable   bool   `json:"reachable"`
	DurationMs  int64  `json:"duration_ms"`
	RCode       string `json:"rcode"`
	AnswerCount int    `json:"answer_count"`
	Error       string `json:"error,omitempty"`
}

type DNSWorkerNodeProbeView struct {
	NodeID          string                     `json:"node_id"`
	NodeName        string                     `json:"node_name"`
	PoolName        string                     `json:"pool_name"`
	Status          string                     `json:"status"`
	CheckedAt       time.Time                  `json:"checked_at"`
	Healthy         bool                       `json:"healthy"`
	ProbeStatus     string                     `json:"probe_status"`
	ProbeAgeSeconds int64                      `json:"probe_age_seconds"`
	ProbeMessage    string                     `json:"probe_message"`
	AverageRTTMs    float64                    `json:"average_rtt_ms"`
	MaxRTTMs        int64                      `json:"max_rtt_ms"`
	Results         []DNSWorkerProbeResultView `json:"results"`
	LastError       string                     `json:"last_error"`
	FailureSamples  int                        `json:"failure_samples"`
}

func ProbeAuthoritativeDNSWorker(id uint, input DNSWorkerProbeInput) (*DNSWorkerProbeView, error) {
	worker, err := model.GetDNSWorkerByID(id)
	if err != nil {
		return nil, err
	}
	target, err := normalizeDNSWorkerProbeAddress(context.Background(), worker.PublicAddress)
	if err != nil {
		return nil, err
	}
	queryName, err := dnsWorkerProbeQueryName(input.ZoneID)
	if err != nil {
		return nil, err
	}

	view := &DNSWorkerProbeView{
		WorkerID:      worker.WorkerID,
		Name:          worker.Name,
		PublicAddress: worker.PublicAddress,
		QueryName:     queryName,
		QueryType:     "SOA",
		CheckedAt:     time.Now().UTC(),
		Results:       make([]DNSWorkerProbeResultView, 0, 2),
	}
	for _, network := range []string{"udp", "tcp"} {
		view.Results = append(view.Results, dnsWorkerProbeExchange(context.Background(), target, network, queryName, dns.TypeSOA, defaultDNSWorkerProbeTimeout))
	}
	if err := persistDNSWorkerProbeResult(worker, view); err != nil {
		return nil, err
	}
	return view, nil
}

func persistDNSWorkerProbeResult(worker *model.DNSWorker, view *DNSWorkerProbeView) error {
	if worker == nil || view == nil {
		return nil
	}
	raw, err := json.Marshal(view.Results)
	if err != nil {
		return err
	}
	checkedAt := view.CheckedAt
	worker.LastProbeAt = &checkedAt
	worker.LastProbeQuery = strings.TrimSpace(view.QueryName + " " + view.QueryType)
	worker.LastProbeResult = string(raw)
	return worker.UpdateProbeResult()
}

func decodeDNSWorkerProbeResults(raw string) []DNSWorkerProbeResultView {
	if strings.TrimSpace(raw) == "" {
		return []DNSWorkerProbeResultView{}
	}
	var results []DNSWorkerProbeResultView
	if err := json.Unmarshal([]byte(raw), &results); err != nil {
		return []DNSWorkerProbeResultView{}
	}
	cleaned := make([]DNSWorkerProbeResultView, 0, len(results))
	for _, result := range results {
		result.Network = strings.ToUpper(strings.TrimSpace(result.Network))
		if result.Network == "" {
			continue
		}
		if result.DurationMs < 0 {
			result.DurationMs = 0
		}
		result.RCode = strings.ToUpper(strings.TrimSpace(result.RCode))
		cleaned = append(cleaned, result)
	}
	return cleaned
}

type dnsWorkerProbeState struct {
	status     string
	healthy    bool
	ageSeconds int64
	message    string
}

func evaluateDNSWorkerProbeState(now time.Time, checkedAt *time.Time, results []DNSWorkerProbeResultView) dnsWorkerProbeState {
	if checkedAt == nil || len(results) == 0 {
		return dnsWorkerProbeState{
			status:  dnsWorkerProbeUnknown,
			message: "尚未执行公网 UDP/TCP 53 探测",
		}
	}
	checked := normalizeDNSWorkerCheckedAt(checkedAt, now, time.Time{})
	age := now.UTC().Sub(checked.UTC())
	ageSeconds := int64(0)
	if age > 0 {
		ageSeconds = int64(age.Seconds())
	}
	if age > defaultDNSWorkerProbeMaxAge {
		return dnsWorkerProbeState{
			status:     dnsWorkerProbeStale,
			ageSeconds: ageSeconds,
			message:    fmt.Sprintf("最近一次公网 UDP/TCP 53 探测超过 %s 未刷新", formatDNSWorkerNodeProbeMaxAge(defaultDNSWorkerProbeMaxAge)),
		}
	}
	reachableByNetwork := map[string]bool{}
	for _, result := range results {
		network := strings.ToUpper(strings.TrimSpace(result.Network))
		if network == "" {
			continue
		}
		reachableByNetwork[network] = result.Reachable
	}
	udpReachable := reachableByNetwork["UDP"]
	tcpReachable := reachableByNetwork["TCP"]
	switch {
	case udpReachable && tcpReachable:
		return dnsWorkerProbeState{
			status:     dnsWorkerProbeHealthy,
			healthy:    true,
			ageSeconds: ageSeconds,
			message:    "UDP/TCP 53 均可达",
		}
	case udpReachable || tcpReachable:
		return dnsWorkerProbeState{
			status:     dnsWorkerProbePartial,
			ageSeconds: ageSeconds,
			message:    "仅部分 DNS 协议可达",
		}
	default:
		return dnsWorkerProbeState{
			status:     dnsWorkerProbeFailed,
			ageSeconds: ageSeconds,
			message:    "UDP/TCP 53 探测均失败",
		}
	}
}

func normalizeDNSWorkerProbeAddress(ctx context.Context, raw string) (string, error) {
	address, err := normalizeDNSWorkerProbeAddressForStorage(raw)
	if err != nil {
		return "", err
	}
	if err := validateDNSWorkerProbeAddress(ctx, address); err != nil {
		return "", err
	}
	return address, nil
}

func validateDNSWorkerPublicAddressForStorage(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", nil
	}
	return normalizeDNSWorkerProbeAddressForStorage(raw)
}

func normalizeDNSWorkerProbeAddressForStorage(raw string) (string, error) {
	address := strings.TrimSpace(raw)
	if address == "" {
		return "", errors.New("DNS Worker public address is empty")
	}
	if strings.Contains(address, "://") {
		if parsed, err := dnsWorkerProbeURLHost(address); err == nil {
			address = parsed
		}
	}
	host, port, err := net.SplitHostPort(address)
	if err == nil {
		host = strings.Trim(host, "[]")
		if strings.TrimSpace(host) == "" {
			return "", errors.New("DNS Worker public address host is empty")
		}
		if strings.TrimSpace(port) == "" {
			port = "53"
		}
		return normalizeDNSWorkerPublicAddressHostPort(host, port)
	}
	if strings.Count(address, ":") > 1 {
		if ip := net.ParseIP(strings.Trim(address, "[]")); ip != nil {
			return normalizeDNSWorkerPublicAddressHostPort(ip.String(), "53")
		}
		return "", errors.New("DNS Worker IPv6 address must be wrapped as [addr]:port or be a valid IPv6 literal")
	}
	if strings.Contains(address, ":") {
		host, port, ok := strings.Cut(address, ":")
		if !ok || strings.TrimSpace(host) == "" {
			return "", errors.New("DNS Worker public address format is invalid")
		}
		if strings.TrimSpace(port) == "" {
			port = "53"
		}
		return normalizeDNSWorkerPublicAddressHostPort(host, port)
	}
	return normalizeDNSWorkerPublicAddressHostPort(address, "53")
}

func normalizeDNSWorkerPublicAddressHostPort(host string, port string) (string, error) {
	host = strings.TrimSpace(strings.Trim(host, "[]"))
	port = strings.TrimSpace(port)
	if host == "" {
		return "", errors.New("DNS Worker public address host is empty")
	}
	if err := security.ValidatePublicHostname(host); err != nil {
		return "", fmt.Errorf("DNS Worker public address host is not public: %w", err)
	}
	if port == "" {
		port = "53"
	}
	if port != "53" {
		return "", errors.New("DNS Worker public address port must be 53")
	}
	if ip := net.ParseIP(host); ip != nil {
		host = ip.String()
	}
	return net.JoinHostPort(host, port), nil
}

func validateDNSWorkerProbeAddress(ctx context.Context, address string) error {
	_, err := resolveDNSWorkerProbeTargets(ctx, address)
	return err
}

func resolveDNSWorkerProbeTargets(ctx context.Context, address string) ([]string, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	if port != "53" {
		return nil, errors.New("DNS Worker public address port must be 53")
	}
	if err := security.ValidatePublicHostname(host); err != nil {
		return nil, fmt.Errorf("DNS Worker public address host is not public: %w", err)
	}
	if ip := net.ParseIP(strings.Trim(host, "[]")); ip != nil {
		if err := security.ValidatePublicIP(ip); err != nil {
			return nil, err
		}
		return []string{net.JoinHostPort(ip.String(), port)}, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	lookupCtx, cancel := context.WithTimeout(ctx, defaultDNSWorkerProbeTimeout)
	defer cancel()
	addresses, err := dnsWorkerAddressLookupIP(lookupCtx, host)
	if err != nil {
		return nil, err
	}
	if len(addresses) == 0 {
		return nil, fmt.Errorf("DNS Worker public address host %s has no addresses", host)
	}
	targets := make([]string, 0, len(addresses))
	for _, resolved := range addresses {
		if err := security.ValidatePublicIP(resolved.IP); err != nil {
			return nil, fmt.Errorf("DNS Worker public address host %s resolved to unsafe ip: %w", host, err)
		}
		targets = append(targets, net.JoinHostPort(resolved.IP.String(), port))
	}
	return targets, nil
}

func dnsWorkerProbeURLHost(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if parsed.Host != "" {
		return parsed.Host, nil
	}
	return parsed.Path, nil
}

func dnsWorkerProbeQueryName(zoneID uint) (string, error) {
	if zoneID > 0 {
		zone, err := model.GetDNSZoneByID(zoneID)
		if err != nil {
			return "", err
		}
		return dns.Fqdn(zone.Name), nil
	}
	var zone model.DNSZone
	if err := model.DB.Where("enabled = ?", true).Order("id asc").First(&zone).Error; err == nil {
		return dns.Fqdn(zone.Name), nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", err
	}
	return ".", nil
}

func exchangeDNSWorkerProbe(ctx context.Context, target string, network string, qname string, qtype uint16, timeout time.Duration) DNSWorkerProbeResultView {
	result := DNSWorkerProbeResultView{
		Network: strings.ToUpper(network),
	}
	if timeout <= 0 {
		timeout = defaultDNSWorkerProbeTimeout
	}
	targets, err := resolveDNSWorkerProbeTargets(ctx, target)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	message := new(dns.Msg)
	message.SetQuestion(dns.Fqdn(qname), qtype)
	client := &dns.Client{
		Net:     network,
		Timeout: timeout,
	}
	startedAt := time.Now()
	var lastErr error
	for _, resolvedTarget := range targets {
		response, _, err := client.ExchangeContext(ctx, message, resolvedTarget)
		if err != nil {
			lastErr = err
			continue
		}
		result.DurationMs = time.Since(startedAt).Milliseconds()
		if response == nil {
			result.Error = "empty DNS response"
			return result
		}
		result.Reachable = true
		result.RCode = dns.RcodeToString[response.Rcode]
		if result.RCode == "" {
			result.RCode = fmt.Sprintf("RCODE%d", response.Rcode)
		}
		result.AnswerCount = len(response.Answer)
		return result
	}
	result.DurationMs = time.Since(startedAt).Milliseconds()
	if lastErr != nil {
		result.Error = lastErr.Error()
		return result
	}
	result.Error = "DNS Worker public address has no dialable addresses"
	return result
}

type dnsWorkerNodeProbeStats struct {
	totalCount        int
	healthyCount      int
	partialCount      int
	failedCount       int
	unknownCount      int
	staleCount        int
	totalAverageRTTMs float64
	averageSamples    int
	maxRTTMs          int64
	probes            []DNSWorkerNodeProbeView
}

type dnsWorkerProbeTargetNode struct {
	NodeID   string
	Name     string
	PoolName string
	Status   string
}

func buildDNSWorkerNodeProbeStats(now time.Time) map[string]*dnsWorkerNodeProbeStats {
	return buildDNSWorkerNodeProbeStatsForWorkerIDs(now, activeDNSWorkerIDs())
}

func buildDNSWorkerNodeProbeStatsForWorkerIDs(now time.Time, workerIDs []string) map[string]*dnsWorkerNodeProbeStats {
	nodes, err := model.ListNodes()
	if err != nil {
		return buildDNSWorkerNodeProbeStatsForNodesAndWorkers(now, nil, workerIDs)
	}
	return buildDNSWorkerNodeProbeStatsForNodesAndWorkers(now, nodes, workerIDs)
}

func buildDNSWorkerNodeProbeStatsForNodes(now time.Time, nodes []*model.Node) map[string]*dnsWorkerNodeProbeStats {
	return buildDNSWorkerNodeProbeStatsForNodesAndWorkers(now, nodes, activeDNSWorkerIDs())
}

func buildDNSWorkerNodeProbeStatsForNodesAndWorkers(now time.Time, nodes []*model.Node, workerIDs []string) map[string]*dnsWorkerNodeProbeStats {
	nodesByID := make(map[string]*model.Node, len(nodes))
	targetNodes := make([]dnsWorkerProbeTargetNode, 0, len(nodes))
	targetNodeIDs := make([]string, 0, len(nodes))
	for _, node := range nodes {
		if node == nil {
			continue
		}
		nodesByID[node.NodeID] = node
		if !shouldExpectAgentDNSProbeForNode(node) {
			continue
		}
		targetNodes = append(targetNodes, dnsWorkerProbeTargetNode{
			NodeID:   strings.TrimSpace(node.NodeID),
			Name:     displayNodeName(node),
			PoolName: strings.TrimSpace(node.PoolName),
			Status:   computeNodeStatus(node),
		})
		targetNodeIDs = append(targetNodeIDs, strings.TrimSpace(node.NodeID))
	}
	sort.SliceStable(targetNodes, func(i, j int) bool {
		if targetNodes[i].PoolName != targetNodes[j].PoolName {
			return targetNodes[i].PoolName < targetNodes[j].PoolName
		}
		if targetNodes[i].Name != targetNodes[j].Name {
			return targetNodes[i].Name < targetNodes[j].Name
		}
		return targetNodes[i].NodeID < targetNodes[j].NodeID
	})

	statsByWorker := make(map[string]*dnsWorkerNodeProbeStats)
	for _, workerID := range workerIDs {
		stats := &dnsWorkerNodeProbeStats{probes: []DNSWorkerNodeProbeView{}}
		statsByWorker[workerID] = stats
		for _, node := range targetNodes {
			if node.NodeID == "" {
				continue
			}
			stats.probes = append(stats.probes, DNSWorkerNodeProbeView{
				NodeID:       node.NodeID,
				NodeName:     node.Name,
				PoolName:     node.PoolName,
				Status:       node.Status,
				ProbeStatus:  dnsWorkerProbeUnknown,
				ProbeMessage: "尚未收到 Agent 多节点探测结果",
				Results:      []DNSWorkerProbeResultView{},
			})
		}
	}

	probes, err := model.ListDNSWorkerNodeProbesByScope(workerIDs, targetNodeIDs)
	if err != nil {
		return statsByWorker
	}
	for _, probe := range probes {
		if probe == nil {
			continue
		}
		workerID := strings.TrimSpace(probe.WorkerID)
		nodeID := strings.TrimSpace(probe.NodeID)
		if workerID == "" || nodeID == "" {
			continue
		}
		node := nodesByID[nodeID]
		if !shouldExpectAgentDNSProbeForNode(node) {
			continue
		}
		stats := statsByWorker[workerID]
		if stats == nil {
			stats = &dnsWorkerNodeProbeStats{probes: []DNSWorkerNodeProbeView{}}
			statsByWorker[workerID] = stats
		}
		nodeName := displayNodeName(node)
		poolName := strings.TrimSpace(node.PoolName)
		nodeStatus := computeNodeStatus(node)
		probeState := evaluateDNSWorkerNodeProbeState(now, probe)
		checkedAt := normalizeDNSWorkerCheckedAt(&probe.CheckedAt, now, probe.UpdatedAt, probe.CreatedAt)
		existingIndex := findDNSWorkerNodeProbeViewIndex(stats.probes, nodeID)
		averageRTTMs, maxRTTMs := normalizeDNSWorkerNodeProbeRTT(probe.AverageRTTMs, probe.MaxRTTMs)
		view := DNSWorkerNodeProbeView{
			NodeID:          nodeID,
			NodeName:        nodeName,
			PoolName:        poolName,
			Status:          nodeStatus,
			CheckedAt:       checkedAt,
			Healthy:         probeState.healthy,
			ProbeStatus:     probeState.status,
			ProbeAgeSeconds: probeState.ageSeconds,
			ProbeMessage:    probeState.message,
			AverageRTTMs:    averageRTTMs,
			MaxRTTMs:        maxRTTMs,
			Results:         decodeDNSWorkerProbeResults(probe.ResultsJSON),
			LastError:       probe.LastError,
			FailureSamples:  probe.FailureSamples,
		}
		if existingIndex >= 0 {
			stats.probes[existingIndex] = view
		} else {
			stats.probes = append(stats.probes, view)
		}
	}
	for _, stats := range statsByWorker {
		recomputeDNSWorkerNodeProbeStats(stats)
		sort.SliceStable(stats.probes, func(i, j int) bool {
			if stats.probes[i].ProbeStatus != stats.probes[j].ProbeStatus {
				return dnsWorkerProbeStatusSortRank(stats.probes[i].ProbeStatus) < dnsWorkerProbeStatusSortRank(stats.probes[j].ProbeStatus)
			}
			if stats.probes[i].Healthy != stats.probes[j].Healthy {
				return stats.probes[i].Healthy
			}
			if !stats.probes[i].CheckedAt.Equal(stats.probes[j].CheckedAt) {
				return stats.probes[i].CheckedAt.After(stats.probes[j].CheckedAt)
			}
			if stats.probes[i].PoolName != stats.probes[j].PoolName {
				return stats.probes[i].PoolName < stats.probes[j].PoolName
			}
			if stats.probes[i].NodeName != stats.probes[j].NodeName {
				return stats.probes[i].NodeName < stats.probes[j].NodeName
			}
			return stats.probes[i].NodeID < stats.probes[j].NodeID
		})
	}
	return statsByWorker
}

func currentSchedulableDNSProbeNodeIDs() map[string]struct{} {
	nodes, err := model.ListNodes()
	if err != nil {
		return map[string]struct{}{}
	}
	result := make(map[string]struct{}, len(nodes))
	for _, node := range nodes {
		if !shouldExpectAgentDNSProbeForNode(node) {
			continue
		}
		nodeID := strings.TrimSpace(node.NodeID)
		if nodeID == "" {
			continue
		}
		result[nodeID] = struct{}{}
	}
	return result
}

func recomputeDNSWorkerNodeProbeStats(stats *dnsWorkerNodeProbeStats) {
	if stats == nil {
		return
	}
	stats.totalCount = 0
	stats.healthyCount = 0
	stats.partialCount = 0
	stats.failedCount = 0
	stats.unknownCount = 0
	stats.staleCount = 0
	stats.totalAverageRTTMs = 0
	stats.averageSamples = 0
	stats.maxRTTMs = 0
	for _, probe := range stats.probes {
		stats.totalCount++
		switch probe.ProbeStatus {
		case dnsWorkerProbeHealthy:
			stats.healthyCount++
		case dnsWorkerProbePartial:
			stats.partialCount++
		case dnsWorkerProbeFailed:
			stats.failedCount++
		case dnsWorkerProbeStale:
			stats.staleCount++
		case dnsWorkerProbeUnknown:
			stats.unknownCount++
		}
		if probe.ProbeStatus != dnsWorkerProbeStale && probe.AverageRTTMs > 0 {
			stats.totalAverageRTTMs += probe.AverageRTTMs
			stats.averageSamples++
		}
		if probe.ProbeStatus != dnsWorkerProbeStale && probe.MaxRTTMs > stats.maxRTTMs {
			stats.maxRTTMs = probe.MaxRTTMs
		}
	}
}

func shouldExpectAgentDNSProbeForNode(node *model.Node) bool {
	if node == nil {
		return false
	}
	if strings.TrimSpace(node.NodeID) == "" {
		return false
	}
	if !node.SchedulingEnabled || node.DrainMode {
		return false
	}
	return computeNodeStatus(node) == NodeStatusOnline
}

func displayNodeName(node *model.Node) string {
	if node == nil {
		return ""
	}
	name := strings.TrimSpace(node.Name)
	if name != "" {
		return name
	}
	return strings.TrimSpace(node.NodeID)
}

func expectedAgentDNSProbeWorkerIDs() []string {
	targets := buildAgentDNSProbeTargets()
	workerIDs := make([]string, 0, len(targets))
	seen := map[string]struct{}{}
	for _, target := range targets {
		workerID := strings.TrimSpace(target.WorkerID)
		if workerID == "" {
			continue
		}
		if _, ok := seen[workerID]; ok {
			continue
		}
		seen[workerID] = struct{}{}
		workerIDs = append(workerIDs, workerID)
	}
	return workerIDs
}

func activeDNSWorkerIDs() []string {
	workers, err := model.ListDNSWorkers()
	if err != nil {
		return []string{}
	}
	workerIDs := make([]string, 0, len(workers))
	seen := make(map[string]struct{}, len(workers))
	for _, worker := range workers {
		if worker == nil {
			continue
		}
		workerID := strings.TrimSpace(worker.WorkerID)
		if workerID == "" {
			continue
		}
		if _, ok := seen[workerID]; ok {
			continue
		}
		seen[workerID] = struct{}{}
		workerIDs = append(workerIDs, workerID)
	}
	return workerIDs
}

func findDNSWorkerNodeProbeViewIndex(probes []DNSWorkerNodeProbeView, nodeID string) int {
	nodeID = strings.TrimSpace(nodeID)
	for index, probe := range probes {
		if strings.TrimSpace(probe.NodeID) == nodeID {
			return index
		}
	}
	return -1
}

func dnsWorkerProbeStatusSortRank(status string) int {
	switch status {
	case dnsWorkerProbeFailed, dnsWorkerProbePartial:
		return 0
	case dnsWorkerProbeStale:
		return 1
	case dnsWorkerProbeUnknown:
		return 2
	case dnsWorkerProbeHealthy:
		return 3
	default:
		return 4
	}
}

func buildDNSWorkerNodeProbeStatsByNode(now time.Time) map[string]*dnsWorkerNodeProbeStats {
	nodes, err := model.ListNodes()
	if err != nil {
		return map[string]*dnsWorkerNodeProbeStats{}
	}
	return buildDNSWorkerNodeProbeStatsByNodeForNodes(now, nodes)
}

func buildDNSWorkerNodeProbeStatsByNodeForNodes(now time.Time, nodes []*model.Node) map[string]*dnsWorkerNodeProbeStats {
	currentNodeIDs := currentSchedulableDNSProbeNodeIDsForNodes(nodes)
	nodeIDs := make([]string, 0, len(currentNodeIDs))
	for nodeID := range currentNodeIDs {
		nodeIDs = append(nodeIDs, nodeID)
	}
	probes, err := model.ListDNSWorkerNodeProbesByScope(activeDNSWorkerIDs(), nodeIDs)
	if err != nil || len(probes) == 0 {
		return map[string]*dnsWorkerNodeProbeStats{}
	}
	statsByNode := make(map[string]*dnsWorkerNodeProbeStats)
	for _, probe := range probes {
		if probe == nil {
			continue
		}
		nodeID := strings.TrimSpace(probe.NodeID)
		if nodeID == "" {
			continue
		}
		if _, ok := currentNodeIDs[nodeID]; !ok {
			continue
		}
		stats := statsByNode[nodeID]
		if stats == nil {
			stats = &dnsWorkerNodeProbeStats{}
			statsByNode[nodeID] = stats
		}
		probeState := evaluateDNSWorkerNodeProbeState(now, probe)
		stats.totalCount++
		switch probeState.status {
		case dnsWorkerProbeHealthy:
			stats.healthyCount++
		case dnsWorkerProbePartial:
			stats.partialCount++
		case dnsWorkerProbeFailed:
			stats.failedCount++
		case dnsWorkerProbeStale:
			stats.staleCount++
		case dnsWorkerProbeUnknown:
			stats.unknownCount++
		}
		averageRTTMs, maxRTTMs := normalizeDNSWorkerNodeProbeRTT(probe.AverageRTTMs, probe.MaxRTTMs)
		if probeState.status != dnsWorkerProbeStale && averageRTTMs > 0 {
			stats.totalAverageRTTMs += averageRTTMs
			stats.averageSamples++
		}
		if probeState.status != dnsWorkerProbeStale && maxRTTMs > stats.maxRTTMs {
			stats.maxRTTMs = maxRTTMs
		}
	}
	return statsByNode
}

func currentSchedulableDNSProbeNodeIDsForNodes(nodes []*model.Node) map[string]struct{} {
	result := make(map[string]struct{}, len(nodes))
	for _, node := range nodes {
		if !shouldExpectAgentDNSProbeForNode(node) {
			continue
		}
		nodeID := strings.TrimSpace(node.NodeID)
		if nodeID == "" {
			continue
		}
		result[nodeID] = struct{}{}
	}
	return result
}

func normalizeDNSWorkerNodeProbeRTT(averageRTTMs float64, maxRTTMs int64) (float64, int64) {
	if averageRTTMs < 0 {
		averageRTTMs = 0
	}
	if maxRTTMs < 0 {
		maxRTTMs = 0
	}
	if averageRTTMs > 0 && float64(maxRTTMs) < averageRTTMs {
		maxRTTMs = int64(math.Ceil(averageRTTMs))
	}
	return averageRTTMs, maxRTTMs
}

type dnsWorkerNodeProbeSummary struct {
	status         string
	message        string
	checkedCount   int
	healthyCount   int
	staleCount     int
	healthyPercent float64
	averageRTTMs   float64
	maxRTTMs       int64
}

func summarizeDNSWorkerNodeProbeStats(stats *dnsWorkerNodeProbeStats) dnsWorkerNodeProbeSummary {
	if stats == nil || stats.totalCount == 0 {
		return dnsWorkerNodeProbeSummary{
			status:  dnsWorkerProbeUnknown,
			message: "尚未收到该节点的 DNS Worker 多点探测结果",
		}
	}
	freshCount := stats.totalCount - stats.staleCount
	summary := dnsWorkerNodeProbeSummary{
		checkedCount:   stats.totalCount,
		healthyCount:   stats.healthyCount,
		staleCount:     stats.staleCount,
		healthyPercent: ratioPercent(int64(stats.healthyCount), int64(stats.totalCount)),
		averageRTTMs:   averageFloat(stats.totalAverageRTTMs, stats.averageSamples),
		maxRTTMs:       stats.maxRTTMs,
	}
	switch {
	case freshCount <= 0:
		summary.status = dnsWorkerProbeStale
		summary.message = "该节点的 DNS Worker 多点探测结果均已过期"
	case stats.healthyCount == freshCount:
		summary.status = dnsWorkerProbeHealthy
		summary.message = fmt.Sprintf("该节点到 DNS Worker 多点探测全部可达（%d/%d）", stats.healthyCount, freshCount)
	case stats.healthyCount > 0:
		summary.status = dnsWorkerProbePartial
		summary.message = fmt.Sprintf("该节点到 DNS Worker 多点探测部分可达（%d/%d）", stats.healthyCount, freshCount)
	case stats.partialCount > 0:
		summary.status = dnsWorkerProbePartial
		summary.message = fmt.Sprintf("该节点到 DNS Worker 多点探测 UDP/TCP 53 未同时可达（0/%d）", freshCount)
	case stats.failedCount > 0:
		summary.status = dnsWorkerProbeFailed
		summary.message = fmt.Sprintf("该节点到 DNS Worker 多点探测均失败（0/%d）", freshCount)
	default:
		summary.status = dnsWorkerProbeUnknown
		summary.message = "尚未收到该节点的 DNS Worker 多点探测结果"
	}
	if stats.staleCount > 0 && freshCount > 0 {
		summary.message += fmt.Sprintf("，另有 %d 个过期", stats.staleCount)
	}
	return summary
}

func dnsWorkerNodeProbeStatsSchedulable(stats *dnsWorkerNodeProbeStats) bool {
	if stats == nil {
		return false
	}
	return stats.healthyCount > 0 && stats.totalCount > stats.staleCount
}

func dnsWorkerNodeProbeThresholdReason(stats *dnsWorkerNodeProbeStats) string {
	summary := summarizeDNSWorkerNodeProbeStats(stats)
	switch summary.status {
	case dnsWorkerProbeStale:
		return "Agent 探测未达到调度门槛：探测结果已过期"
	case dnsWorkerProbePartial:
		return "Agent 探测未达到调度门槛：UDP/TCP 53 未同时可达"
	case dnsWorkerProbeFailed:
		return "Agent 探测未达到调度门槛：UDP/TCP 53 探测均失败"
	default:
		return "Agent 探测未达到调度门槛：尚未收到新鲜成功探测"
	}
}

func evaluateDNSWorkerNodeProbeState(now time.Time, probe *model.DNSWorkerNodeProbe) dnsWorkerProbeState {
	if probe == nil || probe.CheckedAt.IsZero() {
		return dnsWorkerProbeState{
			status:  dnsWorkerProbeUnknown,
			healthy: false,
			message: "尚未收到 Agent 多节点探测结果",
		}
	}
	checkedAt := normalizeDNSWorkerCheckedAt(&probe.CheckedAt, now, probe.UpdatedAt, probe.CreatedAt)
	age := now.Sub(checkedAt)
	ageSeconds := int64(0)
	if age > 0 {
		ageSeconds = int64(age.Seconds())
	}
	if age > defaultDNSWorkerNodeProbeMaxAge {
		return dnsWorkerProbeState{
			status:     dnsWorkerProbeStale,
			healthy:    false,
			ageSeconds: ageSeconds,
			message:    fmt.Sprintf("Agent 多节点探测结果超过 %s 未刷新", formatDNSWorkerNodeProbeMaxAge(defaultDNSWorkerNodeProbeMaxAge)),
		}
	}
	results := decodeDNSWorkerProbeResults(probe.ResultsJSON)
	if probe.Healthy {
		return dnsWorkerProbeState{
			status:     dnsWorkerProbeHealthy,
			healthy:    true,
			ageSeconds: ageSeconds,
			message:    "UDP/TCP 53 均可达",
		}
	}
	for _, result := range results {
		if result.Reachable {
			return dnsWorkerProbeState{
				status:     dnsWorkerProbePartial,
				healthy:    false,
				ageSeconds: ageSeconds,
				message:    strings.TrimSpace(firstNonEmpty(probe.LastError, "UDP/TCP 53 未同时可达")),
			}
		}
	}
	return dnsWorkerProbeState{
		status:     dnsWorkerProbeFailed,
		healthy:    false,
		ageSeconds: ageSeconds,
		message:    strings.TrimSpace(firstNonEmpty(probe.LastError, "Agent 多节点探测失败")),
	}
}

func formatDNSWorkerNodeProbeMaxAge(value time.Duration) string {
	if value <= 0 {
		return "0 秒"
	}
	if value < time.Minute {
		return fmt.Sprintf("%d 秒", int(value.Seconds()))
	}
	if value < time.Hour {
		return fmt.Sprintf("%d 分钟", int(value.Minutes()))
	}
	return fmt.Sprintf("%d 小时", int(value.Hours()))
}
