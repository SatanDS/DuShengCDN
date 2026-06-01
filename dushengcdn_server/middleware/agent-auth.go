package middleware

import (
	"bytes"
	"dushengcdn/service"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
)

func AgentAuth() func(c *gin.Context) {
	return func(c *gin.Context) {
		token := c.GetHeader("X-Agent-Token")
		if service.IsLegacyGlobalAgentToken(token) {
			if isLegacyAgentReadOnlyPath(c) {
				c.Set("legacy_agent_token", true)
				c.Next()
				return
			}
			if legacyNode, legacyErr := authenticateLegacyAgentNode(c, token); legacyErr == nil {
				c.Set("agent_node", legacyNode)
				c.Set("legacy_agent_token", true)
				c.Next()
				return
			}
			abortAgentUnauthorized(c, "无权进行此操作，Agent Token 无效")
			return
		}

		node, err := service.AuthenticateAgentToken(token)
		if err != nil {
			abortAgentUnauthorized(c, "无权进行此操作，Agent Token 无效")
			return
		}
		c.Set("agent_node", node)
		c.Next()
	}
}

func AgentRegisterAuth() func(c *gin.Context) {
	return func(c *gin.Context) {
		token := c.GetHeader("X-Agent-Token")
		if service.IsLegacyGlobalAgentToken(token) {
			if legacyNode, legacyErr := authenticateLegacyAgentNode(c, token); legacyErr == nil {
				c.Set("agent_node", legacyNode)
				c.Set("legacy_agent_token", true)
				c.Next()
				return
			}
			abortAgentUnauthorized(c, "无权进行此操作，Agent Token 无效")
			return
		}

		if node, err := service.AuthenticateAgentToken(token); err == nil {
			c.Set("agent_node", node)
			c.Next()
			return
		}
		if err := service.ValidateDiscoveryToken(token); err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"message": "无权进行此操作，注册 Token 无效",
			})
			c.Abort()
			return
		}
		c.Set("discovery_enabled", true)
		c.Next()
	}
}

func isLegacyAgentReadOnlyPath(c *gin.Context) bool {
	return c.Request.Method == http.MethodGet && c.Request.URL.Path == "/api/agent/config-versions/active"
}

func authenticateLegacyAgentNode(c *gin.Context, token string) (any, error) {
	nodeID, err := readAgentNodeIDFromBody(c)
	if err != nil {
		return nil, err
	}
	return service.AuthenticateLegacyAgentTokenForNode(token, nodeID)
}

func readAgentNodeIDFromBody(c *gin.Context) (string, error) {
	if c.Request.Body == nil {
		return "", io.EOF
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 1024*1024))
	if err != nil {
		return "", err
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(body))

	var payload struct {
		NodeID string `json:"node_id"`
	}
	if err = json.Unmarshal(body, &payload); err != nil {
		return "", err
	}
	return payload.NodeID, nil
}

func abortAgentUnauthorized(c *gin.Context, message string) {
	c.JSON(http.StatusUnauthorized, gin.H{
		"success": false,
		"message": message,
	})
	c.Abort()
}
