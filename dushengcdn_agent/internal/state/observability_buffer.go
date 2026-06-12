package state

import (
	"bytes"
	"dushengcdn-agent/internal/fileutil"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"

	"dushengcdn-agent/internal/protocol"
)

const observabilityBufferWindowSeconds = 60

type ObservabilityBufferRecord struct {
	WindowStartedAtUnix int64                        `json:"window_started_at_unix"`
	Snapshot            *protocol.NodeMetricSnapshot `json:"snapshot,omitempty"`
	TrafficReport       *protocol.NodeTrafficReport  `json:"traffic_report,omitempty"`
	AccessLogs          []protocol.NodeAccessLog     `json:"access_logs,omitempty"`
	QueuedAtUnix        int64                        `json:"queued_at_unix"`
}

type ObservabilityBufferStore struct {
	path             string
	mu               sync.Mutex
	loaded           bool
	cache            []ObservabilityBufferRecord
	encoded          []byte
	encodedCanonical bool
}

func NewObservabilityBufferStore(path string) *ObservabilityBufferStore {
	return &ObservabilityBufferStore{path: filepath.Clean(path)}
}

func (s *ObservabilityBufferStore) Upsert(record ObservabilityBufferRecord, retainAfterUnix int64) error {
	if s == nil || record.WindowStartedAtUnix <= 0 || (record.Snapshot == nil && record.TrafficReport == nil && len(record.AccessLogs) == 0) {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	records, err := s.loadUnlocked()
	if err != nil {
		return err
	}
	records = upsertObservabilityBufferRecord(records, record, retainAfterUnix)
	return s.saveUnlocked(records)
}

func (s *ObservabilityBufferStore) UpsertAndReplayable(record ObservabilityBufferRecord, currentWindowStartedAtUnix int64, retainAfterUnix int64) ([]ObservabilityBufferRecord, error) {
	if s == nil {
		return nil, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	records, err := s.loadUnlocked()
	if err != nil {
		return nil, err
	}
	records = upsertObservabilityBufferRecord(records, record, retainAfterUnix)
	if err = s.saveUnlocked(records); err != nil {
		return nil, err
	}
	return replayableObservabilityBufferRecords(records, currentWindowStartedAtUnix), nil
}

func upsertObservabilityBufferRecord(records []ObservabilityBufferRecord, record ObservabilityBufferRecord, retainAfterUnix int64) []ObservabilityBufferRecord {
	records = pruneObservabilityBufferRecords(records, retainAfterUnix)
	if record.WindowStartedAtUnix <= 0 || (record.Snapshot == nil && record.TrafficReport == nil && len(record.AccessLogs) == 0) {
		return records
	}
	replaced := false
	for index := range records {
		if records[index].WindowStartedAtUnix != record.WindowStartedAtUnix {
			continue
		}
		records[index] = mergeObservabilityBufferRecord(records[index], record)
		replaced = true
		break
	}
	if !replaced {
		records = append(records, cloneObservabilityBufferRecord(record))
	}
	sort.Slice(records, func(i int, j int) bool {
		return records[i].WindowStartedAtUnix < records[j].WindowStartedAtUnix
	})
	return records
}

func mergeObservabilityBufferRecord(existing ObservabilityBufferRecord, incoming ObservabilityBufferRecord) ObservabilityBufferRecord {
	merged := existing
	if incoming.Snapshot != nil {
		merged.Snapshot = incoming.Snapshot
	}
	if incoming.TrafficReport != nil {
		merged.TrafficReport = incoming.TrafficReport
	}
	merged.AccessLogs = mergeAccessLogs(existing.AccessLogs, incoming.AccessLogs)
	if incoming.QueuedAtUnix > 0 {
		merged.QueuedAtUnix = incoming.QueuedAtUnix
	}
	return merged
}

func mergeAccessLogs(existing []protocol.NodeAccessLog, incoming []protocol.NodeAccessLog) []protocol.NodeAccessLog {
	if len(existing) == 0 && len(incoming) == 0 {
		return nil
	}
	merged := make([]protocol.NodeAccessLog, 0, len(existing)+len(incoming))
	seen := make(map[string]struct{}, len(existing)+len(incoming))
	appendIfNeeded := func(items []protocol.NodeAccessLog) {
		for _, item := range items {
			key := accessLogKey(item)
			if key == "" {
				continue
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			merged = append(merged, item)
		}
	}
	appendIfNeeded(existing)
	appendIfNeeded(incoming)
	sort.Slice(merged, func(i int, j int) bool {
		if merged[i].LoggedAtUnix == merged[j].LoggedAtUnix {
			return accessLogKey(merged[i]) < accessLogKey(merged[j])
		}
		return merged[i].LoggedAtUnix < merged[j].LoggedAtUnix
	})
	return merged
}

func accessLogKey(item protocol.NodeAccessLog) string {
	return strconv.FormatInt(item.LoggedAtUnix, 10) + "|" + item.RemoteAddr + "|" + item.Host + "|" + item.Path + "|" + strconv.Itoa(item.StatusCode) + "|" + strconv.FormatInt(item.ResponseBytes, 10)
}

func (s *ObservabilityBufferStore) Replayable(currentWindowStartedAtUnix int64, retainAfterUnix int64) ([]ObservabilityBufferRecord, error) {
	if s == nil {
		return nil, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	records, err := s.loadUnlocked()
	if err != nil {
		return nil, err
	}
	records = pruneObservabilityBufferRecords(records, retainAfterUnix)
	if err = s.saveUnlocked(records); err != nil {
		return nil, err
	}
	return replayableObservabilityBufferRecords(records, currentWindowStartedAtUnix), nil
}

func (s *ObservabilityBufferStore) Ack(windowStartedAtUnix []int64, retainAfterUnix int64) error {
	if s == nil || len(windowStartedAtUnix) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	records, err := s.loadUnlocked()
	if err != nil {
		return err
	}
	acked := make(map[int64]struct{}, len(windowStartedAtUnix))
	for _, value := range windowStartedAtUnix {
		if value > 0 {
			acked[value] = struct{}{}
		}
	}
	filtered := make([]ObservabilityBufferRecord, 0, len(records))
	for _, record := range records {
		if _, ok := acked[record.WindowStartedAtUnix]; ok {
			continue
		}
		filtered = append(filtered, record)
	}
	filtered = pruneObservabilityBufferRecords(filtered, retainAfterUnix)
	return s.saveUnlocked(filtered)
}

func (s *ObservabilityBufferStore) loadUnlocked() ([]ObservabilityBufferRecord, error) {
	if s.loaded {
		return cloneObservabilityBufferRecords(s.cache), nil
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			s.cache = []ObservabilityBufferRecord{}
			s.encoded = nil
			s.encodedCanonical = true
			s.loaded = true
			return []ObservabilityBufferRecord{}, nil
		}
		return nil, err
	}
	if len(data) == 0 {
		s.cache = []ObservabilityBufferRecord{}
		s.encoded = append([]byte(nil), data...)
		s.encodedCanonical = false
		s.loaded = true
		return []ObservabilityBufferRecord{}, nil
	}
	var records []ObservabilityBufferRecord
	if err = json.Unmarshal(data, &records); err != nil {
		return nil, err
	}
	if records == nil {
		records = []ObservabilityBufferRecord{}
	}
	s.cache = cloneObservabilityBufferRecords(records)
	s.encoded = append([]byte(nil), data...)
	s.encodedCanonical = isCanonicalObservabilityBufferEncoding(records, data)
	s.loaded = true
	return cloneObservabilityBufferRecords(records), nil
}

func (s *ObservabilityBufferStore) saveUnlocked(records []ObservabilityBufferRecord) error {
	normalized := cloneObservabilityBufferRecords(records)
	if s.loaded && s.encodedCanonical && observabilityBufferRecordsEqual(s.cache, normalized) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(normalized)
	if err != nil {
		return err
	}
	if s.loaded && bytes.Equal(s.encoded, data) {
		s.cache = cloneObservabilityBufferRecords(normalized)
		s.encodedCanonical = true
		return nil
	}
	if err = fileutil.WriteFileAtomicIfChanged(s.path, data, 0o644); err != nil {
		return err
	}
	s.cache = cloneObservabilityBufferRecords(normalized)
	s.encoded = append([]byte(nil), data...)
	s.encodedCanonical = true
	s.loaded = true
	return nil
}

func ObservabilityWindowStartedAt(snapshot *protocol.NodeMetricSnapshot, traffic *protocol.NodeTrafficReport) int64 {
	if traffic != nil && traffic.WindowStartedAtUnix > 0 {
		return traffic.WindowStartedAtUnix - (traffic.WindowStartedAtUnix % observabilityBufferWindowSeconds)
	}
	if snapshot == nil || snapshot.CapturedAtUnix <= 0 {
		return 0
	}
	return snapshot.CapturedAtUnix - (snapshot.CapturedAtUnix % observabilityBufferWindowSeconds)
}

func pruneObservabilityBufferRecords(records []ObservabilityBufferRecord, retainAfterUnix int64) []ObservabilityBufferRecord {
	if len(records) == 0 {
		return []ObservabilityBufferRecord{}
	}
	filtered := make([]ObservabilityBufferRecord, 0, len(records))
	for _, record := range records {
		if record.WindowStartedAtUnix <= 0 {
			continue
		}
		if retainAfterUnix > 0 && record.WindowStartedAtUnix < retainAfterUnix {
			continue
		}
		filtered = append(filtered, record)
	}
	sort.Slice(filtered, func(i int, j int) bool {
		return filtered[i].WindowStartedAtUnix < filtered[j].WindowStartedAtUnix
	})
	return filtered
}

func replayableObservabilityBufferRecords(records []ObservabilityBufferRecord, currentWindowStartedAtUnix int64) []ObservabilityBufferRecord {
	result := make([]ObservabilityBufferRecord, 0, len(records))
	for _, record := range records {
		if currentWindowStartedAtUnix > 0 && record.WindowStartedAtUnix >= currentWindowStartedAtUnix {
			continue
		}
		result = append(result, cloneObservabilityBufferRecord(record))
	}
	return result
}

func isCanonicalObservabilityBufferEncoding(records []ObservabilityBufferRecord, data []byte) bool {
	encoded, err := json.Marshal(records)
	if err != nil {
		return false
	}
	return bytes.Equal(encoded, data)
}

func cloneObservabilityBufferRecords(records []ObservabilityBufferRecord) []ObservabilityBufferRecord {
	if len(records) == 0 {
		return []ObservabilityBufferRecord{}
	}
	copied := make([]ObservabilityBufferRecord, len(records))
	for index := range records {
		copied[index] = cloneObservabilityBufferRecord(records[index])
	}
	return copied
}

func cloneObservabilityBufferRecord(record ObservabilityBufferRecord) ObservabilityBufferRecord {
	copied := record
	if record.Snapshot != nil {
		snapshot := *record.Snapshot
		copied.Snapshot = &snapshot
	}
	if record.TrafficReport != nil {
		report := *record.TrafficReport
		report.StatusCodes = cloneInt64Map(record.TrafficReport.StatusCodes)
		report.TopDomains = cloneInt64Map(record.TrafficReport.TopDomains)
		report.SourceCountries = cloneInt64Map(record.TrafficReport.SourceCountries)
		copied.TrafficReport = &report
	}
	copied.AccessLogs = append([]protocol.NodeAccessLog(nil), record.AccessLogs...)
	return copied
}

func cloneInt64Map(values map[string]int64) map[string]int64 {
	if values == nil {
		return nil
	}
	copied := make(map[string]int64, len(values))
	for key, value := range values {
		copied[key] = value
	}
	return copied
}

func observabilityBufferRecordsEqual(left []ObservabilityBufferRecord, right []ObservabilityBufferRecord) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if !observabilityBufferRecordEqual(left[index], right[index]) {
			return false
		}
	}
	return true
}

func observabilityBufferRecordEqual(left ObservabilityBufferRecord, right ObservabilityBufferRecord) bool {
	if left.WindowStartedAtUnix != right.WindowStartedAtUnix || left.QueuedAtUnix != right.QueuedAtUnix {
		return false
	}
	if !metricSnapshotsEqual(left.Snapshot, right.Snapshot) || !trafficReportsEqual(left.TrafficReport, right.TrafficReport) {
		return false
	}
	if len(left.AccessLogs) != len(right.AccessLogs) {
		return false
	}
	for index := range left.AccessLogs {
		if left.AccessLogs[index] != right.AccessLogs[index] {
			return false
		}
	}
	return true
}

func metricSnapshotsEqual(left *protocol.NodeMetricSnapshot, right *protocol.NodeMetricSnapshot) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func trafficReportsEqual(left *protocol.NodeTrafficReport, right *protocol.NodeTrafficReport) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.WindowStartedAtUnix == right.WindowStartedAtUnix &&
		left.WindowEndedAtUnix == right.WindowEndedAtUnix &&
		left.RequestCount == right.RequestCount &&
		left.ErrorCount == right.ErrorCount &&
		left.CacheHitCount == right.CacheHitCount &&
		left.CacheMissCount == right.CacheMissCount &&
		left.CacheBypassCount == right.CacheBypassCount &&
		left.CacheExpiredCount == right.CacheExpiredCount &&
		left.CacheStaleCount == right.CacheStaleCount &&
		left.UpstreamErrorCount == right.UpstreamErrorCount &&
		left.UpstreamResponseMS == right.UpstreamResponseMS &&
		left.UniqueVisitorCount == right.UniqueVisitorCount &&
		int64MapsEqual(left.StatusCodes, right.StatusCodes) &&
		int64MapsEqual(left.TopDomains, right.TopDomains) &&
		int64MapsEqual(left.SourceCountries, right.SourceCountries)
}

func int64MapsEqual(left map[string]int64, right map[string]int64) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		if right[key] != value {
			return false
		}
	}
	return true
}
