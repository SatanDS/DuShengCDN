package service

import (
	"context"
	"crypto/sha256"
	"dushengcdn/common"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	dnsSourceDatabaseKindCountry  = "country"
	dnsSourceDatabaseKindASN      = "asn"
	dnsSourceDatabaseKindOperator = "operator"
	dnsSourceDatabaseManifestName = "manifest.json"
)

var (
	dnsSourceDatabaseMirrorHTTPClient = &http.Client{Timeout: 5 * time.Minute}
	dnsSourceDatabaseMirrorMutex      sync.Mutex
)

type DNSSourceDatabaseMirrorManifest struct {
	UpdatedAt time.Time                               `json:"updated_at"`
	Sources   map[string]DNSSourceDatabaseMirrorEntry `json:"sources"`
}

type DNSSourceDatabaseMirrorEntry struct {
	Kind      string                        `json:"kind"`
	UpdatedAt time.Time                     `json:"updated_at"`
	Files     []DNSSourceDatabaseMirrorFile `json:"files"`
	Metadata  map[string]string             `json:"metadata,omitempty"`
}

type DNSSourceDatabaseMirrorFile struct {
	Name      string    `json:"name"`
	Path      string    `json:"path"`
	Size      int64     `json:"size"`
	SHA256    string    `json:"sha256"`
	UpdatedAt time.Time `json:"updated_at"`
}

type dnsSourceDatabaseSource struct {
	kind  string
	files []dnsSourceDatabaseDownload
}

type dnsSourceDatabaseDownload struct {
	name string
	path string
	url  string
}

func RefreshDNSSourceDatabaseMirror(ctx context.Context) error {
	dnsSourceDatabaseMirrorMutex.Lock()
	defer dnsSourceDatabaseMirrorMutex.Unlock()

	root := DNSSourceDatabaseMirrorRoot()
	if err := os.MkdirAll(root, 0o755); err != nil {
		return fmt.Errorf("create DNS source database mirror root failed: %w", err)
	}
	staging, err := os.MkdirTemp(filepath.Dir(root), ".dns-source-databases.")
	if err != nil {
		return fmt.Errorf("create DNS source database staging directory failed: %w", err)
	}
	cleanupStaging := true
	defer func() {
		if cleanupStaging {
			_ = os.RemoveAll(staging)
		}
	}()

	manifest := DNSSourceDatabaseMirrorManifest{
		UpdatedAt: time.Now().UTC(),
		Sources:   map[string]DNSSourceDatabaseMirrorEntry{},
	}
	for _, source := range dnsSourceDatabaseSources() {
		entry, err := downloadDNSSourceDatabaseSource(ctx, staging, source, manifest.UpdatedAt)
		if err != nil {
			return err
		}
		manifest.Sources[source.kind] = entry
	}

	if err := writeDNSSourceDatabaseManifest(staging, manifest); err != nil {
		return err
	}

	current := filepath.Join(root, "current")
	previous := filepath.Join(root, "previous")
	_ = os.RemoveAll(previous)
	if _, err := os.Stat(current); err == nil {
		if err := os.Rename(current, previous); err != nil {
			return fmt.Errorf("rotate previous DNS source database mirror failed: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat current DNS source database mirror failed: %w", err)
	}
	if err := os.Rename(staging, current); err != nil {
		if _, restoreErr := os.Stat(previous); restoreErr == nil {
			_ = os.Rename(previous, current)
		}
		return fmt.Errorf("publish DNS source database mirror failed: %w", err)
	}
	cleanupStaging = false
	cleanupDNSSourceDatabaseMirrorArtifacts(root)
	slog.Info("DNS source database mirror refreshed", "path", current)
	return nil
}

func GetDNSSourceDatabaseMirrorManifest() (*DNSSourceDatabaseMirrorManifest, error) {
	path := filepath.Join(DNSSourceDatabaseMirrorRoot(), "current", dnsSourceDatabaseManifestName)
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var manifest DNSSourceDatabaseMirrorManifest
	if err := json.Unmarshal(content, &manifest); err != nil {
		return nil, err
	}
	return &manifest, nil
}

func OpenDNSSourceDatabaseMirrorFile(kind string, name string) (*os.File, DNSSourceDatabaseMirrorFile, error) {
	kind = normalizeDNSSourceDatabaseKind(kind)
	name = strings.TrimSpace(name)
	if kind == "" || name == "" || filepath.Base(name) != name {
		return nil, DNSSourceDatabaseMirrorFile{}, errors.New("invalid source database file")
	}

	manifest, err := GetDNSSourceDatabaseMirrorManifest()
	if err != nil {
		return nil, DNSSourceDatabaseMirrorFile{}, err
	}
	entry, ok := manifest.Sources[kind]
	if !ok {
		return nil, DNSSourceDatabaseMirrorFile{}, errors.New("source database kind is not mirrored")
	}
	for _, item := range entry.Files {
		if item.Name != name {
			continue
		}
		path := filepath.Join(DNSSourceDatabaseMirrorRoot(), "current", filepath.FromSlash(item.Path))
		file, err := os.Open(path)
		if err != nil {
			return nil, DNSSourceDatabaseMirrorFile{}, err
		}
		return file, item, nil
	}
	return nil, DNSSourceDatabaseMirrorFile{}, errors.New("source database file is not mirrored")
}

func DNSSourceDatabaseMirrorRoot() string {
	if path := strings.TrimSpace(common.DNSSourceDatabaseMirrorPath); path != "" {
		return path
	}
	return filepath.Join(common.UploadPath, "dns-source-databases")
}

func normalizeDNSSourceDatabaseKind(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case dnsSourceDatabaseKindCountry, "geoip", "geoip-country":
		return dnsSourceDatabaseKindCountry
	case dnsSourceDatabaseKindASN, "geoip-asn":
		return dnsSourceDatabaseKindASN
	case dnsSourceDatabaseKindOperator, "operator-cidr":
		return dnsSourceDatabaseKindOperator
	default:
		return ""
	}
}

