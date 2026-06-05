package updater

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"dushengcdn-agent/internal/agent"
	"dushengcdn-agent/internal/config"
	"encoding/base64"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func withAgentReleaseSigningKey(t *testing.T) ed25519.PrivateKey {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate test signing key: %v", err)
	}
	originalPublicKey := config.ReleaseSignaturePublicKey
	config.ReleaseSignaturePublicKey = base64.StdEncoding.EncodeToString(publicKey)
	t.Cleanup(func() {
		config.ReleaseSignaturePublicKey = originalPublicKey
	})
	return privateKey
}

func signAgentReleaseForTest(t *testing.T, privateKey ed25519.PrivateKey, tagName string, assetName string, checksum string) []byte {
	t.Helper()
	return ed25519.Sign(privateKey, releaseSignaturePayload(tagName, assetName, checksum))
}

func TestGetLatestPreviewRelease(t *testing.T) {
	service := &Service{
		httpClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.String() != "https://api.github.com/repos/SatanDS/DuShengCDN/releases?per_page=20" {
					t.Fatalf("unexpected request url: %s", req.URL.String())
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body: io.NopCloser(strings.NewReader(`[
						{"tag_name":"v1.0.0","prerelease":false},
						{"tag_name":"v1.1.0-rc.1","prerelease":true}
					]`)),
				}, nil
			}),
		},
	}

	release, err := service.getRelease(context.Background(), "SatanDS/DuShengCDN", agent.UpdateOptions{Channel: "preview"})
	if err != nil {
		t.Fatalf("expected preview release query to succeed: %v", err)
	}
	if release == nil || release.TagName != "v1.1.0-rc.1" {
		t.Fatalf("unexpected preview release: %#v", release)
	}
}

func TestGetReleaseByTag(t *testing.T) {
	service := &Service{
		httpClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.String() != "https://api.github.com/repos/SatanDS/DuShengCDN/releases/tags/v1.1.0-rc.1" {
					t.Fatalf("unexpected request url: %s", req.URL.String())
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`{"tag_name":"v1.1.0-rc.1","prerelease":true}`)),
				}, nil
			}),
		},
	}

	release, err := service.getRelease(context.Background(), "SatanDS/DuShengCDN", agent.UpdateOptions{Channel: "preview", TagName: "v1.1.0-rc.1", Force: true})
	if err != nil {
		t.Fatalf("expected tag release query to succeed: %v", err)
	}
	if release == nil || release.TagName != "v1.1.0-rc.1" {
		t.Fatalf("unexpected tag release: %#v", release)
	}
}

func TestCheckAndUpdateRequiresChecksumAsset(t *testing.T) {
	originalVersion := config.AgentVersion
	config.AgentVersion = "v1.0.0"
	t.Cleanup(func() {
		config.AgentVersion = originalVersion
	})

	assetName := assetNameForGOOSGOARCH(runtime.GOOS, runtime.GOARCH)
	service := &Service{
		httpClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.String() != "https://api.github.com/repos/SatanDS/DuShengCDN/releases/latest" {
					t.Fatalf("unexpected request url: %s", req.URL.String())
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body: io.NopCloser(strings.NewReader(`{
						"tag_name":"v1.0.1",
						"assets":[
							{"name":"` + assetName + `","browser_download_url":"https://example.test/agent"}
						]
					}`)),
				}, nil
			}),
		},
	}

	err := service.CheckAndUpdate(context.Background(), "SatanDS/DuShengCDN", agent.UpdateOptions{})
	if err == nil || !strings.Contains(err.Error(), "no matching checksum asset") {
		t.Fatalf("expected missing checksum asset error, got %v", err)
	}
}

func TestCheckAndUpdateRequiresSignatureAsset(t *testing.T) {
	originalVersion := config.AgentVersion
	config.AgentVersion = "v1.0.0"
	t.Cleanup(func() {
		config.AgentVersion = originalVersion
	})

	assetName := assetNameForGOOSGOARCH(runtime.GOOS, runtime.GOARCH)
	service := &Service{
		httpClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.String() != "https://api.github.com/repos/SatanDS/DuShengCDN/releases/latest" {
					t.Fatalf("unexpected request url: %s", req.URL.String())
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body: io.NopCloser(strings.NewReader(`{
						"tag_name":"v1.0.1",
						"assets":[
							{"name":"` + assetName + `","browser_download_url":"https://github.com/SatanDS/DuShengCDN/releases/download/v1.0.1/` + assetName + `"},
							{"name":"` + assetName + `.sha256","browser_download_url":"https://github.com/SatanDS/DuShengCDN/releases/download/v1.0.1/` + assetName + `.sha256"}
						]
					}`)),
				}, nil
			}),
		},
	}

	err := service.CheckAndUpdate(context.Background(), "SatanDS/DuShengCDN", agent.UpdateOptions{})
	if err == nil || !strings.Contains(err.Error(), "no matching signature asset") {
		t.Fatalf("expected missing signature asset error, got %v", err)
	}
}

