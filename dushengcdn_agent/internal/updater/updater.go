package updater

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"dushengcdn-agent/internal/agent"
	"dushengcdn-agent/internal/config"
	"dushengcdn-agent/internal/security"
)

const (
	maxChecksumAssetSize  = 64 * 1024
	maxSignatureAssetSize = 8 * 1024
)

var maxAgentBinaryAssetSize int64 = 200 * 1024 * 1024

var replaceAndRestartFunc = replaceAndRestart

var allowedUpdateDownloadHosts = map[string]struct{}{
	"github.com":                            {},
	"objects.githubusercontent.com":         {},
	"github-releases.githubusercontent.com": {},
}

type Service struct {
	httpClient   *http.Client
	lastCheckKey string
}

func New() *Service {
	return &Service{
		httpClient: security.NewPublicHTTPClient(30*time.Second, true),
	}
}

type githubRelease struct {
	TagName    string        `json:"tag_name"`
	Prerelease bool          `json:"prerelease"`
	Draft      bool          `json:"draft"`
	Assets     []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

func (s *Service) CheckAndUpdate(ctx context.Context, repo string, options agent.UpdateOptions) error {
	release, err := s.getRelease(ctx, repo, options)
	if err != nil {
		return fmt.Errorf("check latest release: %w", err)
	}
	if release == nil || release.TagName == "" {
		return nil
	}

	remoteVersion := normalizeVersion(release.TagName)
	localVersion := normalizeVersion(config.AgentVersion)
	checkKey := buildReleaseCheckKey(options, remoteVersion)

	if remoteVersion == localVersion {
		return nil
	}
	if !options.Force && checkKey != "" && checkKey == s.lastCheckKey {
		return nil
	}
	if !isNewer(localVersion, remoteVersion) {
		s.lastCheckKey = checkKey
		return nil
	}

	slog.Info("agent update available", "from", localVersion, "to", remoteVersion)
	assetName := assetNameForGOOSGOARCH(runtime.GOOS, runtime.GOARCH)
	checksumAssetName := assetName + ".sha256"
	signatureAssetName := assetName + ".sig"

	var downloadURL string
	var checksumURL string
	var signatureURL string
	for _, asset := range release.Assets {
		switch asset.Name {
		case assetName:
			downloadURL = asset.BrowserDownloadURL
		case checksumAssetName:
			checksumURL = asset.BrowserDownloadURL
		case signatureAssetName:
			signatureURL = asset.BrowserDownloadURL
		}
	}
	if downloadURL == "" {
		s.lastCheckKey = checkKey
		return fmt.Errorf("no matching asset %q in release %s", assetName, release.TagName)
	}
	if checksumURL == "" {
		return fmt.Errorf("no matching checksum asset %q in release %s", checksumAssetName, release.TagName)
	}
	if signatureURL == "" {
		return fmt.Errorf("no matching signature asset %q in release %s", signatureAssetName, release.TagName)
	}
	if err := validateUpdateDownloadURL(downloadURL); err != nil {
		return fmt.Errorf("invalid update asset url: %w", err)
	}
	if err := validateUpdateDownloadURL(checksumURL); err != nil {
		return fmt.Errorf("invalid update checksum url: %w", err)
	}
	if err := validateUpdateDownloadURL(signatureURL); err != nil {
		return fmt.Errorf("invalid update signature url: %w", err)
	}

	expectedChecksum, err := s.downloadChecksum(ctx, checksumURL, assetName)
	if err != nil {
		return fmt.Errorf("download checksum: %w", err)
	}
	signature, err := s.downloadSignature(ctx, signatureURL)
	if err != nil {
		return fmt.Errorf("download signature: %w", err)
	}
	if err = verifyReleaseSignature(release.TagName, assetName, expectedChecksum, signature); err != nil {
		return fmt.Errorf("verify release signature: %w", err)
	}

	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}
	if err = s.downloadAndRestart(ctx, downloadURL, expectedChecksum, signature, release.TagName, assetName, execPath); err != nil {
		return fmt.Errorf("download and restart: %w", err)
	}
	s.lastCheckKey = checkKey
	return nil
}

