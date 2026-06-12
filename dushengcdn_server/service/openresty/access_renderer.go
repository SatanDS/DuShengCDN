package openresty

import (
	"dushengcdn/utils/security"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	anubisStaticPrefix = "/.within.website/x/cmd/anubis/static/"
	anubisAPIPrefix    = "/.within.website/x/cmd/anubis/api/"
)

func renderRouteAccessBlock(powEnabled bool, regionConfig routeRegionRestrictionConfig, wafConfig routeWAFConfig, ccConfig routeCCConfig) string {
	return renderUnifiedAccessBlock(powEnabled || wafConfig.Enabled || ccConfig.Enabled || (regionConfig.Enabled && len(regionConfig.Countries) > 0))
}

func renderUnifiedAccessBlock(enabled bool) string {
	if !enabled {
		return ""
	}
	return fmt.Sprintf("    access_by_lua_file %s/access.lua;\n", LuaDirPlaceholder)
}

func renderBasicAuthBlock(enabled bool, username, passwordHash string) string {
	expectedHash := strings.TrimSpace(passwordHash)
	if !enabled || username == "" || expectedHash == "" {
		return ""
	}
	return fmt.Sprintf(`        rewrite_by_lua_block {
            local expected_hash = "%s"
            local auth = ngx.var.http_authorization or ""
            local credential = nil
            if string.sub(auth, 1, 6) == "Basic " then
                credential = ngx.decode_base64(string.sub(auth, 7))
            end
            local ok = false
            if credential then
                local sha256 = require "resty.sha256"
                local str = require "resty.string"
                local hasher = sha256:new()
                hasher:update(%s)
                hasher:update(credential)
                ok = str.to_hex(hasher:final()) == expected_hash
            end
            if not ok then
                ngx.header["WWW-Authenticate"] = 'Basic realm="Restricted"'
                return ngx.exit(401)
            end
        }
`, expectedHash, luaStringLiteral(security.BasicAuthCredentialHashMaterial))
}

func BasicAuthCredentialHash(username, password string) string {
	return security.BasicAuthCredentialHash(username, password)
}

func renderRegionRestrictionBlock(config routeRegionRestrictionConfig, wafConfig routeWAFConfig, ccConfig routeCCConfig) string {
	return renderUnifiedAccessBlock(wafConfig.Enabled || ccConfig.Enabled || (config.Enabled && len(config.Countries) > 0))
}

func renderPowLocationBlocks(powEnabled bool) string {
	if !powEnabled {
		return ""
	}
	return fmt.Sprintf("\n    location = %spass-challenge {\n        content_by_lua_file %s/pow/verify.lua;\n    }\n\n    location = %smake-challenge {\n        content_by_lua_file %s/pow/challenge.lua;\n    }\n\n", anubisAPIPrefix, LuaDirPlaceholder, anubisAPIPrefix, LuaDirPlaceholder)
}

func renderPowStaticLocationBlock(powEnabled bool) string {
	if !powEnabled {
		return ""
	}
	return fmt.Sprintf("    location %s {\n        alias %s/;\n        types {\n            text/css css;\n            application/javascript js mjs;\n            application/json json;\n            image/webp webp;\n            font/woff2 woff2;\n        }\n    }\n\n", anubisStaticPrefix, PowStaticDirPlaceholder)
}

func RenderAccessSupportFiles(routes []AccessRoute) ([]SupportFile, error) {
	powConfigJSON, powSupportFiles, err := renderPowConfigBundle(routes)
	if err != nil {
		return nil, err
	}
	regionConfigJSON, regionSupportFiles, err := renderRegionConfigBundle(routes)
	if err != nil {
		return nil, err
	}
	wafConfigJSON, wafSupportFiles, err := renderWAFConfigBundle(routes)
	if err != nil {
		return nil, err
	}
	ccConfigJSON, ccSupportFiles, err := renderCCConfigBundle(routes)
	if err != nil {
		return nil, err
	}
	files := make([]SupportFile, 0, len(powSupportFiles)+len(regionSupportFiles)+len(wafSupportFiles)+len(ccSupportFiles)+4)
	files = append(files, powSupportFiles...)
	files = append(files, regionSupportFiles...)
	files = append(files, wafSupportFiles...)
	files = append(files, ccSupportFiles...)
	files = append(files, SupportFile{Path: "pow_config.json", Content: powConfigJSON})
	files = append(files, SupportFile{Path: "region_config.json", Content: regionConfigJSON})
	files = append(files, SupportFile{Path: "waf_config.json", Content: wafConfigJSON})
	files = append(files, SupportFile{Path: "cc_config.json", Content: ccConfigJSON})
	return files, nil
}

