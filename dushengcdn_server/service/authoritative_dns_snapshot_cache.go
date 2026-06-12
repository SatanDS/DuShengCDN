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
	defaultAuthoritativeDNSSnapshotCacheMaxEntries           = 32
	authoritativeDNSSnapshotRevisionCallbackName             = "dushengcdn:authoritative_dns_snapshot_revision_event"
	authoritativeDNSSnapshotRevisionSentinelTable            = "authoritative_dns_snapshot_revisions"
	authoritativeDNSSnapshotRevisionSentinelKey              = "global"
	authoritativeDNSSnapshotRevisionBumpFunction             = "dushengcdn_bump_adns_snapshot_revision"
	authoritativeDNSSnapshotRevisionNotifyChannel            = "dushengcdn_adns_snapshot_revision"
	authoritativeDNSSnapshotPersistentRevisionVerifyInterval = 30 * time.Second
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
	mu                        sync.Mutex
	db                        *gorm.DB
	staticKey                 string
	revision                  string
	expiresAt                 time.Time
	valid                     bool
	eventSeq                  uint64
	persistentVersion         uint64
	persistentVersionRecorded bool
}

type authoritativeDNSSnapshotRevisionMutationKind uint8

type authoritativeDNSSnapshotPersistentRevisionInstallState struct {
	nextVerifyAt time.Time
}

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
var authoritativeDNSSnapshotPersistentRevisionDBs sync.Map

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
	if ensureAuthoritativeDNSSnapshotPersistentRevision(model.DB) {
		if row, err := loadAuthoritativeDNSSnapshotPersistentRevision(model.DB); err == nil {
			if cached, ok := authoritativeDNSSnapshotRevisionCache.getPersistent(model.DB, staticKey, row.Version); ok {
				return cached, true
			}
			if revision, expiresAt, ok := computeAuthoritativeDNSSnapshotPersistentCacheRevision(model.DB, staticKey, row); ok {
				authoritativeDNSSnapshotRevisionCache.storePersistent(model.DB, staticKey, row.Version, revision, expiresAt)
				return revision, true
			}
		}
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
	authoritativeDNSSnapshotRevisionCache.persistentVersion = 0
	authoritativeDNSSnapshotRevisionCache.persistentVersionRecorded = false
	authoritativeDNSSnapshotRevisionCache.mu.Unlock()
	authoritativeDNSSnapshotPersistentRevisionDBs = sync.Map{}
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
	c.persistentVersion = 0
	c.persistentVersionRecorded = false
	c.mu.Unlock()
}

func (c *authoritativeDNSSnapshotRevisionCacheStore) getPersistent(db *gorm.DB, staticKey string, version uint64) (string, bool) {
	if c == nil || db == nil || strings.TrimSpace(staticKey) == "" {
		return "", false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.valid || c.db != db || c.staticKey != staticKey || c.revision == "" {
		return "", false
	}
	if !c.persistentVersionRecorded || c.persistentVersion != version {
		return "", false
	}
	if !c.expiresAt.IsZero() && !time.Now().Before(c.expiresAt) {
		return "", false
	}
	return c.revision, true
}

func (c *authoritativeDNSSnapshotRevisionCacheStore) storePersistent(db *gorm.DB, staticKey string, version uint64, revision string, expiresAt time.Time) {
	if c == nil || db == nil || strings.TrimSpace(staticKey) == "" || strings.TrimSpace(revision) == "" {
		return
	}
	c.mu.Lock()
	c.db = db
	c.staticKey = staticKey
	c.revision = revision
	c.expiresAt = expiresAt
	c.valid = true
	c.persistentVersion = version
	c.persistentVersionRecorded = true
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
		c.persistentVersion = 0
		c.persistentVersionRecorded = false
		return
	}
	c.eventSeq++
	seed := fmt.Sprintf("%p|%s|%s|%d|%d", c.db, c.staticKey, c.revision, c.eventSeq, time.Now().UTC().UnixNano())
	sum := sha256.Sum256([]byte(seed))
	c.revision = hex.EncodeToString(sum[:])[:24]
	c.persistentVersion = 0
	c.persistentVersionRecorded = false
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
	c.persistentVersion = 0
	c.persistentVersionRecorded = false
}

