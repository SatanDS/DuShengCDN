package service

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"dushengcdn/common"
	"encoding/base64"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const (
	testServerReleaseTag = "v0.5.0"
	testServerAssetName  = "dushengcdn-server-linux-amd64"
	testServerBinaryURL  = "https://github.com/SatanDS/DuShengCDN/releases/download/v0.5.0/server"
	testServerSHAURL     = "https://github.com/SatanDS/DuShengCDN/releases/download/v0.5.0/server.sha256"
	testServerSigURL     = "https://github.com/SatanDS/DuShengCDN/releases/download/v0.5.0/server.sig"
)

func resetServerUpgradeTestState(t *testing.T) {
	t.Helper()
	serverUpgradeState.Lock()
	serverUpgradeState.inProgress = false
	serverUpgradeState.status = ""
	serverUpgradeState.logs = nil
	serverUpgradeState.Unlock()
	manualServerBinaryState.Lock()
	cleanupManualServerBinaryCandidateLocked()
	manualServerBinaryState.Unlock()
}

func fakeServerBinaryFixture(version string) (string, []byte) {
	if runtime.GOOS == "windows" {
		return "dushengcdn-server-test.cmd", []byte("@echo off\r\necho " + version + "\r\n")
	}
	return "dushengcdn-server-test.sh", []byte("#!/bin/sh\necho " + version + "\n")
}

func withServerReleaseSigningKey(t *testing.T) ed25519.PrivateKey {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate test signing key: %v", err)
	}
	originalPublicKey := common.ReleaseSignaturePublicKey
	common.ReleaseSignaturePublicKey = base64.StdEncoding.EncodeToString(publicKey)
	t.Cleanup(func() {
		common.ReleaseSignaturePublicKey = originalPublicKey
	})
	return privateKey
}

func signServerReleaseForTest(t *testing.T, privateKey ed25519.PrivateKey, tagName string, assetName string, checksum string) string {
	t.Helper()
	signature := ed25519.Sign(privateKey, serverReleaseSignaturePayload(tagName, assetName, checksum))
	return base64.StdEncoding.EncodeToString(signature)
}

func TestIsVersionNewer(t *testing.T) {
	testCases := []struct {
		name     string
		current  string
		latest   string
		expected bool
	}{
		{name: "newer patch", current: "v1.2.3", latest: "v1.2.4", expected: true},
		{name: "same version", current: "v1.2.3", latest: "v1.2.3", expected: false},
		{name: "older remote", current: "v1.3.0", latest: "v1.2.9", expected: false},
		{name: "double digit segment", current: "v1.9.9", latest: "v1.10.0", expected: true},
		{name: "stable newer than prerelease", current: "v1.2.3-rc.1", latest: "v1.2.3", expected: true},
		{name: "prerelease not newer than same stable", current: "v1.2.3", latest: "v1.2.3-rc.1", expected: false},
		{name: "newer prerelease sequence", current: "v1.2.3-rc.1", latest: "v1.2.3-rc.2", expected: true},
		{name: "git describe newer than same tag", current: "v0.6.3", latest: "v0.6.3-2-gf4d36be", expected: true},
		{name: "git describe distance compares numerically", current: "v0.6.3-2-gf4d36be", latest: "v0.6.3-5-gabc1234", expected: true},
		{name: "dev build", current: "dev", latest: "v0.4.0", expected: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			actual := isVersionNewer(testCase.current, testCase.latest)
			if actual != testCase.expected {
				t.Fatalf("unexpected compare result: current=%s latest=%s actual=%v expected=%v", testCase.current, testCase.latest, actual, testCase.expected)
			}
		})
	}
}

func TestBuildLatestServerReleaseView(t *testing.T) {
	originalVersion := common.Version
	originalAutoUpgrade := common.ServerAutoUpgradeEnabled
	common.Version = "v0.4.0"
	common.ServerAutoUpgradeEnabled = true
	t.Cleanup(func() {
		common.Version = originalVersion
		common.ServerAutoUpgradeEnabled = originalAutoUpgrade
		serverUpgradeState.Lock()
		serverUpgradeState.inProgress = false
		serverUpgradeState.Unlock()
	})

	serverUpgradeState.Lock()
	serverUpgradeState.inProgress = true
	serverUpgradeState.Unlock()

	view := buildLatestServerReleaseView(&githubReleaseResponse{
		TagName:     "v0.5.0",
		Body:        "release notes",
		HTMLURL:     "https://github.com/SatanDS/DuShengCDN/releases/tag/v0.5.0",
		PublishedAt: "2026-03-11T00:00:00Z",
	}, ReleaseChannelStable)

	if view.CurrentVersion != "v0.4.0" {
		t.Fatalf("unexpected current version: %s", view.CurrentVersion)
	}
	if !view.HasUpdate {
		t.Fatal("expected has_update to be true")
	}
	if !view.InProgress {
		t.Fatal("expected in_progress to reflect upgrade state")
	}
	if !view.AutomaticUpgradeEnabled {
		t.Fatal("expected automatic upgrade flag to be exposed")
	}
	if view.TagName != "v0.5.0" {
		t.Fatalf("unexpected tag name: %s", view.TagName)
	}
	if view.Channel != ReleaseChannelStable.String() {
		t.Fatalf("unexpected channel: %s", view.Channel)
	}
}

