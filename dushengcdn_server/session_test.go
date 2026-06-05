package main

import (
	"bytes"
	"dushengcdn/common"
	"dushengcdn/controller"
	"dushengcdn/model"
	"dushengcdn/utils/security"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestConfigureSessionStoreSetsCommercialCookieFlags(t *testing.T) {
	oldServerAddress := common.ServerAddress
	common.ServerAddress = "https://cdn.example.com"
	t.Cleanup(func() {
		common.ServerAddress = oldServerAddress
	})

	store := cookie.NewStore([]byte("test-secret"))
	configureSessionStore(store)

	router := gin.New()
	router.Use(sessions.Sessions("session", store))
	router.POST("/api/user/login", controller.Login)

	oldDB := model.DB
	model.DB = setupMainSessionTestDB(t)
	t.Cleanup(func() {
		model.DB = oldDB
	})

	payload, err := json.Marshal(map[string]string{
		"username": "root",
		"password": "123456",
	})
	if err != nil {
		t.Fatalf("marshal login payload: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/user/login", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	sessionCookie := findCookie(t, recorder, "session")
	if !sessionCookie.HttpOnly {
		t.Fatal("expected session cookie to be HttpOnly")
	}
	if !sessionCookie.Secure {
		t.Fatal("expected https ServerAddress to enable Secure cookie")
	}
	if sessionCookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("expected SameSite=Lax, got %v", sessionCookie.SameSite)
	}
	if sessionCookie.Path != "/" {
		t.Fatalf("expected cookie path '/', got %q", sessionCookie.Path)
	}
}

func TestValidateRuntimeSecurityConfigRejectsReleasePlaceholders(t *testing.T) {
	oldSecret := common.SessionSecret
	oldDSN := common.SQLDSN
	t.Cleanup(func() {
		common.SessionSecret = oldSecret
		common.SQLDSN = oldDSN
	})

	common.SessionSecret = "replace-with-random-string"
	t.Setenv("SESSION_SECRET", common.SessionSecret)
	if err := validateRuntimeSecurityConfig(gin.ReleaseMode); err == nil {
		t.Fatal("expected release mode to reject placeholder SESSION_SECRET")
	}

	common.SessionSecret = "abcdefghijklmnopqrstuvwxyz1234567890"
	t.Setenv("SESSION_SECRET", common.SessionSecret)
	if err := validateRuntimeSecurityConfig(gin.ReleaseMode); err != nil {
		t.Fatalf("expected strong release settings to pass: %v", err)
	}

	common.SessionSecret = "dev-session-secret"
	common.SQLDSN = "postgres://dushengcdn:replace-with-strong-password@postgres:5432/dushengcdn"
	if err := validateRuntimeSecurityConfig(gin.DebugMode); err != nil {
		t.Fatalf("expected debug mode to allow development secret: %v", err)
	}
}

func setupMainSessionTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "session.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open session test database: %v", err)
	}
	if err = db.AutoMigrate(&model.User{}); err != nil {
		t.Fatalf("migrate user table: %v", err)
	}
	hashedPassword, err := security.Password2Hash("123456")
	if err != nil {
		t.Fatalf("hash root password: %v", err)
	}
	if err = db.Create(&model.User{
		Username:    "root",
		Password:    hashedPassword,
		DisplayName: "Root User",
		Role:        common.RoleRootUser,
		Status:      common.UserStatusEnabled,
	}).Error; err != nil {
		t.Fatalf("create root user: %v", err)
	}
	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func findCookie(t *testing.T, recorder *httptest.ResponseRecorder, name string) *http.Cookie {
	t.Helper()
	for _, cookieValue := range recorder.Result().Cookies() {
		if cookieValue.Name == name {
			return cookieValue
		}
	}
	t.Fatalf("expected %s cookie", name)
	return nil
}
