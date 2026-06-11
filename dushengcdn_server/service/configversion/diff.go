package configversion

import (
	"dushengcdn/service/openresty"
	"fmt"
)

type OptionDiffItem struct {
	Key           string `json:"key"`
	PreviousValue string `json:"previous_value"`
	CurrentValue  string `json:"current_value"`
}

func BuildInitialOpenRestyOptionDiffs(current openresty.ConfigSnapshot) []OptionDiffItem {
	details := DiffOpenRestyOptionDetails(openresty.ConfigSnapshot{}, current)
	result := make([]OptionDiffItem, 0, len(details))
	for _, detail := range details {
		if detail.CurrentValue != "" && detail.CurrentValue != "0" && detail.CurrentValue != "false" {
			result = append(result, detail)
		}
	}
	return result
}

func DiffOpenRestyOptionDetails(left openresty.ConfigSnapshot, right openresty.ConfigSnapshot) []OptionDiffItem {
	comparisons := []struct {
		key   string
		left  string
		right string
	}{
		{"OpenRestyWorkerProcesses", left.WorkerProcesses, right.WorkerProcesses},
		{"OpenRestyWorkerConnections", fmt.Sprintf("%d", left.WorkerConnections), fmt.Sprintf("%d", right.WorkerConnections)},
		{"OpenRestyWorkerRlimitNofile", fmt.Sprintf("%d", left.WorkerRlimitNofile), fmt.Sprintf("%d", right.WorkerRlimitNofile)},
		{"OpenRestyEventsUse", left.EventsUse, right.EventsUse},
		{"OpenRestyEventsMultiAcceptEnabled", fmt.Sprintf("%t", left.EventsMultiAcceptEnabled), fmt.Sprintf("%t", right.EventsMultiAcceptEnabled)},
		{"OpenRestyKeepaliveTimeout", fmt.Sprintf("%d", left.KeepaliveTimeout), fmt.Sprintf("%d", right.KeepaliveTimeout)},
		{"OpenRestyKeepaliveRequests", fmt.Sprintf("%d", left.KeepaliveRequests), fmt.Sprintf("%d", right.KeepaliveRequests)},
		{"OpenRestyClientHeaderTimeout", fmt.Sprintf("%d", left.ClientHeaderTimeout), fmt.Sprintf("%d", right.ClientHeaderTimeout)},
		{"OpenRestyClientBodyTimeout", fmt.Sprintf("%d", left.ClientBodyTimeout), fmt.Sprintf("%d", right.ClientBodyTimeout)},
		{"OpenRestyClientMaxBodySize", left.ClientMaxBodySize, right.ClientMaxBodySize},
		{"OpenRestyLargeClientHeaderBuffers", left.LargeClientHeaderBuffers, right.LargeClientHeaderBuffers},
		{"OpenRestySendTimeout", fmt.Sprintf("%d", left.SendTimeout), fmt.Sprintf("%d", right.SendTimeout)},
		{"OpenRestyProxyConnectTimeout", fmt.Sprintf("%d", left.ProxyConnectTimeout), fmt.Sprintf("%d", right.ProxyConnectTimeout)},
		{"OpenRestyProxySendTimeout", fmt.Sprintf("%d", left.ProxySendTimeout), fmt.Sprintf("%d", right.ProxySendTimeout)},
		{"OpenRestyProxyReadTimeout", fmt.Sprintf("%d", left.ProxyReadTimeout), fmt.Sprintf("%d", right.ProxyReadTimeout)},
		{"OpenRestyWebsocketEnabled", fmt.Sprintf("%t", left.WebsocketEnabled), fmt.Sprintf("%t", right.WebsocketEnabled)},
		{"OpenRestyProxyRequestBufferingEnabled", fmt.Sprintf("%t", left.ProxyRequestBuffering), fmt.Sprintf("%t", right.ProxyRequestBuffering)},
		{"OpenRestyProxyBufferingEnabled", fmt.Sprintf("%t", left.ProxyBufferingEnabled), fmt.Sprintf("%t", right.ProxyBufferingEnabled)},
		{"OpenRestyProxyBuffers", left.ProxyBuffers, right.ProxyBuffers},
		{"OpenRestyProxyBufferSize", left.ProxyBufferSize, right.ProxyBufferSize},
		{"OpenRestyProxyBusyBuffersSize", left.ProxyBusyBuffersSize, right.ProxyBusyBuffersSize},
		{"OpenRestyGzipEnabled", fmt.Sprintf("%t", left.GzipEnabled), fmt.Sprintf("%t", right.GzipEnabled)},
		{"OpenRestyGzipMinLength", fmt.Sprintf("%d", left.GzipMinLength), fmt.Sprintf("%d", right.GzipMinLength)},
		{"OpenRestyGzipCompLevel", fmt.Sprintf("%d", left.GzipCompLevel), fmt.Sprintf("%d", right.GzipCompLevel)},
		{"OpenRestyResolvers", left.Resolvers, right.Resolvers},
		{"OpenRestyCacheEnabled", fmt.Sprintf("%t", left.CacheEnabled), fmt.Sprintf("%t", right.CacheEnabled)},
		{"OpenRestyCachePath", left.CachePath, right.CachePath},
		{"OpenRestyCacheLevels", left.CacheLevels, right.CacheLevels},
		{"OpenRestyCacheInactive", left.CacheInactive, right.CacheInactive},
		{"OpenRestyCacheMaxSize", left.CacheMaxSize, right.CacheMaxSize},
		{"OpenRestyCacheKeyTemplate", left.CacheKeyTemplate, right.CacheKeyTemplate},
		{"OpenRestyCacheLockEnabled", fmt.Sprintf("%t", left.CacheLockEnabled), fmt.Sprintf("%t", right.CacheLockEnabled)},
		{"OpenRestyCacheLockTimeout", left.CacheLockTimeout, right.CacheLockTimeout},
		{"OpenRestyCacheUseStale", left.CacheUseStale, right.CacheUseStale},
	}
	result := make([]OptionDiffItem, 0)
	for _, comparison := range comparisons {
		if comparison.left == comparison.right {
			continue
		}
		result = append(result, OptionDiffItem{
			Key:           comparison.key,
			PreviousValue: comparison.left,
			CurrentValue:  comparison.right,
		})
	}
	return result
}