func (s *Service) getRelease(ctx context.Context, repo string, options agent.UpdateOptions) (*githubRelease, error) {
	tagName := strings.TrimSpace(options.TagName)
	if tagName != "" {
		return s.getReleaseByTag(ctx, repo, tagName)
	}
	if strings.EqualFold(strings.TrimSpace(options.Channel), "preview") {
		return s.getLatestPreviewRelease(ctx, repo)
	}
	return s.getLatestStableRelease(ctx, repo)
}

func (s *Service) getLatestStableRelease(ctx context.Context, repo string) (*githubRelease, error) {
	repo, err := normalizeGitHubRepo(repo)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github api returned %s", resp.Status)
	}

	return decodeRelease(resp.Body)
}

func (s *Service) getLatestPreviewRelease(ctx context.Context, repo string) (*githubRelease, error) {
	repo, err := normalizeGitHubRepo(repo)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases?per_page=20", repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github api returned %s", resp.Status)
	}

	var releases []githubRelease
	if err = json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, err
	}
	for _, release := range releases {
		if release.Draft || !release.Prerelease {
			continue
		}
		releaseCopy := release
		return &releaseCopy, nil
	}
	return nil, nil
}

func (s *Service) getReleaseByTag(ctx context.Context, repo string, tag string) (*githubRelease, error) {
	repo, err := normalizeGitHubRepo(repo)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/tags/%s", repo, url.PathEscape(strings.TrimSpace(tag)))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github api returned %s", resp.Status)
	}

	return decodeRelease(resp.Body)
}

func decodeRelease(reader io.Reader) (*githubRelease, error) {
	var release githubRelease
	if err := json.NewDecoder(reader).Decode(&release); err != nil {
		return nil, err
	}
	return &release, nil
}

func (s *Service) downloadChecksum(ctx context.Context, url string, assetName string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := s.doUpdateDownload(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("checksum download returned %s", resp.Status)
	}
	if resp.Request != nil && resp.Request.URL != nil {
		if err := validateUpdateDownloadURL(resp.Request.URL.String()); err != nil {
			return "", fmt.Errorf("checksum download final url is unsafe: %w", err)
		}
	}

	content, err := io.ReadAll(io.LimitReader(resp.Body, maxChecksumAssetSize+1))
	if err != nil {
		return "", err
	}
	if len(content) > maxChecksumAssetSize {
		return "", fmt.Errorf("checksum asset exceeds %d bytes", maxChecksumAssetSize)
	}
	checksum, err := parseSHA256Checksum(string(content), assetName)
	if err != nil {
		return "", err
	}
	return checksum, nil
}

func (s *Service) downloadSignature(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.doUpdateDownload(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("signature download returned %s", resp.Status)
	}
	if resp.Request != nil && resp.Request.URL != nil {
		if err := validateUpdateDownloadURL(resp.Request.URL.String()); err != nil {
			return nil, fmt.Errorf("signature download final url is unsafe: %w", err)
		}
	}

	content, err := io.ReadAll(io.LimitReader(resp.Body, maxSignatureAssetSize+1))
	if err != nil {
		return nil, err
	}
	if len(content) > maxSignatureAssetSize {
		return nil, fmt.Errorf("signature asset exceeds %d bytes", maxSignatureAssetSize)
	}
	signature, err := parseReleaseSignature(string(content))
	if err != nil {
		return nil, err
	}
	return signature, nil
}

func parseSHA256Checksum(content string, assetName string) (string, error) {
	assetName = strings.TrimSpace(assetName)
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if checksum, ok := parseSHA256Line(line, assetName); ok {
			return checksum, nil
		}
	}
	if assetName == "" {
		return "", fmt.Errorf("checksum asset does not contain a valid sha256 digest")
	}
	return "", fmt.Errorf("checksum asset does not contain a sha256 digest for %q", assetName)
}