func TestBuildLatestServerReleaseViewAutoDisabledStillReportsOnlineSupport(t *testing.T) {
	originalVersion := common.Version
	originalAutoUpgrade := common.ServerAutoUpgradeEnabled
	common.Version = "v0.4.0"
	common.ServerAutoUpgradeEnabled = false
	t.Cleanup(func() {
		common.Version = originalVersion
		common.ServerAutoUpgradeEnabled = originalAutoUpgrade
		resetServerUpgradeTestState(t)
	})

	view := buildLatestServerReleaseView(&githubReleaseResponse{
		TagName: "v0.5.0",
	}, ReleaseChannelStable)

	if view.AutomaticUpgradeEnabled {
		t.Fatal("expected automatic upgrade flag to remain disabled")
	}
	if view.UpgradeSupported != (runtime.GOOS != "windows") {
		t.Fatalf("expected online upgrade support to depend on platform, got %v", view.UpgradeSupported)
	}
}

func TestBuildLatestServerReleaseViewDevBuild(t *testing.T) {
	originalVersion := common.Version
	common.Version = "dev"
	t.Cleanup(func() {
		common.Version = originalVersion
		serverUpgradeState.Lock()
		serverUpgradeState.inProgress = false
		serverUpgradeState.Unlock()
	})

	view := buildLatestServerReleaseView(&githubReleaseResponse{
		TagName: "v0.5.0",
	}, ReleaseChannelStable)

	if !view.HasUpdate {
		t.Fatal("expected dev build to report available release package")
	}
	if view.UpgradeSupported {
		t.Fatal("expected dev build not to support self-upgrade")
	}
}

func TestBuildLatestServerReleaseViewPreviewSameVersion(t *testing.T) {
	originalVersion := common.Version
	common.Version = "v1.9.5-private.123-gabcdef0"
	t.Cleanup(func() {
		common.Version = originalVersion
		resetServerUpgradeTestState(t)
	})

	view := buildLatestServerReleaseView(&githubReleaseResponse{
		TagName:    "v1.9.5-private.123-gabcdef0",
		Prerelease: true,
	}, ReleaseChannelPreview)

	if view.HasUpdate {
		t.Fatal("expected preview release matching current version not to report update")
	}
}

func TestBuildLatestServerReleaseViewPreview(t *testing.T) {
	originalVersion := common.Version
	common.Version = "v0.5.0-rc.1"
	t.Cleanup(func() {
		common.Version = originalVersion
		resetServerUpgradeTestState(t)
	})

	view := buildLatestServerReleaseView(&githubReleaseResponse{
		TagName:     "v0.5.0-rc.2",
		Prerelease:  true,
		PublishedAt: "2026-03-12T00:00:00Z",
	}, ReleaseChannelPreview)

	if !view.HasUpdate {
		t.Fatal("expected preview release to be newer")
	}
	if !view.Prerelease {
		t.Fatal("expected preview flag to be true")
	}
	if view.Channel != ReleaseChannelPreview.String() {
		t.Fatalf("unexpected channel: %s", view.Channel)
	}
}

