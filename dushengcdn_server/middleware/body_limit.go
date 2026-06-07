package middleware

import (
	"dushengcdn/common"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func JSONBodyLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request != nil && c.Request.Body != nil && isJSONContentType(c.GetHeader("Content-Type")) && common.JSONBodyMaxBytes > 0 {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, common.JSONBodyMaxBytes)
		}
		c.Next()
	}
}

func isJSONContentType(contentType string) bool {
	contentType = strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	return contentType == "application/json" || strings.HasSuffix(contentType, "+json")
}
