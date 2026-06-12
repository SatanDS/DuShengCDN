package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"
)

type ProxyRoutePoWListConfig struct {
	IPs         []string `json:"ips"`
	IPCidrs     []string `json:"ip_cidrs"`
	Paths       []string `json:"paths"`
	PathRegexes []string `json:"path_regexes"`
	UserAgents  []string `json:"user_agents"`
}

type ProxyRoutePoWConfig struct {
	Difficulty   int                     `json:"difficulty"`
	Algorithm    string                  `json:"algorithm"`
	SessionTTL   int                     `json:"session_ttl"`
	ChallengeTTL int                     `json:"challenge_ttl"`
	Whitelist    ProxyRoutePoWListConfig `json:"whitelist"`
	Blacklist    ProxyRoutePoWListConfig `json:"blacklist"`
}

var powAlgorithmValues = map[string]bool{"fast": true, "slow": true}

func defaultPoWConfig() ProxyRoutePoWConfig {
	return ProxyRoutePoWConfig{
		Difficulty:   4,
		Algorithm:    "fast",
		SessionTTL:   600,
		ChallengeTTL: 300,
		Whitelist:    ProxyRoutePoWListConfig{IPs: []string{}, IPCidrs: []string{}, Paths: []string{}, PathRegexes: []string{}, UserAgents: []string{}},
		Blacklist:    ProxyRoutePoWListConfig{IPs: []string{}, IPCidrs: []string{}, Paths: []string{}, PathRegexes: []string{}, UserAgents: []string{}},
	}
}

func normalizePoWConfig(enabled bool, raw string) (ProxyRoutePoWConfig, error) {
	if !enabled {
		return defaultPoWConfig(), nil
	}

	cfg := defaultPoWConfig()
	text := strings.TrimSpace(raw)
	if text != "" && text != "{}" {
		if err := json.Unmarshal([]byte(text), &cfg); err != nil {
			return cfg, errors.New("pow_config 格式无效")
		}
	}

	if cfg.Difficulty < 1 || cfg.Difficulty > 16 {
		return cfg, errors.New("pow_config.difficulty 必须在 1-16 之间")
	}
	if !powAlgorithmValues[cfg.Algorithm] {
		return cfg, errors.New("pow_config.algorithm 必须为 fast 或 slow")
	}
	if cfg.SessionTTL < 60 {
		return cfg, errors.New("pow_config.session_ttl 不能小于 60 秒")
	}
	if cfg.ChallengeTTL < 30 {
		return cfg, errors.New("pow_config.challenge_ttl 不能小于 30 秒")
	}

	for _, cidr := range cfg.Whitelist.IPCidrs {
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			return cfg, fmt.Errorf("pow_config 白名单 IP CIDR 格式无效: %s", cidr)
		}
	}
	for _, cidr := range cfg.Blacklist.IPCidrs {
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			return cfg, fmt.Errorf("pow_config 黑名单 IP CIDR 格式无效: %s", cidr)
		}
	}

	for _, re := range cfg.Whitelist.PathRegexes {
		if _, err := regexp.Compile(re); err != nil {
			return cfg, fmt.Errorf("pow_config 白名单路径正则格式无效: %s", re)
		}
	}
	for _, re := range cfg.Blacklist.PathRegexes {
		if _, err := regexp.Compile(re); err != nil {
			return cfg, fmt.Errorf("pow_config 黑名单路径正则格式无效: %s", re)
		}
	}

	for _, ip := range cfg.Whitelist.IPs {
		if net.ParseIP(ip) == nil {
			return cfg, fmt.Errorf("pow_config 白名单 IP 格式无效: %s", ip)
		}
	}
	for _, ip := range cfg.Blacklist.IPs {
		if net.ParseIP(ip) == nil {
			return cfg, fmt.Errorf("pow_config 黑名单 IP 格式无效: %s", ip)
		}
	}

	type dimension struct {
		name string
		wl   []string
		bl   []string
	}
	dimensions := []dimension{
		{"IP", cfg.Whitelist.IPs, cfg.Blacklist.IPs},
		{"IP CIDR", cfg.Whitelist.IPCidrs, cfg.Blacklist.IPCidrs},
		{"路径", cfg.Whitelist.Paths, cfg.Blacklist.Paths},
		{"路径正则", cfg.Whitelist.PathRegexes, cfg.Blacklist.PathRegexes},
		{"User-Agent", cfg.Whitelist.UserAgents, cfg.Blacklist.UserAgents},
	}
	for _, dim := range dimensions {
		if len(dim.wl) > 0 && len(dim.bl) > 0 {
			return cfg, fmt.Errorf("pow_config %s 不能同时配置白名单和黑名单", dim.name)
		}
	}

	return cfg, nil
}