// TestBuildLatestServerReleaseViewPreviewBypassVersionCheck verifies that switching to
// the preview channel always reports has_update=true, even when the preview tag uses a
// "major.minor.patch-git-<commit>" scheme that would otherwise compare as equal-or-older
// than the currently running stable version.
func TestBuildLatestServerReleaseViewPreviewBypassVersionCheck(t *testing.T) {
	originalVersion := common.Version
	common.Version = "v1.0.0"
	t.Cleanup(func() {
		common.Version = originalVersion
		resetServerUpgradeTestState(t)
	})

	// A typical preview tag: same base version as stable but with a git-commit suffix.
	// Without the bypass, isVersionNewer("v1.0.0", "v1.0.0-git-abc1234") returns false
	// because a version without a prerelease identifier is considered higher than one
	// with a prerelease identifier under semver rules.
	view := buildLatestServerReleaseView(&githubReleaseResponse{
		TagName:     "v1.0.0-git-abc1234",
		Prerelease:  true,
		PublishedAt: "2026-03-12T00:00:00Z",
	}, ReleaseChannelPreview)

	if !view.HasUpdate {
		t.Fatal("expected preview channel to bypass version comparison and report has_update=true")
	}
	if view.Channel != ReleaseChannelPreview.String() {
		t.Fatalf("unexpected channel: %s", view.Channel)
	}
}

