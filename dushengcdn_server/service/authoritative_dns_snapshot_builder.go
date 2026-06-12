package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"dushengcdn/common"
	"dushengcdn/internal/dnsworker"
	"dushengcdn/model"
	"dushengcdn/utils/geoip/iputil"
)

func buildAuthoritativeDNSSnapshotWithQueries(worker *model.DNSWorker, schedulingQueries gslbDNSSchedulingDataQueries) (*AuthoritativeDNSSnapshot, error) {
	zones, err := snapshotDNSZones()
	if err != nil {
		return nil, err
	}
	schedulingOptions := authoritativeDNSSchedulingOptions()
	schedulingData, err := loadGSLBDNSSchedulingDataWithQueries(true, schedulingQueries)
	if err != nil {
		return nil, err
	}
	schedulingOptions.Data = schedulingData
	routes, err := snapshotAuthoritativeRoutesWithOptions(schedulingOptions)
	if err != nil {
		return nil, err
	}
	nodes := snapshotNodesWithData(schedulingData)
	schedulingStates, err := snapshotGSLBSchedulingStatesWithData(routes, schedulingData)
	if err != nil {
		return nil, err
	}
	snapshot := &AuthoritativeDNSSnapshot{
		GeneratedAt:                time.Now().UTC(),
		GSLBProbeSchedulingEnabled: common.GSLBProbeSchedulingEnabled,
		WorkerPolicy:               authoritativeDNSWorkerPolicy(),
		Zones:                      zones,
		Routes:                     routes,
		Nodes:                      nodes,
		SchedulingStates:           schedulingStates,
	}
	if worker != nil {
		if err := filterAuthoritativeDNSSnapshotForWorker(snapshot, worker); err != nil {
			return nil, err
		}
	}
	version, err := authoritativeDNSSnapshotVersion(snapshot)
	if err != nil {
		return nil, err
	}
	snapshot.SnapshotVersion = version
	if worker != nil {
		_ = recordDNSWorkerSnapshotPull(worker, version)
	}
	return snapshot, nil
}

func filterAuthoritativeDNSSnapshotForWorker(snapshot *AuthoritativeDNSSnapshot, worker *model.DNSWorker) error {
	if snapshot == nil || worker == nil || worker.ID == 0 || len(snapshot.Zones) == 0 {
		return nil
	}
	zoneIDs := make([]uint, 0, len(snapshot.Zones))
	for _, zone := range snapshot.Zones {
		if zone.ID != 0 {
			zoneIDs = append(zoneIDs, zone.ID)
		}
	}
	assignments, err := model.ListDNSZoneWorkerAssignmentsByZoneIDs(zoneIDs)
	if err != nil {
		return err
	}
	assignedByZone := make(map[uint]map[uint]struct{}, len(assignments))
	for _, assignment := range assignments {
		if assignment == nil || assignment.ZoneID == 0 || assignment.WorkerID == 0 {
			continue
		}
		if _, ok := assignedByZone[assignment.ZoneID]; !ok {
			assignedByZone[assignment.ZoneID] = map[uint]struct{}{}
		}
		assignedByZone[assignment.ZoneID][assignment.WorkerID] = struct{}{}
	}
	allowUnassignedZones := common.AuthoritativeDNSWorkerAllowUnassignedZones || len(assignments) == 0
	allowedZones := map[uint]struct{}{}
	filteredZones := make([]AuthoritativeDNSSnapshotZone, 0, len(snapshot.Zones))
	for _, zone := range snapshot.Zones {
		assignmentsForZone := assignedByZone[zone.ID]
		if len(assignmentsForZone) > 0 {
			if _, ok := assignmentsForZone[worker.ID]; !ok {
				continue
			}
		} else if !allowUnassignedZones {
			continue
		}
		allowedZones[zone.ID] = struct{}{}
		filteredZones = append(filteredZones, zone)
	}
	filteredRoutes := make([]AuthoritativeDNSSnapshotRoute, 0, len(snapshot.Routes))
	allowedRouteIDs := map[uint]struct{}{}
	for _, route := range snapshot.Routes {
		if _, ok := allowedZones[route.ZoneID]; !ok {
			continue
		}
		filteredRoutes = append(filteredRoutes, route)
		allowedRouteIDs[route.ID] = struct{}{}
	}
	filteredStates := make([]AuthoritativeDNSSnapshotSchedulingState, 0, len(snapshot.SchedulingStates))
	for _, state := range snapshot.SchedulingStates {
		if _, ok := allowedRouteIDs[state.RouteID]; ok {
			filteredStates = append(filteredStates, state)
		}
	}
	snapshot.Nodes = filterAuthoritativeDNSSnapshotNodesForRoutes(snapshot.Nodes, filteredRoutes)
	snapshot.Zones = filteredZones
	snapshot.Routes = filteredRoutes
	snapshot.SchedulingStates = filteredStates
	return nil
}

