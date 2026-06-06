package router_test

import (
	"dushengcdn/common"
	"dushengcdn/model"
	"dushengcdn/router"
	"dushengcdn/service"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestDNSSourceDatabaseMirrorRequiresWorkerToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	common.RedisEnabled = false
	setupTestDB(t)

	oldMirrorPath := common.DNSSourceDatabaseMirrorPath
	common.DNSSourceDatabaseMirrorPath = t.TempDir()
	t.Cleanup(func() {
		common.DNSSourceDatabaseMirrorPath = oldMirrorPath
	})

	worker, err := service.CreateAuthoritativeDNSWorker(service.DNSWorkerInput{Name: "ns1"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSWorker: %v", err)
	}
	mirrorFile := filepath.Join(common.DNSSourceDatabaseMirrorPath, "current", "operator", "chinanet.txt")
	if err := os.MkdirAll(filepath.Dir(mirrorFile), 0o755); err != nil {
		t.Fatalf("mkdir mirror: %v", err)
	}
	if err := os.WriteFile(mirrorFile, []byte("1.0.1.0/24\n"), 0o644); err != nil {
		t.Fatalf("write mirror file: %v", err)
	}
	manifest := `{
  "updated_at": "2026-06-07T00:00:00Z",
  "sources": {
    "operator": {
      "kind": "operator",
      "updated_at": "2026-06-07T00:00:00Z",
      "files": [
        {
          "name": "chinanet.txt",
          "path": "operator/chinanet.txt",
          "size": 11,
          "sha256": "testsha",
          "updated_at": "2026-06-07T00:00:00Z"
        }
      ]
    }
  }
}`
	if err := os.WriteFile(filepath.Join(common.DNSSourceDatabaseMirrorPath, "current", "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	engine := gin.New()
	router.SetApiRouter(engine)

	unauthorizedReq := httptest.NewRequest(http.MethodGet, "/api/dns-source-databases/manifest", nil)
	unauthorizedRecorder := httptest.NewRecorder()
	engine.ServeHTTP(unauthorizedRecorder, unauthorizedReq)
	if unauthorizedRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized manifest request, got %d", unauthorizedRecorder.Code)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/dns-source-databases/files/operator/chinanet.txt", nil)
	req.Header.Set("X-DNS-Worker-Token", worker.Token)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected file download status 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if recorder.Header().Get("X-DuShengCDN-Source-Database-SHA256") != "testsha" {
		t.Fatalf("expected checksum header, got %q", recorder.Header().Get("X-DuShengCDN-Source-Database-SHA256"))
	}
	if recorder.Body.String() != "1.0.1.0/24\n" {
		t.Fatalf("unexpected file body: %q", recorder.Body.String())
	}

	if err := model.DB.Where("worker_id = ?", worker.WorkerID).First(&model.DNSWorker{}).Error; err != nil {
		t.Fatalf("expected worker to remain queryable: %v", err)
	}
}