func TestGetLatestServerReleaseStableNotFoundReturnsUnavailable(t *testing.T) {
	originalClient := UpdateHTTPClientForTest()
	originalRepo := common.ServerUpdateRepo
	common.ServerUpdateRepo = "SatanDS/SatanDS-DuShengCDN-releases"
	SetUpdateHTTPClientForTest(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.String() != "https://api.github.com/repos/SatanDS/SatanDS-DuShengCDN-releases/releases/latest" {
				t.Fatalf("unexpected request url: %s", req.URL.String())
			}
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Status:     "404 Not Found",
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"message":"Not Found"}`)),
			}, nil
		}),
	})
	t.Cleanup(func() {
		SetUpdateHTTPClientForTest(originalClient)
		common.ServerUpdateRepo = originalRepo
		resetServerUpgradeTestState(t)
	})

	release, err := GetLatestServerRelease(context.Background(), "stable")
	if err != nil {
		t.Fatalf("expected unavailable stable release instead of error: %v", err)
	}
	if release.Available {
		t.Fatal("expected stable release to be unavailable")
	}
	if release.HasUpdate {
		t.Fatal("expected unavailable release not to report an update")
	}
	if release.Channel != ReleaseChannelStable.String() {
		t.Fatalf("unexpected channel: %s", release.Channel)
	}
}

func TestGetLatestServerReleasePreviewWithoutPrereleaseReturnsUnavailable(t *testing.T) {
	originalClient := UpdateHTTPClientForTest()
	originalRepo := common.ServerUpdateRepo
	common.ServerUpdateRepo = "SatanDS/SatanDS-DuShengCDN-releases"
	SetUpdateHTTPClientForTest(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.String() != "https://api.github.com/repos/SatanDS/SatanDS-DuShengCDN-releases/releases?per_page=20" {
				t.Fatalf("unexpected request url: %s", req.URL.String())
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`[{"tag_name":"v1.0.0","prerelease":false}]`)),
			}, nil
		}),
	})
	t.Cleanup(func() {
		SetUpdateHTTPClientForTest(originalClient)
		common.ServerUpdateRepo = originalRepo
		resetServerUpgradeTestState(t)
	})

	release, err := GetLatestServerRelease(context.Background(), "preview")
	if err != nil {
		t.Fatalf("expected unavailable preview release instead of error: %v", err)
	}
	if release.Available {
		t.Fatal("expected preview release to be unavailable")
	}
	if release.Channel != ReleaseChannelPreview.String() {
		t.Fatalf("unexpected channel: %s", release.Channel)
	}
}

func TestUploadManualServerBinary(t *testing.T) {
	originalVersion := common.Version
	common.Version = "v0.4.0"
	t.Cleanup(func() {
		common.Version = originalVersion
		resetServerUpgradeTestState(t)
	})

	fileName, content := fakeServerBinaryFixture("v0.5.0")
	info, err := UploadManualServerBinary(context.Background(), fileName, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("expected upload to succeed: %v", err)
	}
	if !info.ReadyToUpgrade {
		t.Fatal("expected uploaded binary to be ready for upgrade")
	}
	if info.UploadToken == "" {
		t.Fatal("expected upload token to be returned")
	}
	if info.DetectedVersion != "v0.5.0" {
		t.Fatalf("unexpected detected version: %s", info.DetectedVersion)
	}

	manualServerBinaryState.Lock()
	candidate := manualServerBinaryState.candidate
	manualServerBinaryState.Unlock()
	if candidate == nil {
		t.Fatal("expected manual upgrade candidate to be stored")
	}
	if _, err := os.Stat(candidate.TempPath); err != nil {
		t.Fatalf("expected temporary binary to exist: %v", err)
	}
	if candidate.UploadToken != info.UploadToken {
		t.Fatalf("unexpected stored upload token: %s", candidate.UploadToken)
	}
	execPath, err := os.Executable()
	if err != nil {
		t.Fatalf("failed to get executable path: %v", err)
	}
	if filepath.Dir(candidate.TempPath) != filepath.Dir(execPath) {
		t.Fatalf("expected temporary binary in executable dir, got %s want %s", filepath.Dir(candidate.TempPath), filepath.Dir(execPath))
	}
}

func TestBuildUploadedServerBinaryViewAcceptsGitDescribeNewerThanTag(t *testing.T) {
	info := buildUploadedServerBinaryView("dushengcdn-server-test", "v0.6.3", "v0.6.3-2-gf4d36be", time.Now())
	if !info.HasUpdate || !info.ReadyToUpgrade {
		t.Fatalf("expected git describe binary to be upgradeable: %+v", info)
	}
}

func TestUploadManualServerBinaryRejectsSameVersion(t *testing.T) {
	originalVersion := common.Version
	common.Version = "v0.5.0"
	t.Cleanup(func() {
		common.Version = originalVersion
		resetServerUpgradeTestState(t)
	})

	fileName, content := fakeServerBinaryFixture("v0.5.0")
	info, err := UploadManualServerBinary(context.Background(), fileName, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("expected upload to succeed: %v", err)
	}
	if info.ReadyToUpgrade {
		t.Fatal("expected same-version upload not to be upgradeable")
	}
	if info.UploadToken != "" {
		t.Fatal("expected same-version upload not to issue a token")
	}

	manualServerBinaryState.Lock()
	defer manualServerBinaryState.Unlock()
	if manualServerBinaryState.candidate != nil {
		t.Fatal("expected no pending manual upgrade candidate")
	}
}

func TestUploadManualServerBinaryRejectsOversizedUpload(t *testing.T) {
	originalVersion := common.Version
	originalLimit := ManualServerBinaryMaxBytesForTest()
	common.Version = "v0.4.0"
	SetManualServerBinaryMaxBytesForTest(8)
	t.Cleanup(func() {
		common.Version = originalVersion
		SetManualServerBinaryMaxBytesForTest(originalLimit)
		resetServerUpgradeTestState(t)
	})

	_, err := UploadManualServerBinary(context.Background(), "dushengcdn-server-test", bytes.NewReader([]byte("0123456789")))
	if err == nil {
		t.Fatal("expected oversized upload to fail")
	}
	if !strings.Contains(err.Error(), "超过大小限制") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestConfirmManualServerUpgradeRejectsExpiredCandidate(t *testing.T) {
	originalVersion := common.Version
	common.Version = "v0.4.0"
	t.Cleanup(func() {
		common.Version = originalVersion
		resetServerUpgradeTestState(t)
	})

	fileName, content := fakeServerBinaryFixture("v0.5.0")
	info, err := UploadManualServerBinary(context.Background(), fileName, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("expected upload to succeed: %v", err)
	}

	manualServerBinaryState.Lock()
	if manualServerBinaryState.candidate == nil {
		manualServerBinaryState.Unlock()
		t.Fatal("expected pending candidate")
	}
	tempPath := manualServerBinaryState.candidate.TempPath
	manualServerBinaryState.candidate.UploadedAt = time.Now().Add(-manualServerBinaryTTL - time.Second)
	manualServerBinaryState.Unlock()

	if _, err = ConfirmManualServerUpgrade(info.UploadToken); err == nil {
		t.Fatal("expected expired candidate confirmation to fail")
	}
	if !strings.Contains(err.Error(), "已过期") {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, statErr := os.Stat(tempPath); !os.IsNotExist(statErr) {
		t.Fatalf("expected expired temp file to be removed, got %v", statErr)
	}
}

func TestDetectUploadedServerBinaryVersionHonorsContextTimeout(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "slow-version")
	if runtime.GOOS == "windows" {
		path += ".cmd"
		if err := os.WriteFile(path, []byte("@echo off\r\nping -n 3 127.0.0.1 >NUL\r\necho v9.9.9\r\n"), 0o755); err != nil {
			t.Fatalf("failed to write slow script: %v", err)
		}
	} else {
		if err := os.WriteFile(path, []byte("#!/bin/sh\nsleep 2\necho v9.9.9\n"), 0o755); err != nil {
			t.Fatalf("failed to write slow script: %v", err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := detectUploadedServerBinaryVersion(ctx, path)
	if err == nil {
		t.Fatal("expected version detection to time out")
	}
	if !strings.Contains(err.Error(), "超时") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestConfirmManualServerUpgrade(t *testing.T) {
	originalVersion := common.Version
	originalExecutor := ServerBinaryUpgradeExecutorForTest()
	originalDelay := ServerUpgradeDispatchDelayForTest()
	common.Version = "v0.4.0"
	called := make(chan string, 1)
	SetServerBinaryUpgradeExecutorForTest(func(execPath string, tempPath string) error {
		called <- tempPath
		return nil
	})
	SetServerUpgradeDispatchDelayForTest(0)
	t.Cleanup(func() {
		common.Version = originalVersion
		SetServerBinaryUpgradeExecutorForTest(originalExecutor)
		SetServerUpgradeDispatchDelayForTest(originalDelay)
		resetServerUpgradeTestState(t)
	})

	fileName, content := fakeServerBinaryFixture("v0.5.0")
	info, err := UploadManualServerBinary(context.Background(), fileName, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("expected upload to succeed: %v", err)
	}

	confirmed, err := ConfirmManualServerUpgrade(info.UploadToken)
	if err != nil {
		t.Fatalf("expected confirm to succeed: %v", err)
	}
	if confirmed.UploadToken != info.UploadToken {
		t.Fatalf("unexpected confirmed upload token: %s", confirmed.UploadToken)
	}

	select {
	case tempPath := <-called:
		if tempPath == "" {
			t.Fatal("expected upgrade executor to receive temp path")
		}
	case <-time.After(time.Second):
		t.Fatal("expected manual upgrade executor to be called")
	}
}

func TestBuildLatestServerReleaseViewIncludesUpgradeLogs(t *testing.T) {
	originalVersion := common.Version
	common.Version = "v0.4.0"
	t.Cleanup(func() {
		common.Version = originalVersion
		resetServerUpgradeTestState(t)
	})

	serverUpgradeState.Lock()
	serverUpgradeState.inProgress = true
	serverUpgradeState.status = "running"
	serverUpgradeState.logs = []ServerUpgradeLogRecord{
		{
			Level:     "info",
			Message:   "download started",
			CreatedAt: time.Now(),
		},
	}
	serverUpgradeState.Unlock()

	view := buildLatestServerReleaseView(&githubReleaseResponse{
		TagName: "v0.5.0",
	}, ReleaseChannelStable)

	if view.UpgradeStatus != "running" {
		t.Fatalf("expected upgrade status to be running, got %s", view.UpgradeStatus)
	}
	if len(view.UpgradeLogs) != 1 {
		t.Fatalf("expected one upgrade log, got %d", len(view.UpgradeLogs))
	}
	if view.UpgradeLogs[0].Message != "download started" {
		t.Fatalf("unexpected upgrade log message: %s", view.UpgradeLogs[0].Message)
	}
}

func TestScheduleServerUpgradeFetchesReleaseWhenAutoDisabled(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("online server self-upgrade is not supported on Windows")
	}

	originalAutoUpgrade := common.ServerAutoUpgradeEnabled
	originalVersion := common.Version
	originalClient := UpdateHTTPClientForTest()
	common.Version = "v0.4.0"
	common.ServerAutoUpgradeEnabled = false
	requests := 0
	SetUpdateHTTPClientForTest(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			requests++
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"tag_name":"v0.5.0","assets":[]}`)),
			}, nil
		}),
	})
	t.Cleanup(func() {
		common.Version = originalVersion
		common.ServerAutoUpgradeEnabled = originalAutoUpgrade
		SetUpdateHTTPClientForTest(originalClient)
		resetServerUpgradeTestState(t)
	})

	_, err := ScheduleServerUpgrade("stable")
	if err == nil {
		t.Fatal("expected release validation to reject the incomplete test package")
	}
	if strings.Contains(err.Error(), "自动升级默认关闭") {
		t.Fatalf("manual upgrade request was blocked by automatic upgrade flag: %v", err)
	}
	if requests != 1 {
		t.Fatalf("expected manual upgrade request to fetch release metadata once, got %d", requests)
	}
}

