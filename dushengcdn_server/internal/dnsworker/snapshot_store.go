package dnsworker

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	persistedSnapshotFormat        = "dushengcdn.dns_worker.snapshot"
	persistedSnapshotFormatVersion = 1
)

type SnapshotStore struct {
	path      string
	mu        sync.RWMutex
	snapshot  *Snapshot
	loadedAt  time.Time
	lastError string
	maxAge    time.Duration
	index     snapshotIndex
}

type snapshotIndex struct {
	zonesByID         map[uint]*SnapshotZone
	zonesByName       map[string]*SnapshotZone
	routesByDomain    map[string][]*SnapshotRoute
	recordsByNameType map[recordKey][]SnapshotRecord
	namesByZone       map[uint]map[string]struct{}
}

type recordKey struct {
	ZoneID uint
	Name   string
	Type   string
}

type persistedSnapshotEnvelope struct {
	Format        string    `json:"format"`
	FormatVersion int       `json:"format_version"`
	SavedAt       time.Time `json:"saved_at"`
	Checksum      string    `json:"checksum"`
	Snapshot      *Snapshot `json:"snapshot"`
}

func NewSnapshotStore(path string, maxAge time.Duration) *SnapshotStore {
	if maxAge <= 0 {
		maxAge = DefaultSnapshotMaxAge
	}
	return &SnapshotStore{
		path:   path,
		maxAge: maxAge,
	}
}

func (s *SnapshotStore) LoadFromDisk() error {
	if s == nil || strings.TrimSpace(s.path) == "" {
		return nil
	}
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		s.setLastError(err)
		return err
	}
	if snapshot, ok, err := decodePersistedSnapshotEnvelope(raw); ok {
		if err != nil {
			s.setLastError(err)
			return err
		}
		if err := s.Set(snapshot); err != nil {
			s.setLastError(err)
			return err
		}
		return nil
	}
	var snapshot Snapshot
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		s.setLastError(err)
		return err
	}
	if err := s.Set(&snapshot); err != nil {
		s.setLastError(err)
		return err
	}
	return nil
}

func (s *SnapshotStore) Set(snapshot *Snapshot) error {
	if s == nil {
		return errors.New("snapshot store is nil")
	}
	if snapshot == nil || strings.TrimSpace(snapshot.SnapshotVersion) == "" {
		return errors.New("snapshot is invalid")
	}
	normalized := normalizeSnapshot(snapshot)
	normalizeSnapshotRuntimeTimes(normalized, time.Now().UTC())
	index := buildSnapshotIndex(normalized)
	s.mu.Lock()
	s.snapshot = normalized
	s.index = index
	s.loadedAt = time.Now().UTC()
	s.lastError = ""
	s.mu.Unlock()
	return nil
}

func (s *SnapshotStore) Save(snapshot *Snapshot) error {
	if s == nil || strings.TrimSpace(s.path) == "" || snapshot == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		s.setLastError(err)
		return err
	}
	normalized, err := prepareSnapshotForPersistence(snapshot)
	if err != nil {
		s.setLastError(err)
		return err
	}
	normalizeSnapshotRuntimeTimes(normalized, time.Now().UTC())
	checksum, err := checksumSnapshot(normalized)
	if err != nil {
		s.setLastError(err)
		return err
	}
	envelope := persistedSnapshotEnvelope{
		Format:        persistedSnapshotFormat,
		FormatVersion: persistedSnapshotFormatVersion,
		SavedAt:       time.Now().UTC(),
		Checksum:      checksum,
		Snapshot:      normalized,
	}
	raw, err := json.MarshalIndent(envelope, "", "  ")
	if err != nil {
		s.setLastError(err)
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		s.setLastError(err)
		return err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		if removeErr := os.Remove(s.path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			s.setLastError(err)
			return err
		}
		if retryErr := os.Rename(tmp, s.path); retryErr != nil {
			s.setLastError(retryErr)
			return retryErr
		}
	}
	return nil
}