func TestValidateUpdateDownloadURL(t *testing.T) {
	validURLs := []string{
		"https://github.com/SatanDS/DuShengCDN/releases/download/v1.0.0/dushengcdn-agent-linux-amd64",
		"https://objects.githubusercontent.com/github-production-release-asset/file",
		"https://github-releases.githubusercontent.com/asset",
	}
	for _, value := range validURLs {
		if err := validateUpdateDownloadURL(value); err != nil {
			t.Fatalf("expected URL %q to be accepted: %v", value, err)
		}
	}

	invalidURLs := []string{
		"http://github.com/SatanDS/DuShengCDN/releases/download/v1.0.0/agent",
		"https://github.com:444/SatanDS/DuShengCDN/releases/download/v1.0.0/agent",
		"https://user:pass@github.com/SatanDS/DuShengCDN/releases/download/v1.0.0/agent",
		"https://example.test/agent",
		"https://127.0.0.1/agent",
		"not a url",
	}
	for _, value := range invalidURLs {
		if err := validateUpdateDownloadURL(value); err == nil {
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

func TestDownloadChecksumRejectsUnsafeFinalURL(t *testing.T) {
	service := &Service{
		httpClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				finalReq := req.Clone(req.Context())
				finalReq.URL.Scheme = "https"
				finalReq.URL.Host = "example.test"
				finalReq.URL.Path = "/agent.sha256"
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Request:    finalReq,
					Body:       io.NopCloser(strings.NewReader(strings.Repeat("a", sha256.Size*2))),
				}, nil
			}),
		},
	}

	_, err := service.downloadChecksum(context.Background(), "https://github.com/SatanDS/DuShengCDN/releases/download/v1.0.0/agent.sha256", "agent")
	if err == nil || !strings.Contains(err.Error(), "final url is unsafe") {
		t.Fatalf("expected unsafe final URL error, got %v", err)
	}
}

func TestDownloadAndRestartRejectsUnsafeInitialURL(t *testing.T) {
	targetPath := filepath.Join(t.TempDir(), "dushengcdn-agent")
	if err := os.WriteFile(targetPath, []byte("old-agent-binary"), 0o755); err != nil {
		t.Fatalf("write target: %v", err)
	}
	service := &Service{
		httpClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				t.Fatal("download request should not be sent for unsafe initial URL")
				return nil, nil
			}),
		},
	}

	err := service.downloadAndRestart(context.Background(), "https://example.test/agent", strings.Repeat("0", sha256.Size*2), nil, "v1.0.0", "agent", targetPath)
	if err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("expected unsafe initial URL error, got %v", err)
	}
}

func TestParseSHA256Checksum(t *testing.T) {
	checksum := strings.Repeat("a", sha256.Size*2)
	testCases := []struct {
		name    string
		content string
		asset   string
		want    string
	}{
		{name: "single digest", content: checksum + "\n", asset: "dushengcdn-agent-linux-amd64", want: checksum},
		{name: "sha256sum format", content: checksum + "  dushengcdn-agent-linux-amd64\n", asset: "dushengcdn-agent-linux-amd64", want: checksum},
		{name: "bsd format", content: "SHA256(dushengcdn-agent-linux-amd64)= " + checksum + "\n", asset: "dushengcdn-agent-linux-amd64", want: checksum},
		{name: "selects matching file", content: strings.Repeat("b", sha256.Size*2) + "  other\n" + checksum + "  dushengcdn-agent-linux-amd64\n", asset: "dushengcdn-agent-linux-amd64", want: checksum},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := parseSHA256Checksum(testCase.content, testCase.asset)
			if err != nil {
				t.Fatalf("expected checksum parse to succeed: %v", err)
			}
			if got != testCase.want {
				t.Fatalf("unexpected checksum: got %s want %s", got, testCase.want)
			}
		})
	}
}