func TestValidateServerUpdateDownloadURL(t *testing.T) {
	validURLs := []string{
		"https://github.com/SatanDS/DuShengCDN/releases/download/v0.5.0/dushengcdn-server-linux-amd64",
		"https://objects.githubusercontent.com/github-production-release-asset/file",
		"https://github-releases.githubusercontent.com/asset",
	}
	for _, value := range validURLs {
		if err := validateServerUpdateDownloadURL(value); err != nil {
			t.Fatalf("expected URL %q to be accepted: %v", value, err)
		}
	}

	invalidURLs := []string{
		"http://github.com/SatanDS/DuShengCDN/releases/download/v0.5.0/server",
		"https://github.com:444/SatanDS/DuShengCDN/releases/download/v0.5.0/server",
		"https://user:pass@github.com/SatanDS/DuShengCDN/releases/download/v0.5.0/server",
		"https://example.test/server",
		"https://127.0.0.1/server",
		"not a url",
	}
	for _, value := range invalidURLs {
		if err := validateServerUpdateDownloadURL(value); err == nil {
			t.Fatalf("expected URL %q to be rejected", value)
		}
	}
}

func TestNormalizeGitHubRepo(t *testing.T) {
	if got, err := normalizeGitHubRepo(" SatanDS/DuShengCDN "); err != nil || got != "SatanDS/DuShengCDN" {
		t.Fatalf("unexpected normalized repo: got %q err %v", got, err)
	}
	for _, value := range []string{
		"https://github.com/SatanDS/DuShengCDN",
		"SatanDS/DuShengCDN/releases",
		"SatanDS/../DuShengCDN",
		"SatanDS/DuShengCDN?x=1",
		"SatanDS/",
	} {
		if _, err := normalizeGitHubRepo(value); err == nil {
			t.Fatalf("expected repo %q to be rejected", value)
		}
	}
}