func filterAuthoritativeDNSSnapshotNodesForRoutes(nodes []AuthoritativeDNSSnapshotNode, routes []AuthoritativeDNSSnapshotRoute) []AuthoritativeDNSSnapshotNode {
	if len(nodes) == 0 || len(routes) == 0 {
		return []AuthoritativeDNSSnapshotNode{}
	}
	allowedPools := map[string]struct{}{}
	allowedNodeIDs := map[string]struct{}{}
	for _, route := range routes {
		for _, pool := range authoritativeDNSRouteCandidatePools(route) {
			if pool != "" {
				allowedPools[pool] = struct{}{}
			}
		}
		for _, nodeID := range authoritativeDNSRouteCandidateNodeIDs(route) {
			if nodeID != "" {
				allowedNodeIDs[nodeID] = struct{}{}
			}
		}
	}
	if len(allowedPools) == 0 && len(allowedNodeIDs) == 0 {
		return []AuthoritativeDNSSnapshotNode{}
	}
	filtered := make([]AuthoritativeDNSSnapshotNode, 0, len(nodes))
	for _, node := range nodes {
		if _, ok := allowedNodeIDs[strings.TrimSpace(node.NodeID)]; ok {
			filtered = append(filtered, node)
			continue
		}
		if _, ok := allowedPools[normalizeNodePoolName(node.PoolName)]; ok {
			filtered = append(filtered, node)
		}
	}
	return filtered
}

func authoritativeDNSRouteCandidatePools(route AuthoritativeDNSSnapshotRoute) []string {
	pools := map[string]struct{}{}
	if route.DDOSActive && route.DDOSTarget != "" {
		pools[normalizeNodePoolName(route.DDOSTarget)] = struct{}{}
	} else if route.NodePool != "" {
		pools[normalizeNodePoolName(route.NodePool)] = struct{}{}
	}
	if route.GSLBEnabled {
		for _, pool := range route.GSLBPolicy.Pools {
			if !pool.Enabled {
				continue
			}
			name := normalizeNodePoolName(pool.Name)
			if name != "" {
				pools[name] = struct{}{}
			}
		}
	}
	result := make([]string, 0, len(pools))
	for pool := range pools {
		result = append(result, pool)
	}
	sort.Strings(result)
	return result
}

func authoritativeDNSRouteCandidateNodeIDs(route AuthoritativeDNSSnapshotRoute) []string {
	nodeIDs := map[string]struct{}{}
	if route.GSLBEnabled {
		for _, pool := range route.GSLBPolicy.Pools {
			for _, nodeID := range pool.NodeIDs {
				if trimmed := strings.TrimSpace(nodeID); trimmed != "" {
					nodeIDs[trimmed] = struct{}{}
				}
			}
		}
	}
	result := make([]string, 0, len(nodeIDs))
	for nodeID := range nodeIDs {
		result = append(result, nodeID)
	}
	sort.Strings(result)
	return result
}

func snapshotDNSZones() ([]AuthoritativeDNSSnapshotZone, error) {
	return snapshotDNSZonesWithQueries(defaultAuthoritativeDNSSnapshotZoneQueries)
}

type authoritativeDNSSnapshotZoneQueries struct {
	ListDNSRecordsByZoneIDs func([]uint) ([]*model.DNSRecord, error)
	ListDNSSECKeysByZoneIDs func([]uint) ([]*model.DNSSECKey, error)
}

var defaultAuthoritativeDNSSnapshotZoneQueries = authoritativeDNSSnapshotZoneQueries{
	ListDNSRecordsByZoneIDs: model.ListDNSRecordsByZoneIDs,
	ListDNSSECKeysByZoneIDs: model.ListDNSSECKeysByZoneIDs,
}