func TestDownloadAndRestartVerifiesChecksum(t *testing.T) {
	payload := []byte("new-agent-binary")
	sum := sha256.Sum256(payload)
	expectedChecksum := hex.EncodeToString(sum[:])
	assetName := "dushengcdn-agent-linux-amd64"
	privateKey := withAgentReleaseSigningKey(t)
	signature := signAgentReleaseForTest(t, privateKey, "v1.0.0", assetName, expectedChecksum)
	targetPath := filepath.Join(t.TempDir(), "dushengcdn-agent")
	if err := os.WriteFile(targetPath, []byte("old-agent-binary"), 0o755); err != nil {
		t.Fatalf("write target: %v", err)
	}

	var replacedTarget string
	var replacedTemp string
	originalReplace := replaceAndRestartFunc
	replaceAndRestartFunc = func(execPath string, tmpPath string) error {
		replacedTarget = execPath
		replacedTemp = tmpPath
		return nil
	}
	t.Cleanup(func() {
		replaceAndRestartFunc = originalReplace
	})

	service := &Service{
		httpClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(string(payload))),
				}, nil
			}),
		},
	}

	if err := service.downloadAndRestart(context.Background(), "https://github.com/SatanDS/DuShengCDN/releases/download/v1.0.0/agent", expectedChecksum, signature, "v1.0.0", assetName, targetPath); err != nil {
		t.Fatalf("expected verified download to succeed: %v", err)
	}
	if replacedTarget != targetPath {
		t.Fatalf("unexpected replace target: %s", replacedTarget)
	}
	if replacedTemp == "" {
		t.Fatal("expected replacement temp path to be recorded")
	}
	if _, err := os.Stat(replacedTemp); err != nil {
		t.Fatalf("expected verified temp binary to remain for replacement: %v", err)
	}
}

func TestDownloadAndRestartRejectsChecksumMismatch(t *testing.T) {
	targetPath := filepath.Join(t.TempDir(), "dushengcdn-agent")
	if err := os.WriteFile(targetPath, []byte("old-agent-binary"), 0o755); err != nil {
		t.Fatalf("write target: %v", err)
	}

	originalReplace := replaceAndRestartFunc
	replaceAndRestartFunc = func(execPath string, tmpPath string) error {
		t.Fatal("replace should not run on checksum mismatch")
		return nil
	}
	t.Cleanup(func() {
		replaceAndRestartFunc = originalReplace
	})

	service := &Service{
		httpClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader("tampered")),
				}, nil
			}),
		},
	}

	err := service.downloadAndRestart(context.Background(), "https://github.com/SatanDS/DuShengCDN/releases/download/v1.0.0/agent", strings.Repeat("0", sha256.Size*2), nil, "v1.0.0", "agent", targetPath)
	if err == nil || !strings.Contains(err.Error(), "sha256 checksum mismatch") {
		t.Fatalf("expected checksum mismatch error, got %v", err)
	}
	if _, err = os.Stat(targetPath + ".update"); !os.IsNotExist(err) {
		t.Fatalf("expected temp update file to be removed, stat err=%v", err)
	}
}

func TestDownloadAndRestartRejectsInvalidSignature(t *testing.T) {
	payload := []byte("new-agent-binary")
	sum := sha256.Sum256(payload)
	expectedChecksum := hex.EncodeToString(sum[:])
	assetName := "dushengcdn-agent-linux-amd64"
	privateKey := withAgentReleaseSigningKey(t)
	signature := signAgentReleaseForTest(t, privateKey, "v0.9.0", assetName, expectedChecksum)
	targetPath := filepath.Join(t.TempDir(), "dushengcdn-agent")
	if err := os.WriteFile(targetPath, []byte("old-agent-binary"), 0o755); err != nil {
		t.Fatalf("write target: %v", err)
	}

	originalReplace := replaceAndRestartFunc
	replaceAndRestartFunc = func(execPath string, tmpPath string) error {
		t.Fatal("replace should not run when signature verification fails")
		return nil
	}
	t.Cleanup(func() {
		replaceAndRestartFunc = originalReplace
	})

	service := &Service{
		httpClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(string(payload))),
				}, nil
			}),
		},
	}

	err := service.downloadAndRestart(context.Background(), "https://github.com/SatanDS/DuShengCDN/releases/download/v1.0.0/agent", expectedChecksum, signature, "v1.0.0", assetName, targetPath)
	if err == nil || !strings.Contains(err.Error(), "signature verification failed") {
		t.Fatalf("expected signature verification error, got %v", err)
	}
	if _, err = os.Stat(targetPath + ".update"); !os.IsNotExist(err) {
		t.Fatalf("expected temp update file to be removed, stat err=%v", err)
	}
}