func decodeStoredPoWConfig(enabled bool, raw string) (*ProxyRoutePoWConfig, error) {
	if !enabled {
		cfg := defaultPoWConfig()
		return &cfg, nil
	}
	text := strings.TrimSpace(raw)
	if text == "" || text == "{}" {
		cfg := defaultPoWConfig()
		return &cfg, nil
	}
	var cfg ProxyRoutePoWConfig
	if err := json.Unmarshal([]byte(text), &cfg); err != nil {
		return nil, errors.New("pow_config 格式无效")
	}
	return &cfg, nil
}

type ProxyRouteWAFCustomRules struct {
	PathContains   []string `json:"path_contains"`
	PathRegexes    []string `json:"path_regexes"`
	QueryContains  []string `json:"query_contains"`
	HeaderContains []string `json:"header_contains"`
	UserAgents     []string `json:"user_agents"`
}

type ProxyRouteWAFWhitelistConfig struct {
	IPs     []string `json:"ips"`
	IPCidrs []string `json:"ip_cidrs"`
	Paths   []string `json:"paths"`
}

type ProxyRouteWAFConfig struct {
	BuiltinRules []string                     `json:"builtin_rules"`
	Whitelist    ProxyRouteWAFWhitelistConfig `json:"whitelist"`
	BlockRules   ProxyRouteWAFCustomRules     `json:"block_rules"`
}

var wafBuiltinRuleValues = map[string]bool{
	"sqli":            true,
	"xss":             true,
	"path_traversal":  true,
	"sensitive_paths": true,
	"bad_bots":        true,
}

func defaultWAFConfig() ProxyRouteWAFConfig {
	return ProxyRouteWAFConfig{
		BuiltinRules: []string{"sqli", "xss", "path_traversal", "sensitive_paths", "bad_bots"},
		Whitelist: ProxyRouteWAFWhitelistConfig{
			IPs:     []string{},
			IPCidrs: []string{},
			Paths:   []string{},
		},
		BlockRules: ProxyRouteWAFCustomRules{
			PathContains:   []string{},
			PathRegexes:    []string{},
			QueryContains:  []string{},
			HeaderContains: []string{},
			UserAgents:     []string{},
		},
	}
}

func normalizeWAFMode(raw string) string {
	mode := strings.ToLower(strings.TrimSpace(raw))
	switch mode {
	case proxyRouteWAFModeLog, proxyRouteWAFModeBlock:
		return mode
	default:
		return proxyRouteWAFModeBlock
	}
}

func normalizeStringList(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	normalized := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		item := strings.TrimSpace(value)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		normalized = append(normalized, item)
	}
	return normalized
}