func snapshotDNSZonesWithQueries(queries authoritativeDNSSnapshotZoneQueries) ([]AuthoritativeDNSSnapshotZone, error) {
	var zones []*model.DNSZone
	if err := model.DB.Where("enabled = ?", true).Order("name asc").Find(&zones).Error; err != nil {
		return nil, err
	}
	zoneIDs := make([]uint, 0, len(zones))
	for _, zone := range zones {
		if zone != nil && zone.ID != 0 {
			zoneIDs = append(zoneIDs, zone.ID)
		}
	}
	listRecordsByZoneIDs := queries.ListDNSRecordsByZoneIDs
	if listRecordsByZoneIDs == nil {
		listRecordsByZoneIDs = model.ListDNSRecordsByZoneIDs
	}
	records, err := listRecordsByZoneIDs(zoneIDs)
	if err != nil {
		return nil, err
	}
	recordsByZoneID := make(map[uint][]*model.DNSRecord, len(zones))
	for _, record := range records {
		if record == nil {
			continue
		}
		recordsByZoneID[record.ZoneID] = append(recordsByZoneID[record.ZoneID], record)
	}
	listDNSSECKeysByZoneIDs := queries.ListDNSSECKeysByZoneIDs
	if listDNSSECKeysByZoneIDs == nil {
		listDNSSECKeysByZoneIDs = model.ListDNSSECKeysByZoneIDs
	}
	dnssecKeys, err := listDNSSECKeysByZoneIDs(zoneIDs)
	if err != nil {
		return nil, err
	}
	dnssecKeysByZoneID := make(map[uint][]*model.DNSSECKey, len(zones))
	for _, key := range dnssecKeys {
		if key == nil {
			continue
		}
		dnssecKeysByZoneID[key.ZoneID] = append(dnssecKeysByZoneID[key.ZoneID], key)
	}
	result := make([]AuthoritativeDNSSnapshotZone, 0, len(zones))
	for _, zone := range zones {
		records := recordsByZoneID[zone.ID]
		item := AuthoritativeDNSSnapshotZone{
			ID:          zone.ID,
			Name:        zone.Name,
			SOAEmail:    zone.SOAEmail,
			PrimaryNS:   zone.PrimaryNS,
			NameServers: decodeStoredStringList(zone.NameServers),
			DefaultTTL:  normalizeDNSZoneTTL(zone.DefaultTTL),
			Serial:      zone.Serial,
			DNSSEC: AuthoritativeDNSSnapshotDNSSECPolicy{
				Enabled:                  zone.DNSSECEnabled,
				DenialMode:               normalizeDNSSECDenialMode(zone.DNSSECDenialMode),
				NSEC3Salt:                strings.TrimSpace(zone.DNSSECNSEC3Salt),
				NSEC3Iterations:          normalizeDNSSECNSEC3Iterations(zone.DNSSECNSEC3Iterations),
				SignatureValiditySeconds: normalizeDNSSECSignatureValidity(zone.DNSSECSignatureValidity),
			},
			DNSSECKeys: make([]AuthoritativeDNSSnapshotDNSSECKey, 0, len(dnssecKeysByZoneID[zone.ID])),
			Records:    make([]AuthoritativeDNSSnapshotRecord, 0, len(records)),
		}
		for _, key := range dnssecKeysByZoneID[zone.ID] {
			if key == nil || normalizeDNSSECKeyStatus(key.Status) != dnssecKeyStatusActive {
				continue
			}
			item.DNSSECKeys = append(item.DNSSECKeys, AuthoritativeDNSSnapshotDNSSECKey{
				ID:                  key.ID,
				Role:                normalizeDNSSECKeyRole(key.Role),
				Flags:               key.Flags,
				Algorithm:           key.Algorithm,
				PublicKey:           key.PublicKey,
				EncryptedPrivateKey: key.EncryptedPrivateKey,
				KeyTag:              key.KeyTag,
				Status:              normalizeDNSSECKeyStatus(key.Status),
			})
		}
		for _, record := range records {
			if record == nil || !record.Enabled {
				continue
			}
			item.Records = append(item.Records, AuthoritativeDNSSnapshotRecord{
				ID:       record.ID,
				Name:     record.Name,
				Type:     record.Type,
				Value:    record.Value,
				TTL:      normalizeAuthoritativeTTL(record.TTL, item.DefaultTTL),
				Priority: record.Priority,
			})
		}
		result = append(result, item)
	}
	return result, nil
}

func snapshotAuthoritativeRoutes() ([]AuthoritativeDNSSnapshotRoute, error) {
	return snapshotAuthoritativeRoutesWithOptions(authoritativeDNSSchedulingOptions())
}

func snapshotAuthoritativeRoutesWithOptions(schedulingOptions gslbDNSSchedulingOptions) ([]AuthoritativeDNSSnapshotRoute, error) {
	return snapshotAuthoritativeRoutesWithQueries(schedulingOptions, defaultAuthoritativeDNSSnapshotRouteQueries)
}

type authoritativeDNSSnapshotRouteQueries struct {
	ListDNSZonesByIDs func([]uint) ([]*model.DNSZone, error)
}

var defaultAuthoritativeDNSSnapshotRouteQueries = authoritativeDNSSnapshotRouteQueries{
	ListDNSZonesByIDs: model.ListDNSZonesByIDs,
}