func TestDownloadAndRestartRejectsOversizedContentLength(t *testing.T) {
	restoreMaxAgentBinaryAssetSize(t, 8)
	targetPath := filepath.Join(t.TempDir(), "dushengcdn-agent")
	if err := os.WriteFile(targetPath, []byte("old-agent-binary"), 0o755); err != nil {
		t.Fatalf("write target: %v", err)
	}

	originalReplace := replaceAndRestartFunc
	replaceAndRestartFunc = func(execPath string, tmpPath string) error {
		t.Fatal("replace should not run on oversized download")
		return nil
	}
	t.Cleanup(func() {
		replaceAndRestartFunc = originalReplace
	})

	service := &Service{
		httpClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode:    http.StatusOK,
					Header:        make(http.Header),
					ContentLength: 9,
					Body:          io.NopCloser(strings.NewReader("")),
				}, nil
			}),
		},
	}

	err := service.downloadAndRestart(context.Background(), "https://github.com/SatanDS/DuShengCDN/releases/download/v1.0.0/agent", strings.Repeat("0", sha256.Size*2), nil, "v1.0.0", "agent", targetPath)
	if err == nil || !strings.Contains(err.Error(), "agent binary asset exceeds") {
		t.Fatalf("expected oversized asset error, got %v", err)
	}
	if _, err = os.Stat(targetPath + ".update"); !os.IsNotExist(err) {
		t.Fatalf("expected no temp update file, stat err=%v", err)
	}
}

func TestDownloadAndRestartRejectsOversizedStream(t *testing.T) {
	restoreMaxAgentBinaryAssetSize(t, 8)
	targetPath := filepath.Join(t.TempDir(), "dushengcdn-agent")
	if err := os.WriteFile(targetPath, []byte("old-agent-binary"), 0o755); err != nil {
		t.Fatalf("write target: %v", err)
	}

	service := &Service{
		httpClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode:    http.StatusOK,
					Header:        make(http.Header),
					ContentLength: -1,
					Body:          io.NopCloser(strings.NewReader("123456789")),
				}, nil
			}),
		},
	}

	err := service.downloadAndRestart(context.Background(), "https://github.com/SatanDS/DuShengCDN/releases/download/v1.0.0/agent", strings.Repeat("0", sha256.Size*2), nil, "v1.0.0", "agent", targetPath)
	if err == nil || !strings.Contains(err.Error(), "agent binary asset exceeds") {
		t.Fatalf("expected oversized asset error, got %v", err)
	}
	if _, err = os.Stat(targetPath + ".update"); !os.IsNotExist(err) {
		t.Fatalf("expected temp update file to be removed, stat err=%v", err)
	}
}

func TestSafeUpdateTargetPathRejectsInvalidTargets(t *testing.T) {
	dir := t.TempDir()
	if _, err := safeUpdateTargetPath("relative-agent"); err == nil {
		t.Fatal("expected relative path to be rejected")
	}
	if _, err := safeUpdateTargetPath(dir); err == nil {
		t.Fatal("expected directory target to be rejected")
	}
	if _, err := safeUpdateTargetPath(filepath.Join(dir, "missing-agent")); err == nil {
		t.Fatal("expected missing target to be rejected")
	}
}

func restoreMaxAgentBinaryAssetSize(t *testing.T, size int64) {
	t.Helper()
	original := maxAgentBinaryAssetSize
	maxAgentBinaryAssetSize = size
	t.Cleanup(func() {
		maxAgentBinaryAssetSize = original
	})
}

func TestIsNewerSupportsPrerelease(t *testing.T) {
	testCases := []struct {
		name     string
		local    string
		remote   string
		expected bool
	}{
		{name: "stable newer than prerelease", local: "1.2.3-rc.1", remote: "1.2.3", expected: true},
		{name: "same stable not newer", local: "1.2.3", remote: "1.2.3-rc.1", expected: false},
		{name: "higher prerelease sequence", local: "1.2.3-rc.1", remote: "1.2.3-rc.2", expected: true},
		{name: "higher minor", local: "1.2.3", remote: "1.3.0-rc.1", expected: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if actual := isNewer(testCase.local, testCase.remote); actual != testCase.expected {
				t.Fatalf("unexpected compare result: local=%s remote=%s actual=%v expected=%v", testCase.local, testCase.remote, actual, testCase.expected)
			}
		})
	}
}