func renderPowConfigBundle(routes []AccessRoute) (string, []SupportFile, error) {
	type domainEntry struct {
		Domains []string       `json:"domains"`
		Enabled bool           `json:"enabled"`
		Config  map[string]any `json:"config"`
	}
	entries := make([]domainEntry, 0)
	hasPow := false
	for _, route := range routes {
		ccMode := normalizeCCMode(route.CCMode)
		ccRequiresPow := route.CCEnabled && ccMode == CCModePoW
		if !route.PoWEnabled && !ccRequiresPow {
			continue
		}
		hasPow = true
		var cfg map[string]any
		powConfig := route.PoWConfigJSON
		if strings.TrimSpace(powConfig) == "" || strings.TrimSpace(powConfig) == "{}" {
			data, err := json.Marshal(route.DefaultPoWConfig)
			if err != nil {
				return "", nil, err
			}
			powConfig = string(data)
		}
		if err := json.Unmarshal([]byte(powConfig), &cfg); err != nil {
			return "", nil, fmt.Errorf("route %s pow_config is invalid", route.Domain)
		}
		if !route.PoWEnabled && ccRequiresPow {
			cfg["force_only"] = true
		}
		entries = append(entries, domainEntry{
			Domains: route.Domains,
			Enabled: true,
			Config:  cfg,
		})
	}
	if !hasPow {
		return "{}", nil, nil
	}
	data, err := json.Marshal(entries)
	if err != nil {
		return "", nil, err
	}
	return string(data), nil, nil
}

func renderCCConfigBundle(routes []AccessRoute) (string, []SupportFile, error) {
	type domainEntry struct {
		Domains []string `json:"domains"`
		Enabled bool     `json:"enabled"`
		Mode    string   `json:"mode"`
		Config  any      `json:"config"`
	}
	entries := make([]domainEntry, 0)
	hasCC := false
	for _, route := range routes {
		if !route.CCEnabled {
			continue
		}
		hasCC = true
		entries = append(entries, domainEntry{
			Domains: route.Domains,
			Enabled: true,
			Mode:    normalizeCCMode(route.CCMode),
			Config:  route.CCConfig,
		})
	}
	if !hasCC {
		return "{}", nil, nil
	}
	data, err := json.Marshal(entries)
	if err != nil {
		return "", nil, err
	}
	return string(data), nil, nil
}

func renderRegionConfigBundle(routes []AccessRoute) (string, []SupportFile, error) {
	type domainEntry struct {
		Domains   []string `json:"domains"`
		Enabled   bool     `json:"enabled"`
		Mode      string   `json:"mode"`
		Countries []string `json:"countries"`
	}
	entries := make([]domainEntry, 0)
	hasRegionRestriction := false
	for _, route := range routes {
		if !route.RegionRestrictionEnabled || len(route.RegionRestrictionCountries) == 0 {
			continue
		}
		hasRegionRestriction = true
		entries = append(entries, domainEntry{
			Domains:   route.Domains,
			Enabled:   true,
			Mode:      normalizeRegionMode(route.RegionRestrictionMode),
			Countries: route.RegionRestrictionCountries,
		})
	}
	if !hasRegionRestriction {
		return "{}", nil, nil
	}
	data, err := json.Marshal(entries)
	if err != nil {
		return "", nil, err
	}
	return string(data), nil, nil
}

func renderWAFConfigBundle(routes []AccessRoute) (string, []SupportFile, error) {
	type domainEntry struct {
		Domains []string `json:"domains"`
		Enabled bool     `json:"enabled"`
		Mode    string   `json:"mode"`
		Config  any      `json:"config"`
	}
	entries := make([]domainEntry, 0)
	hasWAF := false
	for _, route := range routes {
		if !route.WAFEnabled {
			continue
		}
		hasWAF = true
		entries = append(entries, domainEntry{
			Domains: route.Domains,
			Enabled: true,
			Mode:    normalizeWAFMode(route.WAFMode),
			Config:  route.WAFConfig,
		})
	}
	if !hasWAF {
		return "{}", nil, nil
	}
	data, err := json.Marshal(entries)
	if err != nil {
		return "", nil, err
	}
	return string(data), nil, nil
}

func normalizeRegionMode(raw string) string {
	mode := strings.ToLower(strings.TrimSpace(raw))
	switch mode {
	case RegionModeAllow, RegionModeBlock:
		return mode
	default:
		return RegionModeBlock
	}
}

func normalizeWAFMode(raw string) string {
	mode := strings.ToLower(strings.TrimSpace(raw))
	switch mode {
	case "log", WAFModeBlock:
		return mode
	default:
		return WAFModeBlock
	}
}

func normalizeCCMode(raw string) string {
	mode := strings.ToLower(strings.TrimSpace(raw))
	switch mode {
	case "log", CCModeBlock, CCModePoW:
		return mode
	default:
		return CCModeBlock
	}
}