func (c *authoritativeDNSSnapshotRevisionCacheStore) invalidate() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.valid = false
	c.revision = ""
	c.expiresAt = time.Time{}
	c.persistentVersion = 0
	c.persistentVersionRecorded = false
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

type authoritativeDNSSnapshotPersistentRevisionRow struct {
	Version uint64
}

type authoritativeDNSSnapshotPersistentRevisionDependency struct {
	table         string
	updateColumns []string
}

func computeAuthoritativeDNSSnapshotPersistentCacheRevision(db *gorm.DB, staticKey string, row authoritativeDNSSnapshotPersistentRevisionRow) (string, time.Time, bool) {
	if db == nil || strings.TrimSpace(staticKey) == "" {
		return "", time.Time{}, false
	}
	now := time.Now().UTC()
	expiresAt, ok := computeAuthoritativeDNSSnapshotPersistentRevisionExpiresAt(db, now)
	if !ok {
		return "", time.Time{}, false
	}
	revision, ok := authoritativeDNSSnapshotPersistentCacheRevisionValue(staticKey, row.Version, expiresAt)
	if !ok {
		return "", time.Time{}, false
	}
	return revision, expiresAt, true
}

func authoritativeDNSSnapshotPersistentCacheRevisionValue(staticKey string, version uint64, expiresAt time.Time) (string, bool) {
	staticKey = strings.TrimSpace(staticKey)
	if staticKey == "" {
		return "", false
	}
	builder := strings.Builder{}
	builder.WriteString(staticKey)
	builder.WriteString("|persistent_revision:")
	builder.WriteString(strconv.FormatUint(version, 10))
	if !expiresAt.IsZero() {
		builder.WriteString("|expires_at:")
		builder.WriteString(expiresAt.UTC().Format(time.RFC3339Nano))
	}
	sum := sha256.Sum256([]byte(builder.String()))
	return hex.EncodeToString(sum[:])[:24], true
}

func loadAuthoritativeDNSSnapshotPersistentRevision(db *gorm.DB) (authoritativeDNSSnapshotPersistentRevisionRow, error) {
	var row authoritativeDNSSnapshotPersistentRevisionRow
	err := db.Table(authoritativeDNSSnapshotRevisionSentinelTable).
		Select("version").
		Where("revision_key = ?", authoritativeDNSSnapshotRevisionSentinelKey).
		Take(&row).Error
	return row, err
}

func ensureAuthoritativeDNSSnapshotPersistentRevision(db *gorm.DB) bool {
	if db == nil {
		return false
	}
	now := time.Now()
	if loaded, ok := authoritativeDNSSnapshotPersistentRevisionDBs.Load(db); ok {
		if state, _ := loaded.(*authoritativeDNSSnapshotPersistentRevisionInstallState); state != nil && now.Before(state.nextVerifyAt) {
			return true
		}
	}
	if err := installAuthoritativeDNSSnapshotPersistentRevision(db); err != nil {
		return false
	}
	authoritativeDNSSnapshotPersistentRevisionDBs.Store(db, &authoritativeDNSSnapshotPersistentRevisionInstallState{
		nextVerifyAt: now.Add(authoritativeDNSSnapshotPersistentRevisionVerifyInterval),
	})
	return true
}

func installAuthoritativeDNSSnapshotPersistentRevision(db *gorm.DB) error {
	backend := model.DatabaseDialectorName(db)
	switch backend {
	case "sqlite":
		return installAuthoritativeDNSSnapshotPersistentRevisionSQLite(db)
	case "postgres":
		return installAuthoritativeDNSSnapshotPersistentRevisionPostgres(db)
	default:
		return fmt.Errorf("unsupported database backend %s", backend)
	}
}

