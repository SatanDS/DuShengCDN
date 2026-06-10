package controller

import (
	"dushengcdn/common"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

type normalizedWebSocketOrigin struct {
	origin string
	scheme string
	host   string
}

func rejectInvalidWebSocketOrigin(c *gin.Context) bool {
	if isAllowedWebSocketOrigin(c.Request) {
		return false
	}
	c.JSON(http.StatusForbidden, gin.H{
		"success": false,
		"message": "invalid websocket origin",
	})
	c.Abort()
	return true
}

func isAllowedWebSocketOrigin(request *http.Request) bool {
	origin := strings.TrimSpace(request.Header.Get("Origin"))
	if origin == "" {
		return true
	}

	requestOrigin, ok := normalizeWebSocketOrigin(origin, false)
	if !ok {
		return false
	}
	if requestHost, ok := normalizeWebSocketHost(request.Host, ""); ok && requestOrigin.host == requestHost && isAllowedSameHostWebSocketScheme(requestOrigin.scheme) {
		return true
	}
	if configuredOrigin, ok := normalizeWebSocketOrigin(common.ServerAddress, true); ok {
		return requestOrigin.origin == configuredOrigin.origin
	}
	return false
}

func normalizeWebSocketOrigin(value string, allowPath bool) (normalizedWebSocketOrigin, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return normalizedWebSocketOrigin{}, false
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed == nil || parsed.Scheme == "" || parsed.Host == "" {
		return normalizedWebSocketOrigin{}, false
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return normalizedWebSocketOrigin{}, false
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return normalizedWebSocketOrigin{}, false
	}
	if !allowPath && strings.Trim(parsed.EscapedPath(), "/") != "" {
		return normalizedWebSocketOrigin{}, false
	}
	host, ok := normalizeWebSocketHost(parsed.Host, scheme)
	if !ok {
		return normalizedWebSocketOrigin{}, false
	}
	return normalizedWebSocketOrigin{
		origin: scheme + "://" + host,
		scheme: scheme,
		host:   host,
	}, true
}

func isAllowedSameHostWebSocketScheme(originScheme string) bool {
	originScheme = strings.ToLower(strings.TrimSpace(originScheme))
	if originScheme == "https" {
		return true
	}
	configuredOrigin, ok := normalizeWebSocketOrigin(common.ServerAddress, true)
	if !ok {
		return originScheme == "http"
	}
	if configuredOrigin.scheme == "https" {
		return false
	}
	return originScheme == "http"
}

func normalizeWebSocketHost(rawHost string, scheme string) (string, bool) {
	trimmed := strings.TrimSpace(rawHost)
	if trimmed == "" {
		return "", false
	}
	parsed, err := url.Parse("http://" + trimmed)
	if err != nil || parsed == nil || parsed.Host == "" {
		return "", false
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.EscapedPath() != "" {
		return "", false
	}
	hostname := strings.ToLower(parsed.Hostname())
	if hostname == "" {
		return "", false
	}
	port := parsed.Port()
	if port != "" {
		if _, err := strconv.Atoi(port); err != nil {
			return "", false
		}
	}
	displayHost := hostname
	if strings.Contains(hostname, ":") {
		displayHost = "[" + hostname + "]"
	}
	if port == "" || (scheme == "http" && port == "80") || (scheme == "https" && port == "443") || (scheme == "" && (port == "80" || port == "443")) {
		return displayHost, true
	}
	return net.JoinHostPort(hostname, port), true
}
