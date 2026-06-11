package openresty

import "fmt"

const (
	openRestyObservabilityInitLuaPath = "init.lua"
	openRestyObservabilityLogLuaPath  = "log.lua"
	openRestyObservabilityReadLuaPath = "read.lua"
)

func renderOpenRestyObservabilityTemplateBlock() string {
	return stringsJoinLines(
		"    lua_shared_dict dushengcdn_observability 10m;",
		"    lua_shared_dict dushengcdn_pow_config 1m;",
		"    lua_shared_dict dushengcdn_waf_config 1m;",
		"    lua_shared_dict dushengcdn_cc_config 1m;",
		"    lua_shared_dict dushengcdn_cc_counters 20m;",
		"    lua_shared_dict dushengcdn_pow_challenges 10m;",
		"    lua_shared_dict dushengcdn_pow_sessions 20m;",
		"    lua_shared_dict dushengcdn_geoip_cache 20m;",
		"    lua_shared_dict dushengcdn_region_config 1m;",
		fmt.Sprintf("    init_worker_by_lua_file %s/%s;", LuaDirPlaceholder, openRestyObservabilityInitLuaPath),
		fmt.Sprintf("    log_by_lua_file %s/%s;", LuaDirPlaceholder, openRestyObservabilityLogLuaPath),
		"",
		fmt.Sprintf("    server {"),
		fmt.Sprintf("        listen %s;", ObservabilityListenPlaceholder),
		"        server_name dushengcdn-observability;",
		"        access_log off;",
		"        allow 127.0.0.1;",
		"        allow ::1;",
		"        deny all;",
		"",
		"        location = /dushengcdn/observability {",
		"            default_type application/json;",
		fmt.Sprintf("            content_by_lua_file %s/%s;", LuaDirPlaceholder, openRestyObservabilityReadLuaPath),
		"        }",
		"",
		"        location = /dushengcdn/stub_status {",
		"            stub_status;",
		"        }",
		"    }",
		"",
	)
}

func stringsJoinLines(lines ...string) string {
	if len(lines) == 0 {
		return ""
	}
	result := ""
	for index, line := range lines {
		if index > 0 {
			result += "\n"
		}
		result += line
	}
	return result + "\n"
}