func ExtractOptionDiffKeys(details []OptionDiffItem) []string {
	keys := make([]string, 0, len(details))
	for _, detail := range details {
		keys = append(keys, detail.Key)
	}
	return keys
}

func OpenRestyOptionKeys() []string {
	return []string{
		"OpenRestyWorkerProcesses",
		"OpenRestyWorkerConnections",
		"OpenRestyWorkerRlimitNofile",
		"OpenRestyEventsUse",
		"OpenRestyEventsMultiAcceptEnabled",
		"OpenRestyKeepaliveTimeout",
		"OpenRestyKeepaliveRequests",
		"OpenRestyClientHeaderTimeout",
		"OpenRestyClientBodyTimeout",
		"OpenRestyClientMaxBodySize",
		"OpenRestyLargeClientHeaderBuffers",
		"OpenRestySendTimeout",
		"OpenRestyProxyConnectTimeout",
		"OpenRestyProxySendTimeout",
		"OpenRestyProxyReadTimeout",
		"OpenRestyWebsocketEnabled",
		"OpenRestyProxyRequestBufferingEnabled",
		"OpenRestyProxyBufferingEnabled",
		"OpenRestyProxyBuffers",
		"OpenRestyProxyBufferSize",
		"OpenRestyProxyBusyBuffersSize",
		"OpenRestyGzipEnabled",
		"OpenRestyGzipMinLength",
		"OpenRestyGzipCompLevel",
		"OpenRestyCacheEnabled",
		"OpenRestyCachePath",
		"OpenRestyCacheLevels",
		"OpenRestyCacheInactive",
		"OpenRestyCacheMaxSize",
		"OpenRestyCacheKeyTemplate",
		"OpenRestyCacheLockEnabled",
		"OpenRestyCacheLockTimeout",
		"OpenRestyCacheUseStale",
	}
}