func installAuthoritativeDNSSnapshotPersistentRevisionSQLite(db *gorm.DB) error {
	table := authoritativeDNSSnapshotQuoteIdentifier(authoritativeDNSSnapshotRevisionSentinelTable)
	if err := db.Exec(fmt.Sprintf(
		`CREATE TABLE IF NOT EXISTS %s (`+
			`"revision_key" TEXT PRIMARY KEY, `+
			`"version" INTEGER NOT NULL DEFAULT 0, `+
			`"updated_at" TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP`+
			`)`,
		table,
	)).Error; err != nil {
		return err
	}
	if err := insertAuthoritativeDNSSnapshotPersistentRevisionSeed(db); err != nil {
		return err
	}
	dependencies, err := authoritativeDNSSnapshotPersistentRevisionDependencies(db)
	if err != nil {
		return err
	}
	for _, dependency := range dependencies {
		if err := installAuthoritativeDNSSnapshotPersistentRevisionSQLiteTriggers(db, dependency); err != nil {
			return err
		}
	}
	return nil
}

func installAuthoritativeDNSSnapshotPersistentRevisionPostgres(db *gorm.DB) error {
	table := authoritativeDNSSnapshotQuoteIdentifier(authoritativeDNSSnapshotRevisionSentinelTable)
	if err := db.Exec(fmt.Sprintf(
		`CREATE TABLE IF NOT EXISTS %s (`+
			`"revision_key" text PRIMARY KEY, `+
			`"version" bigint NOT NULL DEFAULT 0, `+
			`"updated_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP`+
			`)`,
		table,
	)).Error; err != nil {
		return err
	}
	if err := insertAuthoritativeDNSSnapshotPersistentRevisionSeed(db); err != nil {
		return err
	}
	if err := db.Exec(fmt.Sprintf(
		`CREATE OR REPLACE FUNCTION %s() RETURNS trigger AS $$`+
			`BEGIN `+
			`%s; `+
			`IF TG_OP = 'DELETE' THEN `+
			`RETURN OLD; `+
			`END IF; `+
			`RETURN NEW; `+
			`END;`+
			`$$ LANGUAGE plpgsql`,
		authoritativeDNSSnapshotQuoteIdentifier(authoritativeDNSSnapshotRevisionBumpFunction),
		authoritativeDNSSnapshotPersistentRevisionPostgresBumpSQL(),
	)).Error; err != nil {
		return err
	}
	dependencies, err := authoritativeDNSSnapshotPersistentRevisionDependencies(db)
	if err != nil {
		return err
	}
	for _, dependency := range dependencies {
		if err := installAuthoritativeDNSSnapshotPersistentRevisionPostgresTriggers(db, dependency); err != nil {
			return err
		}
	}
	return nil
}

func insertAuthoritativeDNSSnapshotPersistentRevisionSeed(db *gorm.DB) error {
	return db.Exec(fmt.Sprintf(
		`INSERT INTO %s ("revision_key", "version", "updated_at") `+
			`VALUES (?, 0, CURRENT_TIMESTAMP) `+
			`ON CONFLICT ("revision_key") DO NOTHING`,
		authoritativeDNSSnapshotQuoteIdentifier(authoritativeDNSSnapshotRevisionSentinelTable),
	), authoritativeDNSSnapshotRevisionSentinelKey).Error
}