func snapshotAuthoritativeRoutesWithQueries(schedulingOptions gslbDNSSchedulingOptions, queries authoritativeDNSSnapshotRouteQueries) ([]AuthoritativeDNSSnapshotRoute, error) {
	var routes []*model.ProxyRoute
	if err := model.DB.
		Where("enabled = ? AND dns_provider_mode = ? AND dns_zone_id_ref IS NOT NULL", true, DNSProviderModeAuthoritative).
		Order("site_name asc").
		Find(&routes).Error; err != nil {
		return nil, err
	}
	zoneIDs := make([]uint, 0, len(routes))
	seenZoneIDs := make(map[uint]struct{}, len(routes))
	for _, route := range routes {
		if route == nil || route.DNSZoneIDRef == nil || *route.DNSZoneIDRef == 0 {
			continue
		}
		zoneID := *route.DNSZoneIDRef
		if _, ok := seenZoneIDs[zoneID]; ok {
			continue
		}
		seenZoneIDs[zoneID] = struct{}{}
		zoneIDs = append(zoneIDs, zoneID)
	}
	listZonesByIDs := queries.ListDNSZonesByIDs
	if listZonesByIDs == nil {
		listZonesByIDs = model.ListDNSZonesByIDs
	}
	zones, err := listZonesByIDs(zoneIDs)
	if err != nil {
		return nil, err
	}
	zonesByID := make(map[uint]*model.DNSZone, len(zones))
	for _, zone := range zones {
		if zone == nil || zone.ID == 0 {
			continue
		}
		zonesByID[zone.ID] = zone
	}
	result := make([]AuthoritativeDNSSnapshotRoute, 0, len(routes))
	for _, route := range routes {
		if route == nil || route.DNSZoneIDRef == nil || *route.DNSZoneIDRef == 0 {
			continue
		}
		zone := zonesByID[*route.DNSZoneIDRef]
		if zone == nil || !zone.Enabled {
			continue
		}
		domains, err := decodeStoredDomains(route.Domains, route.Domain)
		if err != nil {
			return nil, err
		}
		recordType := normalizeDNSRecordType(route.DNSRecordType)
		policy := defaultGSLBPolicy(route.NodePool, route.DNSTargetCount, route.DNSScheduleMode, route.DNSTTL)
		if route.GSLBEnabled {
			policy, err = decodeStoredGSLBPolicy(route.GSLBPolicy)
			if err != nil {
				return nil, err
			}
		}
		ddosActive := routeDDOSProtectionActive(route) &&
			normalizeDDOSProtectionProvider(route.DDOSProtectionProvider) == DDOSProtectionProviderCustom
		effectiveGSLBEnabled := route.GSLBEnabled && !ddosActive
		item := AuthoritativeDNSSnapshotRoute{
			ID:           route.ID,
			SiteName:     normalizeProxyRouteSiteNameInput(route, route.SiteName, route.Domain),
			Domains:      domains,
			ZoneID:       *route.DNSZoneIDRef,
			NodePool:     normalizeNodePoolName(route.NodePool),
			RecordType:   recordType,
			TargetCount:  normalizeDNSTargetCount(route.DNSTargetCount),
			ScheduleMode: normalizeDNSScheduleMode(route.DNSScheduleMode),
			TTL:          normalizeAuthoritativeRouteTTL(route.DNSTTL),
			GSLBEnabled:  effectiveGSLBEnabled,
			GSLBPolicy:   policy,
			DDOSActive:   ddosActive,
			DDOSProvider: normalizeDDOSProtectionProvider(route.DDOSProtectionProvider),
			DDOSTarget:   normalizeNodePoolName(route.DDOSProtectionTarget),
		}
		selectRoute := route
		if ddosActive {
			selectRouteCopy := *route
			selectRouteCopy.GSLBEnabled = false
			selectRouteCopy.NodePool = normalizeNodePoolName(route.DDOSProtectionTarget)
			selectRoute = &selectRouteCopy
		}
		selection, selectErr := selectProxyRouteDNSTargetsWithOptions(selectRoute, recordType, schedulingOptions)
		if selectErr != nil {
			item.TargetError = selectErr.Error()
		} else {
			item.CurrentTargets = selection.Targets
			item.TTL = normalizeAuthoritativeRouteTTL(selection.TTL)
		}
		result = append(result, item)
	}
	return result, nil
}

