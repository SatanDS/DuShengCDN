package state

import (
	"bytes"
	"crypto/rand"
	"dushengcdn-agent/internal/fileutil"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

type Snapshot struct {
	NodeID                 string `json:"node_id"`
	CurrentVersion         string `json:"current_version"`
	CurrentChecksum        string `json:"current_checksum"`
	BlockedVersion         string `json:"blocked_version"`
	BlockedChecksum        string `json:"blocked_checksum"`
	BlockedReason          string `json:"blocked_reason"`
	LastError              string `json:"last_error"`
	OpenrestyStatus        string `json:"openresty_status"`
	OpenrestyMessage       string `json:"openresty_message"`
	LastProfileFingerprint string `json:"last_profile_fingerprint"`
	LastCPUStatTotal       uint64 `json:"last_cpu_stat_total"`
	LastCPUStatIdle        uint64 `json:"last_cpu_stat_idle"`
	LastMetricAtUnix       int64  `json:"last_metric_at_unix"`
	AccessLogOffset        int64  `json:"access_log_offset"`
	AccessLogOffsetReady   bool   `json:"access_log_offset_ready"`
}

type Store struct {
	path             string
	mu               sync.Mutex
	loaded           bool
	cache            *Snapshot
	encoded          []byte
	encodedCanonical bool
}

func NewStore(path string) *Store {
	return &Store{path: filepath.Clean(path)}
}

func (s *Store) Load() (*Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadUnlocked()
}

func (s *Store) EnsureNodeID() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	snapshot, err := s.loadUnlocked()
	if err != nil {
		return "", err
	}
	if snapshot.NodeID != "" {
		return snapshot.NodeID, nil
	}
	snapshot.NodeID, err = newNodeID()
	if err != nil {
		return "", err
	}
	if err = s.saveUnlocked(snapshot); err != nil {
		return "", err
	}
	return snapshot.NodeID, nil
}

func (s *Store) Save(snapshot *Snapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveUnlocked(snapshot)
}

func (s *Store) loadUnlocked() (*Snapshot, error) {
	if s.loaded {
		return cloneSnapshot(s.cache), nil
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			snapshot := &Snapshot{}
			s.cache = cloneSnapshot(snapshot)
			s.encoded = nil
			s.encodedCanonical = false
			s.loaded = true
			return snapshot, nil
		}
		return nil, err
	}
	snapshot := &Snapshot{}
	if len(data) == 0 {
		s.cache = cloneSnapshot(snapshot)
		s.encoded = append([]byte(nil), data...)
		s.encodedCanonical = false
		s.loaded = true
		return snapshot, nil
	}
	if err = json.Unmarshal(data, snapshot); err != nil {
		return nil, err
	}
	s.cache = cloneSnapshot(snapshot)
	s.encoded = append([]byte(nil), data...)
	s.encodedCanonical = isCanonicalSnapshotEncoding(snapshot, data)
	s.loaded = true
	return snapshot, nil
}

func (s *Store) saveUnlocked(snapshot *Snapshot) error {
	normalized := cloneSnapshot(snapshot)
	if s.loaded && s.cache != nil && *s.cache == *normalized && s.encodedCanonical {
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
		s.cache = cloneSnapshot(normalized)
		s.encodedCanonical = true
		return nil
	}
	if err = fileutil.WriteFileAtomicIfChanged(s.path, data, 0o644); err != nil {
		return err
	}
	s.cache = cloneSnapshot(normalized)
	s.encoded = append([]byte(nil), data...)
	s.encodedCanonical = true
	s.loaded = true
	return nil
}

func isCanonicalSnapshotEncoding(snapshot *Snapshot, data []byte) bool {
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return false
	}
	return bytes.Equal(encoded, data)
}

func cloneSnapshot(snapshot *Snapshot) *Snapshot {
	if snapshot == nil {
		return &Snapshot{}
	}
	copied := *snapshot
	return &copied
}

func newNodeID() (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "node-" + hex.EncodeToString(buf), nil
}