func installAuthoritativeDNSSnapshotPersistentRevisionSQLiteTriggers(db *gorm.DB, dependency authoritativeDNSSnapshotPersistentRevisionDependency) error {
	operations := []string{"insert", "update", "delete"}
	for _, operation := range operations {
		triggerName := authoritativeDNSSnapshotRevisionTriggerName(dependency.table, operation)
		triggerOperation := strings.ToUpper(operation)
		if operation == "update" && len(dependency.updateColumns) > 0 {
			triggerOperation = "UPDATE OF " + authoritativeDNSSnapshotJoinQuotedIdentifiers(dependency.updateColumns)
		}
		if err := db.Exec(fmt.Sprintf(
			`DROP TRIGGER IF EXISTS %s`,
			authoritativeDNSSnapshotQuoteIdentifier(triggerName),
		)).Error; err != nil {
			return err
		}
		sql := fmt.Sprintf(
			`CREATE TRIGGER %s AFTER %s ON %s BEGIN %s; END`,
			authoritativeDNSSnapshotQuoteIdentifier(triggerName),
			triggerOperation,
			authoritativeDNSSnapshotQuoteIdentifier(dependency.table),
			authoritativeDNSSnapshotPersistentRevisionSQLiteBumpSQL(),
		)
		if err := db.Exec(sql).Error; err != nil {
			return err
		}
	}
	return nil
}

func installAuthoritativeDNSSnapshotPersistentRevisionPostgresTriggers(db *gorm.DB, dependency authoritativeDNSSnapshotPersistentRevisionDependency) error {
	operations := []string{"insert", "update", "delete"}
	for _, operation := range operations {
		triggerName := authoritativeDNSSnapshotRevisionTriggerName(dependency.table, operation)
		triggerOperation := strings.ToUpper(operation)
		if operation == "update" && len(dependency.updateColumns) > 0 {
			triggerOperation = "UPDATE OF " + authoritativeDNSSnapshotJoinQuotedIdentifiers(dependency.updateColumns)
		}
		sql := fmt.Sprintf(
			`DO $$ BEGIN `+
				`DROP TRIGGER IF EXISTS %s ON %s; `+
				`CREATE TRIGGER %s AFTER %s ON %s FOR EACH ROW EXECUTE FUNCTION %s(); `+
				`END $$`,
			authoritativeDNSSnapshotQuoteIdentifier(triggerName),
			authoritativeDNSSnapshotQuoteIdentifier(dependency.table),
			authoritativeDNSSnapshotQuoteIdentifier(triggerName),
			triggerOperation,
			authoritativeDNSSnapshotQuoteIdentifier(dependency.table),
			authoritativeDNSSnapshotQuoteIdentifier(authoritativeDNSSnapshotRevisionBumpFunction),
		)
		if err := db.Exec(sql).Error; err != nil {
			return err
		}
	}
	return nil
}

func authoritativeDNSSnapshotPersistentRevisionSQLiteBumpSQL() string {
	return fmt.Sprintf(
		`INSERT INTO %s ("revision_key", "version", "updated_at") `+
			`VALUES (%s, 1, CURRENT_TIMESTAMP) `+
			`ON CONFLICT ("revision_key") DO UPDATE SET `+
			`"version" = "version" + 1, `+
			`"updated_at" = CURRENT_TIMESTAMP`,
		authoritativeDNSSnapshotQuoteIdentifier(authoritativeDNSSnapshotRevisionSentinelTable),
		authoritativeDNSSnapshotSQLStringLiteral(authoritativeDNSSnapshotRevisionSentinelKey),
	)
}

func authoritativeDNSSnapshotPersistentRevisionPostgresBumpSQL() string {
	table := authoritativeDNSSnapshotQuoteIdentifier(authoritativeDNSSnapshotRevisionSentinelTable)
	return fmt.Sprintf(
		`INSERT INTO %s ("revision_key", "version", "updated_at") `+
			`VALUES (%s, 1, CURRENT_TIMESTAMP) `+
			`ON CONFLICT ("revision_key") DO UPDATE SET `+
			`"version" = %s."version" + 1, `+
			`"updated_at" = CURRENT_TIMESTAMP; `+
			`PERFORM pg_notify(%s, %s)`,
		table,
		authoritativeDNSSnapshotSQLStringLiteral(authoritativeDNSSnapshotRevisionSentinelKey),
		table,
		authoritativeDNSSnapshotSQLStringLiteral(authoritativeDNSSnapshotRevisionNotifyChannel),
		authoritativeDNSSnapshotSQLStringLiteral(authoritativeDNSSnapshotRevisionSentinelKey),
	)
}

