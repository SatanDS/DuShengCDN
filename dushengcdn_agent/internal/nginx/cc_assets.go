package nginx

import "dushengcdn-agent/internal/protocol"

const openRestyCCCheckLua = `local M = {}

local ok_cjson, cjson = pcall(require, "cjson.safe")
local ipmatcher = require "shared.ipmatcher"

local cc_config_dict = ngx.shared.dushengcdn_cc_config
local cc_counters = ngx.shared.dushengcdn_cc_counters

local function load_cc_config()
    if not ok_cjson or not cjson or not cc_config_dict then
        return
    end
    local config_paths = {
        "__DUSHENGCDN_RUNTIME_CONFIG_DIR__/cc_config.json",
        "/etc/nginx/dushengcdn-lua/cc_config.json",
        "/usr/local/openresty/nginx/conf/cc_config.json"
    }
    for _, config_path in ipairs(config_paths) do
        local f = io.open(config_path, "r")
        if f then
            local content = f:read("*a")
            f:close()
            local current_hash = ngx.md5(content or "")

            if current_hash == cc_config_dict:get("_config_hash") then
                return
            end

            local old_keys = cc_config_dict:get("_domain_keys")
            if old_keys then
                for domain in string.gmatch(old_keys, "[^\n]+") do
                    cc_config_dict:delete(domain)
                end
            end

            local domain_keys = {}
            if content and content ~= "" and content ~= "{}" then
                local ok, entries = pcall(cjson.decode, content)
                if ok and entries and type(entries) == "table" then
                    for _, entry in ipairs(entries) do
                        if entry.domains then
                            for _, domain in ipairs(entry.domains) do
                                cc_config_dict:set(domain, cjson.encode(entry), 0)
                                domain_keys[#domain_keys+1] = domain
                            end
                        end
                    end
                end
            end

            cc_config_dict:set("_domain_keys", table.concat(domain_keys, "\n"), 0)
            cc_config_dict:set("_config_hash", current_hash, 0)
            return
        end
    end
end

local function client_ip()
    local cf_ip = ngx.var.http_cf_connecting_ip
    if cf_ip and cf_ip ~= "" then
        return cf_ip
    end
    local real_ip = ngx.var.http_x_real_ip
    if real_ip and real_ip ~= "" then
        return real_ip
    end
    local forwarded = ngx.var.http_x_forwarded_for
    if forwarded and forwarded ~= "" then
        local first = string.match(forwarded, "^%s*([^,%s]+)")
        if first and first ~= "" then
            return first
        end
    end
    return ngx.var.remote_addr or ""
end

local function path_match(uri, pattern)
    if not pattern or pattern == "" then
        return false
    end
    if string.sub(pattern, -1) == "*" then
        local prefix = string.sub(pattern, 1, -2)
        return string.sub(uri, 1, #prefix) == prefix
    end
    return uri == pattern
end

local function list_contains_text(list, value)
    if not list or not value then
        return false
    end
    local lower_value = string.lower(value)
    for _, item in ipairs(list) do
        local lower_item = string.lower(item or "")
        if lower_item ~= "" and string.find(lower_value, lower_item, 1, true) then
            return true
        end
    end
    return false
end

local function match_list(list, ip, ua, uri)
    if not list then
        return false
    end
    if ipmatcher.match_ips(ip, list.ips) then
        return true
    end
    if ipmatcher.match_cidrs(ip, list.ip_cidrs) then
        return true
    end
    if list.paths then
        for _, path in ipairs(list.paths) do
            if path_match(uri, path) then
                return true
            end
        end
    end
    if list_contains_text(list.user_agents, ua) then
        return true
    end
    return false
end

local function safe_key(value)
    local escaped = ngx.escape_uri(value or "")
    if #escaped > 180 then
        escaped = ngx.md5(escaped)
    end
    return escaped
end

local function incr_counter(key, ttl)
    if not cc_counters then
        return 0
    end
    local value, err = cc_counters:incr(key, 1, 0, ttl)
    if err then
        return 0
    end
    return value or 0
end

local function set_reason(reason)
    ngx.var.dushengcdn_request_reason = reason
end

local function handle_hit(mode, reason, ttl)
    set_reason(reason)
    if cc_counters and ttl and ttl > 0 then
        cc_counters:set("blocked:" .. (ngx.var.host or "") .. ":" .. client_ip(), reason, ttl)
    end
    if mode == "log" then
        ngx.header["X-DuShengCDN-CC"] = "matched; mode=log; rule=" .. reason
        return
    end
    if mode == "pow" then
        if ngx.ctx then
            ngx.ctx.dushengcdn_force_pow = true
            ngx.ctx.dushengcdn_force_pow_reason = reason
        end
        ngx.header["X-DuShengCDN-CC"] = "matched; mode=pow; rule=" .. reason
        return
    end
    ngx.status = 429
    ngx.header["X-DuShengCDN-CC"] = "blocked; rule=" .. reason
    return ngx.exit(429)
end

function M.run()
    load_cc_config()
    if not ok_cjson or not cjson or not cc_config_dict or not cc_counters then
        return
    end

    local host = ngx.var.host
    if not host or host == "" then
        return
    end
    local raw = cc_config_dict:get(host)
    if not raw then
        return
    end
    local ok, entry = pcall(cjson.decode, raw)
    if not ok or not entry or not entry.enabled then
        return
    end

    local config = entry.config or {}
    local mode = entry.mode or "block"
    local uri = ngx.var.uri or ""
    local ua = ngx.var.http_user_agent or ""
    local ip = client_ip()

    local agent_api_prefix = "/api/agent/"
    if string.sub(uri, 1, #agent_api_prefix) == agent_api_prefix and ngx.var.http_x_agent_token and ngx.var.http_x_agent_token ~= "" then
        return
    end
    local authorization_header = ngx.var.http_authorization or ""
    if (uri == "/api/dns-snapshot" or uri == "/api/dns-worker-heartbeat") and (
        (ngx.var.http_x_dns_worker_token and ngx.var.http_x_dns_worker_token ~= "") or
        string.find(string.lower(authorization_header), "^bearer%s+")
    ) then
        return
    end

    if match_list(config.whitelist, ip, ua, uri) or match_list(config.exclude, ip, ua, uri) then
        return
    end

    local block_key = "blocked:" .. host .. ":" .. ip
    local blocked_reason = cc_counters:get(block_key)
    if blocked_reason then
        return handle_hit(mode, blocked_reason, tonumber(config.block_duration_seconds) or 300)
    end

    local now = ngx.time()
    local window = tonumber(config.window_seconds) or 10
    local max_requests = tonumber(config.max_requests) or 120
    local site_bucket = math.floor(now / window)
    local site_key = "site:" .. host .. ":" .. ip .. ":" .. tostring(site_bucket)
    local site_count = incr_counter(site_key, window + 2)
    if site_count > max_requests then
        return handle_hit(mode, "CC 防护：同一来源 " .. tostring(window) .. " 秒内请求 " .. tostring(site_count) .. " 次，超过阈值 " .. tostring(max_requests), tonumber(config.block_duration_seconds) or 300)
    end

    local path_window = tonumber(config.path_window_seconds) or 10
    local path_max_requests = tonumber(config.path_max_requests) or 60
    local path_bucket = math.floor(now / path_window)
    local path_key = "path:" .. host .. ":" .. ip .. ":" .. safe_key(uri) .. ":" .. tostring(path_bucket)
    local path_count = incr_counter(path_key, path_window + 2)
    if path_count > path_max_requests then
        return handle_hit(mode, "CC 防护：同一来源访问同一路径在 " .. tostring(path_window) .. " 秒内请求 " .. tostring(path_count) .. " 次，超过阈值 " .. tostring(path_max_requests), tonumber(config.block_duration_seconds) or 300)
    end
end

return M
`

func ManagedCCLuaFiles() []protocol.SupportFile {
	return []protocol.SupportFile{
		{Path: "cc/check.lua", Content: openRestyCCCheckLua},
	}
}