func (s *SnapshotStore) Current() (*Snapshot, snapshotIndex, time.Time, string) {
	if s == nil {
		return nil, snapshotIndex{}, time.Time{}, ""
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snapshot, s.index, s.loadedAt, s.lastError
}

func (s *SnapshotStore) Version() string {
	snapshot, _, _, _ := s.Current()
	if snapshot == nil {
		return ""
	}
	return snapshot.SnapshotVersion
}

func (s *SnapshotStore) LoadedAt() *time.Time {
	_, _, loadedAt, _ := s.Current()
	if loadedAt.IsZero() {
		return nil
	}
	return &loadedAt
}

func (s *SnapshotStore) IsFresh(now time.Time) bool {
	snapshot, _, _, _ := s.Current()
	if snapshot == nil || snapshot.GeneratedAt.IsZero() {
		return false
	}
	return now.Sub(snapshot.GeneratedAt) <= s.maxAge
}

func (s *SnapshotStore) LastError() string {
	_, _, _, lastError := s.Current()
	return lastError
}

func (s *SnapshotStore) SetLastError(err error) {
	s.setLastError(err)
}

func (s *SnapshotStore) setLastError(err error) {
	if s == nil || err == nil {
		return
	}
	s.mu.Lock()
	s.lastError = err.Error()
	s.mu.Unlock()
}

func decodePersistedSnapshotEnvelope(raw []byte) (*Snapshot, bool, error) {
	var envelope persistedSnapshotEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, false, nil
	}
	if envelope.Snapshot == nil && strings.TrimSpace(envelope.Format) == "" && strings.TrimSpace(envelope.Checksum) == "" {
		return nil, false, nil
	}
	if strings.TrimSpace(envelope.Format) != "" && envelope.Format != persistedSnapshotFormat {
		return nil, true, fmt.Errorf("unsupported snapshot cache format %q", envelope.Format)
	}
	if envelope.FormatVersion != 0 && envelope.FormatVersion != persistedSnapshotFormatVersion {
		return nil, true, fmt.Errorf("unsupported snapshot cache format version %d", envelope.FormatVersion)
	}
	if envelope.Snapshot == nil {
		return nil, true, errors.New("snapshot cache envelope is missing snapshot")
	}
	if strings.TrimSpace(envelope.Checksum) == "" {
		return nil, true, errors.New("snapshot cache envelope is missing checksum")
	}
	expected, err := checksumSnapshot(envelope.Snapshot)
	if err != nil {
		return nil, true, err
	}
	if !strings.EqualFold(strings.TrimSpace(envelope.Checksum), expected) {
		return nil, true, fmt.Errorf("snapshot checksum mismatch: expected %s", expected)
	}
	return envelope.Snapshot, true, nil
}

func prepareSnapshotForPersistence(snapshot *Snapshot) (*Snapshot, error) {
	if snapshot == nil || strings.TrimSpace(snapshot.SnapshotVersion) == "" {
		return nil, errors.New("snapshot is invalid")
	}
	return normalizeSnapshot(snapshot), nil
}

func checksumSnapshot(snapshot *Snapshot) (string, error) {
	normalized, err := prepareSnapshotForPersistence(snapshot)
	if err != nil {
		return "", err
	}
	raw, err := json.Marshal(normalized)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return fmt.Sprintf("sha256:%x", sum), nil
}

func normalizeSnapshot(input *Snapshot) *Snapshot {
	out := *input
	out.Zones = append([]SnapshotZone(nil), input.Zones...)
	out.Routes = append([]SnapshotRoute(nil), input.Routes...)
	out.Nodes = append([]SnapshotNode(nil), input.Nodes...)
	out.SchedulingStates = append([]SnapshotSchedulingState(nil), input.SchedulingStates...)
	for i := range out.Zones {
		out.Zones[i].Name = normalizeDomain(out.Zones[i].Name)
		out.Zones[i].PrimaryNS = normalizeDomain(out.Zones[i].PrimaryNS)
		out.Zones[i].DefaultTTL = normalizeStaticTTL(out.Zones[i].DefaultTTL, DefaultZoneTTL)
		out.Zones[i].NameServers = normalizeDomainList(out.Zones[i].NameServers)
		out.Zones[i].Records = append([]SnapshotRecord(nil), out.Zones[i].Records...)
		for j := range out.Zones[i].Records {
			out.Zones[i].Records[j].Name = normalizeRecordName(out.Zones[i].Name, out.Zones[i].Records[j].Name)
			out.Zones[i].Records[j].Type = normalizeRecordType(out.Zones[i].Records[j].Type)
			out.Zones[i].Records[j].TTL = normalizeStaticTTL(out.Zones[i].Records[j].TTL, out.Zones[i].DefaultTTL)
			if out.Zones[i].Records[j].Type == "CNAME" || out.Zones[i].Records[j].Type == "MX" || out.Zones[i].Records[j].Type == "NS" {
				out.Zones[i].Records[j].Value = normalizeDomain(out.Zones[i].Records[j].Value)
			}
		}
	}
	for i := range out.Routes {
		out.Routes[i].Domains = normalizeDomainList(out.Routes[i].Domains)
		out.Routes[i].NodePool = normalizeNodePoolName(out.Routes[i].NodePool)
		out.Routes[i].RecordType = normalizeAddressRecordType(out.Routes[i].RecordType)
		out.Routes[i].TargetCount = normalizeTargetCount(out.Routes[i].TargetCount)
		out.Routes[i].ScheduleMode = normalizeStrategy(out.Routes[i].ScheduleMode)
		out.Routes[i].TTL = normalizeAuthoritativeTTL(out.Routes[i].TTL)
		out.Routes[i].GSLBPolicy = normalizePolicy(out.Routes[i].GSLBPolicy, out.Routes[i])
		out.Routes[i].CurrentTargets = normalizeIPList(out.Routes[i].CurrentTargets, out.Routes[i].RecordType)
	}
	for i := range out.Nodes {
		out.Nodes[i].PoolName = normalizeNodePoolName(out.Nodes[i].PoolName)
		out.Nodes[i].PublicIPs = normalizeIPList(out.Nodes[i].PublicIPs, "")
		out.Nodes[i].Weight = normalizeWeight(out.Nodes[i].Weight)
		out.Nodes[i].Status = strings.ToLower(strings.TrimSpace(out.Nodes[i].Status))
		out.Nodes[i].OpenrestyStatus = strings.ToLower(strings.TrimSpace(out.Nodes[i].OpenrestyStatus))
	}
	states := make([]SnapshotSchedulingState, 0, len(out.SchedulingStates))
	for _, state := range out.SchedulingStates {
		state.RecordType = normalizeAddressRecordType(state.RecordType)
		state.ScopeKey = normalizeSourceScope(state.ScopeKey)
		state.SelectedTargets = normalizeIPList(state.SelectedTargets, state.RecordType)
		state.DesiredTargets = normalizeIPList(state.DesiredTargets, state.RecordType)
		if state.RouteID == 0 || len(state.SelectedTargets) == 0 {
			continue
		}
		states = append(states, state)
	}
	out.SchedulingStates = states
	return &out
}