func authoritativeDNSSnapshotPersistentRevisionDependencies(db *gorm.DB) ([]authoritativeDNSSnapshotPersistentRevisionDependency, error) {
	dependencies := []authoritativeDNSSnapshotPersistentRevisionDependency{
		{
			table: strings.ToLower((&model.DNSZone{}).TableName()),
			updateColumns: []string{
				"name", "soa_email", "primary_ns", "name_servers", "default_ttl", "serial",
				"dnssec_enabled", "dnssec_denial_mode", "dnssec_nsec3_salt", "dnssec_nsec3_iterations",
				"dnssec_signature_validity", "enabled", "updated_at",
			},
		},
		{
			table: strings.ToLower((&model.DNSRecord{}).TableName()),
			updateColumns: []string{
				"zone_id", "name", "type", "value", "ttl", "priority", "enabled", "updated_at",
			},
		},
		{
			table: strings.ToLower((&model.DNSSECKey{}).TableName()),
			updateColumns: []string{
				"zone_id", "role", "flags", "algorithm", "public_key", "encrypted_private_key",
				"key_tag", "ds_digest_sha256", "status", "activated_at", "retired_at", "updated_at",
			},
		},
		{
			table:         strings.ToLower((&model.DNSZoneWorkerAssignment{}).TableName()),
			updateColumns: []string{"zone_id", "worker_id", "updated_at"},
		},
		{
			table: strings.ToLower((&model.Node{}).TableName()),
			updateColumns: []string{
				"node_id", "name", "ip", "pool_name", "weight", "public_ips", "scheduling_enabled",
				"drain_mode", "openresty_status", "status", "last_seen_at",
			},
		},
		{
			table: strings.ToLower((&model.ProxyRoute{}).TableName()),
			updateColumns: []string{
				"site_name", "domain", "domains", "node_pool", "enabled", "dns_provider_mode",
				"dns_zone_id_ref", "dns_record_type", "dns_target_count", "dns_schedule_mode",
				"dns_ttl", "gslb_enabled", "gslb_policy", "ddos_protection_mode",
				"ddos_protection_provider", "ddos_protection_target", "updated_at",
			},
		},
		{
			table: strings.ToLower((&model.ProxySite{}).TableName()),
			updateColumns: []string{
				"proxy_route_id", "name", "node_pool", "enabled", "updated_at",
			},
		},
		{
			table: strings.ToLower((&model.ProxySiteDomain{}).TableName()),
			updateColumns: []string{
				"proxy_site_id", "proxy_route_id", "domain", "is_primary", "sort_order", "updated_at",
			},
		},
		{
			table: strings.ToLower((&model.DNSBinding{}).TableName()),
			updateColumns: []string{
				"proxy_route_id", "dns_record_type", "dns_auto_target", "dns_target_count",
				"dns_schedule_mode", "dns_ttl", "dns_provider_mode", "dns_zone_id_ref",
				"gslb_enabled", "gslb_policy_json", "updated_at",
			},
		},
		{
			table: strings.ToLower((&model.SecurityPolicy{}).TableName()),
			updateColumns: []string{
				"proxy_route_id", "is_default", "ddos_protection_mode",
				"ddos_protection_provider", "ddos_protection_target", "updated_at",
			},
		},
		{
			table: strings.ToLower((&model.GSLBSchedulingState{}).TableName()),
			updateColumns: []string{
				"proxy_route_id", "dns_record_type", "scope_key", "selected_targets", "desired_targets",
				"unhealthy_count", "recovery_count", "last_reason", "last_changed_at",
				"last_evaluated_at", "created_at", "updated_at",
			},
		},
		{
			table: strings.ToLower((&model.DNSWorkerNodeProbe{}).TableName()),
			updateColumns: []string{
				"worker_id", "node_id", "public_address", "query_name", "query_type", "checked_at",
				"results_json", "healthy", "average_rtt_ms", "max_rtt_ms", "last_error",
				"failure_samples", "created_at", "updated_at",
			},
		},
	}
	metricTables, err := authoritativeDNSSnapshotPhysicalTablesWithPrefix(db, strings.ToLower((&model.NodeMetricSnapshot{}).TableName()))
	if err != nil {
		return nil, err
	}
	for _, table := range metricTables {
		dependencies = append(dependencies, authoritativeDNSSnapshotPersistentRevisionDependency{
			table: table,
			updateColumns: []string{
				"node_id", "captured_at", "cpu_usage_percent", "memory_used_bytes",
				"memory_total_bytes", "openresty_connections", "created_at",
			},
		})
	}
	filtered := make([]authoritativeDNSSnapshotPersistentRevisionDependency, 0, len(dependencies))
	for _, dependency := range dependencies {
		if dependency.table == "" || !db.Migrator().HasTable(dependency.table) {
			continue
		}
		filtered = append(filtered, dependency)
	}
	return filtered, nil
}

