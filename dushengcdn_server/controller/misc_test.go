package controller

import (
	"bytes"
	"dushengcdn/common"
	"dushengcdn/model"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSendEmailVerificationDisabledDoesNotSendEmail(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupControllerTestDB(t)

	oldRegisterEnabled := common.RegisterEnabled
	oldPasswordRegisterEnabled := common.PasswordRegisterEnabled
	oldEmailVerificationEnabled := common.EmailVerificationEnabled
	common.RegisterEnabled = false
	common.PasswordRegisterEnabled = true
	common.EmailVerificationEnabled = true
	t.Cleanup(func() {
		common.RegisterEnabled = oldRegisterEnabled
		common.PasswordRegisterEnabled = oldPasswordRegisterEnabled
		common.EmailVerificationEnabled = oldEmailVerificationEnabled
	})

	called := false
	previousSend := sendEmailVerificationCodeAsyncFunc
	sendEmailVerificationCodeAsyncFunc = func(email string, key string, purpose string, logMessage string) {
		called = true
	}
	t.Cleanup(func() {
		sendEmailVerificationCodeAsyncFunc = previousSend
	})

	engine := gin.New()
	engine.POST("/api/verification", SendEmailVerification)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/verification", bytes.NewBufferString(`{"email":"new@example.com"}`))
	request.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}
	if called {
		t.Fatal("expected disabled public email verification not to send email")
	}
}

func setupControllerTestDB(t *testing.T) {
	t.Helper()
	previousSQLitePath := common.SQLitePath
	previousSQLDSN := common.SQLDSN
	previousInitialRootPassword := common.InitialRootPassword
	common.SQLDSN = ""
	common.SQLitePath = filepath.Join(t.TempDir(), "controller.db")
	common.InitialRootPassword = "123456"
	if err := model.InitDB(); err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	t.Cleanup(func() {
		if err := model.CloseDB(); err != nil {
			t.Fatalf("failed to close db: %v", err)
		}
		common.SQLitePath = previousSQLitePath
		common.SQLDSN = previousSQLDSN
		common.InitialRootPassword = previousInitialRootPassword
	})
}