func dnsSourceDatabaseSources() []dnsSourceDatabaseSource {
	operatorFiles := strings.Fields(common.DNSSourceDatabaseOperatorCIDRFiles)
	operatorDownloads := make([]dnsSourceDatabaseDownload, 0, len(operatorFiles))
	for _, name := range operatorFiles {
		name = strings.TrimSpace(name)
		if name == "" || filepath.Base(name) != name {
			continue
		}
		operatorDownloads = append(operatorDownloads, dnsSourceDatabaseDownload{
			name: name,
			path: filepath.ToSlash(filepath.Join(dnsSourceDatabaseKindOperator, name)),
			url:  strings.TrimRight(common.DNSSourceDatabaseOperatorCIDRBaseURL, "/") + "/" + name,
		})
	}
	return []dnsSourceDatabaseSource{
		{
			kind: dnsSourceDatabaseKindCountry,
			files: []dnsSourceDatabaseDownload{{
				name: "GeoLite2-Country.mmdb",
				path: filepath.ToSlash(filepath.Join(dnsSourceDatabaseKindCountry, "GeoLite2-Country.mmdb")),
				url:  common.DNSSourceDatabaseCountryURL,
			}},
		},
		{
			kind: dnsSourceDatabaseKindASN,
			files: []dnsSourceDatabaseDownload{{
				name: "GeoLite2-ASN.mmdb",
				path: filepath.ToSlash(filepath.Join(dnsSourceDatabaseKindASN, "GeoLite2-ASN.mmdb")),
				url:  common.DNSSourceDatabaseASNURL,
			}},
		},
		{
			kind:  dnsSourceDatabaseKindOperator,
			files: operatorDownloads,
		},
	}
}

