package middleware

import (
	"dushengcdn/common"
	"dushengcdn/model"
	"dushengcdn/utils/security"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestAgentAuthRejectsConfiguredLegacyTokenWhenCompatibilityDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	configureAgentAuthTestDB(t)
	configureLegacyAgentToken(t, false)

	router := gin.New()
	router.POST("/guarded", AgentAuth(), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/guarded", strings.NewReader(`{"node_id":"legacy-node"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Agent-Token", "legacy-token")
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected disabled legacy token to be rejected, got %d", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "compatibility is disabled") {
		t.Fatalf("expected migration message, got %s", recorder.Body.String())
	}
}

func TestAgentAuthAllowsLegacyTokenWhenCompatibilityEnabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	configureAgentAuthTestDB(t)
	configureLegacyAgentToken(t, true)
	seedMiddlewareTestNode(t, &model.Node{
		NodeID:       "legacy-node",
		Name:         "legacy-node",
		IP:           "10.0.0.1",
		AgentVersion: "0.1.0",
		Status:       "offline",
	})

	router := gin.New()
	router.POST("/guarded", AgentAuth(), func(c *gin.Context) {
		node, ok := c.Get("agent_node")
		if !ok || node.(*model.Node).NodeID != "legacy-node" {
			t.Fatalf("expected legacy node in context, got %#v", node)
		}
		legacy, _ := c.Get("legacy_agent_token")
		if legacy != true {
			t.Fatalf("expected legacy context marker, got %#v", legacy)
		}
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/guarded", strings.NewReader(`{"node_id":"legacy-node"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Agent-Token", "legacy-token")
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("expected enabled legacy token to pass, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestAgentAuthRejectsLegacyTokenForHashedNodeToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	configureAgentAuthTestDB(t)
	configureLegacyAgentToken(t, true)
	seedMiddlewareTestNode(t, &model.Node{
		NodeID:           "hashed-node",
		Name:             "hashed-node",
		IP:               "10.0.0.4",
		AgentTokenHash:   security.HashSecretToken("node-specific-token"),
		AgentTokenPrefix: security.SecretTokenPrefix("node-specific-token"),
		AgentVersion:     "0.2.0",
		Status:           "online",
	})

	router := gin.New()
	router.POST("/guarded", AgentAuth(), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/guarded", strings.NewReader(`{"node_id":"hashed-node"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Agent-Token", "legacy-token")
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected legacy token to be rejected for hashed node, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestAgentAuthRejectsLegacyTokenForActiveConfigFetch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	configureAgentAuthTestDB(t)
	configureLegacyAgentToken(t, true)
	seedMiddlewareTestNode(t, &model.Node{
		NodeID:       "legacy-node",
		Name:         "legacy-node",
		IP:           "10.0.0.5",
		AgentVersion: "0.1.0",
		Status:       "online",
	})

	router := gin.New()
	router.GET("/api/agent/config-versions/active", AgentAuth(), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/agent/config-versions/active", strings.NewReader(`{"node_id":"legacy-node"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Agent-Token", "legacy-token")
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected legacy token active config fetch to be rejected, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "cannot fetch active configs") {
		t.Fatalf("expected active config migration message, got %s", recorder.Body.String())
	}
}

