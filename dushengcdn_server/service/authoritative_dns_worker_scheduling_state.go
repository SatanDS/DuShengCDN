package service

import (
	"dushengcdn/common"
	"dushengcdn/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strings"
	"time"
)

func persistDNSWorkerSchedulingStates(inputs []AuthoritativeDNSSnapshotSchedulingState) error {
	return persistDNSWorkerSchedulingStatesWithDB(model.DB, nil, inputs)
}

func persistDNSWorkerSchedulingStatesWithDB(db *gorm.DB, worker *model.DNSWorker, inputs []AuthoritativeDNSSnapshotSchedulingState) error {
	if db == nil {
		db = model.DB
	}
	if len(inputs) == 0 {
		return nil
	}
	routeIDs := make([]uint, 0, len(inputs))
	seenRoutes := map[uint]struct{}{}
	for _, input := range inputs {
		if input.RouteID == 0 {
			continue
		}
		if _, ok := seenRoutes[input.RouteID]; ok {
			continue
		}
		seenRoutes[input.RouteID] = struct{}{}
		routeIDs = append(routeIDs, input.RouteID)
	}
	if len(routeIDs) == 0 {
		return nil
	}
	acl, err := buildDNSWorkerHeartbeatACL(db, worker)
	if err != nil {
		return err
	}
	var routes []*model.ProxyRoute
	if err := db.
		Select("id", "dns_record_type", "dns_record_content", "dns_zone_id_ref", "node_pool", "gslb_enabled", "gslb_policy", "ddos_protection_mode", "ddos_protection_provider", "ddos_protection_target").
		Where("id IN ? AND enabled = ? AND dns_provider_mode = ? AND dns_zone_id_ref IS NOT NULL", routeIDs, true, DNSProviderModeAuthoritative).
		Find(&routes).Error; err != nil {
		return err
	}
	routeRecordTypes := make(map[uint]string, len(routes))
	routeAllowedTargets := make(map[uint]map[string]struct{}, len(routes))
	allowedTargetNodes, err := loadAllowedDNSSchedulingTargetNodesWithDB(db, routes)
	if err != nil {
		allowedTargetNodes = nil
	}
	for _, route := range routes {
		if route == nil || route.ID == 0 {
			continue
		}
		if !acl.allowsRoute(route.ID) {
			continue
		}
		recordType := normalizeDNSRecordType(route.DNSRecordType)
		if recordType != "A" && recordType != "AAAA" {
			continue
		}
		routeRecordTypes[route.ID] = recordType
		routeAllowedTargets[route.ID] = allowedDNSSchedulingTargetsForRouteWithNodes(route, recordType, allowedTargetNodes)
	}
	now := time.Now().UTC()
	updates := make([]dnsWorkerSchedulingStateUpdate, 0, len(inputs))
	for _, input := range inputs {
		expectedType, ok := routeRecordTypes[input.RouteID]
		if !ok {
			continue
		}
		recordType := normalizeDNSRecordType(input.RecordType)
		if recordType != expectedType {
			continue
		}
		scopeKey := normalizeDNSSourceScope(input.ScopeKey)
		selectedTargets, err := normalizeDNSRecordContents(recordType, input.SelectedTargets)
		if err != nil || len(selectedTargets) == 0 {
			continue
		}
		allowedTargets := routeAllowedTargets[input.RouteID]
		if acl.enforce && (len(allowedTargets) == 0 || !allTargetsEligible(selectedTargets, allowedTargets)) {
			continue
		}
		desiredTargets := input.DesiredTargets
		if len(desiredTargets) > 0 {
			desiredTargets, err = normalizeDNSRecordContents(recordType, desiredTargets)
			if err != nil {
				desiredTargets = []string{}
			}
			if acl.enforce && len(desiredTargets) > 0 && !allTargetsEligible(desiredTargets, allowedTargets) {
				desiredTargets = []string{}
			}
		}
		lastChangedAt := normalizeGSLBSchedulingStateChangedAt(input.LastChangedAt, now)
		if lastChangedAt == nil || lastChangedAt.IsZero() {
			lastChangedAt = &now
		}
		updates = append(updates, dnsWorkerSchedulingStateUpdate{
			key: dnsWorkerSchedulingStateKey{
				routeID:    input.RouteID,
				recordType: recordType,
				scopeKey:   scopeKey,
			},
			selectedTargets: selectedTargets,
			desiredTargets:  desiredTargets,
			unhealthyCount:  normalizeDebounceCounter(input.UnhealthyCount),
			recoveryCount:   normalizeDebounceCounter(input.RecoveryCount),
			lastChangedAt:   *lastChangedAt,
		})
	}
	if len(updates) == 0 {
		return nil
	}
	statesByKey, err := loadGSLBSchedulingStatesForUpdatesWithDB(db, updates)
	if err != nil {
		return err
	}
	dirtyKeys := make([]dnsWorkerSchedulingStateKey, 0, len(updates))
	seenDirtyKeys := make(map[dnsWorkerSchedulingStateKey]struct{}, len(updates))
	for _, update := range updates {
		state := statesByKey[update.key]
		if state == nil {
			state = &model.GSLBSchedulingState{
				ProxyRouteID:  update.key.routeID,
				DNSRecordType: update.key.recordType,
				ScopeKey:      update.key.scopeKey,
				CreatedAt:     now,
			}
			statesByKey[update.key] = state
		}
		if !applyDNSWorkerSchedulingStateUpdate(state, update, now) {
			continue
		}
		if _, ok := seenDirtyKeys[update.key]; ok {
			continue
		}
		seenDirtyKeys[update.key] = struct{}{}
		dirtyKeys = append(dirtyKeys, update.key)
	}
	if len(dirtyKeys) == 0 {
		return nil
	}
	rows := make([]*model.GSLBSchedulingState, 0, len(dirtyKeys))
	for _, key := range dirtyKeys {
		state := statesByKey[key]
		if state == nil {
			continue
		}
		rows = append(rows, &model.GSLBSchedulingState{
			ProxyRouteID:    state.ProxyRouteID,
			DNSRecordType:   state.DNSRecordType,
			ScopeKey:        state.ScopeKey,
			SelectedTargets: state.SelectedTargets,
			DesiredTargets:  state.DesiredTargets,
			UnhealthyCount:  state.UnhealthyCount,
			RecoveryCount:   state.RecoveryCount,
			LastReason:      state.LastReason,
			LastChangedAt:   state.LastChangedAt,
			LastEvaluatedAt: state.LastEvaluatedAt,
			CreatedAt:       state.CreatedAt,
			UpdatedAt:       now,
		})
	}
	return db.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "proxy_route_id"},
			{Name: "dns_record_type"},
			{Name: "scope_key"},
		},
		DoUpdates: clause.AssignmentColumns([]string{
			"selected_targets",
			"desired_targets",
			"unhealthy_count",
			"recovery_count",
			"last_reason",
			"last_changed_at",
			"last_evaluated_at",
			"updated_at",
		}),
	}).CreateInBatches(rows, 100).Error
}

