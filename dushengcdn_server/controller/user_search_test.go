package controller

import (
	"dushengcdn/common"
	"dushengcdn/model"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSearchUsersLimitsResultsAndHandlesNonNumericKeyword(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupControllerTestDB(t)

	for i := 0; i < common.ItemsPerPage+3; i++ {
		user := &model.User{
			Username:    "search" + strconv.Itoa(i),
			DisplayName: "Search User",
			Email:       "search" + strconv.Itoa(i) + "@example.com",
			Password:    "password123",
			Status:      common.UserStatusEnabled,
		}
		if err := user.Insert(); err != nil {
			t.Fatalf("insert user %d failed: %v", i, err)
		}
	}

	engine := gin.New()
	engine.GET("/api/user/search", SearchUsers)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/user/search?keyword=search", nil)

	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}
	var payload struct {
		Success bool          `json:"success"`
		Data    []*model.User `json:"data"`
		Message string        `json:"message"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if !payload.Success {
		t.Fatalf("expected success response, got %s", payload.Message)
	}
	if len(payload.Data) != common.ItemsPerPage {
		t.Fatalf("expected result limit %d, got %d", common.ItemsPerPage, len(payload.Data))
	}
}

func TestSearchUsersNumericKeywordCanMatchID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupControllerTestDB(t)

	user := &model.User{
		Username:    "numeric",
		DisplayName: "Numeric",
		Email:       "numeric@example.com",
		Password:    "password123",
		Status:      common.UserStatusEnabled,
	}
	if err := user.Insert(); err != nil {
		t.Fatalf("insert user failed: %v", err)
	}

	engine := gin.New()
	engine.GET("/api/user/search", SearchUsers)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/user/search?keyword="+strconv.Itoa(user.Id), nil)

	engine.ServeHTTP(recorder, request)

	var payload struct {
		Success bool          `json:"success"`
		Data    []*model.User `json:"data"`
		Message string        `json:"message"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if !payload.Success {
		t.Fatalf("expected success response, got %s", payload.Message)
	}
	for _, result := range payload.Data {
		if result.Id == user.Id {
			return
		}
	}
	t.Fatalf("expected numeric keyword to match user id %d, got %#v", user.Id, payload.Data)
}