func TestAgentAuthKeepsNodeSpecificTokenWorkingWhenLegacyDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	configureAgentAuthTestDB(t)
	configureLegacyAgentToken(t, false)
	seedMiddlewareTestNode(t, &model.Node{
		NodeID:       "dedicated-node",
		Name:         "dedicated-node",
		IP:           "10.0.0.2",
		AgentToken:   "node-token",
		AgentVersion: "0.2.0",
		Status:       "online",
		LastSeenAt:   time.Now(),
	})

	router := gin.New()
	router.GET("/guarded", AgentAuth(), func(c *gin.Context) {
		node, ok := c.Get("agent_node")
		if !ok || node.(*model.Node).NodeID != "dedicated-node" {
			t.Fatalf("expected dedicated node in context, got %#v", node)
		}
		if legacy, _ := c.Get("legacy_agent_token"); legacy == true {
			t.Fatal("expected node-specific token not to be marked legacy")
		}
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/guarded", nil)
	req.Header.Set("X-Agent-Token", "node-token")
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("expected node-specific token to pass, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestAgentRegisterAuthKeepsDiscoveryTokenWorkingWhenLegacyDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	configureAgentAuthTestDB(t)
	configureLegacyAgentToken(t, false)
	configureDiscoveryTokenOption(t, "legacy-token")

	router := gin.New()
	router.POST("/register", AgentRegisterAuth(), func(c *gin.Context) {
		discovery, _ := c.Get("discovery_enabled")
		if discovery != true {
			t.Fatalf("expected discovery registration context marker, got %#v", discovery)
		}
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(`{"ip":"10.0.0.3"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Agent-Token", "legacy-token")
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("expected discovery token to pass, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestAgentRegisterAuthReportsDisabledLegacyTokenAfterDiscoveryFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	configureAgentAuthTestDB(t)
	configureLegacyAgentToken(t, false)
	configureDiscoveryTokenOption(t, "discovery-token")

	router := gin.New()
	router.POST("/register", AgentRegisterAuth(), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(`{"node_id":"legacy-node"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Agent-Token", "legacy-token")
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected disabled legacy register token to be rejected, got %d", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "compatibility is disabled") {
		t.Fatalf("expected migration message, got %s", recorder.Body.String())
	}
}

func TestAgentRegisterAuthDoesNotInitializeDiscoveryTokenOnFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	configureAgentAuthTestDB(t)
	configureLegacyAgentToken(t, false)
	configureDiscoveryTokenOption(t, "")

	router := gin.New()
	router.POST("/register", AgentRegisterAuth(), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(`{"ip":"10.0.0.3"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Agent-Token", "junk-token")
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected invalid discovery token to be rejected, got %d", recorder.Code)
	}
	if got := strings.TrimSpace(common.GetOptionValue("AgentDiscoveryToken")); got != "" {
		t.Fatalf("expected failed public registration not to initialize discovery token, got %q", got)
	}
}

func configureAgentAuthTestDB(t *testing.T) {
	t.Helper()
	oldDB := model.DB
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/agent-auth.db"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if err = db.AutoMigrate(&model.Node{}, &model.Option{}); err != nil {
		t.Fatalf("migrate node table: %v", err)
	}
	model.DB = db
	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
		model.DB = oldDB
	})
}

func configureLegacyAgentToken(t *testing.T, enabled bool) {
	t.Helper()
	oldAgentToken := common.AgentToken
	oldLegacyEnabled := common.AgentLegacyGlobalTokenEnabled
	common.AgentToken = "legacy-token"
	common.AgentLegacyGlobalTokenEnabled = enabled
	t.Cleanup(func() {
		common.AgentToken = oldAgentToken
		common.AgentLegacyGlobalTokenEnabled = oldLegacyEnabled
	})
}

func configureDiscoveryTokenOption(t *testing.T, token string) {
	t.Helper()
	common.OptionMapRWMutex.Lock()
	oldOptionMap := common.OptionMap
	common.OptionMapRWMutex.Unlock()
	oldDiscoveryToken := common.AgentDiscoveryToken
	if err := model.UpdateOption("AgentDiscoveryToken", token); err != nil {
		t.Fatalf("configure discovery token option: %v", err)
	}
	common.OptionMapRWMutex.Lock()
	common.OptionMap = map[string]string{"AgentDiscoveryToken": token}
	common.OptionMapRWMutex.Unlock()
	common.AgentDiscoveryToken = token
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = oldOptionMap
		common.OptionMapRWMutex.Unlock()
		common.AgentDiscoveryToken = oldDiscoveryToken
	})
}

func seedMiddlewareTestNode(t *testing.T, node *model.Node) {
	t.Helper()
	if err := node.Insert(); err != nil {
		t.Fatalf("seed node: %v", err)
	}
}