func snapshotNodesWithData(data *gslbDNSSchedulingData) []AuthoritativeDNSSnapshotNode {
	if data == nil {
		data = &gslbDNSSchedulingData{}
	}
	nodes := data.Nodes
	metrics := data.MetricsByNode
	probeStatsByNode := map[string]*dnsWorkerNodeProbeStats{}
	if data.ProbeStatsByNode != nil {
		probeStatsByNode = data.ProbeStatsByNode
	}
	result := make([]AuthoritativeDNSSnapshotNode, 0, len(nodes))
	for _, node := range nodes {
		if node == nil {
			continue
		}
		item := AuthoritativeDNSSnapshotNode{
			NodeID:            node.NodeID,
			Name:              node.Name,
			PoolName:          normalizeNodePoolName(node.PoolName),
			PublicIPs:         publicNodeIPsForSnapshot(node),
			Weight:            normalizeNodeWeight(node.Weight),
			SchedulingEnabled: isNodeSchedulableForDNS(node),
			DrainMode:         node.DrainMode,
			Status:            computeNodeStatus(node),
			OpenrestyStatus:   normalizeOpenrestyStatus(node.OpenrestyStatus),
			LastSeenAt:        node.LastSeenAt,
		}
		if metric := metrics[node.NodeID]; metric != nil {
			capturedAt := metric.CapturedAt
			item.OpenrestyConnections = metric.OpenrestyConnections
			item.CPUUsagePercent = metric.CPUUsagePercent
			item.MemoryUsagePercent = nodeMetricMemoryUsagePercent(metric)
			item.MetricCapturedAt = &capturedAt
		}
		if probeStats := probeStatsByNode[node.NodeID]; probeStats != nil {
			item.DNSProbeHealthy = dnsWorkerNodeProbeStatsSchedulable(probeStats)
			item.DNSProbeCheckedCount = probeStats.totalCount
			item.DNSProbeHealthyCount = probeStats.healthyCount
			item.DNSProbeStaleCount = probeStats.staleCount
			item.DNSProbeAverageRTTMs = averageFloat(probeStats.totalAverageRTTMs, probeStats.averageSamples)
			item.DNSProbeMaxRTTMs = probeStats.maxRTTMs
		}
		result = append(result, item)
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].PoolName != result[j].PoolName {
			return result[i].PoolName < result[j].PoolName
		}
		return result[i].NodeID < result[j].NodeID
	})
	return result
}

func snapshotGSLBSchedulingStatesWithData(routes []AuthoritativeDNSSnapshotRoute, data *gslbDNSSchedulingData) ([]AuthoritativeDNSSnapshotSchedulingState, error) {
	routeIDs := make([]uint, 0, len(routes))
	routeRecordTypes := make(map[uint]string, len(routes))
	routeIDSet := make(map[uint]struct{}, len(routes))
	for _, route := range routes {
		if route.ID == 0 {
			continue
		}
		recordType := normalizeDNSRecordType(route.RecordType)
		if recordType != "A" && recordType != "AAAA" {
			continue
		}
		routeIDs = append(routeIDs, route.ID)
		routeIDSet[route.ID] = struct{}{}
		routeRecordTypes[route.ID] = recordType
	}
	if len(routeIDs) == 0 {
		return []AuthoritativeDNSSnapshotSchedulingState{}, nil
	}
	states := make([]*model.GSLBSchedulingState, 0)
	if data != nil && data.SchedulingStatesLoaded {
		states = make([]*model.GSLBSchedulingState, 0, len(data.SchedulingStates))
		for key, state := range data.SchedulingStates {
			if state == nil {
				continue
			}
			if _, ok := routeIDSet[key.routeID]; !ok {
				continue
			}
			states = append(states, state)
		}
		sort.SliceStable(states, func(i, j int) bool {
			if states[i].ProxyRouteID != states[j].ProxyRouteID {
				return states[i].ProxyRouteID < states[j].ProxyRouteID
			}
			if states[i].DNSRecordType != states[j].DNSRecordType {
				return states[i].DNSRecordType < states[j].DNSRecordType
			}
			return states[i].ScopeKey < states[j].ScopeKey
		})
	} else {
		if err := model.DB.
			Where("proxy_route_id IN ?", routeIDs).
			Order("proxy_route_id asc, dns_record_type asc, scope_key asc").
			Find(&states).Error; err != nil {
			return nil, err
		}
	}
	now := time.Now().UTC()
	result := make([]AuthoritativeDNSSnapshotSchedulingState, 0, len(states))
	for _, state := range states {
		if state == nil || state.ProxyRouteID == 0 {
			continue
		}
		expectedType, ok := routeRecordTypes[state.ProxyRouteID]
		if !ok {
			continue
		}
		recordType := normalizeDNSRecordType(state.DNSRecordType)
		if recordType != expectedType {
			continue
		}
		selectedTargets, err := normalizeDNSRecordContents(recordType, decodeGSLBTargetList(state.SelectedTargets))
		if err != nil || len(selectedTargets) == 0 {
			continue
		}
		desiredTargets := decodeGSLBTargetList(state.DesiredTargets)
		if len(desiredTargets) > 0 {
			desiredTargets, err = normalizeDNSRecordContents(recordType, desiredTargets)
			if err != nil {
				desiredTargets = []string{}
			}
		}
		result = append(result, AuthoritativeDNSSnapshotSchedulingState{
			RouteID:         state.ProxyRouteID,
			RecordType:      recordType,
			ScopeKey:        normalizeDNSSourceScope(state.ScopeKey),
			SelectedTargets: selectedTargets,
			DesiredTargets:  desiredTargets,
			UnhealthyCount:  normalizeDebounceCounter(state.UnhealthyCount),
			RecoveryCount:   normalizeDebounceCounter(state.RecoveryCount),
			LastChangedAt:   normalizeGSLBSchedulingStateChangedAt(state.LastChangedAt, now, state.LastEvaluatedAt, &state.UpdatedAt, &state.CreatedAt),
		})
	}
	return result, nil
}

