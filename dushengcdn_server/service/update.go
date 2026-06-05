package service

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"dushengcdn/common"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	githubReleasesAPIBase        = "https://api.github.com/repos/%s/releases"
	manualServerBinaryTTL        = 15 * time.Minute
	uploadedBinaryVersionTimeout = 10 * time.Second
)

var manualServerBinaryMaxBytes int64 = 200 * 1024 * 1024
var allowedServerUpdateDownloadHosts = map[string]struct{}{
	"api.github.com":                        {},
	"github.com":                            {},
	"objects.githubusercontent.com":         {},
	"github-releases.githubusercontent.com": {},
}

type ReleaseChannel string

const (
	ReleaseChannelStable  ReleaseChannel = "stable"
	ReleaseChannelPreview ReleaseChannel = "preview"
)

var updateHTTPClient = &http.Client{
	Timeout: 30 * time.Second,
}

var serverUpgradeState struct {
	sync.Mutex
	inProgress bool
	status     string
	logs       []ServerUpgradeLogRecord
}

var serverUpgradeSubscribers struct {
	sync.Mutex
	nextID    int
	listeners map[int]chan ServerUpgradeStreamSnapshot
}

var manualServerBinaryState struct {
	sync.Mutex
	candidate *manualServerBinaryCandidate
}

var serverBinaryUpgradeExecutor = replaceAndRestartServer

var serverUpgradeDispatchDelay = 500 * time.Millisecond

type LatestServerRelease struct {
	TagName                 string                   `json:"tag_name"`
	Body                    string                   `json:"body"`
	HTMLURL                 string                   `json:"html_url"`
	PublishedAt             string                   `json:"published_at"`
	Channel                 string                   `json:"channel"`
	Prerelease              bool                     `json:"prerelease"`
	CurrentVersion          string                   `json:"current_version"`
	HasUpdate               bool                     `json:"has_update"`
	UpgradeSupported        bool                     `json:"upgrade_supported"`
	AutomaticUpgradeEnabled bool                     `json:"automatic_upgrade_enabled"`
	InProgress              bool                     `json:"in_progress"`
	UpgradeStatus           string                   `json:"upgrade_status"`
	UpgradeLogs             []ServerUpgradeLogRecord `json:"upgrade_logs"`
}