type dnsWorkerHeartbeatACL struct {
	enforce         bool
	allowedZones    map[uint]struct{}
	allowedRoutes   map[uint]struct{}
	routeToZone     map[uint]uint
	routeRecordType map[uint]string
}

func buildDNSWorkerHeartbeatACL(db *gorm.DB, worker *model.DNSWorker) (*dnsWorkerHeartbeatACL, error) {
	acl := &dnsWorkerHeartbeatACL{
		enforce:         worker != nil && worker.ID != 0,
		allowedZones:    map[uint]struct{}{},
		allowedRoutes:   map[uint]struct{}{},
		routeToZone:     map[uint]uint{},
		routeRecordType: map[uint]string{},
	}
	if db == nil {
		db = model.DB
	}
	var zones []*model.DNSZone
	if err := db.Select("id").Where("enabled = ?", true).Find(&zones).Error; err != nil {
		return nil, err
	}
	zoneIDs := make([]uint, 0, len(zones))
	for _, zone := range zones {
		if zone == nil || zone.ID == 0 {
			continue
		}
		zoneIDs = append(zoneIDs, zone.ID)
	}
	var assignments []*model.DNSZoneWorkerAssignment
	if len(zoneIDs) > 0 {
		err := db.Where("zone_id IN ?", zoneIDs).Order("zone_id asc").Order("worker_id asc").Find(&assignments).Error
		if err != nil {
			return nil, err
		}
	}
	if len(zoneIDs) == 0 {
		assignments = []*model.DNSZoneWorkerAssignment{}
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
	for _, zoneID := range zoneIDs {
		assignmentsForZone := assignedByZone[zoneID]
		if acl.enforce && len(assignmentsForZone) > 0 {
			if _, ok := assignmentsForZone[worker.ID]; !ok {
				continue
			}
		} else if acl.enforce && len(assignmentsForZone) == 0 && !allowUnassignedZones {
			continue
		}
		acl.allowedZones[zoneID] = struct{}{}
	}
	var routes []*model.ProxyRoute
	if err := db.
		Select("id", "dns_zone_id_ref", "dns_record_type").
		Where("enabled = ? AND dns_provider_mode = ? AND dns_zone_id_ref IS NOT NULL", true, DNSProviderModeAuthoritative).
		Find(&routes).Error; err != nil {
		return nil, err
	}
	for _, route := range routes {
		if route == nil || route.ID == 0 || route.DNSZoneIDRef == nil || *route.DNSZoneIDRef == 0 {
			continue
		}
		if _, ok := acl.allowedZones[*route.DNSZoneIDRef]; !ok {
			continue
		}
		acl.allowedRoutes[route.ID] = struct{}{}
		acl.routeToZone[route.ID] = *route.DNSZoneIDRef
		acl.routeRecordType[route.ID] = normalizeDNSRecordType(route.DNSRecordType)
	}
	return acl, nil
}

func (acl *dnsWorkerHeartbeatACL) allowsRoute(routeID uint) bool {
	if acl == nil || !acl.enforce {
		return true
	}
	_, ok := acl.allowedRoutes[routeID]
	return ok
}

func (acl *dnsWorkerHeartbeatACL) allowsZone(zoneID uint) bool {
	if acl == nil || !acl.enforce {
		return true
	}
	if zoneID == 0 {
		return false
	}
	_, ok := acl.allowedZones[zoneID]
	return ok
}

func (acl *dnsWorkerHeartbeatACL) allowsRollup(input DNSQueryRollupInput) bool {
	if acl == nil || !acl.enforce {
		return true
	}
	if input.ProxyRouteID != 0 {
		if !acl.allowsRoute(input.ProxyRouteID) {
			return false
		}
		if input.ZoneID != 0 && acl.routeToZone[input.ProxyRouteID] != 0 && input.ZoneID != acl.routeToZone[input.ProxyRouteID] {
			return false
		}
		expectedType := acl.routeRecordType[input.ProxyRouteID]
		if expectedType != "" {
			qtype := normalizeAuthoritativeDNSRecordTypeOrDefault(input.QType)
			if qtype != expectedType {
				return false
			}
		}
		return true
	}
	return acl.allowsZone(input.ZoneID)
}

func allowedDNSSchedulingTargetsForRoute(db *gorm.DB, route *model.ProxyRoute, recordType string) map[string]struct{} {
	if db == nil {
		db = model.DB
	}
	nodes, err := loadAllowedDNSSchedulingTargetNodesWithDB(db, []*model.ProxyRoute{route})
	if err != nil {
		nodes = nil
	}
	return allowedDNSSchedulingTargetsForRouteWithNodes(route, recordType, nodes)
}

func loadAllowedDNSSchedulingTargetNodesWithDB(db *gorm.DB, routes []*model.ProxyRoute) ([]*model.Node, error) {
	if db == nil {
		db = model.DB
	}
	needsNodes := false
	for _, route := range routes {
		if route == nil {
			continue
		}
		if len(allowedDNSSchedulingPoolsForRoute(route)) > 0 || len(allowedDNSSchedulingNodeIDsForRoute(route)) > 0 {
			needsNodes = true
			break
		}
	}
	if !needsNodes {
		return []*model.Node{}, nil
	}
	var nodes []*model.Node
	if err := db.Find(&nodes).Error; err != nil {
		return nil, err
	}
	return nodes, nil
}

func allowedDNSSchedulingTargetsForRouteWithNodes(route *model.ProxyRoute, recordType string, nodes []*model.Node) map[string]struct{} {
	allowed := map[string]struct{}{}
	if route == nil {
		return allowed
	}
	for _, target := range splitDNSRecordContent(route.DNSRecordContent) {
		normalized, err := normalizeDNSRecordContents(recordType, []string{target})
		if err == nil {
			for _, value := range normalized {
				allowed[value] = struct{}{}
			}
		}
	}
	pools := allowedDNSSchedulingPoolsForRoute(route)
	nodeIDs := allowedDNSSchedulingNodeIDsForRoute(route)
	if len(pools) == 0 && len(nodeIDs) == 0 {
		return allowed
	}
	for _, node := range nodes {
		if node == nil {
			continue
		}
		nodeID := strings.TrimSpace(node.NodeID)
		poolName := normalizeNodePoolName(node.PoolName)
		if _, ok := nodeIDs[nodeID]; !ok {
			if _, ok := pools[poolName]; !ok {
				continue
			}
		}
		for _, target := range nodeDNSContents(node, recordType) {
			allowed[target] = struct{}{}
		}
	}
	return allowed
}

func allowedDNSSchedulingPoolsForRoute(route *model.ProxyRoute) map[string]struct{} {
	allowed := map[string]struct{}{}
	if route == nil {
		return allowed
	}
	if routeDDOSProtectionActive(route) && normalizeDDOSProtectionProvider(route.DDOSProtectionProvider) == DDOSProtectionProviderCustom {
		if pool := normalizeNodePoolName(route.DDOSProtectionTarget); pool != "" {
			allowed[pool] = struct{}{}
		}
	} else if pool := normalizeNodePoolName(route.NodePool); pool != "" {
		allowed[pool] = struct{}{}
	}
	if route.GSLBEnabled {
		policy, err := decodeStoredGSLBPolicy(route.GSLBPolicy)
		if err == nil {
			if normalized, err := normalizeGSLBPolicy(policy, route.NodePool, route.DNSTargetCount, route.DNSScheduleMode, route.DNSTTL); err == nil {
				for _, pool := range normalized.Pools {
					if pool.Enabled {
						if name := normalizeNodePoolName(pool.Name); name != "" {
							allowed[name] = struct{}{}
						}
					}
				}
			}
		}
	}
	return allowed
}

func allowedDNSSchedulingNodeIDsForRoute(route *model.ProxyRoute) map[string]struct{} {
	allowed := map[string]struct{}{}
	if route == nil || !route.GSLBEnabled {
		return allowed
	}
	policy, err := decodeStoredGSLBPolicy(route.GSLBPolicy)
	if err != nil {
		return allowed
	}
	normalized, err := normalizeGSLBPolicy(policy, route.NodePool, route.DNSTargetCount, route.DNSScheduleMode, route.DNSTTL)
	if err != nil {
		return allowed
	}
	for _, pool := range normalized.Pools {
		if !pool.Enabled {
			continue
		}
		for _, nodeID := range pool.NodeIDs {
			if trimmed := strings.TrimSpace(nodeID); trimmed != "" {
				allowed[trimmed] = struct{}{}
			}
		}
	}
	return allowed
}

type dnsWorkerSchedulingStateKey struct {
	routeID    uint
	recordType string
	scopeKey   string
}

type dnsWorkerSchedulingStateUpdate struct {
	key             dnsWorkerSchedulingStateKey
	selectedTargets []string
	desiredTargets  []string
	unhealthyCount  int
	recoveryCount   int
	lastChangedAt   time.Time
}

func loadGSLBSchedulingStatesForUpdatesWithDB(db *gorm.DB, updates []dnsWorkerSchedulingStateUpdate) (map[dnsWorkerSchedulingStateKey]*model.GSLBSchedulingState, error) {
	if db == nil {
		db = model.DB
	}
	statesByKey := make(map[dnsWorkerSchedulingStateKey]*model.GSLBSchedulingState, len(updates))
	routeIDs := make([]uint, 0, len(updates))
	recordTypes := make([]string, 0, len(updates))
	scopeKeys := make([]string, 0, len(updates))
	seenRouteIDs := make(map[uint]struct{}, len(updates))
	seenRecordTypes := make(map[string]struct{}, len(updates))
	seenScopeKeys := make(map[string]struct{}, len(updates))
	updateKeys := make(map[dnsWorkerSchedulingStateKey]struct{}, len(updates))
	for _, update := range updates {
		key := update.key
		updateKeys[key] = struct{}{}
		if _, ok := seenRouteIDs[key.routeID]; !ok {
			seenRouteIDs[key.routeID] = struct{}{}
			routeIDs = append(routeIDs, key.routeID)
		}
		if _, ok := seenRecordTypes[key.recordType]; !ok {
			seenRecordTypes[key.recordType] = struct{}{}
			recordTypes = append(recordTypes, key.recordType)
		}
		if _, ok := seenScopeKeys[key.scopeKey]; !ok {
			seenScopeKeys[key.scopeKey] = struct{}{}
			scopeKeys = append(scopeKeys, key.scopeKey)
		}
	}
	if len(routeIDs) == 0 || len(recordTypes) == 0 || len(scopeKeys) == 0 {
		return statesByKey, nil
	}
	var states []*model.GSLBSchedulingState
	if err := db.
		Where("proxy_route_id IN ? AND dns_record_type IN ? AND scope_key IN ?", routeIDs, recordTypes, scopeKeys).
		Find(&states).Error; err != nil {
		return nil, err
	}
	for _, state := range states {
		if state == nil {
			continue
		}
		key := dnsWorkerSchedulingStateKey{
			routeID:    state.ProxyRouteID,
			recordType: normalizeDNSRecordType(state.DNSRecordType),
			scopeKey:   normalizeDNSSourceScope(state.ScopeKey),
		}
		if _, ok := updateKeys[key]; !ok {
			continue
		}
		statesByKey[key] = state
	}
	return statesByKey, nil
}

func applyDNSWorkerSchedulingStateUpdate(state *model.GSLBSchedulingState, update dnsWorkerSchedulingStateUpdate, evaluatedAt time.Time) bool {
	if state == nil {
		return false
	}
	changedAt := update.lastChangedAt.UTC()
	evaluated := evaluatedAt.UTC()
	if state.LastChangedAt != nil {
		existingChangedAt := state.LastChangedAt.UTC()
		if !existingChangedAt.After(evaluated) && existingChangedAt.After(changedAt) {
			return false
		}
	}
	state.DNSRecordType = update.key.recordType
	state.ScopeKey = update.key.scopeKey
	state.SelectedTargets = encodeGSLBTargetList(update.selectedTargets)
	state.DesiredTargets = encodeGSLBTargetList(update.desiredTargets)
	state.UnhealthyCount = normalizeDebounceCounter(update.unhealthyCount)
	state.RecoveryCount = normalizeDebounceCounter(update.recoveryCount)
	state.LastReason = "reported by DNS Worker heartbeat"
	state.LastChangedAt = &changedAt
	state.LastEvaluatedAt = &evaluated
	return true
}

func normalizeGSLBSchedulingStateChangedAt(changedAt *time.Time, now time.Time, fallbacks ...*time.Time) *time.Time {
	if changedAt == nil {
		return nil
	}
	normalizedNow := now.UTC()
	normalized := changedAt.UTC()
	if !normalized.After(normalizedNow) {
		return &normalized
	}
	for _, fallback := range fallbacks {
		if fallback == nil || fallback.IsZero() {
			continue
		}
		normalized = fallback.UTC()
		if !normalized.After(normalizedNow) {
			return &normalized
		}
	}
	normalized = normalizedNow
	return &normalized
}