func parseSHA256Line(line string, assetName string) (string, bool) {
	fields := strings.Fields(line)
	if len(fields) == 1 && isSHA256Hex(fields[0]) {
		return strings.ToLower(fields[0]), true
	}
	if len(fields) >= 2 && isSHA256Hex(fields[0]) {
		fileName := strings.TrimPrefix(strings.TrimSpace(fields[1]), "*")
		if assetName == "" || fileName == assetName {
			return strings.ToLower(fields[0]), true
		}
	}

	prefix := "SHA256("
	if strings.HasPrefix(line, prefix) {
		closing := strings.Index(line, ")")
		if closing > len(prefix) && closing+1 < len(line) {
			fileName := strings.TrimSpace(line[len(prefix):closing])
			rest := strings.TrimSpace(line[closing+1:])
			rest = strings.TrimPrefix(rest, "=")
			rest = strings.TrimSpace(rest)
			if isSHA256Hex(rest) && (assetName == "" || fileName == assetName) {
				return strings.ToLower(rest), true
			}
		}
	}
	return "", false
}

func parseReleaseSignature(content string) ([]byte, error) {
	value := strings.TrimSpace(content)
	if value == "" {
		return nil, fmt.Errorf("signature asset is empty")
	}
	fields := strings.Fields(value)
	if len(fields) > 0 {
		value = fields[0]
	}
	signature, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		signature, err = base64.RawStdEncoding.DecodeString(value)
	}
	if err != nil {
		return nil, fmt.Errorf("signature asset is not valid base64")
	}
	if len(signature) != ed25519.SignatureSize {
		return nil, fmt.Errorf("signature length is invalid")
	}
	return signature, nil
}

func releaseSignaturePayload(tagName string, assetName string, checksum string) []byte {
	return []byte(strings.Join([]string{
		"dushengcdn-release-v1",
		strings.TrimSpace(tagName),
		strings.TrimSpace(assetName),
		strings.ToLower(strings.TrimSpace(checksum)),
		"",
	}, "\n"))
}

func verifyReleaseSignature(tagName string, assetName string, checksum string, signature []byte) error {
	publicKeyText := strings.TrimSpace(config.ReleaseSignaturePublicKey)
	if publicKeyText == "" {
		return fmt.Errorf("release signature public key is not configured")
	}
	publicKey, err := base64.StdEncoding.DecodeString(publicKeyText)
	if err != nil {
		publicKey, err = base64.RawStdEncoding.DecodeString(publicKeyText)
	}
	if err != nil || len(publicKey) != ed25519.PublicKeySize {
		return fmt.Errorf("release signature public key is invalid")
	}
	if len(signature) != ed25519.SignatureSize {
		return fmt.Errorf("release signature is invalid")
	}
	if !ed25519.Verify(ed25519.PublicKey(publicKey), releaseSignaturePayload(tagName, assetName, checksum), signature) {
		return fmt.Errorf("release signature verification failed")
	}
	return nil
}

func isSHA256Hex(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func validateUpdateDownloadURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("download url format is invalid")
	}
	if parsed.Scheme != "https" {
		return fmt.Errorf("download url must use https")
	}
	if parsed.User != nil {
		return fmt.Errorf("download url must not contain user info")
	}
	if port := parsed.Port(); port != "" && port != "443" {
		return fmt.Errorf("download url must use the default https port")
	}
	host := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(parsed.Hostname())), ".")
	if _, ok := allowedUpdateDownloadHosts[host]; !ok {
		return fmt.Errorf("download host %q is not allowed", host)
	}
	return nil
}

func normalizeGitHubRepo(repo string) (string, error) {
	repo = strings.TrimSpace(repo)
	parts := strings.Split(repo, "/")
	if len(parts) != 2 {
		return "", fmt.Errorf("github repo must use owner/repo format")
	}
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return "", fmt.Errorf("github repo must use owner/repo format")
		}
		for _, r := range part {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
				continue
			}
			return "", fmt.Errorf("github repo contains invalid characters")
		}
	}
	return repo, nil
}