func convertAuthoritativeSnapshotToWorker(snapshot *AuthoritativeDNSSnapshot) *dnsworker.Snapshot {
	if snapshot == nil {
		return nil
	}
	result := &dnsworker.Snapshot{
		SnapshotVersion:            snapshot.SnapshotVersion,
		GeneratedAt:                snapshot.GeneratedAt,
		GSLBProbeSchedulingEnabled: snapshot.GSLBProbeSchedulingEnabled,
		WorkerPolicy:               snapshot.WorkerPolicy,
		Zones:                      make([]dnsworker.SnapshotZone, 0, len(snapshot.Zones)),
		Routes:                     make([]dnsworker.SnapshotRoute, 0, len(snapshot.Routes)),
		Nodes:                      make([]dnsworker.SnapshotNode, 0, len(snapshot.Nodes)),
		SchedulingStates:           make([]dnsworker.SnapshotSchedulingState, 0, len(snapshot.SchedulingStates)),
	}
	for _, zone := range snapshot.Zones {
		item := dnsworker.SnapshotZone{
			ID:          zone.ID,
			Name:        zone.Name,
			SOAEmail:    zone.SOAEmail,
			PrimaryNS:   zone.PrimaryNS,
			NameServers: append([]string(nil), zone.NameServers...),
			DefaultTTL:  zone.DefaultTTL,
			Serial:      zone.Serial,
			DNSSEC: dnsworker.SnapshotDNSSECPolicy{
				Enabled:                  zone.DNSSEC.Enabled,
				DenialMode:               zone.DNSSEC.DenialMode,
				NSEC3Salt:                zone.DNSSEC.NSEC3Salt,
				NSEC3Iterations:          zone.DNSSEC.NSEC3Iterations,
				SignatureValiditySeconds: zone.DNSSEC.SignatureValiditySeconds,
			},
			DNSSECKeys: make([]dnsworker.SnapshotDNSSECKey, 0, len(zone.DNSSECKeys)),
			Records:    make([]dnsworker.SnapshotRecord, 0, len(zone.Records)),
		}
		for _, key := range zone.DNSSECKeys {
			item.DNSSECKeys = append(item.DNSSECKeys, dnsworker.SnapshotDNSSECKey{
				ID:                  key.ID,
				Role:                key.Role,
				Flags:               key.Flags,
				Algorithm:           key.Algorithm,
				PublicKey:           key.PublicKey,
				EncryptedPrivateKey: key.EncryptedPrivateKey,
				KeyTag:              key.KeyTag,
				Status:              key.Status,
			})
		}
		for _, record := range zone.Records {
			item.Records = append(item.Records, dnsworker.SnapshotRecord{
				ID:       record.ID,
				Name:     record.Name,
				Type:     record.Type,
				Value:    record.Value,
				TTL:      record.TTL,
				Priority: record.Priority,
			})
		}
		result.Zones = append(result.Zones, item)
	}
	for _, route := range snapshot.Routes {
		result.Routes = append(result.Routes, dnsworker.SnapshotRoute{
			ID:             route.ID,
			SiteName:       route.SiteName,
			Domains:        append([]string(nil), route.Domains...),
			ZoneID:         route.ZoneID,
			NodePool:       route.NodePool,
			RecordType:     route.RecordType,
			TargetCount:    route.TargetCount,
			ScheduleMode:   route.ScheduleMode,
			TTL:            route.TTL,
			GSLBEnabled:    route.GSLBEnabled,
			GSLBPolicy:     convertAuthoritativeGSLBPolicyToWorker(route.GSLBPolicy),
			CurrentTargets: append([]string(nil), route.CurrentTargets...),
			TargetError:    route.TargetError,
			DDOSActive:     route.DDOSActive,
			DDOSProvider:   route.DDOSProvider,
			DDOSTarget:     route.DDOSTarget,
		})
	}
	for _, node := range snapshot.Nodes {
		result.Nodes = append(result.Nodes, dnsworker.SnapshotNode{
			NodeID:               node.NodeID,
			Name:                 node.Name,
			PoolName:             node.PoolName,
			PublicIPs:            append([]string(nil), node.PublicIPs...),
			Weight:               node.Weight,
			SchedulingEnabled:    node.SchedulingEnabled,
			DrainMode:            node.DrainMode,
			Status:               node.Status,
			OpenrestyStatus:      node.OpenrestyStatus,
			LastSeenAt:           node.LastSeenAt,
			OpenrestyConnections: node.OpenrestyConnections,
			CPUUsagePercent:      node.CPUUsagePercent,
			MemoryUsagePercent:   node.MemoryUsagePercent,
			MetricCapturedAt:     node.MetricCapturedAt,
			DNSProbeHealthy:      node.DNSProbeHealthy,
			DNSProbeCheckedCount: node.DNSProbeCheckedCount,
			DNSProbeHealthyCount: node.DNSProbeHealthyCount,
			DNSProbeStaleCount:   node.DNSProbeStaleCount,
			DNSProbeAverageRTTMs: node.DNSProbeAverageRTTMs,
			DNSProbeMaxRTTMs:     node.DNSProbeMaxRTTMs,
		})
	}
	for _, state := range snapshot.SchedulingStates {
		result.SchedulingStates = append(result.SchedulingStates, dnsworker.SnapshotSchedulingState{
			RouteID:         state.RouteID,
			RecordType:      state.RecordType,
			ScopeKey:        state.ScopeKey,
			SelectedTargets: append([]string(nil), state.SelectedTargets...),
			DesiredTargets:  append([]string(nil), state.DesiredTargets...),
			UnhealthyCount:  state.UnhealthyCount,
			RecoveryCount:   state.RecoveryCount,
			LastChangedAt:   state.LastChangedAt,
		})
	}
	return result
}

