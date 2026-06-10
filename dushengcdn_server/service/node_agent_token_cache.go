package service

import (
	"dushengcdn/model"
	"dushengcdn/utils/security"
	"errors"
	"log/slog"
	"strings"
	"time"

	ristretto "github.com/dgraph-io/ristretto/v2"
	"gorm.io/gorm"
)

const (
	agentTokenPositiveCacheTTL = 2 * time.Minute
	agentTokenNegativeCacheTTL = 10 * time.Minute
	agentTokenNegativeCacheCap = 10000
)

type cachedAgentNode struct {
	node      *model.Node
	expiresAt time.Time
}

type cachedMissingAgentToken struct {
	expiresAt time.Time
}

type agentTokenAuthCache struct {
	positive        *ristretto.Cache[string, cachedAgentNode]
	negative        *ristretto.Cache[string, cachedMissingAgentToken]
	now             func() time.Time
	loadNodeByToken func(string) (*model.Node, error)
}

var nodeAgentTokenCache = newAgentTokenAuthCache()

func newAgentTokenAuthCache() *agentTokenAuthCache {
	positive, positiveErr := newAgentTokenPositiveCache()
	negative, negativeErr := newAgentTokenNegativeCache()
	if positiveErr != nil || negativeErr != nil {
		slog.Warn(
			"agent token cache disabled",
			"positive_error", positiveErr,
			"negative_error", negativeErr,
		)
		positive = nil
		negative = nil
	}

	return &agentTokenAuthCache{
		positive: positive,
		negative: negative,
		now:      time.Now,
		loadNodeByToken: func(token string) (*model.Node, error) {
			return model.GetNodeByAgentToken(token)
		},
	}
}

func newAgentTokenPositiveCache() (*ristretto.Cache[string, cachedAgentNode], error) {
	return ristretto.NewCache(&ristretto.Config[string, cachedAgentNode]{
		NumCounters: 1e5,
		MaxCost:     2e4,
		BufferItems: 64,
	})
}

func newAgentTokenNegativeCache() (*ristretto.Cache[string, cachedMissingAgentToken], error) {
	return ristretto.NewCache(&ristretto.Config[string, cachedMissingAgentToken]{
		NumCounters: 1e5,
		MaxCost:     agentTokenNegativeCacheCap,
		BufferItems: 64,
	})
}

func (c *agentTokenAuthCache) authenticate(token string) (*model.Node, error) {
	if c == nil {
		return model.GetNodeByAgentToken(token)
	}

	nowFunc := c.now
	if nowFunc == nil {
		nowFunc = time.Now
	}
	loadNodeByToken := c.loadNodeByToken
	if loadNodeByToken == nil {
		loadNodeByToken = model.GetNodeByAgentToken
	}

	now := nowFunc()
	cacheKey := agentTokenCacheKey(token)
	if node, ok := c.getNode(cacheKey, now); ok {
		return node, nil
	}
	if c.isMissing(cacheKey, now) {
		return nil, gorm.ErrRecordNotFound
	}

	node, err := loadNodeByToken(token)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.storeMissing(cacheKey, now.Add(agentTokenNegativeCacheTTL))
		}
		return nil, err
	}

	c.storeNode(cacheKey, node, now.Add(agentTokenPositiveCacheTTL))
	return cloneCachedNode(node), nil
}

func (c *agentTokenAuthCache) getNode(token string, now time.Time) (*model.Node, bool) {
	if c == nil || c.positive == nil {
		return nil, false
	}
	entry, ok := c.positive.Get(token)
	if !ok {
		return nil, false
	}
	if now.After(entry.expiresAt) {
		c.positive.Del(token)
		return nil, false
	}
	return cloneCachedNode(entry.node), true
}

func (c *agentTokenAuthCache) isMissing(token string, now time.Time) bool {
	if c == nil || c.negative == nil {
		return false
	}
	entry, ok := c.negative.Get(token)
	if !ok {
		return false
	}
	if now.After(entry.expiresAt) {
		c.negative.Del(token)
		return false
	}
	return true
}

func (c *agentTokenAuthCache) storeNode(token string, node *model.Node, expiresAt time.Time) {
	cacheKey := agentTokenCacheKey(token)
	if c == nil || cacheKey == "" || node == nil {
		return
	}
	if c.negative != nil {
		c.negative.Del(cacheKey)
	}
	if c.positive != nil {
		c.positive.Set(cacheKey, cachedAgentNode{
			node:      cloneCachedNode(node),
			expiresAt: expiresAt,
		}, 1)
		c.positive.Wait()
	}
}

func (c *agentTokenAuthCache) storeMissing(token string, expiresAt time.Time) {
	cacheKey := agentTokenCacheKey(token)
	if c == nil || cacheKey == "" {
		return
	}
	if c.positive != nil {
		c.positive.Del(cacheKey)
	}
	if c.negative != nil {
		c.negative.Set(cacheKey, cachedMissingAgentToken{
			expiresAt: expiresAt,
		}, 1)
		c.negative.Wait()
	}
}

func (c *agentTokenAuthCache) invalidate(token string) {
	cacheKey := agentTokenCacheKey(token)
	if c == nil || cacheKey == "" {
		return
	}
	if c.positive != nil {
		c.positive.Del(cacheKey)
	}
	if c.negative != nil {
		c.negative.Del(cacheKey)
	}
}

func (c *agentTokenAuthCache) reset() {
	if c == nil {
		return
	}
	if c.positive != nil {
		c.positive.Clear()
	}
	if c.negative != nil {
		c.negative.Clear()
	}
}

func cloneCachedNode(node *model.Node) *model.Node {
	if node == nil {
		return nil
	}
	cloned := *node
	return &cloned
}

func authenticateAgentTokenWithCache(token string) (*model.Node, error) {
	return nodeAgentTokenCache.authenticate(token)
}

func refreshAgentTokenCache(node *model.Node) {
	if node == nil {
		return
	}
	token := strings.TrimSpace(node.AgentToken)
	if token == "" {
		token = strings.TrimSpace(node.AgentTokenHash)
	}
	refreshAgentTokenCacheForToken(token, node)
}

func refreshAgentTokenCacheForToken(token string, node *model.Node) {
	if node == nil {
		return
	}
	nowFunc := nodeAgentTokenCache.now
	if nowFunc == nil {
		nowFunc = time.Now
	}
	nodeAgentTokenCache.storeNode(
		token,
		node,
		nowFunc().Add(agentTokenPositiveCacheTTL),
	)
}

func invalidateAgentTokenCache(token string) {
	nodeAgentTokenCache.invalidate(token)
}

func invalidateAgentTokenCacheForNode(node *model.Node) {
	if node == nil {
		return
	}
	invalidateAgentTokenCache(node.AgentToken)
	invalidateAgentTokenCache(node.AgentTokenHash)
}

func agentTokenCacheKey(token string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return ""
	}
	if security.IsHashedSecretToken(token) {
		return token
	}
	return security.HashSecretToken(token)
}