func TestReleaseAssetDownloadURLUsesAPIURLForPrivateRepo(t *testing.T) {
	originalToken := common.GitHubReleaseToken
	common.GitHubReleaseToken = "github_pat_test"
	t.Cleanup(func() {
		common.GitHubReleaseToken = originalToken
	})

	asset := githubAsset{
		URL:                "https://api.github.com/repos/SatanDS/SatanDS-DuShengCDN-releases/releases/assets/123",
		BrowserDownloadURL: "https://github.com/SatanDS/SatanDS-DuShengCDN-releases/releases/download/v0.5.0/server",
	}
	if got := releaseAssetDownloadURL(asset); got != asset.URL {
		t.Fatalf("expected asset API URL, got %s", got)
	}

	common.GitHubReleaseToken = ""
	if got := releaseAssetDownloadURL(asset); got != asset.BrowserDownloadURL {
		t.Fatalf("expected browser download URL without token, got %s", got)
	}
}

func TestServerUpdateDownloadRequestAuthScope(t *testing.T) {
	originalToken := common.GitHubReleaseToken
	common.GitHubReleaseToken = "github_pat_test"
	t.Cleanup(func() {
		common.GitHubReleaseToken = originalToken
	})

	apiReq, err := newServerUpdateDownloadRequest(context.Background(), "https://api.github.com/repos/SatanDS/SatanDS-DuShengCDN-releases/releases/assets/123", "text/plain")
	if err != nil {
		t.Fatalf("build api request: %v", err)
	}
	if apiReq.Header.Get("Authorization") != "Bearer github_pat_test" {
		t.Fatalf("expected GitHub API request to carry auth header")
	}
	if apiReq.Header.Get("Accept") != "application/octet-stream" {
		t.Fatalf("expected GitHub API asset request to use octet-stream accept, got %s", apiReq.Header.Get("Accept"))
	}

	objectReq, err := newServerUpdateDownloadRequest(context.Background(), "https://objects.githubusercontent.com/github-production-release-asset/file", "application/octet-stream")
	if err != nil {
		t.Fatalf("build object request: %v", err)
	}
	if objectReq.Header.Get("Authorization") != "" {
		t.Fatalf("object storage request must not carry auth header")
	}
}

func TestDownloadServerChecksumRejectsUnsafeFinalURL(t *testing.T) {
	originalClient := UpdateHTTPClientForTest()
	SetUpdateHTTPClientForTest(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			finalReq := req.Clone(req.Context())
			finalReq.URL.Scheme = "https"
			finalReq.URL.Host = "example.test"
			finalReq.URL.Path = "/server.sha256"
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     make(http.Header),
				Request:    finalReq,
				Body:       io.NopCloser(strings.NewReader(strings.Repeat("a", sha256.Size*2))),
			}, nil
		}),
	})
	t.Cleanup(func() {
		SetUpdateHTTPClientForTest(originalClient)
	})

	_, err := downloadServerChecksum(context.Background(), "https://github.com/SatanDS/DuShengCDN/releases/download/v0.5.0/server.sha256", "server")
	if err == nil || !strings.Contains(err.Error(), "final url is unsafe") {
		t.Fatalf("expected unsafe final URL error, got %v", err)
	}
}

