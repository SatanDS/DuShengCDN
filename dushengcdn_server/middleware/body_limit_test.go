package middleware

import (
	"dushengcdn/common"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestJSONBodyLimitRejectsOversizedJSON(t *testing.T) {
	oldLimit := common.JSONBodyMaxBytes
	oldMode := gin.Mode()
	common.JSONBodyMaxBytes = 8
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() {
		common.JSONBodyMaxBytes = oldLimit
		gin.SetMode(oldMode)
	})

	router := gin.New()
	router.Use(JSONBodyLimit())
	router.POST("/json", func(c *gin.Context) {
		var payload map[string]string
		if err := json.NewDecoder(c.Request.Body).Decode(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": ""})
	})

	request := httptest.NewRequest(http.MethodPost, "/json", strings.NewReader(`{"message":"too large"}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected oversized JSON to fail, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "request body too large") {
		t.Fatalf("expected body limit error, got %s", recorder.Body.String())
	}
}

func TestJSONBodyLimitRejectsOversizedUnsafeRequestWithoutJSONContentType(t *testing.T) {
	oldLimit := common.JSONBodyMaxBytes
	oldMode := gin.Mode()
	common.JSONBodyMaxBytes = 8
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() {
		common.JSONBodyMaxBytes = oldLimit
		gin.SetMode(oldMode)
	})

	router := gin.New()
	router.Use(JSONBodyLimit())
	router.POST("/text", func(c *gin.Context) {
		_, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": ""})
	})

	request := httptest.NewRequest(http.MethodPost, "/text", strings.NewReader("this text body is intentionally long"))
	request.Header.Set("Content-Type", "text/plain")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected oversized non-JSON unsafe request to fail, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "request body too large") {
		t.Fatalf("expected body limit error, got %s", recorder.Body.String())
	}
}

func TestJSONBodyLimitOnlySkipsKnownMultipartUploadRoutes(t *testing.T) {
	oldLimit := common.JSONBodyMaxBytes
	oldMode := gin.Mode()
	common.JSONBodyMaxBytes = 8
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() {
		common.JSONBodyMaxBytes = oldLimit
		gin.SetMode(oldMode)
	})

	router := gin.New()
	router.Use(JSONBodyLimit())
	handler := func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"size": len(body)})
	}
	router.POST("/api/tls-certificates/import-file", handler)
	router.POST("/api/user/login", handler)

	uploadReq := httptest.NewRequest(http.MethodPost, "/api/tls-certificates/import-file", strings.NewReader("this multipart upload is intentionally long"))
	uploadReq.Header.Set("Content-Type", "multipart/form-data; boundary=test")
	uploadRecorder := httptest.NewRecorder()
	router.ServeHTTP(uploadRecorder, uploadReq)
	if uploadRecorder.Code != http.StatusOK {
		t.Fatalf("expected known upload route to bypass global JSON body limit, got %d: %s", uploadRecorder.Code, uploadRecorder.Body.String())
	}

	forgedReq := httptest.NewRequest(http.MethodPost, "/api/user/login", strings.NewReader("this forged multipart body is intentionally long"))
	forgedReq.Header.Set("Content-Type", "multipart/form-data; boundary=test")
	forgedRecorder := httptest.NewRecorder()
	router.ServeHTTP(forgedRecorder, forgedReq)
	if forgedRecorder.Code != http.StatusBadRequest {
		t.Fatalf("expected forged multipart request to normal API route to fail, got %d: %s", forgedRecorder.Code, forgedRecorder.Body.String())
	}
	if !strings.Contains(forgedRecorder.Body.String(), "request body too large") {
		t.Fatalf("expected body limit error, got %s", forgedRecorder.Body.String())
	}
}