func normalizeWAFConfig(enabled bool, raw string) (ProxyRouteWAFConfig, error) {
	cfg := defaultWAFConfig()
	if !enabled {
		return cfg, nil
	}
	text := strings.TrimSpace(raw)
	if text != "" && text != "{}" {
		if err := json.Unmarshal([]byte(text), &cfg); err != nil {
			return cfg, errors.New("waf_config 格式无效")
		}
	}

	cfg.BuiltinRules = normalizeStringList(cfg.BuiltinRules)
	for _, rule := range cfg.BuiltinRules {
		if !wafBuiltinRuleValues[rule] {
			return cfg, fmt.Errorf("waf_config.builtin_rules 不支持规则: %s", rule)
		}
	}

	cfg.Whitelist.IPs = normalizeStringList(cfg.Whitelist.IPs)
	cfg.Whitelist.IPCidrs = normalizeStringList(cfg.Whitelist.IPCidrs)
	cfg.Whitelist.Paths = normalizeStringList(cfg.Whitelist.Paths)
	cfg.BlockRules.PathContains = normalizeStringList(cfg.BlockRules.PathContains)
	cfg.BlockRules.PathRegexes = normalizeStringList(cfg.BlockRules.PathRegexes)
	cfg.BlockRules.QueryContains = normalizeStringList(cfg.BlockRules.QueryContains)
	cfg.BlockRules.HeaderContains = normalizeStringList(cfg.BlockRules.HeaderContains)
	cfg.BlockRules.UserAgents = normalizeStringList(cfg.BlockRules.UserAgents)

	for _, ip := range cfg.Whitelist.IPs {
		if net.ParseIP(ip) == nil {
			return cfg, fmt.Errorf("waf_config 白名单 IP 格式无效: %s", ip)
		}
	}
	for _, cidr := range cfg.Whitelist.IPCidrs {
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			return cfg, fmt.Errorf("waf_config 白名单 IP CIDR 格式无效: %s", cidr)
		}
	}
	for _, path := range cfg.Whitelist.Paths {
		if !strings.HasPrefix(path, "/") || strings.ContainsAny(path, "\r\n") {
			return cfg, fmt.Errorf("waf_config 白名单路径格式无效: %s", path)
		}
	}
	for _, re := range cfg.BlockRules.PathRegexes {
		if _, err := regexp.Compile(re); err != nil {
			return cfg, fmt.Errorf("waf_config 路径正则格式无效: %s", re)
		}
	}

	return cfg, nil
}