type ServerUpgradeLogRecord struct {
	Level     string    `json:"level"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}

type ServerUpgradeStreamSnapshot struct {
	InProgress    bool                     `json:"in_progress"`
	UpgradeStatus string                   `json:"upgrade_status"`
	UpgradeLogs   []ServerUpgradeLogRecord `json:"upgrade_logs"`
}

type githubReleaseResponse struct {
	TagName     string        `json:"tag_name"`
	Body        string        `json:"body"`
	HTMLURL     string        `json:"html_url"`
	PublishedAt string        `json:"published_at"`
	Prerelease  bool          `json:"prerelease"`
	Draft       bool          `json:"draft"`
	Assets      []githubAsset `json:"assets"`
}

type githubAsset struct {
	URL                string `json:"url"`
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type preparedServerUpgrade struct {
	release       *LatestServerRelease
	downloadURL   string
	checksumURL   string
	signatureURL  string
	assetName     string
	checksumName  string
	signatureName string
	execPath      string
}

type UploadedServerBinary struct {
	UploadToken       string    `json:"upload_token"`
	FileName          string    `json:"file_name"`
	DetectedVersion   string    `json:"detected_version"`
	CurrentVersion    string    `json:"current_version"`
	HasUpdate         bool      `json:"has_update"`
	UpgradeSupported  bool      `json:"upgrade_supported"`
	ReadyToUpgrade    bool      `json:"ready_to_upgrade"`
	ComparisonMessage string    `json:"comparison_message"`
	UploadedAt        time.Time `json:"uploaded_at"`
}

type manualServerBinaryCandidate struct {
	UploadToken     string
	FileName        string
	DetectedVersion string
	CurrentVersion  string
	TempPath        string
	ExecPath        string
	UploadedAt      time.Time
}

func GetLatestServerRelease(ctx context.Context, channel string) (*LatestServerRelease, error) {
	normalizedChannel := normalizeReleaseChannel(channel)
	release, err := fetchLatestRelease(ctx, normalizedChannel)
	if err != nil {
		return nil, err
	}
	return buildLatestServerReleaseView(release, normalizedChannel), nil
}

func ScheduleServerUpgrade(channel string) (*LatestServerRelease, error) {
	if !common.ServerAutoUpgradeEnabled {
		return nil, fmt.Errorf("服务端自动升级默认关闭；如需启用，请设置 DUSHENGCDN_SERVER_AUTO_UPGRADE_ENABLED=true，并确认 Release 同时包含 Server 二进制、同名 .sha256 校验文件和 .sig 签名文件。也可以上传已审阅的 Server 二进制进行手动升级")
	}
	return scheduleServerUpgradeFromRelease(channel)
}

func scheduleServerUpgradeFromRelease(channel string) (*LatestServerRelease, error) {
	normalizedChannel := normalizeReleaseChannel(channel)
	serverUpgradeState.Lock()
	if serverUpgradeState.inProgress {
		serverUpgradeState.Unlock()
		return nil, fmt.Errorf("服务升级正在执行中，请稍后再试")
	}

	resetServerUpgradeLogsLocked()
	serverUpgradeState.status = "running"
	appendServerUpgradeLogLocked("info", fmt.Sprintf("Automatic upgrade scheduled for channel: %s.", normalizedChannel.String()))
	serverUpgradeState.Unlock()
	broadcastServerUpgradeSnapshot()

	prepared, err := prepareServerUpgrade(context.Background(), normalizedChannel)
	if err != nil {
		serverUpgradeState.Lock()
		serverUpgradeState.status = "failed"
		appendServerUpgradeLogLocked("error", err.Error())
		serverUpgradeState.Unlock()
		broadcastServerUpgradeSnapshot()
		return nil, err
	}

	serverUpgradeState.Lock()
	serverUpgradeState.inProgress = true
	serverUpgradeState.Unlock()
	broadcastServerUpgradeSnapshot()

	prepared.release.InProgress = true

	go func(task *preparedServerUpgrade) {
		time.Sleep(serverUpgradeDispatchDelay)
		if err := executeServerUpgrade(task); err != nil {
			recordServerUpgradeFailure(err)
			slog.Error("server self-update failed", "error", err)
		}
	}(prepared)

	return prepared.release, nil
}

func UploadManualServerBinary(ctx context.Context, fileName string, reader io.Reader) (*UploadedServerBinary, error) {
	inProgress, _, _ := snapshotServerUpgradeState()
	if inProgress {
		return nil, fmt.Errorf("服务升级正在执行中，请稍后再试")
	}
	if strings.TrimSpace(fileName) == "" {
		return nil, fmt.Errorf("缺少上传文件名")
	}
	if reader == nil {
		return nil, fmt.Errorf("缺少上传文件内容")
	}

	execPath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("获取当前服务程序路径失败: %v", err)
	}
	if err = verifyExecutableDirectoryWritable(execPath); err != nil {
		return nil, err
	}
	tempPath, err := persistUploadedServerBinary(filepath.Dir(execPath), fileName, reader)
	if err != nil {
		return nil, err
	}

	detectedVersion, err := detectUploadedServerBinaryVersion(ctx, tempPath)
	if err != nil {
		_ = os.Remove(tempPath)
		return nil, err
	}

	currentVersion := strings.TrimSpace(common.Version)
	uploadedAt := time.Now()
	info := buildUploadedServerBinaryView(fileName, currentVersion, detectedVersion, uploadedAt)
	if !info.ReadyToUpgrade {
		_ = os.Remove(tempPath)
		return info, nil
	}

	uploadToken, err := newUpgradeToken()
	if err != nil {
		_ = os.Remove(tempPath)
		return nil, fmt.Errorf("生成升级令牌失败: %v", err)
	}

	manualServerBinaryState.Lock()
	cleanupManualServerBinaryCandidateLocked()
	manualServerBinaryState.candidate = &manualServerBinaryCandidate{
		UploadToken:     uploadToken,
		FileName:        fileName,
		DetectedVersion: detectedVersion,
		CurrentVersion:  currentVersion,
		TempPath:        tempPath,
		ExecPath:        execPath,
		UploadedAt:      uploadedAt,
	}
	manualServerBinaryState.Unlock()

	info.UploadToken = uploadToken
	return info, nil
}

func ManualServerBinaryMaxBytes() int64 {
	return manualServerBinaryMaxBytes
}

func ConfirmManualServerUpgrade(uploadToken string) (*UploadedServerBinary, error) {
	uploadToken = strings.TrimSpace(uploadToken)
	if uploadToken == "" {
		return nil, fmt.Errorf("缺少升级令牌")
	}

	serverUpgradeState.Lock()
	if serverUpgradeState.inProgress {
		serverUpgradeState.Unlock()
		return nil, fmt.Errorf("服务升级正在执行中，请稍后再试")
	}
	serverUpgradeState.Unlock()

	manualServerBinaryState.Lock()
	candidate := manualServerBinaryState.candidate
	if candidate == nil {
		manualServerBinaryState.Unlock()
		return nil, fmt.Errorf("未找到待确认的上传升级包，请重新上传")
	}
	if isManualServerBinaryCandidateExpired(candidate) {
		cleanupManualServerBinaryCandidateLocked()
		manualServerBinaryState.Unlock()
		return nil, fmt.Errorf("升级包已过期，请重新上传")
	}
	if candidate.UploadToken != uploadToken {
		manualServerBinaryState.Unlock()
		return nil, fmt.Errorf("升级令牌无效或已过期，请重新上传")
	}
	manualServerBinaryState.candidate = nil
	manualServerBinaryState.Unlock()

	info := buildUploadedServerBinaryView(candidate.FileName, candidate.CurrentVersion, candidate.DetectedVersion, candidate.UploadedAt)
	info.UploadToken = candidate.UploadToken
	if !info.ReadyToUpgrade {
		_ = os.Remove(candidate.TempPath)
		return nil, fmt.Errorf("当前上传的二进制不满足升级条件")
	}

	serverUpgradeState.Lock()
	serverUpgradeState.inProgress = true
	resetServerUpgradeLogsLocked()
	serverUpgradeState.status = "running"
	appendServerUpgradeLogLocked("info", fmt.Sprintf("Manual upgrade confirmed for version: %s.", strings.TrimSpace(candidate.DetectedVersion)))
	serverUpgradeState.Unlock()
	broadcastServerUpgradeSnapshot()

	go func(task *manualServerBinaryCandidate) {
		time.Sleep(serverUpgradeDispatchDelay)
		if err := executeServerBinaryCandidateUpgrade(task, "manual"); err != nil {
			recordServerUpgradeFailure(err)
			slog.Error("server manual upgrade failed", "error", err)
			_ = os.Remove(task.TempPath)
		}
	}(candidate)

	return info, nil
}

func fetchLatestRelease(ctx context.Context, channel ReleaseChannel) (*githubReleaseResponse, error) {
	return fetchLatestGitHubRelease(ctx, common.ServerUpdateRepo, channel)
}

func fetchLatestGitHubRelease(ctx context.Context, repo string, channel ReleaseChannel) (*githubReleaseResponse, error) {
	switch normalizeReleaseChannel(string(channel)) {
	case ReleaseChannelPreview:
		return fetchLatestPreviewGitHubRelease(ctx, repo)
	default:
		return fetchLatestStableGitHubRelease(ctx, repo)
	}
}

func fetchLatestStableGitHubRelease(ctx context.Context, repo string) (*githubReleaseResponse, error) {
	repo, err := normalizeGitHubRepo(repo)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf(githubReleasesAPIBase+"/latest", repo)
	req, err := newGitHubReleaseRequest(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("创建更新请求失败")
	}

	resp, err := updateHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("获取最新版本失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub 返回异常状态: %s", resp.Status)
	}

	return decodeGitHubRelease(resp.Body)
}

func fetchLatestPreviewGitHubRelease(ctx context.Context, repo string) (*githubReleaseResponse, error) {
	repo, err := normalizeGitHubRepo(repo)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf(githubReleasesAPIBase+"?per_page=20", repo)
	req, err := newGitHubReleaseRequest(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("创建更新请求失败")
	}

	resp, err := updateHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("获取 preview 版本失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub 返回异常状态: %s", resp.Status)
	}

	var releases []githubReleaseResponse
	if err = json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, fmt.Errorf("解析 preview 版本信息失败")
	}
	for _, release := range releases {
		if release.Draft || !release.Prerelease {
			continue
		}
		releaseCopy := release
		return &releaseCopy, nil
	}
	return nil, fmt.Errorf("当前没有可用的 preview 发布")
}

func fetchGitHubReleaseByTag(ctx context.Context, repo string, tag string) (*githubReleaseResponse, error) {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return nil, fmt.Errorf("缺少发布版本号")
	}
	repo, err := normalizeGitHubRepo(repo)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf(githubReleasesAPIBase+"/tags/%s", repo, url.PathEscape(tag))
	req, err := newGitHubReleaseRequest(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("创建更新请求失败")
	}

	resp, err := updateHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("获取指定版本失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("未找到指定版本: %s", tag)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub 返回异常状态: %s", resp.Status)
	}

	return decodeGitHubRelease(resp.Body)
}

func newGitHubReleaseRequest(ctx context.Context, url string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	applyGitHubReleaseAuth(req)
	req.Header.Set("User-Agent", "DuShengCDN-Server")
	return req, nil
}

func decodeGitHubRelease(reader io.Reader) (*githubReleaseResponse, error) {
	var release githubReleaseResponse
	if err := json.NewDecoder(reader).Decode(&release); err != nil {
		return nil, fmt.Errorf("解析版本信息失败")
	}
	return &release, nil
}

func buildLatestServerReleaseView(release *githubReleaseResponse, channel ReleaseChannel) *LatestServerRelease {
	currentVersion := strings.TrimSpace(common.Version)
	isDevBuild := currentVersion == "" || strings.EqualFold(currentVersion, "dev")
	platformUpgradeSupported := !isDevBuild && runtime.GOOS != "windows"
	hasUpdate := false
	if release != nil {
		switch {
		case isDevBuild:
			hasUpdate = parseVersionInfo(release.TagName).Valid
		case channel == ReleaseChannelPreview:
			hasUpdate = normalizeVersion(currentVersion) != normalizeVersion(release.TagName)
		default:
			hasUpdate = isVersionNewer(currentVersion, release.TagName)
		}
	}

	inProgress, upgradeStatus, upgradeLogs := snapshotServerUpgradeState()

	view := &LatestServerRelease{
		Channel:                 channel.String(),
		CurrentVersion:          currentVersion,
		HasUpdate:               hasUpdate,
		UpgradeSupported:        platformUpgradeSupported && common.ServerAutoUpgradeEnabled,
		AutomaticUpgradeEnabled: common.ServerAutoUpgradeEnabled,
		InProgress:              inProgress,
		UpgradeStatus:           upgradeStatus,
		UpgradeLogs:             upgradeLogs,
	}
	if release != nil {
		view.TagName = release.TagName
		view.Body = release.Body
		view.HTMLURL = release.HTMLURL
		view.PublishedAt = release.PublishedAt
		view.Prerelease = release.Prerelease
	}
	return view
}

func prepareServerUpgrade(ctx context.Context, channel ReleaseChannel) (*preparedServerUpgrade, error) {
	release, err := fetchLatestRelease(ctx, channel)
	if err != nil {
		return nil, err
	}

	view := buildLatestServerReleaseView(release, channel)
	if !view.HasUpdate {
		return nil, fmt.Errorf("当前已经是最新版本")
	}
	if !view.UpgradeSupported {
		return nil, fmt.Errorf("当前平台暂不支持自动升级")
	}

	assetName := serverAssetName(runtime.GOOS, runtime.GOARCH)
	checksumAssetName := assetName + ".sha256"
	signatureAssetName := assetName + ".sig"
	recordServerUpgradeLog("info", fmt.Sprintf("Matching release asset: %s.", assetName))

	var downloadURL string
	var checksumURL string
	var signatureURL string
	for _, asset := range release.Assets {
		switch asset.Name {
		case assetName:
			downloadURL = releaseAssetDownloadURL(asset)
		case checksumAssetName:
			checksumURL = releaseAssetDownloadURL(asset)
		case signatureAssetName:
			signatureURL = releaseAssetDownloadURL(asset)
		}
	}
	if downloadURL == "" {
		return nil, fmt.Errorf("最新版本缺少当前平台的服务端二进制: %s", assetName)
	}
	if checksumURL == "" {
		return nil, fmt.Errorf("最新版本缺少当前平台的服务端校验文件: %s", checksumAssetName)
	}
	if signatureURL == "" {
		return nil, fmt.Errorf("最新版本缺少当前平台的服务端签名文件: %s", signatureAssetName)
	}
	if err := validateServerUpdateDownloadURL(downloadURL); err != nil {
		return nil, fmt.Errorf("服务端升级包下载地址不安全: %w", err)
	}
	if err := validateServerUpdateDownloadURL(checksumURL); err != nil {
		return nil, fmt.Errorf("服务端升级包校验文件地址不安全: %w", err)
	}
	if err := validateServerUpdateDownloadURL(signatureURL); err != nil {
		return nil, fmt.Errorf("服务端升级包签名文件地址不安全: %w", err)
	}

	execPath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("获取当前服务程序路径失败: %v", err)
	}
	if err = verifyExecutableDirectoryWritable(execPath); err != nil {
		return nil, err
	}
	recordServerUpgradeLog("info", "Verified current executable directory is writable.")

	return &preparedServerUpgrade{
		release:       view,
		downloadURL:   downloadURL,
		checksumURL:   checksumURL,
		signatureURL:  signatureURL,
		assetName:     assetName,
		checksumName:  checksumAssetName,
		signatureName: signatureAssetName,
		execPath:      execPath,
	}, nil
}

func verifyExecutableDirectoryWritable(execPath string) error {
	dir := filepath.Dir(execPath)
	tempFile, err := os.CreateTemp(dir, "dushengcdn-server-upgrade-check-*")
	if err != nil {
		return fmt.Errorf("当前服务二进制目录不可写，无法升级: %v", err)
	}
	tempPath := tempFile.Name()
	if closeErr := tempFile.Close(); closeErr != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("校验服务升级目录失败: %v", closeErr)
	}
	if err = os.Remove(tempPath); err != nil {
		return fmt.Errorf("清理升级校验文件失败: %v", err)
	}
	return nil
}

func validateServerUpdateDownloadURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return errors.New("download url format is invalid")
	}
	if parsed.Scheme != "https" {
		return errors.New("download url must use https")
	}
	if parsed.User != nil {
		return errors.New("download url must not contain user info")
	}
	if port := parsed.Port(); port != "" && port != "443" {
		return errors.New("download url must use the default https port")
	}
	host := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(parsed.Hostname())), ".")
	if _, ok := allowedServerUpdateDownloadHosts[host]; !ok {
		return fmt.Errorf("download host %q is not allowed", host)
	}
	return nil
}

func releaseAssetDownloadURL(asset githubAsset) string {
	if strings.TrimSpace(common.GitHubReleaseToken) != "" {
		if url := strings.TrimSpace(asset.URL); url != "" {
			return url
		}
	}
	return strings.TrimSpace(asset.BrowserDownloadURL)
}

func newServerUpdateDownloadRequest(ctx context.Context, rawURL string, accept string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", strings.TrimSpace(accept))
	if isGitHubAPIHost(req.URL) {
		req.Header.Set("Accept", "application/octet-stream")
	}
	applyGitHubReleaseAuth(req)
	req.Header.Set("User-Agent", "DuShengCDN-Server")
	return req, nil
}

func applyGitHubReleaseAuth(req *http.Request) {
	if req == nil || req.URL == nil {
		return
	}
	token := strings.TrimSpace(common.GitHubReleaseToken)
	if token == "" {
		return
	}
	host := normalizedURLHostname(req.URL)
	if host != "api.github.com" && host != "github.com" {
		return
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
}

func isGitHubAPIHost(parsed *url.URL) bool {
	return normalizedURLHostname(parsed) == "api.github.com"
}

func normalizedURLHostname(parsed *url.URL) string {
	if parsed == nil {
		return ""
	}
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(parsed.Hostname())), ".")
}

func normalizeGitHubRepo(repo string) (string, error) {
	repo = strings.TrimSpace(repo)
	parts := strings.Split(repo, "/")
	if len(parts) != 2 {
		return "", errors.New("github repo must use owner/repo format")
	}
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return "", errors.New("github repo must use owner/repo format")
		}
		for _, r := range part {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
				continue
			}
			return "", errors.New("github repo contains invalid characters")
		}
	}
	return repo, nil
}

func doServerUpdateDownload(req *http.Request) (*http.Response, error) {
	if req == nil || req.URL == nil {
		return nil, errors.New("download request is invalid")
	}
	if err := validateServerUpdateDownloadURL(req.URL.String()); err != nil {
		return nil, err
	}
	baseClient := updateHTTPClient
	if baseClient == nil {
		baseClient = http.DefaultClient
	}
	client := *baseClient
	previousCheckRedirect := client.CheckRedirect
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if req == nil || req.URL == nil {
			return errors.New("download redirect request is invalid")
		}
		if err := validateServerUpdateDownloadURL(req.URL.String()); err != nil {
			return err
		}
		req.Header.Del("Authorization")
		req.Header.Del("X-GitHub-Api-Version")
		applyGitHubReleaseAuth(req)
		if previousCheckRedirect != nil {
			return previousCheckRedirect(req, via)
		}
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		return nil
	}
	return client.Do(req)
}

func executeServerUpgrade(task *preparedServerUpgrade) error {
	recordServerUpgradeLog("info", fmt.Sprintf("Downloading automatic upgrade package for version: %s.", strings.TrimSpace(task.release.TagName)))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	expectedChecksum, err := downloadServerChecksum(ctx, task.checksumURL, task.assetName)
	if err != nil {
		return fmt.Errorf("下载服务端升级包校验文件失败: %w", err)
	}
	recordServerUpgradeLog("info", fmt.Sprintf("Downloaded checksum asset: %s.", strings.TrimSpace(task.checksumName)))
	signature, err := downloadServerSignature(ctx, task.signatureURL)
	if err != nil {
		return fmt.Errorf("下载服务端升级包签名文件失败: %w", err)
	}
	recordServerUpgradeLog("info", fmt.Sprintf("Downloaded signature asset: %s.", strings.TrimSpace(task.signatureName)))
	if err = verifyServerReleaseSignature(task.release.TagName, task.assetName, expectedChecksum, signature); err != nil {
		return fmt.Errorf("服务端升级包签名校验失败: %w", err)
	}
	recordServerUpgradeLog("info", "Release signature verified.")

	req, err := newServerUpdateDownloadRequest(ctx, task.downloadURL, "application/octet-stream")
	if err != nil {
		return err
	}

	resp, err := doServerUpdateDownload(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载服务端升级包失败: %s", resp.Status)
	}
	if resp.Request != nil && resp.Request.URL != nil {
		if err := validateServerUpdateDownloadURL(resp.Request.URL.String()); err != nil {
			return fmt.Errorf("服务端升级包最终下载地址不安全: %w", err)
		}
	}

	recordServerUpgradeLog("info", "Download finished, validating checksum and signature.")
	tempPath, err := persistUploadedServerBinary(filepath.Dir(task.execPath), task.assetName, resp.Body)
	if err != nil {
		return err
	}
	if err = verifyServerBinaryChecksum(tempPath, expectedChecksum); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	recordServerUpgradeLog("info", "Binary checksum verified.")
	if err = verifyServerReleaseSignature(task.release.TagName, task.assetName, expectedChecksum, signature); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	recordServerUpgradeLog("info", "Binary signature verified.")
	recordServerUpgradeLog("info", "Validating binary version.")
	candidate, err := buildDownloadedServerBinaryCandidate(ctx, task.execPath, task.release.TagName, task.assetName, tempPath)
	if err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	return executeServerBinaryCandidateUpgrade(candidate, "auto")
}

func downloadServerChecksum(ctx context.Context, url string, assetName string) (string, error) {
	url = strings.TrimSpace(url)
	if url == "" {
		return "", errors.New("checksum url is empty")
	}
	req, err := newServerUpdateDownloadRequest(ctx, url, "text/plain")
	if err != nil {
		return "", err
	}
	resp, err := doServerUpdateDownload(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("checksum download returned %s", resp.Status)
	}
	if resp.Request != nil && resp.Request.URL != nil {
		if err := validateServerUpdateDownloadURL(resp.Request.URL.String()); err != nil {
			return "", fmt.Errorf("checksum download final url is unsafe: %w", err)
		}
	}
	content, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024+1))
	if err != nil {
		return "", err
	}
	if len(content) > 64*1024 {
		return "", errors.New("checksum asset exceeds 64 KB")
	}
	return parseServerChecksum(string(content), assetName)
}

func downloadServerSignature(ctx context.Context, url string) ([]byte, error) {
	url = strings.TrimSpace(url)
	if url == "" {
		return nil, errors.New("signature url is empty")
	}
	req, err := newServerUpdateDownloadRequest(ctx, url, "text/plain")
	if err != nil {
		return nil, err
	}
	resp, err := doServerUpdateDownload(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("signature download returned %s", resp.Status)
	}
	if resp.Request != nil && resp.Request.URL != nil {
		if err := validateServerUpdateDownloadURL(resp.Request.URL.String()); err != nil {
			return nil, fmt.Errorf("signature download final url is unsafe: %w", err)
		}
	}
	content, err := io.ReadAll(io.LimitReader(resp.Body, 8*1024+1))
	if err != nil {
		return nil, err
	}
	if len(content) > 8*1024 {
		return nil, errors.New("signature asset exceeds 8 KB")
	}
	return parseServerReleaseSignature(string(content))
}

func parseServerChecksum(content string, assetName string) (string, error) {
	assetName = strings.TrimSpace(assetName)
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 1 && isSHA256Hex(fields[0]) {
			return strings.ToLower(fields[0]), nil
		}
		if len(fields) >= 2 && isSHA256Hex(fields[0]) {
			fileName := strings.TrimPrefix(strings.TrimSpace(fields[1]), "*")
			if assetName == "" || fileName == assetName || filepath.Base(fileName) == assetName {
				return strings.ToLower(fields[0]), nil
			}
		}
		if strings.HasPrefix(strings.ToLower(line), "sha256(") {
			prefixEnd := strings.Index(line, ")=")
			if prefixEnd > len("sha256(") {
				fileName := line[len("sha256("):prefixEnd]
				value := strings.TrimSpace(line[prefixEnd+2:])
				if isSHA256Hex(value) && (assetName == "" || fileName == assetName || filepath.Base(fileName) == assetName) {
					return strings.ToLower(value), nil
				}
			}
		}
	}
	if assetName == "" {
		return "", errors.New("checksum asset does not contain a valid sha256 digest")
	}
	return "", fmt.Errorf("checksum asset does not contain a sha256 digest for %q", assetName)
}

func parseServerReleaseSignature(content string) ([]byte, error) {
	value := strings.TrimSpace(content)
	if value == "" {
		return nil, errors.New("signature asset is empty")
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
		return nil, errors.New("signature asset is not valid base64")
	}
	if len(signature) != ed25519.SignatureSize {
		return nil, errors.New("signature length is invalid")
	}
	return signature, nil
}

func serverReleaseSignaturePayload(tagName string, assetName string, checksum string) []byte {
	return []byte(strings.Join([]string{
		"dushengcdn-release-v1",
		strings.TrimSpace(tagName),
		strings.TrimSpace(assetName),
		strings.ToLower(strings.TrimSpace(checksum)),
		"",
	}, "\n"))
}

func verifyServerReleaseSignature(tagName string, assetName string, checksum string, signature []byte) error {
	publicKeyText := strings.TrimSpace(common.ReleaseSignaturePublicKey)
	if publicKeyText == "" {
		return errors.New("release signature public key is not configured")
	}
	publicKey, err := base64.StdEncoding.DecodeString(publicKeyText)
	if err != nil {
		publicKey, err = base64.RawStdEncoding.DecodeString(publicKeyText)
	}
	if err != nil || len(publicKey) != ed25519.PublicKeySize {
		return errors.New("release signature public key is invalid")
	}
	if len(signature) != ed25519.SignatureSize {
		return errors.New("release signature is invalid")
	}
	if !ed25519.Verify(ed25519.PublicKey(publicKey), serverReleaseSignaturePayload(tagName, assetName, checksum), signature) {
		return errors.New("release signature verification failed")
	}
	return nil
}

func verifyServerBinaryChecksum(path string, expectedChecksum string) error {
	expectedChecksum = strings.ToLower(strings.TrimSpace(expectedChecksum))
	if !isSHA256Hex(expectedChecksum) {
		return errors.New("invalid expected sha256 checksum")
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	hasher := sha256.New()
	if _, err = io.Copy(hasher, file); err != nil {
		return err
	}
	actualChecksum := hex.EncodeToString(hasher.Sum(nil))
	if actualChecksum != expectedChecksum {
		return fmt.Errorf("服务端升级包 SHA256 校验失败: expected %s, got %s", expectedChecksum, actualChecksum)
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

func executeServerBinaryCandidateUpgrade(task *manualServerBinaryCandidate, source string) error {
	recordServerUpgradeLog("info", fmt.Sprintf("Validated binary version: %s -> %s.", strings.TrimSpace(task.CurrentVersion), strings.TrimSpace(task.DetectedVersion)))
	recordServerUpgradeLog("info", "Replacing executable and preparing restart.")
	if source == "manual" {
		slog.Info("server manual self-update starting", "from", strings.TrimSpace(task.CurrentVersion), "to", strings.TrimSpace(task.DetectedVersion))
	} else {
		slog.Info("server self-update starting", "from", strings.TrimSpace(task.CurrentVersion), "to", strings.TrimSpace(task.DetectedVersion))
	}
	markServerUpgradeSucceeded()
	return serverBinaryUpgradeExecutor(task.ExecPath, task.TempPath)
}

func serverAssetName(goos string, goarch string) string {
	name := fmt.Sprintf("dushengcdn-server-%s-%s", goos, goarch)
	if goos == "windows" {
		return name + ".exe"
	}
	return name
}

func normalizeReleaseChannel(channel string) ReleaseChannel {
	switch strings.ToLower(strings.TrimSpace(channel)) {
	case string(ReleaseChannelPreview):
		return ReleaseChannelPreview
	default:
		return ReleaseChannelStable
	}
}

func (channel ReleaseChannel) String() string {
	if channel == ReleaseChannelPreview {
		return string(ReleaseChannelPreview)
	}
	return string(ReleaseChannelStable)
}

func isVersionNewer(current string, latest string) bool {
	currentInfo := parseVersionInfo(current)
	latestInfo := parseVersionInfo(latest)
	if currentInfo.IsDev {
		return latestInfo.Valid
	}
	if !currentInfo.Valid || !latestInfo.Valid {
		return false
	}
	return compareVersionInfo(currentInfo, latestInfo) < 0
}

type versionInfo struct {
	Valid               bool
	IsDev               bool
	Numbers             []int
	Prerelease          []string
	GitDescribeDistance int
	GitDescribeTail     []string
}

func parseVersionInfo(version string) versionInfo {
	normalized := strings.TrimSpace(strings.TrimPrefix(version, "v"))
	if normalized == "" || normalized == "dev" {
		return versionInfo{IsDev: strings.EqualFold(normalized, "dev")}
	}
	base := normalized
	prerelease := ""
	if separator := strings.IndexRune(normalized, '-'); separator >= 0 {
		base = normalized[:separator]
		prerelease = normalized[separator+1:]
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
			parts = append(parts, 0)
			continue
		}
		value, err := strconv.Atoi(numeric.String())
		if err != nil {
			return versionInfo{}
		}
		parts = append(parts, value)
	}
	info := versionInfo{Valid: len(parts) > 0, Numbers: parts}
	if prerelease != "" {
		identifiers := splitPrereleaseIdentifiers(prerelease)
		if distance, tail, ok := parseGitDescribeIdentifiers(identifiers); ok {
			info.GitDescribeDistance = distance
			info.GitDescribeTail = tail
		} else {
			info.Prerelease = identifiers
		}
	}
	return info
}

func parseGitDescribeIdentifiers(identifiers []string) (int, []string, bool) {
	if len(identifiers) < 2 {
		return 0, nil, false
	}
	distance, err := strconv.Atoi(strings.TrimSpace(identifiers[0]))
	if err != nil || distance <= 0 {
		return 0, nil, false
	}
	commitToken := strings.TrimSpace(identifiers[1])
	if commitToken == "" || !strings.HasPrefix(strings.ToLower(commitToken), "g") {
		return 0, nil, false
	}
	return distance, identifiers[1:], true
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

func compareVersionInfo(left versionInfo, right versionInfo) int {
	maxLen := len(left.Numbers)
	if len(right.Numbers) > maxLen {
		maxLen = len(right.Numbers)
	}
	for index := 0; index < maxLen; index++ {
		leftValue := 0
		rightValue := 0
		if index < len(left.Numbers) {
			leftValue = left.Numbers[index]
		}
		if index < len(right.Numbers) {
			rightValue = right.Numbers[index]
		}
		if leftValue < rightValue {
			return -1
		}
		if leftValue > rightValue {
			return 1
		}
	}

	if left.GitDescribeDistance != right.GitDescribeDistance {
		if left.GitDescribeDistance < right.GitDescribeDistance {
			return -1
		}
		return 1
	}
	if left.GitDescribeDistance > 0 || right.GitDescribeDistance > 0 {
		maxLen = len(left.GitDescribeTail)
		if len(right.GitDescribeTail) > maxLen {
			maxLen = len(right.GitDescribeTail)
		}
		for index := 0; index < maxLen; index++ {
			if index >= len(left.GitDescribeTail) {
				return -1
			}
			if index >= len(right.GitDescribeTail) {
				return 1
			}
			if left.GitDescribeTail[index] < right.GitDescribeTail[index] {
				return -1
			}
			if left.GitDescribeTail[index] > right.GitDescribeTail[index] {
				return 1
			}
		}
		return 0
	}

	if len(left.Prerelease) == 0 && len(right.Prerelease) == 0 {
		return 0
	}
	if len(left.Prerelease) == 0 {
		return 1
	}
	if len(right.Prerelease) == 0 {
		return -1
	}

	maxLen = len(left.Prerelease)
	if len(right.Prerelease) > maxLen {
		maxLen = len(right.Prerelease)
	}
	for index := 0; index < maxLen; index++ {
		if index >= len(left.Prerelease) {
			return -1
		}
		if index >= len(right.Prerelease) {
			return 1
		}
		leftPart := left.Prerelease[index]
		rightPart := right.Prerelease[index]
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

func buildUploadedServerBinaryView(fileName string, currentVersion string, detectedVersion string, uploadedAt time.Time) *UploadedServerBinary {
	upgradeSupported := isManualServerUpgradeSupported(currentVersion)
	hasUpdate := false
	comparisonMessage := ""

	switch {
	case !upgradeSupported:
		comparisonMessage = "当前服务版本不支持手动升级确认流程"
	case normalizeVersion(currentVersion) == normalizeVersion(detectedVersion):
		comparisonMessage = "上传二进制与当前服务版本一致，无需升级"
	case isVersionNewer(currentVersion, detectedVersion):
		hasUpdate = true
		comparisonMessage = fmt.Sprintf("检测到可升级版本：%s -> %s", strings.TrimSpace(currentVersion), strings.TrimSpace(detectedVersion))
	default:
		comparisonMessage = "上传二进制版本不高于当前服务版本，已拒绝升级"
	}

	return &UploadedServerBinary{
		FileName:          strings.TrimSpace(fileName),
		DetectedVersion:   strings.TrimSpace(detectedVersion),
		CurrentVersion:    strings.TrimSpace(currentVersion),
		HasUpdate:         hasUpdate,
		UpgradeSupported:  upgradeSupported,
		ReadyToUpgrade:    upgradeSupported && hasUpdate,
		ComparisonMessage: comparisonMessage,
		UploadedAt:        uploadedAt,
	}
}

func isManualServerUpgradeSupported(currentVersion string) bool {
	normalized := strings.TrimSpace(strings.TrimPrefix(currentVersion, "v"))
	return normalized != "" && !strings.EqualFold(normalized, "dev")
}

func persistUploadedServerBinary(tempDir string, fileName string, reader io.Reader) (string, error) {
	suffix := safeServerBinarySuffix(fileName)
	if runtime.GOOS == "windows" && suffix == "" {
		suffix = ".exe"
	}
	tempDir = strings.TrimSpace(tempDir)
	if tempDir == "" {
		tempDir = os.TempDir()
	}
	tempFile, err := os.CreateTemp(tempDir, "dushengcdn-server-manual-upgrade-*"+suffix)
	if err != nil {
		return "", fmt.Errorf("创建临时升级文件失败: %v", err)
	}
	tempPath := tempFile.Name()
	limitedReader := io.LimitReader(reader, manualServerBinaryMaxBytes+1)
	written, err := io.Copy(tempFile, limitedReader)
	if err != nil {
		_ = tempFile.Close()
		_ = os.Remove(tempPath)
		return "", fmt.Errorf("写入上传二进制失败: %v", err)
	}
	if written > manualServerBinaryMaxBytes {
		_ = tempFile.Close()
		_ = os.Remove(tempPath)
		return "", fmt.Errorf("上传二进制超过大小限制（最大 %d MB）", manualServerBinaryMaxBytes/1024/1024)
	}
	if err = tempFile.Close(); err != nil {
		_ = os.Remove(tempPath)
		return "", fmt.Errorf("关闭临时升级文件失败: %v", err)
	}
	if err = os.Chmod(tempPath, 0o755); err != nil && runtime.GOOS != "windows" {
		_ = os.Remove(tempPath)
		return "", fmt.Errorf("设置临时升级文件权限失败: %v", err)
	}
	return tempPath, nil
}

func safeServerBinarySuffix(fileName string) string {
	suffix := strings.ToLower(filepath.Ext(filepath.Base(strings.TrimSpace(fileName))))
	switch suffix {
	case ".exe", ".bin", ".run", ".sh", ".cmd":
		return suffix
	default:
		return ""
	}
}

func detectUploadedServerBinaryVersion(ctx context.Context, filePath string) (string, error) {
	commandCtx := ctx
	if commandCtx == nil {
		commandCtx = context.Background()
	}
	commandCtx, cancel := context.WithTimeout(commandCtx, uploadedBinaryVersionTimeout)
	defer cancel()
	cmd := exec.CommandContext(commandCtx, filePath, "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		if errors.Is(commandCtx.Err(), context.DeadlineExceeded) {
			return "", fmt.Errorf("检查上传二进制版本超时")
		}
		return "", fmt.Errorf("检查上传二进制版本失败: %w: %s", err, strings.TrimSpace(string(output)))
	}
	version := strings.TrimSpace(string(output))
	if version == "" {
		return "", fmt.Errorf("上传二进制未返回有效版本号")
	}
	for _, line := range strings.Split(version, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			return trimmed, nil
		}
	}
	return "", fmt.Errorf("上传二进制未返回有效版本号")
}

func persistDownloadedServerBinary(ctx context.Context, execPath string, releaseTag string, reader io.Reader) (*manualServerBinaryCandidate, error) {
	fileName := serverAssetName(runtime.GOOS, runtime.GOARCH)
	tempPath, err := persistUploadedServerBinary(filepath.Dir(execPath), fileName, reader)
	if err != nil {
		return nil, err
	}
	candidate, err := buildDownloadedServerBinaryCandidate(ctx, execPath, releaseTag, fileName, tempPath)
	if err != nil {
		_ = os.Remove(tempPath)
		return nil, err
	}
	return candidate, nil
}

func buildDownloadedServerBinaryCandidate(ctx context.Context, execPath string, releaseTag string, fileName string, tempPath string) (*manualServerBinaryCandidate, error) {
	detectedVersion, err := detectUploadedServerBinaryVersion(ctx, tempPath)
	if err != nil {
		return nil, err
	}
	recordServerUpgradeLog("info", fmt.Sprintf("Detected downloaded binary version: %s.", strings.TrimSpace(detectedVersion)))

	if normalizeVersion(detectedVersion) != normalizeVersion(releaseTag) {
		return nil, fmt.Errorf("下载包版本校验失败：release=%s，binary=%s", strings.TrimSpace(releaseTag), strings.TrimSpace(detectedVersion))
	}

	info := buildUploadedServerBinaryView(fileName, common.Version, detectedVersion, time.Now())
	if !info.ReadyToUpgrade {
		return nil, errors.New(info.ComparisonMessage)
	}

	return &manualServerBinaryCandidate{
		FileName:        fileName,
		DetectedVersion: detectedVersion,
		CurrentVersion:  strings.TrimSpace(common.Version),
		TempPath:        tempPath,
		ExecPath:        execPath,
		UploadedAt:      time.Now(),
	}, nil
}

func cleanupManualServerBinaryCandidateLocked() {
	if manualServerBinaryState.candidate == nil {
		return
	}
	_ = os.Remove(manualServerBinaryState.candidate.TempPath)
	manualServerBinaryState.candidate = nil
}

func isManualServerBinaryCandidateExpired(candidate *manualServerBinaryCandidate) bool {
	if candidate == nil {
		return true
	}
	if candidate.UploadedAt.IsZero() {
		return true
	}
	return time.Since(candidate.UploadedAt) > manualServerBinaryTTL
}

func newUpgradeToken() (string, error) {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}

func normalizeVersion(version string) string {
	return strings.TrimSpace(strings.TrimPrefix(version, "v"))
}

func snapshotServerUpgradeState() (bool, string, []ServerUpgradeLogRecord) {
	serverUpgradeState.Lock()
	defer serverUpgradeState.Unlock()

	status := strings.TrimSpace(serverUpgradeState.status)
	if status == "" {
		status = "idle"
	}
	logs := make([]ServerUpgradeLogRecord, len(serverUpgradeState.logs))
	copy(logs, serverUpgradeState.logs)
	return serverUpgradeState.inProgress, status, logs
}

func snapshotServerUpgradeStream() ServerUpgradeStreamSnapshot {
	inProgress, status, logs := snapshotServerUpgradeState()
	return ServerUpgradeStreamSnapshot{
		InProgress:    inProgress,
		UpgradeStatus: status,
		UpgradeLogs:   logs,
	}
}

func resetServerUpgradeLogsLocked() {
	serverUpgradeState.logs = nil
}

func appendServerUpgradeLogLocked(level string, message string) {
	serverUpgradeState.logs = append(serverUpgradeState.logs, ServerUpgradeLogRecord{
		Level:     strings.TrimSpace(level),
		Message:   strings.TrimSpace(message),
		CreatedAt: time.Now(),
	})
	if len(serverUpgradeState.logs) > 100 {
		serverUpgradeState.logs = append([]ServerUpgradeLogRecord(nil), serverUpgradeState.logs[len(serverUpgradeState.logs)-100:]...)
	}
}

func recordServerUpgradeLog(level string, message string) {
	serverUpgradeState.Lock()
	appendServerUpgradeLogLocked(level, message)
	serverUpgradeState.Unlock()
	broadcastServerUpgradeSnapshot()
}

func markServerUpgradeSucceeded() {
	serverUpgradeState.Lock()
	serverUpgradeState.inProgress = false
	serverUpgradeState.status = "succeeded"
	appendServerUpgradeLogLocked("info", "Upgrade binary is ready; server restart will begin.")
	serverUpgradeState.Unlock()
	broadcastServerUpgradeSnapshot()
}

func recordServerUpgradeFailure(err error) {
	serverUpgradeState.Lock()
	serverUpgradeState.inProgress = false
	serverUpgradeState.status = "failed"
	if err != nil {
		appendServerUpgradeLogLocked("error", err.Error())
	}
	serverUpgradeState.Unlock()
	broadcastServerUpgradeSnapshot()
}

func SubscribeServerUpgradeStream() (<-chan ServerUpgradeStreamSnapshot, func()) {
	serverUpgradeSubscribers.Lock()
	if serverUpgradeSubscribers.listeners == nil {
		serverUpgradeSubscribers.listeners = make(map[int]chan ServerUpgradeStreamSnapshot)
	}
	serverUpgradeSubscribers.nextID++
	listenerID := serverUpgradeSubscribers.nextID
	listener := make(chan ServerUpgradeStreamSnapshot, 8)
	serverUpgradeSubscribers.listeners[listenerID] = listener
	serverUpgradeSubscribers.Unlock()

	listener <- snapshotServerUpgradeStream()

	unsubscribe := func() {
		serverUpgradeSubscribers.Lock()
		ch, ok := serverUpgradeSubscribers.listeners[listenerID]
		if ok {
			delete(serverUpgradeSubscribers.listeners, listenerID)
		}
		serverUpgradeSubscribers.Unlock()
		if ok {
			close(ch)
		}
	}

	return listener, unsubscribe
}

func broadcastServerUpgradeSnapshot() {
	snapshot := snapshotServerUpgradeStream()

	serverUpgradeSubscribers.Lock()
	if len(serverUpgradeSubscribers.listeners) == 0 {
		serverUpgradeSubscribers.Unlock()
		return
	}
	listeners := make([]chan ServerUpgradeStreamSnapshot, 0, len(serverUpgradeSubscribers.listeners))
	for _, listener := range serverUpgradeSubscribers.listeners {
		listeners = append(listeners, listener)
	}
	serverUpgradeSubscribers.Unlock()

	for _, listener := range listeners {
		select {
		case listener <- snapshot:
		default:
			select {
			case <-listener:
			default:
			}
			select {
			case listener <- snapshot:
			default:
			}
		}
	}
}

func UpdateHTTPClientForTest() *http.Client {
	return updateHTTPClient
}

func SetUpdateHTTPClientForTest(client *http.Client) {
	updateHTTPClient = client
}

func ServerBinaryUpgradeExecutorForTest() func(string, string) error {
	return serverBinaryUpgradeExecutor
}

func SetServerBinaryUpgradeExecutorForTest(executor func(string, string) error) {
	if executor == nil {
		serverBinaryUpgradeExecutor = replaceAndRestartServer
		return
	}
	serverBinaryUpgradeExecutor = executor
}

func ServerUpgradeDispatchDelayForTest() time.Duration {
	return serverUpgradeDispatchDelay
}

func SetServerUpgradeDispatchDelayForTest(delay time.Duration) {
	if delay < 0 {
		delay = 0
	}
	serverUpgradeDispatchDelay = delay
}

func ManualServerBinaryMaxBytesForTest() int64 {
	return manualServerBinaryMaxBytes
}

func SetManualServerBinaryMaxBytesForTest(maxBytes int64) {
	if maxBytes <= 0 {
		maxBytes = 200 * 1024 * 1024
	}
	manualServerBinaryMaxBytes = maxBytes
}