func normalizeSnapshotRuntimeTimes(snapshot *Snapshot, now time.Time) {
	if snapshot == nil {
		return
	}
	normalizedNow := now.UTC()
	if snapshot.GeneratedAt.After(normalizedNow) {
		snapshot.GeneratedAt = normalizedNow
	}
	for i := range snapshot.SchedulingStates {
		snapshot.SchedulingStates[i].LastChangedAt = normalizeSnapshotSchedulingStateChangedAt(snapshot.SchedulingStates[i].LastChangedAt, normalizedNow, snapshot.GeneratedAt)
	}
}

func normalizeSnapshotSchedulingStateChangedAt(changedAt *time.Time, now time.Time, generatedAt time.Time) *time.Time {
	if changedAt == nil {
		return nil
	}
	normalizedNow := now.UTC()
	normalized := changedAt.UTC()
	if !normalized.After(normalizedNow) {
		return &normalized
	}
	if !generatedAt.IsZero() {
		generated := generatedAt.UTC()
		if !generated.After(normalizedNow) {
			return &generated
		}
	}
	normalized = normalizedNow
	return &normalized
}

func buildSnapshotIndex(snapshot *Snapshot) snapshotIndex {
	index := snapshotIndex{
		zonesByID:         map[uint]*SnapshotZone{},
		zonesByName:       map[string]*SnapshotZone{},
		routesByDomain:    map[string][]*SnapshotRoute{},
		recordsByNameType: map[recordKey][]SnapshotRecord{},
		namesByZone:       map[uint]map[string]struct{}{},
	}
	if snapshot == nil {
		return index
	}
	for i := range snapshot.Zones {
		zone := &snapshot.Zones[i]
		index.zonesByID[zone.ID] = zone
		index.zonesByName[zone.Name] = zone
		index.namesByZone[zone.ID] = map[string]struct{}{zone.Name: {}}
		for _, record := range zone.Records {
			key := recordKey{
				ZoneID: zone.ID,
				Name:   record.Name,
				Type:   record.Type,
			}
			index.recordsByNameType[key] = append(index.recordsByNameType[key], record)
			index.namesByZone[zone.ID][record.Name] = struct{}{}
		}
	}
	for i := range snapshot.Routes {
		route := &snapshot.Routes[i]
		for _, domain := range route.Domains {
			domain = normalizeDomain(domain)
			if domain == "" {
				continue
			}
			index.routesByDomain[domain] = append(index.routesByDomain[domain], route)
			if route.ZoneID != 0 {
				if _, ok := index.namesByZone[route.ZoneID]; !ok {
					index.namesByZone[route.ZoneID] = map[string]struct{}{}
				}
				index.namesByZone[route.ZoneID][domain] = struct{}{}
			}
		}
	}
	return index
}
