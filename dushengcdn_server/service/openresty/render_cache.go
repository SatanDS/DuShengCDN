package openresty

import (
	"fmt"
	"regexp"
	"strings"
)

func renderOpenRestyCacheTemplateBlock(cfg ConfigSnapshot) string {
	lines := make([]string, 0, 12)
	lines = append(lines, renderOpenRestyLimitZoneBlock())
	if !cfg.CacheEnabled {
		lines = append(lines, renderOpenRestyObservabilityTemplateBlock())
		return strings.Join(lines, "")
	}
	lines = append(lines, strings.Join([]string{
		fmt.Sprintf("    proxy_cache_path %s levels=%s keys_zone=dushengcdn_cache:10m inactive=%s max_size=%s;", cfg.CachePath, cfg.CacheLevels, cfg.CacheInactive, cfg.CacheMaxSize),
		fmt.Sprintf("    proxy_cache_key %s;", QuoteNginxStringLiteral(cfg.CacheKeyTemplate)),
		fmt.Sprintf("    proxy_cache_lock %s;", onOff(cfg.CacheLockEnabled)),
		fmt.Sprintf("    proxy_cache_lock_timeout %s;", cfg.CacheLockTimeout),
		fmt.Sprintf("    proxy_cache_use_stale %s;", cfg.CacheUseStale),
		"",
	}, "\n"))
	lines = append(lines, renderOpenRestyObservabilityTemplateBlock())
	return strings.Join(lines, "")
}

func renderRouteCacheBlock(cacheConfig routeCacheConfig, cfg ConfigSnapshot) string {
	if !cfg.CacheEnabled || !cacheConfig.Enabled {
		return ""
	}
	var builder strings.Builder
	builder.WriteString("        set $dushengcdn_skip_cache 0;\n")
	builder.WriteString("        if ($request_method != GET) {\n            set $dushengcdn_skip_cache 1;\n        }\n")
	builder.WriteString("        if ($http_authorization != \"\") {\n            set $dushengcdn_skip_cache 1;\n        }\n")
	builder.WriteString("        if ($http_cookie ~* \"(session|sess|token|auth|jwt|logged_in|remember|laravel_session|connect\\\\.sid|_session)\") {\n            set $dushengcdn_skip_cache 1;\n        }\n")
	builder.WriteString("        if ($http_cache_control ~* \"(no-cache|no-store|private)\") {\n            set $dushengcdn_skip_cache 1;\n        }\n")
	builder.WriteString("        if ($http_range != \"\") {\n            set $dushengcdn_skip_cache 1;\n        }\n")
	if policyCondition := renderRouteCachePolicyCondition(cacheConfig); policyCondition != "" {
		builder.WriteString(policyCondition)
	}
	builder.WriteString("        proxy_cache dushengcdn_cache;\n")
	builder.WriteString("        proxy_cache_methods GET;\n")
	builder.WriteString("        proxy_cache_valid 200 301 302 10m;\n")
	builder.WriteString("        add_header X-DuShengCDN-Cache $upstream_cache_status always;\n")
	builder.WriteString("        proxy_cache_bypass $dushengcdn_skip_cache;\n")
	builder.WriteString("        proxy_no_cache $dushengcdn_skip_cache;\n")
	return builder.String()
}

func renderRouteCachePolicyCondition(cacheConfig routeCacheConfig) string {
	switch cacheConfig.Policy {
	case CachePolicySuffix:
		return fmt.Sprintf("        if ($uri !~* %s) {\n            set $dushengcdn_skip_cache 1;\n        }\n", QuoteNginxStringLiteral(buildSuffixMatchPattern(cacheConfig.Rules)))
	case CachePolicyPathPrefix:
		return fmt.Sprintf("        if ($uri !~ %s) {\n            set $dushengcdn_skip_cache 1;\n        }\n", QuoteNginxStringLiteral(buildPathPrefixMatchPattern(cacheConfig.Rules)))
	case CachePolicyPathContains:
		return fmt.Sprintf("        if ($uri !~* %s) {\n            set $dushengcdn_skip_cache 1;\n        }\n", QuoteNginxStringLiteral(buildPathContainsMatchPattern(cacheConfig.Rules)))
	case CachePolicyPathContainsAll:
		return renderPathContainsAllCachePolicyCondition(cacheConfig.Rules)
	case CachePolicyPathExact:
		return fmt.Sprintf("        if ($uri !~ %s) {\n            set $dushengcdn_skip_cache 1;\n        }\n", QuoteNginxStringLiteral(buildPathExactMatchPattern(cacheConfig.Rules)))
	default:
		return ""
	}
}

func renderPathContainsAllCachePolicyCondition(rules []string) string {
	var builder strings.Builder
	for _, rule := range rules {
		builder.WriteString(fmt.Sprintf("        if ($uri !~* %s) {\n            set $dushengcdn_skip_cache 1;\n        }\n", QuoteNginxStringLiteral(regexp.QuoteMeta(rule))))
	}
	return builder.String()
}

func buildSuffixMatchPattern(rules []string) string {
	parts := make([]string, 0, len(rules))
	for _, rule := range rules {
		parts = append(parts, regexp.QuoteMeta(rule))
	}
	return fmt.Sprintf("\\.(?:%s)$", strings.Join(parts, "|"))
}

func buildPathPrefixMatchPattern(rules []string) string {
	parts := make([]string, 0, len(rules))
	for _, rule := range rules {
		trimmed := strings.TrimRight(rule, "/")
		if trimmed == "" {
			trimmed = "/"
		}
		if trimmed == "/" {
			parts = append(parts, "/")
			continue
		}
		parts = append(parts, fmt.Sprintf("%s(?:/|$)", regexp.QuoteMeta(trimmed)))
	}
	return fmt.Sprintf("^(?:%s)", strings.Join(parts, "|"))
}

func buildPathContainsMatchPattern(rules []string) string {
	parts := make([]string, 0, len(rules))
	for _, rule := range rules {
		parts = append(parts, regexp.QuoteMeta(rule))
	}
	return fmt.Sprintf("(?:%s)", strings.Join(parts, "|"))
}

func buildPathExactMatchPattern(rules []string) string {
	parts := make([]string, 0, len(rules))
	for _, rule := range rules {
		parts = append(parts, regexp.QuoteMeta(rule))
	}
	return fmt.Sprintf("^(?:%s)$", strings.Join(parts, "|"))
}