func (s *Service) doUpdateDownload(req *http.Request) (*http.Response, error) {
	if req == nil || req.URL == nil {
		return nil, fmt.Errorf("download request is invalid")
	}
	if err := validateUpdateDownloadURL(req.URL.String()); err != nil {
		return nil, err
	}
	baseClient := s.httpClient
	if baseClient == nil {
		baseClient = security.NewPublicHTTPClient(30*time.Second, true)
	}
	client := *baseClient
	previousCheckRedirect := client.CheckRedirect
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if req == nil || req.URL == nil {
			return fmt.Errorf("download redirect request is invalid")
		}
		if err := validateUpdateDownloadURL(req.URL.String()); err != nil {
			return err
		}
		if previousCheckRedirect != nil {
			return previousCheckRedirect(req, via)
		}
		if len(via) >= 10 {
			return fmt.Errorf("stopped after 10 redirects")
		}
		return nil
	}
	return client.Do(req)
}

func (s *Service) downloadAndRestart(ctx context.Context, url string, expectedChecksum string, signature []byte, tagName string, assetName string, targetPath string) error {
	expectedChecksum = strings.ToLower(strings.TrimSpace(expectedChecksum))
	if !isSHA256Hex(expectedChecksum) {
		return fmt.Errorf("invalid expected sha256 checksum")
	}
	targetPath, err := safeUpdateTargetPath(targetPath)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := s.doUpdateDownload(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned %s", resp.Status)
	}
	if resp.Request != nil && resp.Request.URL != nil {
		if err := validateUpdateDownloadURL(resp.Request.URL.String()); err != nil {
			return fmt.Errorf("download final url is unsafe: %w", err)
		}
	}
	if resp.ContentLength > maxAgentBinaryAssetSize {
		return fmt.Errorf("agent binary asset exceeds %d bytes", maxAgentBinaryAssetSize)
	}

	tmpPath := targetPath + ".update"
	if runtime.GOOS == "windows" && !strings.HasSuffix(strings.ToLower(tmpPath), ".exe") {
		tmpPath += ".exe"
	}
	tmpFile, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	hasher := sha256.New()
	written, err := io.Copy(io.MultiWriter(tmpFile, hasher), io.LimitReader(resp.Body, maxAgentBinaryAssetSize+1))
	if err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return err
	}
	if err = tmpFile.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	if written > maxAgentBinaryAssetSize {
		os.Remove(tmpPath)
		return fmt.Errorf("agent binary asset exceeds %d bytes", maxAgentBinaryAssetSize)
	}
	actualChecksum := hex.EncodeToString(hasher.Sum(nil))
	if actualChecksum != expectedChecksum {
		os.Remove(tmpPath)
		return fmt.Errorf("sha256 checksum mismatch: expected %s, got %s", expectedChecksum, actualChecksum)
	}
	if err = verifyReleaseSignature(tagName, assetName, actualChecksum, signature); err != nil {
		os.Remove(tmpPath)
		return err
	}
	if err = os.Chmod(tmpPath, 0o755); err != nil && runtime.GOOS != "windows" {
		os.Remove(tmpPath)
		return fmt.Errorf("set executable permission: %w", err)
	}

	slog.Info("agent binary updated, restarting")
	return replaceAndRestartFunc(targetPath, tmpPath)
}