func convertAuthoritativeGSLBPolicyToWorker(policy ProxyRouteGSLBPolicy) dnsworker.GSLBPolicy {
	result := dnsworker.GSLBPolicy{
		Mode:                   policy.Mode,
		Strategy:               policy.Strategy,
		PoolMatchMode:          policy.PoolMatchMode,
		TargetCount:            policy.TargetCount,
		TTL:                    policy.TTL,
		SourcePoolFallbackMode: policy.SourcePoolFallbackMode,
		SourceIP: dnsworker.GSLBSourceIPProvider{
			Provider: policy.SourceIP.Provider,
			APIURL:   policy.SourceIP.APIURL,
			APIToken: policy.SourceIP.APIToken,
		},
		LoadThresholds: dnsworker.GSLBLoadThresholds{
			MaxOpenrestyConnections: policy.LoadThresholds.MaxOpenrestyConnections,
			MaxCPUPercent:           policy.LoadThresholds.MaxCPUPercent,
			MaxMemoryPercent:        policy.LoadThresholds.MaxMemoryPercent,
		},
		Debounce: dnsworker.GSLBDebounce{
			CooldownSeconds:    policy.Debounce.CooldownSeconds,
			UnhealthyThreshold: policy.Debounce.UnhealthyThreshold,
			RecoveryThreshold:  policy.Debounce.RecoveryThreshold,
		},
		Pools: make([]dnsworker.GSLBPoolPolicy, 0, len(policy.Pools)),
	}
	for _, pool := range policy.Pools {
		result.Pools = append(result.Pools, dnsworker.GSLBPoolPolicy{
			Name:               pool.Name,
			Weight:             pool.Weight,
			Countries:          append([]string(nil), pool.Countries...),
			SourceCIDRs:        append([]string(nil), pool.SourceCIDRs...),
			Operators:          append([]string(nil), pool.Operators...),
			ASNs:               append([]uint32(nil), pool.ASNs...),
			ExcludeCountries:   append([]string(nil), pool.ExcludeCountries...),
			ExcludeSourceCIDRs: append([]string(nil), pool.ExcludeSourceCIDRs...),
			ExcludeOperators:   append([]string(nil), pool.ExcludeOperators...),
			ExcludeASNs:        append([]uint32(nil), pool.ExcludeASNs...),
			NodeIDs:            append([]string(nil), pool.NodeIDs...),
			Enabled:            pool.Enabled,
		})
	}
	return result
}

func authoritativeDNSSnapshotVersion(snapshot *AuthoritativeDNSSnapshot) (string, error) {
	payload := struct {
		GSLBProbeSchedulingEnabled bool                                   `json:"gslb_probe_scheduling_enabled"`
		WorkerPolicy               dnsworker.WorkerPolicy                 `json:"worker_policy"`
		Zones                      []AuthoritativeDNSSnapshotZone         `json:"zones"`
		Routes                     []authoritativeDNSSnapshotVersionRoute `json:"routes"`
		Nodes                      []authoritativeDNSSnapshotVersionNode  `json:"nodes"`
	}{
		GSLBProbeSchedulingEnabled: snapshot.GSLBProbeSchedulingEnabled,
		WorkerPolicy:               snapshot.WorkerPolicy,
		Zones:                      snapshot.Zones,
		Routes:                     authoritativeDNSSnapshotVersionRoutes(snapshot.Routes),
		Nodes:                      authoritativeDNSSnapshotVersionNodes(snapshot.Nodes),
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])[:24], nil
}