func decodeStoredWAFConfig(enabled bool, raw string) (*ProxyRouteWAFConfig, error) {
	cfg, err := normalizeWAFConfig(enabled, raw)
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

type ProxyRouteCCListConfig struct {
	IPs        []string `json:"ips"`
	IPCidrs    []string `json:"ip_cidrs"`
	Paths      []string `json:"paths"`
	UserAgents []string `json:"user_agents"`
}

type ProxyRouteCCConfig struct {
	WindowSeconds        int                    `json:"window_seconds"`
	MaxRequests          int                    `json:"max_requests"`
	PathWindowSeconds    int                    `json:"path_window_seconds"`
	PathMaxRequests      int                    `json:"path_max_requests"`
	BlockDurationSeconds int                    `json:"block_duration_seconds"`
	Whitelist            ProxyRouteCCListConfig `json:"whitelist"`
	Exclude              ProxyRouteCCListConfig `json:"exclude"`
}

func defaultCCConfig() ProxyRouteCCConfig {
	return ProxyRouteCCConfig{
		WindowSeconds:        10,
		MaxRequests:          120,
		PathWindowSeconds:    10,
		PathMaxRequests:      60,
		BlockDurationSeconds: 300,
		Whitelist:            ProxyRouteCCListConfig{IPs: []string{}, IPCidrs: []string{}, Paths: []string{}, UserAgents: []string{}},
		Exclude:              ProxyRouteCCListConfig{IPs: []string{}, IPCidrs: []string{}, Paths: []string{}, UserAgents: []string{}},
	}
}

func normalizeCCMode(raw string) string {
	mode := strings.ToLower(strings.TrimSpace(raw))
	switch mode {
	case proxyRouteCCModeLog, proxyRouteCCModeBlock, proxyRouteCCModePoW:
		return mode
	default:
		return proxyRouteCCModeBlock
	}
}

func normalizeCCConfig(enabled bool, raw string) (ProxyRouteCCConfig, error) {
	cfg := defaultCCConfig()
	if !enabled {
		return cfg, nil
	}
	text := strings.TrimSpace(raw)
	if text != "" && text != "{}" {
		if err := json.Unmarshal([]byte(text), &cfg); err != nil {
			return cfg, errors.New("cc_config 格式无效")
		}
	}

	if cfg.WindowSeconds < 1 || cfg.WindowSeconds > 3600 {
		return cfg, errors.New("cc_config.window_seconds 必须在 1-3600 秒之间")
	}
	if cfg.MaxRequests < 1 || cfg.MaxRequests > 1000000 {
		return cfg, errors.New("cc_config.max_requests 必须在 1-1000000 之间")
	}
	if cfg.PathWindowSeconds < 1 || cfg.PathWindowSeconds > 3600 {
		return cfg, errors.New("cc_config.path_window_seconds 必须在 1-3600 秒之间")
	}
	if cfg.PathMaxRequests < 1 || cfg.PathMaxRequests > 1000000 {
		return cfg, errors.New("cc_config.path_max_requests 必须在 1-1000000 之间")
	}
	if cfg.BlockDurationSeconds < 1 || cfg.BlockDurationSeconds > 86400 {
		return cfg, errors.New("cc_config.block_duration_seconds 必须在 1-86400 秒之间")
	}

	cfg.Whitelist.IPs = normalizeStringList(cfg.Whitelist.IPs)
	cfg.Whitelist.IPCidrs = normalizeStringList(cfg.Whitelist.IPCidrs)
	cfg.Whitelist.Paths = normalizeStringList(cfg.Whitelist.Paths)
	cfg.Whitelist.UserAgents = normalizeStringList(cfg.Whitelist.UserAgents)
	cfg.Exclude.IPs = normalizeStringList(cfg.Exclude.IPs)
	cfg.Exclude.IPCidrs = normalizeStringList(cfg.Exclude.IPCidrs)
	cfg.Exclude.Paths = normalizeStringList(cfg.Exclude.Paths)
	cfg.Exclude.UserAgents = normalizeStringList(cfg.Exclude.UserAgents)

	for _, item := range []struct {
		name string
		ips  []string
	}{
		{name: "白名单", ips: cfg.Whitelist.IPs},
		{name: "排除", ips: cfg.Exclude.IPs},
	} {
		for _, ip := range item.ips {
			if net.ParseIP(ip) == nil {
				return cfg, fmt.Errorf("cc_config %s IP 格式无效: %s", item.name, ip)
			}
		}
	}
	for _, item := range []struct {
		name  string
		cidrs []string
	}{
		{name: "白名单", cidrs: cfg.Whitelist.IPCidrs},
		{name: "排除", cidrs: cfg.Exclude.IPCidrs},
	} {
		for _, cidr := range item.cidrs {
			_, _, err := net.ParseCIDR(cidr)
			if err != nil {
				return cfg, fmt.Errorf("cc_config %s IP CIDR 格式无效: %s", item.name, cidr)
			}
		}
	}
	for _, item := range []struct {
		name  string
		paths []string
	}{
		{name: "白名单", paths: cfg.Whitelist.Paths},
		{name: "排除", paths: cfg.Exclude.Paths},
	} {
		for _, path := range item.paths {
			if !strings.HasPrefix(path, "/") || strings.ContainsAny(path, "\r\n") {
				return cfg, fmt.Errorf("cc_config %s路径格式无效: %s", item.name, path)
			}
		}
	}

	return cfg, nil
}

func decodeStoredCCConfig(enabled bool, raw string) (*ProxyRouteCCConfig, error) {
	cfg, err := normalizeCCConfig(enabled, raw)
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}
