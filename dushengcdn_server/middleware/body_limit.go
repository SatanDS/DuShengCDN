package middleware

import (
	"dushengcdn/common"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func JSONBodyLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request != nil && c.Request.Body != nil && shouldLimitRequestBody(c.Request.Method, c.Request.URL.Path, c.GetHeader("Content-Type")) && common.JSONBodyMaxBytes > 0 {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, common.JSONBodyMaxBytes)
		}
		c.Next()
	}
}

func shouldLimitRequestBody(method string, path string, contentType string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
	default:
		return false
	}

	return !isMultipartUploadRoute(method, path, contentType)
}

func isMultipartUploadRoute(method string, path string, contentType string) bool {
	if method != http.MethodPost {
		return false
	}

	contentType = strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	if contentType != "multipart/form-data" {
		return false
	}

	switch path {
	case "/api/tls-certificates/import-file", "/api/update/manual-upload":
		return true
	default:
		return false
	}
}