func safeUpdateTargetPath(path string) (string, error) {
	candidate := strings.TrimSpace(path)
	if candidate == "" {
		return "", fmt.Errorf("target path cannot be empty")
	}
	if strings.Contains(candidate, "\x00") || strings.ContainsAny(candidate, "\r\n") {
		return "", fmt.Errorf("target path contains invalid characters")
	}
	cleaned := filepath.Clean(candidate)
	if !filepath.IsAbs(cleaned) {
		return "", fmt.Errorf("target path %q must be absolute", path)
	}
	info, err := os.Stat(cleaned)
	if err != nil {
		return "", fmt.Errorf("stat target path: %w", err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("target path %q is a directory", path)
	}
	return cleaned, nil
}

func assetNameForGOOSGOARCH(goos string, goarch string) string {
	name := fmt.Sprintf("dushengcdn-agent-%s-%s", goos, goarch)
	if goos == "windows" {
		return name + ".exe"
	}
	return name
}

func normalizeVersion(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	return v
}

func isNewer(local, remote string) bool {
	return compareVersions(local, remote) < 0
}

func buildReleaseCheckKey(options agent.UpdateOptions, remoteVersion string) string {
	channel := strings.TrimSpace(options.Channel)
	if channel == "" {
		channel = "stable"
	}
	if tagName := strings.TrimSpace(options.TagName); tagName != "" {
		return channel + ":" + tagName
	}
	return channel + ":" + remoteVersion
}

type versionInfo struct {
	valid      bool
	isDev      bool
	numbers    []int
	prerelease []string
}

func parseVersionInfo(version string) versionInfo {
	normalized := normalizeVersion(version)
	if normalized == "" || strings.EqualFold(normalized, "dev") {
		return versionInfo{isDev: strings.EqualFold(normalized, "dev")}
	}
	base := normalized
	prerelease := ""
	if index := strings.IndexRune(normalized, '-'); index >= 0 {
		base = normalized[:index]
		prerelease = normalized[index+1:]
	}
	segments := strings.Split(base, ".")
	parts := make([]int, 0, len(segments))
	for _, segment := range segments {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			parts = append(parts, 0)
			continue
		}
		numeric := strings.Builder{}
		for _, r := range segment {
			if r < '0' || r > '9' {
				break
			}
			numeric.WriteRune(r)
		}
		if numeric.Len() == 0 {
			return versionInfo{}
		}
		value, err := strconv.Atoi(numeric.String())
		if err != nil {
			return versionInfo{}
		}
		parts = append(parts, value)
	}
	info := versionInfo{valid: len(parts) > 0, numbers: parts}
	if prerelease != "" {
		info.prerelease = splitPrereleaseIdentifiers(prerelease)
	}
	return info
}

func splitPrereleaseIdentifiers(value string) []string {
	parts := strings.FieldsFunc(strings.TrimSpace(value), func(r rune) bool {
		return r == '.' || r == '-'
	})
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			filtered = append(filtered, part)
		}
	}
	return filtered
}

func compareVersions(local string, remote string) int {
	left := parseVersionInfo(local)
	right := parseVersionInfo(remote)
	if left.isDev {
		if right.valid {
			return -1
		}
		return 0
	}
	if !left.valid || !right.valid {
		return 0
	}

	maxLen := len(left.numbers)
	if len(right.numbers) > maxLen {
		maxLen = len(right.numbers)
	}
	for index := 0; index < maxLen; index++ {
		leftValue := 0
		rightValue := 0
		if index < len(left.numbers) {
			leftValue = left.numbers[index]
		}
		if index < len(right.numbers) {
			rightValue = right.numbers[index]
		}
		if leftValue < rightValue {
			return -1
		}
		if leftValue > rightValue {
			return 1
		}
	}
	if len(left.prerelease) == 0 && len(right.prerelease) == 0 {
		return 0
	}
	if len(left.prerelease) == 0 {
		return 1
	}
	if len(right.prerelease) == 0 {
		return -1
	}
	maxLen = len(left.prerelease)
	if len(right.prerelease) > maxLen {
		maxLen = len(right.prerelease)
	}
	for index := 0; index < maxLen; index++ {
		if index >= len(left.prerelease) {
			return -1
		}
		if index >= len(right.prerelease) {
			return 1
		}
		leftPart := left.prerelease[index]
		rightPart := right.prerelease[index]
		leftNumber, leftErr := strconv.Atoi(leftPart)
		rightNumber, rightErr := strconv.Atoi(rightPart)
		switch {
		case leftErr == nil && rightErr == nil:
			if leftNumber < rightNumber {
				return -1
			}
			if leftNumber > rightNumber {
				return 1
			}
		case leftErr == nil && rightErr != nil:
			return -1
		case leftErr != nil && rightErr == nil:
			return 1
		default:
			if leftPart < rightPart {
				return -1
			}
			if leftPart > rightPart {
				return 1
			}
		}
	}
	return 0
}