func TestDownloadServerChecksumRejectsUnsafeInitialURL(t *testing.T) {
	originalClient := UpdateHTTPClientForTest()
	SetUpdateHTTPClientForTest(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			t.Fatal("download request should not be sent for unsafe initial URL")
			return nil, nil
		}),
	})
	t.Cleanup(func() {
		SetUpdateHTTPClientForTest(originalClient)
	})

	_, err := downloadServerChecksum(context.Background(), "https://example.test/server.sha256", "server")
	if err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("expected unsafe initial URL error, got %v", err)
	}
}

func TestParseServerChecksum(t *testing.T) {
	checksum := strings.Repeat("a", sha256.Size*2)
	testCases := []struct {
		name    string
		content string
		asset   string
		want    string
	}{
		{name: "single digest", content: checksum + "\n", asset: "dushengcdn-server-linux-amd64", want: checksum},
		{name: "sha256sum format", content: checksum + "  dushengcdn-server-linux-amd64\n", asset: "dushengcdn-server-linux-amd64", want: checksum},
		{name: "bsd format", content: "SHA256(dushengcdn-server-linux-amd64)= " + checksum + "\n", asset: "dushengcdn-server-linux-amd64", want: checksum},
		{name: "select matching asset", content: strings.Repeat("b", sha256.Size*2) + "  other\n" + checksum + "  dushengcdn-server-linux-amd64\n", asset: "dushengcdn-server-linux-amd64", want: checksum},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := parseServerChecksum(testCase.content, testCase.asset)
			if err != nil {
				t.Fatalf("parse checksum: %v", err)
			}
			if got != testCase.want {
				t.Fatalf("unexpected checksum: got %s want %s", got, testCase.want)
			}
		})
	}
}

func TestExecuteServerUpgradeRejectsChecksumMismatch(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test binary fixtures require unix execution semantics")
	}
	originalVersion := common.Version
	originalClient := UpdateHTTPClientForTest()
	originalExecutor := ServerBinaryUpgradeExecutorForTest()
	common.Version = "v0.4.0"
	executed := false
	SetServerBinaryUpgradeExecutorForTest(func(execPath string, tempPath string) error {
		executed = true
		return nil
	})
	t.Cleanup(func() {
		common.Version = originalVersion
		SetUpdateHTTPClientForTest(originalClient)
		SetServerBinaryUpgradeExecutorForTest(originalExecutor)
		resetServerUpgradeTestState(t)
	})

	_, content := fakeServerBinaryFixture(testServerReleaseTag)
	signature := base64.StdEncoding.EncodeToString(make([]byte, ed25519.SignatureSize))
	SetUpdateHTTPClientForTest(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.String() {
			case testServerSHAURL:
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Body:       io.NopCloser(strings.NewReader(strings.Repeat("0", sha256.Size*2) + "  " + testServerAssetName + "\n")),
					Header:     make(http.Header),
				}, nil
			case testServerSigURL:
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Body:       io.NopCloser(strings.NewReader(signature)),
					Header:     make(http.Header),
				}, nil
			case testServerBinaryURL:
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Body:       io.NopCloser(bytes.NewReader(content)),
					Header:     make(http.Header),
				}, nil
			default:
				t.Fatalf("unexpected request url: %s", req.URL.String())
				return nil, nil
			}
		}),
	})

	err := executeServerUpgrade(&preparedServerUpgrade{
		release: &LatestServerRelease{
			TagName: testServerReleaseTag,
		},
		downloadURL:   testServerBinaryURL,
		checksumURL:   testServerSHAURL,
		signatureURL:  testServerSigURL,
		assetName:     testServerAssetName,
		checksumName:  testServerAssetName + ".sha256",
		signatureName: testServerAssetName + ".sig",
		execPath:      os.Args[0],
	})
	if err == nil || !strings.Contains(err.Error(), "SHA256") {
		t.Fatalf("expected checksum mismatch error, got %v", err)
	}
	if executed {
		t.Fatal("upgrade executor must not run when checksum mismatches")
	}
}