type authoritativeDNSSnapshotVersionRoute struct {
	ID             uint                 `json:"id"`
	SiteName       string               `json:"site_name"`
	Domains        []string             `json:"domains"`
	ZoneID         uint                 `json:"zone_id"`
	NodePool       string               `json:"node_pool"`
	RecordType     string               `json:"record_type"`
	TargetCount    int                  `json:"target_count"`
	ScheduleMode   string               `json:"schedule_mode"`
	TTL            int                  `json:"ttl"`
	GSLBEnabled    bool                 `json:"gslb_enabled"`
	GSLBPolicy     ProxyRouteGSLBPolicy `json:"gslb_policy"`
	CurrentTargets []string             `json:"current_targets,omitempty"`
	DDOSActive     bool                 `json:"ddos_active,omitempty"`
	DDOSProvider   string               `json:"ddos_provider,omitempty"`
	DDOSTarget     string               `json:"ddos_target,omitempty"`
}

func authoritativeDNSSnapshotVersionRoutes(routes []AuthoritativeDNSSnapshotRoute) []authoritativeDNSSnapshotVersionRoute {
	result := make([]authoritativeDNSSnapshotVersionRoute, 0, len(routes))
	for _, route := range routes {
		item := authoritativeDNSSnapshotVersionRoute{
			ID:           route.ID,
			SiteName:     route.SiteName,
			Domains:      append([]string(nil), route.Domains...),
			ZoneID:       route.ZoneID,
			NodePool:     route.NodePool,
			RecordType:   normalizeDNSRecordType(route.RecordType),
			TargetCount:  route.TargetCount,
			ScheduleMode: route.ScheduleMode,
			TTL:          route.TTL,
			GSLBEnabled:  route.GSLBEnabled,
			GSLBPolicy:   route.GSLBPolicy,
			DDOSActive:   route.DDOSActive,
			DDOSProvider: route.DDOSProvider,
			DDOSTarget:   route.DDOSTarget,
		}
		if !route.GSLBEnabled {
			item.CurrentTargets = append([]string(nil), route.CurrentTargets...)
			sort.Strings(item.CurrentTargets)
		}
		result = append(result, item)
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].ZoneID != result[j].ZoneID {
			return result[i].ZoneID < result[j].ZoneID
		}
		if result[i].SiteName != result[j].SiteName {
			return result[i].SiteName < result[j].SiteName
		}
		return result[i].ID < result[j].ID
	})
	return result
}

type authoritativeDNSSnapshotVersionNode struct {
	NodeID            string   `json:"node_id"`
	Name              string   `json:"name"`
	PoolName          string   `json:"pool_name"`
	PublicIPs         []string `json:"public_ips"`
	Weight            int      `json:"weight"`
	SchedulingEnabled bool     `json:"scheduling_enabled"`
	DrainMode         bool     `json:"drain_mode"`
	Status            string   `json:"status"`
	OpenrestyStatus   string   `json:"openresty_status"`
	DNSProbeHealthy   bool     `json:"dns_probe_healthy"`
}

func authoritativeDNSSnapshotVersionNodes(nodes []AuthoritativeDNSSnapshotNode) []authoritativeDNSSnapshotVersionNode {
	result := make([]authoritativeDNSSnapshotVersionNode, 0, len(nodes))
	for _, node := range nodes {
		result = append(result, authoritativeDNSSnapshotVersionNode{
			NodeID:            node.NodeID,
			Name:              node.Name,
			PoolName:          node.PoolName,
			PublicIPs:         append([]string(nil), node.PublicIPs...),
			Weight:            node.Weight,
			SchedulingEnabled: node.SchedulingEnabled,
			DrainMode:         node.DrainMode,
			Status:            node.Status,
			OpenrestyStatus:   node.OpenrestyStatus,
			DNSProbeHealthy:   node.DNSProbeHealthy,
		})
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].PoolName != result[j].PoolName {
			return result[i].PoolName < result[j].PoolName
		}
		if result[i].NodeID != result[j].NodeID {
			return result[i].NodeID < result[j].NodeID
		}
		return result[i].Name < result[j].Name
	})
	return result
}

func publicNodeIPsForSnapshot(node *model.Node) []string {
	ips := make([]string, 0)
	seen := map[string]struct{}{}
	for _, value := range resolveNodePublicIPs(node) {
		ip := iputil.NormalizeIP(value)
		if ip == "" || !iputil.IsPublicString(ip) {
			continue
		}
		if _, ok := seen[ip]; ok {
			continue
		}
		seen[ip] = struct{}{}
		ips = append(ips, ip)
	}
	sort.Strings(ips)
	return ips
}