func downloadDNSSourceDatabaseSource(ctx context.Context, root string, source dnsSourceDatabaseSource, updatedAt time.Time) (DNSSourceDatabaseMirrorEntry, error) {
	if len(source.files) == 0 {
		return DNSSourceDatabaseMirrorEntry{}, fmt.Errorf("DNS source database %s has no files configured", source.kind)
	}

	entry := DNSSourceDatabaseMirrorEntry{
		Kind:      source.kind,
		UpdatedAt: updatedAt,
		Files:     make([]DNSSourceDatabaseMirrorFile, 0, len(source.files)),
	}
	for _, item := range source.files {
		if strings.TrimSpace(item.url) == "" {
			return DNSSourceDatabaseMirrorEntry{}, fmt.Errorf("DNS source database %s download URL is empty", source.kind)
		}
		file, err := downloadDNSSourceDatabaseFile(ctx, root, item, updatedAt)
		if err != nil {
			return DNSSourceDatabaseMirrorEntry{}, err
		}
		entry.Files = append(entry.Files, file)
	}
	sort.Slice(entry.Files, func(i, j int) bool {
		return entry.Files[i].Path < entry.Files[j].Path
	})
	return entry, nil
}

func downloadDNSSourceDatabaseFile(ctx context.Context, root string, item dnsSourceDatabaseDownload, updatedAt time.Time) (DNSSourceDatabaseMirrorFile, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, item.url, nil)
	if err != nil {
		return DNSSourceDatabaseMirrorFile{}, err
	}
	resp, err := dnsSourceDatabaseMirrorHTTPClient.Do(req)
	if err != nil {
		return DNSSourceDatabaseMirrorFile{}, fmt.Errorf("download %s failed: %w", item.name, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return DNSSourceDatabaseMirrorFile{}, fmt.Errorf("download %s returned %s", item.name, resp.Status)
	}

	target := filepath.Join(root, filepath.FromSlash(item.path))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return DNSSourceDatabaseMirrorFile{}, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(target), ".download.")
	if err != nil {
		return DNSSourceDatabaseMirrorFile{}, err
	}
	tmpPath := tmp.Name()
	hasher := sha256.New()
	size, copyErr := io.Copy(io.MultiWriter(tmp, hasher), resp.Body)
	closeErr := tmp.Close()
	if copyErr != nil {
		_ = os.Remove(tmpPath)
		return DNSSourceDatabaseMirrorFile{}, copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return DNSSourceDatabaseMirrorFile{}, closeErr
	}
	if min := minimumDNSSourceDatabaseFileSize(item.name); size < min {
		_ = os.Remove(tmpPath)
		return DNSSourceDatabaseMirrorFile{}, fmt.Errorf("download %s is too small: %d bytes", item.name, size)
	}
	if err := os.Rename(tmpPath, target); err != nil {
		_ = os.Remove(tmpPath)
		return DNSSourceDatabaseMirrorFile{}, err
	}
	return DNSSourceDatabaseMirrorFile{
		Name:      item.name,
		Path:      item.path,
		Size:      size,
		SHA256:    hex.EncodeToString(hasher.Sum(nil)),
		UpdatedAt: updatedAt,
	}, nil
}

func minimumDNSSourceDatabaseFileSize(name string) int64 {
	if strings.HasSuffix(strings.ToLower(name), ".mmdb") {
		return 1024
	}
	return 0
}

func writeDNSSourceDatabaseManifest(root string, manifest DNSSourceDatabaseMirrorManifest) error {
	content, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, dnsSourceDatabaseManifestName), content, 0o644)
}

func cleanupDNSSourceDatabaseMirrorArtifacts(root string) {
	_ = os.RemoveAll(filepath.Join(root, "previous"))

	parent := filepath.Dir(root)
	entries, err := os.ReadDir(parent)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-24 * time.Hour)
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), ".dns-source-databases.") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(cutoff) {
			continue
		}
		_ = os.RemoveAll(filepath.Join(parent, entry.Name()))
	}
}