func TestExecuteServerUpgradeVerifiesChecksumBeforeReplace(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test binary fixtures require unix execution semantics")
	}
	originalVersion := common.Version
	originalClient := UpdateHTTPClientForTest()
	originalExecutor := ServerBinaryUpgradeExecutorForTest()
	common.Version = "v0.4.0"
	executed := false
	SetServerBinaryUpgradeExecutorForTest(func(execPath string, tempPath string) error {
		executed = true
		return nil
	})
	t.Cleanup(func() {
		common.Version = originalVersion
		SetUpdateHTTPClientForTest(originalClient)
		SetServerBinaryUpgradeExecutorForTest(originalExecutor)
		resetServerUpgradeTestState(t)
	})

	_, content := fakeServerBinaryFixture(testServerReleaseTag)
	sum := sha256.Sum256(content)
	checksum := hex.EncodeToString(sum[:])
	privateKey := withServerReleaseSigningKey(t)
	signature := signServerReleaseForTest(t, privateKey, testServerReleaseTag, testServerAssetName, checksum)
	SetUpdateHTTPClientForTest(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.String() {
			case testServerSHAURL:
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Body:       io.NopCloser(strings.NewReader(checksum + "  " + testServerAssetName + "\n")),
					Header:     make(http.Header),
				}, nil
			case testServerSigURL:
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Body:       io.NopCloser(strings.NewReader(signature)),
					Header:     make(http.Header),
				}, nil
			case testServerBinaryURL:
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Body:       io.NopCloser(bytes.NewReader(content)),
					Header:     make(http.Header),
				}, nil
			default:
				t.Fatalf("unexpected request url: %s", req.URL.String())
				return nil, nil
			}
		}),
	})

	if err := executeServerUpgrade(&preparedServerUpgrade{
		release: &LatestServerRelease{
			TagName: testServerReleaseTag,
		},
		downloadURL:   testServerBinaryURL,
		checksumURL:   testServerSHAURL,
		signatureURL:  testServerSigURL,
		assetName:     testServerAssetName,
		checksumName:  testServerAssetName + ".sha256",
		signatureName: testServerAssetName + ".sig",
		execPath:      os.Args[0],
	}); err != nil {
		t.Fatalf("expected upgrade to pass checksum verification: %v", err)
	}
	if !executed {
		t.Fatal("expected upgrade executor to run after checksum verification")
	}
}

func TestExecuteServerUpgradeRejectsInvalidSignature(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test binary fixtures require unix execution semantics")
	}
	originalVersion := common.Version
	originalClient := UpdateHTTPClientForTest()
	originalExecutor := ServerBinaryUpgradeExecutorForTest()
	common.Version = "v0.4.0"
	executed := false
	SetServerBinaryUpgradeExecutorForTest(func(execPath string, tempPath string) error {
		executed = true
		return nil
	})
	t.Cleanup(func() {
		common.Version = originalVersion
		SetUpdateHTTPClientForTest(originalClient)
		SetServerBinaryUpgradeExecutorForTest(originalExecutor)
		resetServerUpgradeTestState(t)
	})

	_, content := fakeServerBinaryFixture(testServerReleaseTag)
	sum := sha256.Sum256(content)
	checksum := hex.EncodeToString(sum[:])
	privateKey := withServerReleaseSigningKey(t)
	signature := signServerReleaseForTest(t, privateKey, "v0.4.9", testServerAssetName, checksum)
	SetUpdateHTTPClientForTest(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.String() {
			case testServerSHAURL:
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Body:       io.NopCloser(strings.NewReader(checksum + "  " + testServerAssetName + "\n")),
					Header:     make(http.Header),
				}, nil
			case testServerSigURL:
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Body:       io.NopCloser(strings.NewReader(signature)),
					Header:     make(http.Header),
				}, nil
			case testServerBinaryURL:
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Body:       io.NopCloser(bytes.NewReader(content)),
					Header:     make(http.Header),
				}, nil
			default:
				t.Fatalf("unexpected request url: %s", req.URL.String())
				return nil, nil
			}
		}),
	})

	err := executeServerUpgrade(&preparedServerUpgrade{
		release: &LatestServerRelease{
			TagName: testServerReleaseTag,
		},
		downloadURL:   testServerBinaryURL,
		checksumURL:   testServerSHAURL,
		signatureURL:  testServerSigURL,
		assetName:     testServerAssetName,
		checksumName:  testServerAssetName + ".sha256",
		signatureName: testServerAssetName + ".sig",
		execPath:      os.Args[0],
	})
	if err == nil || !strings.Contains(err.Error(), "signature verification failed") {
		t.Fatalf("expected signature verification error, got %v", err)
	}
	if executed {
		t.Fatal("upgrade executor must not run when signature verification fails")
	}
}