func authoritativeDNSSnapshotPhysicalTablesWithPrefix(db *gorm.DB, prefix string) ([]string, error) {
	tables, err := db.Migrator().GetTables()
	if err != nil {
		return nil, err
	}
	prefix = strings.ToLower(strings.TrimSpace(prefix))
	result := make([]string, 0)
	seen := map[string]struct{}{}
	for _, table := range tables {
		normalized := strings.ToLower(strings.TrimSpace(table))
		if normalized != prefix && !strings.HasPrefix(normalized, prefix+"_") {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}
	sort.Strings(result)
	return result, nil
}

func computeAuthoritativeDNSSnapshotPersistentRevisionExpiresAt(db *gorm.DB, now time.Time) (time.Time, bool) {
	expiresAt := time.Time{}
	nodeExpiresAt, err := authoritativeDNSSnapshotNodeStatusRevisionExpiresAt(db, now)
	if err != nil {
		return time.Time{}, false
	}
	expiresAt = earlierFutureTime(now, expiresAt, nodeExpiresAt)
	metricExpiresAt, err := authoritativeDNSSnapshotMetricRevisionExpiresAt(db, now)
	if err != nil {
		return time.Time{}, false
	}
	expiresAt = earlierFutureTime(now, expiresAt, metricExpiresAt)
	if common.GSLBProbeSchedulingEnabled {
		probeExpiresAt, err := authoritativeDNSSnapshotProbeRevisionExpiresAt(db, now)
		if err != nil {
			return time.Time{}, false
		}
		expiresAt = earlierFutureTime(now, expiresAt, probeExpiresAt)
	}
	return expiresAt, true
}

func authoritativeDNSSnapshotNodeStatusRevisionExpiresAt(db *gorm.DB, now time.Time) (time.Time, error) {
	threshold := common.NodeOfflineThreshold
	if threshold <= 0 {
		return time.Time{}, nil
	}
	type nodeStatusExpiryRow struct {
		LastSeenAt time.Time
	}
	var rows []nodeStatusExpiryRow
	query := db.Model(&model.Node{}).
		Select("last_seen_at").
		Where("last_seen_at > ?", now.Add(-threshold)).
		Order("last_seen_at asc").
		Limit(1)
	if connectedNodes := connectedAgentWSNodeIDsForSnapshotRevision(); len(connectedNodes) > 0 {
		query = query.Where("node_id NOT IN ?", connectedNodes)
	}
	if err := query.Find(&rows).Error; err != nil {
		return time.Time{}, err
	}
	if len(rows) == 0 || rows[0].LastSeenAt.IsZero() {
		return time.Time{}, nil
	}
	return rows[0].LastSeenAt.UTC().Add(threshold), nil
}

func authoritativeDNSSnapshotMetricRevisionExpiresAt(db *gorm.DB, now time.Time) (time.Time, error) {
	freshness := time.Duration(common.GSLBMetricFreshnessSeconds) * time.Second
	if freshness <= 0 {
		freshness = 120 * time.Second
	}
	tables, err := authoritativeDNSSnapshotPhysicalTablesWithPrefix(db, strings.ToLower((&model.NodeMetricSnapshot{}).TableName()))
	if err != nil {
		return time.Time{}, err
	}
	if len(tables) == 0 {
		return time.Time{}, nil
	}
	since := now.Add(-freshness)
	expiresAt := time.Time{}
	type metricExpiryRow struct {
		CapturedAt time.Time
	}
	for _, table := range tables {
		var rows []metricExpiryRow
		if err := db.Table(table).
			Select("captured_at").
			Where("captured_at >= ? AND captured_at <= ?", since, now).
			Order("captured_at asc").
			Limit(1).
			Find(&rows).Error; err != nil {
			return time.Time{}, err
		}
		if len(rows) == 0 || rows[0].CapturedAt.IsZero() {
			continue
		}
		expiresAt = earlierFutureTime(now, expiresAt, rows[0].CapturedAt.UTC().Add(freshness))
	}
	return expiresAt, nil
}

func authoritativeDNSSnapshotProbeRevisionExpiresAt(db *gorm.DB, now time.Time) (time.Time, error) {
	type probeExpiryRow struct {
		CheckedAt time.Time
		CreatedAt time.Time
		UpdatedAt time.Time
	}
	var rows []probeExpiryRow
	if err := db.Model(&model.DNSWorkerNodeProbe{}).
		Select([]string{"checked_at", "created_at", "updated_at"}).
		Find(&rows).Error; err != nil {
		return time.Time{}, err
	}
	expiresAt := time.Time{}
	for index := range rows {
		checkedAt := normalizeDNSWorkerCheckedAt(&rows[index].CheckedAt, now, rows[index].UpdatedAt, rows[index].CreatedAt)
		expiresAt = earlierFutureTime(now, expiresAt, checkedAt.Add(defaultDNSWorkerNodeProbeMaxAge))
	}
	return expiresAt, nil
}

func authoritativeDNSSnapshotRevisionTriggerName(table string, operation string) string {
	name := "ds_adns_rev_" + strings.ToLower(strings.TrimSpace(table)) + "_" + strings.ToLower(strings.TrimSpace(operation))
	if len(name) <= 60 {
		return name
	}
	sum := sha256.Sum256([]byte(name))
	suffix := hex.EncodeToString(sum[:])[:10]
	return name[:49] + "_" + suffix
}

func authoritativeDNSSnapshotJoinQuotedIdentifiers(values []string) string {
	quoted := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		quoted = append(quoted, authoritativeDNSSnapshotQuoteIdentifier(value))
	}
	return strings.Join(quoted, ", ")
}

func authoritativeDNSSnapshotQuoteIdentifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

func authoritativeDNSSnapshotSQLStringLiteral(value string) string {
	return `'` + strings.ReplaceAll(value, `'`, `''`) + `'`
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
		if authoritativeDNSSnapshotRevisionStatementMutationKind(tx) != authoritativeDNSSnapshotRevisionMutationNone {
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
		strings.ToLower((&model.ProxySite{}).TableName()),
		strings.ToLower((&model.ProxySiteDomain{}).TableName()),
		strings.ToLower((&model.DNSBinding{}).TableName()),
		strings.ToLower((&model.SecurityPolicy{}).TableName()),
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
	CreatedAt       time.Time
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
			"created_at",
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
