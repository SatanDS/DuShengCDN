package service

import (
	"crypto/sha256"
	"dushengcdn/common"
	"dushengcdn/internal/dnsworker"
	"dushengcdn/model"
	"dushengcdn/utils/security"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

const (
	defaultAuthoritativeDNSSnapshotCacheMaxEntries = 32
	authoritativeDNSSnapshotRevisionCallbackName   = "dushengcdn:authoritative_dns_snapshot_revision_event"
)

type authoritativeDNSSnapshotCacheEntry struct {
	snapshot      *AuthoritativeDNSSnapshot
	signed        map[string]*dnsworker.SignedSnapshot
	lastAccessAt  time.Time
	lastAccessSeq uint64
}

type authoritativeDNSSnapshotCacheStore struct {
	mu            sync.Mutex
	maxEntries    int
	nextAccessSeq uint64
	entries       map[string]*authoritativeDNSSnapshotCacheEntry
}

type authoritativeDNSSnapshotRevisionCacheStore struct {
	mu        sync.Mutex
	db        *gorm.DB
	staticKey string
	revision  string
	expiresAt time.Time
	valid     bool
	eventSeq  uint64
}

type authoritativeDNSSnapshotRevisionMutationKind uint8

const (
	authoritativeDNSSnapshotRevisionMutationNone authoritativeDNSSnapshotRevisionMutationKind = iota
	authoritativeDNSSnapshotRevisionMutationStatic
	authoritativeDNSSnapshotRevisionMutationDynamic
)

var authoritativeDNSSnapshotCache = &authoritativeDNSSnapshotCacheStore{
	maxEntries: defaultAuthoritativeDNSSnapshotCacheMaxEntries,
	entries:    map[string]*authoritativeDNSSnapshotCacheEntry{},
}

var authoritativeDNSSnapshotRevisionCache = &authoritativeDNSSnapshotRevisionCacheStore{}
var authoritativeDNSSnapshotRevisionCallbackDBs sync.Map

func resetAuthoritativeDNSSnapshotCache() {
	authoritativeDNSSnapshotCache.mu.Lock()
	authoritativeDNSSnapshotCache.nextAccessSeq = 0
	authoritativeDNSSnapshotCache.entries = map[string]*authoritativeDNSSnapshotCacheEntry{}
	authoritativeDNSSnapshotCache.mu.Unlock()
	resetAuthoritativeDNSSnapshotRevisionCache()
}

func (c *authoritativeDNSSnapshotCacheStore) getSnapshot(key string) (*AuthoritativeDNSSnapshot, bool) {
	if c == nil || strings.TrimSpace(key) == "" {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	entry := c.entries[key]
	if entry == nil {
		return nil, false
	}
	if entry.snapshot == nil {
		return nil, false
	}
	c.touchLocked(entry)
	return entry.snapshot, true
}

func (c *authoritativeDNSSnapshotCacheStore) getSigned(key string, token string) (*dnsworker.SignedSnapshot, bool) {
	if c == nil || strings.TrimSpace(key) == "" {
		return nil, false
	}
	cacheToken := authoritativeDNSSnapshotTokenCacheKey(token)
	if cacheToken == "" {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	entry := c.entries[key]
	if entry == nil {
		return nil, false
	}
	if entry.signed == nil {
		return nil, false
	}
	signed := entry.signed[cacheToken]
	if signed == nil {
		return nil, false
	}
	c.touchLocked(entry)
	return signed, true
}

func (c *authoritativeDNSSnapshotCacheStore) storeSnapshot(key string, snapshot *AuthoritativeDNSSnapshot) {
	if c == nil || strings.TrimSpace(key) == "" || snapshot == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	c.nextAccessSeq++
	c.entries[key] = &authoritativeDNSSnapshotCacheEntry{
		snapshot:      snapshot,
		signed:        map[string]*dnsworker.SignedSnapshot{},
		lastAccessAt:  now,
		lastAccessSeq: c.nextAccessSeq,
	}
	c.pruneLocked()
}

func (c *authoritativeDNSSnapshotCacheStore) storeSigned(key string, token string, signed *dnsworker.SignedSnapshot) {
	if c == nil || strings.TrimSpace(key) == "" || signed == nil {
		return
	}
	cacheToken := authoritativeDNSSnapshotTokenCacheKey(token)
	if cacheToken == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	entry := c.entries[key]
	if entry == nil {
		return
	}
	if entry.signed == nil {
		entry.signed = map[string]*dnsworker.SignedSnapshot{}
	}
	entry.signed[cacheToken] = signed
	c.touchLocked(entry)
	c.pruneLocked()
}

func (c *authoritativeDNSSnapshotCacheStore) touchLocked(entry *authoritativeDNSSnapshotCacheEntry) {
	if c == nil || entry == nil {
		return
	}
	c.nextAccessSeq++
	entry.lastAccessAt = time.Now()
	entry.lastAccessSeq = c.nextAccessSeq
}

func (c *authoritativeDNSSnapshotCacheStore) pruneLocked() {
	if c == nil || c.maxEntries <= 0 || len(c.entries) <= c.maxEntries {
		return
	}
	for len(c.entries) > c.maxEntries {
		var oldestKey string
		var oldestAccessSeq uint64
		for key, entry := range c.entries {
			accessSeq := uint64(0)
			if entry != nil {
				accessSeq = entry.lastAccessSeq
			}
			if oldestKey == "" || accessSeq < oldestAccessSeq {
				oldestKey = key
				oldestAccessSeq = accessSeq
			}
		}
		if oldestKey == "" {
			return
		}
		delete(c.entries, oldestKey)
	}
}

func authoritativeDNSSnapshotCacheKey(worker *model.DNSWorker, schedulingQueries gslbDNSSchedulingDataQueries) (string, bool) {
	if schedulingQueries.ListNodes != nil || schedulingQueries.ListGSLBSchedulingStates != nil {
		return "", false
	}
	revision, ok := authoritativeDNSSnapshotCacheRevision()
	if !ok {
		return "", false
	}
	workerKey := "global"
	if worker == nil || worker.ID == 0 {
		return "authoritative-dns:" + workerKey + ":" + revision, true
	}
	workerKey = fmt.Sprintf("worker:%d", worker.ID)
	return "authoritative-dns:" + workerKey + ":" + revision, true
}

func authoritativeDNSSnapshotCacheRevision() (string, bool) {
	if model.DB == nil {
		return "", false
	}
	ensureAuthoritativeDNSSnapshotRevisionCallbacks(model.DB)
	staticKey := authoritativeDNSSnapshotRevisionStaticKey(model.DB)
	if cached, ok := authoritativeDNSSnapshotRevisionCache.get(model.DB, staticKey); ok {
		return cached, true
	}
	revision, expiresAt, ok := computeAuthoritativeDNSSnapshotCacheRevision(staticKey)
	if !ok {
		return "", false
	}
	authoritativeDNSSnapshotRevisionCache.store(model.DB, staticKey, revision, expiresAt)
	return revision, true
}

func resetAuthoritativeDNSSnapshotRevisionCache() {
	authoritativeDNSSnapshotRevisionCache.mu.Lock()
	authoritativeDNSSnapshotRevisionCache.db = nil
	authoritativeDNSSnapshotRevisionCache.staticKey = ""
	authoritativeDNSSnapshotRevisionCache.revision = ""
	authoritativeDNSSnapshotRevisionCache.expiresAt = time.Time{}
	authoritativeDNSSnapshotRevisionCache.valid = false
	authoritativeDNSSnapshotRevisionCache.eventSeq = 0
	authoritativeDNSSnapshotRevisionCache.mu.Unlock()
}

func (c *authoritativeDNSSnapshotRevisionCacheStore) get(db *gorm.DB, staticKey string) (string, bool) {
	if c == nil || db == nil || strings.TrimSpace(staticKey) == "" {
		return "", false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.valid || c.db != db || c.staticKey != staticKey || c.revision == "" {
		return "", false
	}
	if !c.expiresAt.IsZero() && !time.Now().Before(c.expiresAt) {
		return "", false
	}
	return c.revision, true
}

func (c *authoritativeDNSSnapshotRevisionCacheStore) store(db *gorm.DB, staticKey string, revision string, expiresAt time.Time) {
	if c == nil || db == nil || strings.TrimSpace(staticKey) == "" || strings.TrimSpace(revision) == "" {
		return
	}
	c.mu.Lock()
	c.db = db
	c.staticKey = staticKey
	c.revision = revision
	c.expiresAt = expiresAt
	c.valid = true
	c.mu.Unlock()
}

func (c *authoritativeDNSSnapshotRevisionCacheStore) bumpStaticMutation(db *gorm.DB) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if db == nil || !c.valid || c.db != db || c.staticKey == "" || c.revision == "" {
		return
	}
	if !c.expiresAt.IsZero() && !time.Now().Before(c.expiresAt) {
		c.valid = false
		c.revision = ""
		c.expiresAt = time.Time{}
		return
	}
	c.eventSeq++
	seed := fmt.Sprintf("%p|%s|%s|%d|%d", c.db, c.staticKey, c.revision, c.eventSeq, time.Now().UTC().UnixNano())
	sum := sha256.Sum256([]byte(seed))
	c.revision = hex.EncodeToString(sum[:])[:24]
}

func (c *authoritativeDNSSnapshotRevisionCacheStore) invalidateForDB(db *gorm.DB) {
	if c == nil || db == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.db != db {
		return
	}
	c.valid = false
	c.revision = ""
	c.expiresAt = time.Time{}
}

func (c *authoritativeDNSSnapshotRevisionCacheStore) invalidate() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.valid = false
	c.revision = ""
	c.expiresAt = time.Time{}
	c.mu.Unlock()
}

func authoritativeDNSSnapshotRevisionStaticKey(db *gorm.DB) string {
	builder := strings.Builder{}
	builder.WriteString(fmt.Sprintf("%p", db))
	builder.WriteByte('|')
	builder.WriteString(strconv.FormatBool(common.GSLBProbeSchedulingEnabled))
	builder.WriteByte('|')
	builder.WriteString(strconv.FormatBool(common.AuthoritativeDNSWorkerAllowUnassignedZones))
	builder.WriteByte('|')
	builder.WriteString(common.NodeOfflineThreshold.String())
	builder.WriteByte('|')
	builder.WriteString(strconv.Itoa(common.GSLBMetricFreshnessSeconds))
	if connectedNodes := connectedAgentWSNodeIDsForSnapshotRevision(); len(connectedNodes) > 0 {
		builder.WriteString("|agent_ws:")
		builder.WriteString(strings.Join(connectedNodes, ","))
	}
	if policyRaw, err := json.Marshal(authoritativeDNSWorkerPolicy()); err == nil {
		builder.WriteString("|worker_policy:")
		builder.Write(policyRaw)
	}
	return builder.String()
}

func connectedAgentWSNodeIDsForSnapshotRevision() []string {
	clients := snapshotAgentWSClients()
	if len(clients) == 0 {
		return []string{}
	}
	seen := make(map[string]struct{}, len(clients))
	nodeIDs := make([]string, 0, len(clients))
	for _, client := range clients {
		if client == nil {
			continue
		}
		select {
		case <-client.Done():
			continue
		default:
		}
		nodeID := strings.TrimSpace(client.NodeID())
		if nodeID == "" {
			continue
		}
		if _, ok := seen[nodeID]; ok {
			continue
		}
		seen[nodeID] = struct{}{}
		nodeIDs = append(nodeIDs, nodeID)
	}
	sort.Strings(nodeIDs)
	return nodeIDs
}

func computeAuthoritativeDNSSnapshotCacheRevision(staticKey string) (string, time.Time, bool) {
	now := time.Now().UTC()
	expiresAt := time.Time{}
	parts := []struct {
		name string
		load func(time.Time) (any, time.Time, error)
	}{
		{name: "dns_zones", load: authoritativeDNSSnapshotDNSZoneRevisionRows},
		{name: "dns_records", load: authoritativeDNSSnapshotDNSRecordRevisionRows},
		{name: "dnssec_keys", load: authoritativeDNSSnapshotDNSSECKeyRevisionRows},
		{name: "dns_zone_worker_assignments", load: authoritativeDNSSnapshotAssignmentRevisionRows},
		{name: "nodes", load: authoritativeDNSSnapshotNodeRevisionRows},
		{name: "node_metric_snapshots", load: authoritativeDNSSnapshotNodeMetricRevisionRows},
		{name: "proxy_routes", load: authoritativeDNSSnapshotProxyRouteRevisionRows},
		{name: "gslb_scheduling_states", load: authoritativeDNSSnapshotGSLBStateRevisionRows},
	}
	if common.GSLBProbeSchedulingEnabled {
		parts = append(parts, struct {
			name string
			load func(time.Time) (any, time.Time, error)
		}{name: "dns_worker_node_probes", load: authoritativeDNSSnapshotNodeProbeRevisionRows})
	}
	rowsByPart := make([]struct {
		name string
		rows any
	}, 0, len(parts))
	for _, part := range parts {
		rows, partExpiresAt, err := part.load(now)
		if err != nil {
			return "", time.Time{}, false
		}
		expiresAt = earlierFutureTime(now, expiresAt, partExpiresAt)
		rowsByPart = append(rowsByPart, struct {
			name string
			rows any
		}{name: part.name, rows: rows})
	}

	builder := strings.Builder{}
	builder.WriteString(staticKey)
	if !expiresAt.IsZero() {
		builder.WriteString("|expires_at:")
		builder.WriteString(expiresAt.UTC().Format(time.RFC3339Nano))
	}
	for _, part := range rowsByPart {
		raw, err := json.Marshal(part.rows)
		if err != nil {
			return "", time.Time{}, false
		}
		builder.WriteByte('|')
		builder.WriteString(part.name)
		builder.WriteByte(':')
		builder.Write(raw)
	}
	sum := sha256.Sum256([]byte(builder.String()))
	return hex.EncodeToString(sum[:])[:24], expiresAt, true
}

func earlierFutureTime(now time.Time, current time.Time, candidate time.Time) time.Time {
	if candidate.IsZero() || !candidate.After(now) {
		return current
	}
	if current.IsZero() || candidate.Before(current) {
		return candidate
	}
	return current
}

func ensureAuthoritativeDNSSnapshotRevisionCallbacks(db *gorm.DB) {
	if db == nil {
		return
	}
	if _, loaded := authoritativeDNSSnapshotRevisionCallbackDBs.LoadOrStore(db, struct{}{}); loaded {
		return
	}
	registeredDB := db
	applyMutation := func(tx *gorm.DB) {
		switch authoritativeDNSSnapshotRevisionStatementMutationKind(tx) {
		case authoritativeDNSSnapshotRevisionMutationStatic:
			authoritativeDNSSnapshotRevisionCache.bumpStaticMutation(registeredDB)
		case authoritativeDNSSnapshotRevisionMutationDynamic:
			authoritativeDNSSnapshotRevisionCache.invalidateForDB(registeredDB)
		}
	}
	_ = db.Callback().Create().After("gorm:create").Register(authoritativeDNSSnapshotRevisionCallbackName, applyMutation)
	_ = db.Callback().Update().After("gorm:update").Register(authoritativeDNSSnapshotRevisionCallbackName, applyMutation)
	_ = db.Callback().Delete().After("gorm:delete").Register(authoritativeDNSSnapshotRevisionCallbackName, applyMutation)
}

func authoritativeDNSSnapshotRevisionStatementAffectsSnapshot(db *gorm.DB) bool {
	return authoritativeDNSSnapshotRevisionStatementMutationKind(db) != authoritativeDNSSnapshotRevisionMutationNone
}

func authoritativeDNSSnapshotRevisionStatementMutationKind(db *gorm.DB) authoritativeDNSSnapshotRevisionMutationKind {
	if db == nil || db.Statement == nil {
		return authoritativeDNSSnapshotRevisionMutationNone
	}
	table := strings.TrimSpace(db.Statement.Table)
	if table == "" && db.Statement.Schema != nil {
		table = db.Statement.Schema.Table
	}
	if kind := authoritativeDNSSnapshotRevisionTableMutationKind(table); kind != authoritativeDNSSnapshotRevisionMutationNone {
		return kind
	}
	return authoritativeDNSSnapshotRevisionSchemaMutationKind(db.Statement.Schema)
}

func authoritativeDNSSnapshotRevisionSchemaAffectsSnapshot(parsed *schema.Schema) bool {
	return authoritativeDNSSnapshotRevisionSchemaMutationKind(parsed) != authoritativeDNSSnapshotRevisionMutationNone
}

func authoritativeDNSSnapshotRevisionSchemaMutationKind(parsed *schema.Schema) authoritativeDNSSnapshotRevisionMutationKind {
	if parsed == nil {
		return authoritativeDNSSnapshotRevisionMutationNone
	}
	return authoritativeDNSSnapshotRevisionTableMutationKind(parsed.Table)
}

func authoritativeDNSSnapshotRevisionTableAffectsSnapshot(table string) bool {
	return authoritativeDNSSnapshotRevisionTableMutationKind(table) != authoritativeDNSSnapshotRevisionMutationNone
}

func authoritativeDNSSnapshotRevisionTableMutationKind(table string) authoritativeDNSSnapshotRevisionMutationKind {
	normalized := strings.ToLower(strings.TrimSpace(table))
	if normalized == "" {
		return authoritativeDNSSnapshotRevisionMutationNone
	}
	if strings.HasPrefix(normalized, strings.ToLower((&model.NodeMetricSnapshot{}).TableName())+"_") {
		return authoritativeDNSSnapshotRevisionMutationDynamic
	}
	switch normalized {
	case strings.ToLower((&model.DNSZone{}).TableName()),
		strings.ToLower((&model.DNSRecord{}).TableName()),
		strings.ToLower((&model.DNSSECKey{}).TableName()),
		strings.ToLower((&model.DNSZoneWorkerAssignment{}).TableName()),
		strings.ToLower((&model.ProxyRoute{}).TableName()),
		strings.ToLower((&model.GSLBSchedulingState{}).TableName()):
		return authoritativeDNSSnapshotRevisionMutationStatic
	case strings.ToLower((&model.Node{}).TableName()),
		strings.ToLower((&model.NodeMetricSnapshot{}).TableName()),
		strings.ToLower((&model.DNSWorkerNodeProbe{}).TableName()):
		return authoritativeDNSSnapshotRevisionMutationDynamic
	default:
		return authoritativeDNSSnapshotRevisionMutationNone
	}
}

type authoritativeDNSSnapshotDNSZoneRevisionRow struct {
	ID                      uint
	Name                    string
	SOAEmail                string
	PrimaryNS               string
	NameServers             string
	DefaultTTL              int
	Serial                  uint64
	DNSSECEnabled           bool
	DNSSECDenialMode        string
	DNSSECNSEC3Salt         string
	DNSSECNSEC3Iterations   int
	DNSSECSignatureValidity int
	Enabled                 bool
	UpdatedAt               time.Time
}

func authoritativeDNSSnapshotDNSZoneRevisionRows(time.Time) (any, time.Time, error) {
	var rows []authoritativeDNSSnapshotDNSZoneRevisionRow
	err := model.DB.Model(&model.DNSZone{}).
		Select([]string{
			"id",
			"name",
			"soa_email",
			"primary_ns",
			"name_servers",
			"default_ttl",
			"serial",
			"dnssec_enabled",
			"dnssec_denial_mode",
			"dnssec_nsec3_salt",
			"dnssec_nsec3_iterations",
			"dnssec_signature_validity",
			"enabled",
			"updated_at",
		}).
		Order("id asc").
		Find(&rows).Error
	return rows, time.Time{}, err
}

type authoritativeDNSSnapshotDNSRecordRevisionRow struct {
	ID        uint
	ZoneID    uint
	Name      string
	Type      string
	Value     string
	TTL       int
	Priority  int
	Enabled   bool
	UpdatedAt time.Time
}

func authoritativeDNSSnapshotDNSRecordRevisionRows(time.Time) (any, time.Time, error) {
	var rows []authoritativeDNSSnapshotDNSRecordRevisionRow
	err := model.DB.Model(&model.DNSRecord{}).
		Select([]string{
			"id",
			"zone_id",
			"name",
			"type",
			"value",
			"ttl",
			"priority",
			"enabled",
			"updated_at",
		}).
		Order("zone_id asc").
		Order("id asc").
		Find(&rows).Error
	return rows, time.Time{}, err
}

type authoritativeDNSSnapshotDNSSECKeyRevisionRow struct {
	ID                  uint
	ZoneID              uint
	Role                string
	Flags               uint16
	Algorithm           uint8
	PublicKey           string
	EncryptedPrivateKey string
	KeyTag              uint16
	DSDigestSHA256      string
	Status              string
	ActivatedAt         *time.Time
	RetiredAt           *time.Time
	UpdatedAt           time.Time
}

func authoritativeDNSSnapshotDNSSECKeyRevisionRows(time.Time) (any, time.Time, error) {
	var rows []authoritativeDNSSnapshotDNSSECKeyRevisionRow
	err := model.DB.Model(&model.DNSSECKey{}).
		Select([]string{
			"id",
			"zone_id",
			"role",
			"flags",
			"algorithm",
			"public_key",
			"encrypted_private_key",
			"key_tag",
			"ds_digest_sha256",
			"status",
			"activated_at",
			"retired_at",
			"updated_at",
		}).
		Order("zone_id asc").
		Order("role asc").
		Order("id asc").
		Find(&rows).Error
	return rows, time.Time{}, err
}

type authoritativeDNSSnapshotAssignmentRevisionRow struct {
	ID        uint
	ZoneID    uint
	WorkerID  uint
	UpdatedAt time.Time
}

func authoritativeDNSSnapshotAssignmentRevisionRows(time.Time) (any, time.Time, error) {
	var rows []authoritativeDNSSnapshotAssignmentRevisionRow
	err := model.DB.Model(&model.DNSZoneWorkerAssignment{}).
		Select([]string{
			"id",
			"zone_id",
			"worker_id",
			"updated_at",
		}).
		Order("zone_id asc").
		Order("worker_id asc").
		Order("id asc").
		Find(&rows).Error
	return rows, time.Time{}, err
}

type authoritativeDNSSnapshotNodeRevisionRow struct {
	ID                uint
	NodeID            string
	Name              string
	IP                string
	PoolName          string
	Weight            int
	PublicIPs         string
	SchedulingEnabled bool
	DrainMode         bool
	OpenrestyStatus   string
	Status            string
	EffectiveStatus   string
	LastSeenAt        time.Time
}

func authoritativeDNSSnapshotNodeRevisionRows(now time.Time) (any, time.Time, error) {
	var rows []authoritativeDNSSnapshotNodeRevisionRow
	err := model.DB.Model(&model.Node{}).
		Select([]string{
			"id",
			"node_id",
			"name",
			"ip",
			"pool_name",
			"weight",
			"public_ips",
			"scheduling_enabled",
			"drain_mode",
			"openresty_status",
			"status",
			"last_seen_at",
		}).
		Order("id asc").
		Find(&rows).Error
	if err != nil {
		return rows, time.Time{}, err
	}
	expiresAt := time.Time{}
	for index := range rows {
		node := &model.Node{
			NodeID:     rows[index].NodeID,
			LastSeenAt: rows[index].LastSeenAt,
		}
		rows[index].EffectiveStatus = computeNodeStatus(node)
		if rows[index].LastSeenAt.IsZero() || IsAgentWSConnected(rows[index].NodeID) {
			continue
		}
		expiresAt = earlierFutureTime(now, expiresAt, rows[index].LastSeenAt.UTC().Add(common.NodeOfflineThreshold))
	}
	return rows, expiresAt, nil
}

type authoritativeDNSSnapshotNodeMetricRevisionRow struct {
	ID                   uint
	NodeID               string
	CapturedAt           time.Time
	CPUUsagePercent      float64
	MemoryUsedBytes      int64
	MemoryTotalBytes     int64
	OpenrestyConnections int64
	CreatedAt            time.Time
}

func authoritativeDNSSnapshotNodeMetricRevisionRows(now time.Time) (any, time.Time, error) {
	freshness := time.Duration(common.GSLBMetricFreshnessSeconds) * time.Second
	if freshness <= 0 {
		freshness = 120 * time.Second
	}
	rows, err := model.ListLatestMetricSnapshotsByNode(now.Add(-freshness), now)
	if err != nil {
		return nil, time.Time{}, err
	}
	result := make([]authoritativeDNSSnapshotNodeMetricRevisionRow, 0, len(rows))
	expiresAt := time.Time{}
	for _, row := range rows {
		if row == nil || strings.TrimSpace(row.NodeID) == "" {
			continue
		}
		result = append(result, authoritativeDNSSnapshotNodeMetricRevisionRow{
			ID:                   row.ID,
			NodeID:               row.NodeID,
			CapturedAt:           row.CapturedAt,
			CPUUsagePercent:      row.CPUUsagePercent,
			MemoryUsedBytes:      row.MemoryUsedBytes,
			MemoryTotalBytes:     row.MemoryTotalBytes,
			OpenrestyConnections: row.OpenrestyConnections,
			CreatedAt:            row.CreatedAt,
		})
		expiresAt = earlierFutureTime(now, expiresAt, row.CapturedAt.UTC().Add(freshness))
	}
	return result, expiresAt, nil
}

type authoritativeDNSSnapshotProxyRouteRevisionRow struct {
	ID                     uint
	SiteName               string
	Domain                 string
	Domains                string
	NodePool               string
	Enabled                bool
	DNSProviderMode        string
	DNSZoneIDRef           *uint
	DNSRecordType          string
	DNSTargetCount         int
	DNSScheduleMode        string
	DNSTTL                 int
	GSLBEnabled            bool
	GSLBPolicy             string
	DDOSProtectionMode     string
	DDOSProtectionProvider string
	DDOSProtectionTarget   string
	UpdatedAt              time.Time
}

func authoritativeDNSSnapshotProxyRouteRevisionRows(time.Time) (any, time.Time, error) {
	var rows []authoritativeDNSSnapshotProxyRouteRevisionRow
	err := model.DB.Model(&model.ProxyRoute{}).
		Select([]string{
			"id",
			"site_name",
			"domain",
			"domains",
			"node_pool",
			"enabled",
			"dns_provider_mode",
			"dns_zone_id_ref",
			"dns_record_type",
			"dns_target_count",
			"dns_schedule_mode",
			"dns_ttl",
			"gslb_enabled",
			"gslb_policy",
			"ddos_protection_mode",
			"ddos_protection_provider",
			"ddos_protection_target",
			"updated_at",
		}).
		Order("id asc").
		Find(&rows).Error
	return rows, time.Time{}, err
}

type authoritativeDNSSnapshotGSLBStateRevisionRow struct {
	ID              uint
	ProxyRouteID    uint
	DNSRecordType   string
	ScopeKey        string
	SelectedTargets string
	DesiredTargets  string
	UnhealthyCount  int
	RecoveryCount   int
	LastReason      string
	LastChangedAt   *time.Time
	LastEvaluatedAt *time.Time
	UpdatedAt       time.Time
}

func authoritativeDNSSnapshotGSLBStateRevisionRows(time.Time) (any, time.Time, error) {
	var rows []authoritativeDNSSnapshotGSLBStateRevisionRow
	err := model.DB.Model(&model.GSLBSchedulingState{}).
		Select([]string{
			"id",
			"proxy_route_id",
			"dns_record_type",
			"scope_key",
			"selected_targets",
			"desired_targets",
			"unhealthy_count",
			"recovery_count",
			"last_reason",
			"last_changed_at",
			"last_evaluated_at",
			"updated_at",
		}).
		Order("proxy_route_id asc").
		Order("dns_record_type asc").
		Order("scope_key asc").
		Order("id asc").
		Find(&rows).Error
	return rows, time.Time{}, err
}

type authoritativeDNSSnapshotNodeProbeRevisionRow struct {
	ID             uint
	WorkerID       string
	NodeID         string
	PublicAddress  string
	QueryName      string
	QueryType      string
	CheckedAt      time.Time
	EffectiveFresh bool
	ResultsJSON    string
	Healthy        bool
	AverageRTTMs   float64
	MaxRTTMs       int64
	LastError      string
	FailureSamples int
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func authoritativeDNSSnapshotNodeProbeRevisionRows(now time.Time) (any, time.Time, error) {
	var rows []authoritativeDNSSnapshotNodeProbeRevisionRow
	err := model.DB.Model(&model.DNSWorkerNodeProbe{}).
		Select([]string{
			"id",
			"worker_id",
			"node_id",
			"public_address",
			"query_name",
			"query_type",
			"checked_at",
			"results_json",
			"healthy",
			"average_rtt_ms",
			"max_rtt_ms",
			"last_error",
			"failure_samples",
			"created_at",
			"updated_at",
		}).
		Order("worker_id asc").
		Order("node_id asc").
		Order("id asc").
		Find(&rows).Error
	if err != nil {
		return rows, time.Time{}, err
	}
	expiresAt := time.Time{}
	for index := range rows {
		checkedAt := normalizeDNSWorkerCheckedAt(&rows[index].CheckedAt, now, rows[index].UpdatedAt, rows[index].CreatedAt)
		expiresAt = earlierFutureTime(now, expiresAt, checkedAt.Add(defaultDNSWorkerNodeProbeMaxAge))
		rows[index].EffectiveFresh = now.Sub(checkedAt) <= defaultDNSWorkerNodeProbeMaxAge
	}
	return rows, expiresAt, nil
}

func authoritativeDNSSnapshotTokenCacheKey(token string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return ""
	}
	if security.IsHashedSecretToken(token) {
		return token
	}
	return security.HashSecretToken(token)
}
