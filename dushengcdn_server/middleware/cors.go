package middleware

import (
	"dushengcdn/common"
	"net"
	"net/url"
	"strings"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func CORS() gin.HandlerFunc {
	config := cors.DefaultConfig()
	config.AllowCredentials = true
	config.AllowHeaders = []string{"Origin", "Content-Length", "Content-Type", "Authorization", "X-Agent-Token", "X-DNS-Worker-Token", "Accept"}
	config.AllowOriginFunc = func(origin string) bool {
		return isAllowedCORSOrigin(origin)
	}
	return cors.New(config)
}

func isAllowedCORSOrigin(origin string) bool {
	allowedOrigin, ok := normalizeConfiguredCORSOrigin(common.ServerAddress)
	if !ok {
		return false
	}
	requestOrigin, ok := normalizeRequestCORSOrigin(origin)
	if !ok {
		return false
	}
	return requestOrigin == allowedOrigin
}

func normalizeConfiguredCORSOrigin(value string) (string, bool) {
	return normalizeCORSOrigin(value, true)
}

func normalizeRequestCORSOrigin(value string) (string, bool) {
	return normalizeCORSOrigin(value, false)
}

func normalizeCORSOrigin(value string, allowPath bool) (string, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", false
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed == nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", false
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", false
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", false
	}
	if !allowPath && strings.Trim(parsed.EscapedPath(), "/") != "" {
		return "", false
	}
	host, ok := normalizeCORSHost(parsed, scheme)
	if !ok {
		return "", false
	}
	return scheme + "://" + host, true
}

func normalizeCORSHost(parsed *url.URL, scheme string) (string, bool) {
	rawHostname := strings.ToLower(parsed.Hostname())
	if rawHostname == "" {
		return "", false
	}
	hostname := rawHostname
	if strings.Contains(hostname, ":") {
		hostname = "[" + hostname + "]"
	}
	port := parsed.Port()
	if port == "" || (scheme == "http" && port == "80") || (scheme == "https" && port == "443") {
		return hostname, true
	}
	return net.JoinHostPort(rawHostname, port), true
}
