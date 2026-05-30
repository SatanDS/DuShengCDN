package nginx

import "dushengcdn-agent/internal/protocol"

const openRestyGeoIPAccessLua = `local M = {}

local ok_cjson, cjson = pcall(require, "cjson.safe")
local ok_mmdb, maxminddb = pcall(require, "resty.maxminddb")
local ok_http, http = pcall(require, "resty.http")
local ip_cache = ngx.shared.dushengcdn_geoip_cache
local region_config_dict = ngx.shared.dushengcdn_region_config
local db_path = "__DUSHENGCDN_GEOIP_DATABASE_PATH__"
local api_url = __DUSHENGCDN_GEOIP_LOOKUP_API_URL__
local api_token = __DUSHENGCDN_GEOIP_LOOKUP_API_TOKEN__
local api_timeout = __DUSHENGCDN_GEOIP_LOOKUP_API_TIMEOUT__
local db_ready = false
local db_checked_at = 0
local db_check_interval = 60
local unknown_country = "ZZ"

local function ensure_mmdb()
    if db_ready then
        return true
    end
    if not ok_mmdb or not maxminddb then
        return false
    end
    local now = ngx.now()
    if db_checked_at > 0 and now - db_checked_at < db_check_interval then
        return false
    end
    db_checked_at = now
    local ok, init_result = pcall(maxminddb.init, db_path)
    if not ok or init_result == false then
        db_ready = false
        return false
    end
    if maxminddb.initted then
        local ok_initted, initted = pcall(maxminddb.initted)
        db_ready = ok_initted and initted
    else
        db_ready = true
    end
    return db_ready
end

local function is_private_ip(ip)
    if not ip or ip == "" then
        return true
    end
    if ip == "127.0.0.1" or ip == "::1" then
        return true
    end
    if string.match(ip, "^10%.") or string.match(ip, "^192%.168%.") then
        return true
    end
    local second = tonumber(string.match(ip, "^172%.(%d+)%."))
    if second and second >= 16 and second <= 31 then
        return true
    end
    if string.match(ip, "^169%.254%.") then
        return true
    end
    local lower_ip = string.lower(ip)
    if string.match(lower_ip, "^fc") or string.match(lower_ip, "^fd") or string.match(lower_ip, "^fe80:") then
        return true
    end
    return false
end

local function first_header_ip(value)
    if not value or value == "" then
        return ""
    end
    local ip = string.match(value, "^%s*([^,%s]+)")
    return ip or ""
end

local function client_ip()
    local trusted_header_ip = first_header_ip(ngx.var.http_cf_connecting_ip)
    if trusted_header_ip ~= "" then
        return trusted_header_ip
    end
    local candidates = {
        first_header_ip(ngx.var.http_x_real_ip),
        first_header_ip(ngx.var.http_x_forwarded_for),
        ngx.var.remote_addr or "",
    }
    for _, ip in ipairs(candidates) do
        if ip ~= "" and not is_private_ip(ip) then
            return ip
        end
    end
    return candidates[#candidates] or ""
end

local function normalize_country_code(value)
    if not value then
        return ""
    end
    local code = string.upper(tostring(value))
    if string.match(code, "^[A-Z][A-Z]$") then
        return code
    end
    return ""
end

local function country_from_table(value)
    if type(value) ~= "table" then
        return normalize_country_code(value)
    end
    local fields = {
        "country_code",
        "countryCode",
        "iso_code",
        "isoCode",
        "code",
    }
    for _, field in ipairs(fields) do
        local code = normalize_country_code(value[field])
        if code ~= "" then
            return code
        end
    end
    if type(value.country) ~= "table" then
        return normalize_country_code(value.country)
    end
    return country_from_table(value.country)
end

local function country_from_api_payload(payload)
    if not ok_cjson or not cjson or not payload or payload == "" then
        return ""
    end
    local ok, decoded = pcall(cjson.decode, payload)
    if not ok or type(decoded) ~= "table" then
        return ""
    end
    local direct_fields = {
        "country_code",
        "countryCode",
        "iso_code",
        "isoCode",
        "code",
        "country",
    }
    for _, field in ipairs(direct_fields) do
        local code = country_from_table(decoded[field])
        if code ~= "" then
            return code
        end
    end
    for _, parent in ipairs({"data", "result", "geoip", "geo"}) do
        local value = decoded[parent]
        if type(value) == "table" then
            local code = country_from_table(value)
            if code ~= "" then
                return code
            end
        end
    end
    return ""
end

local function build_api_lookup_url(ip)
    if not api_url or api_url == "" or not ip or ip == "" then
        return ""
    end
    local separator = "?"
    if string.find(api_url, "?", 1, true) then
        separator = "&"
    end
    return api_url .. separator .. "ip=" .. ngx.escape_uri(ip)
end

local function lookup_country_with_api(ip)
    if not ok_http or not http or not api_url or api_url == "" then
        return ""
    end
    local request_url = build_api_lookup_url(ip)
    if request_url == "" then
        return ""
    end
    local httpc = http.new()
    if not httpc then
        return ""
    end
    if api_timeout and api_timeout > 0 then
        httpc:set_timeout(api_timeout)
    end
    local headers = {
        ["Accept"] = "application/json",
        ["User-Agent"] = "DuShengCDN-Agent-GeoIP",
    }
    if api_token and api_token ~= "" then
        headers["Authorization"] = "Bearer " .. api_token
    end
    local res, err = httpc:request_uri(request_url, {
        method = "GET",
        headers = headers,
        keepalive = false,
    })
    if not res or err or res.status < 200 or res.status >= 300 then
        return ""
    end
    return country_from_api_payload(res.body)
end

function M.country_code(ip)
    ip = ip or client_ip()
    if ip == "" then
        return unknown_country
    end
    if ip_cache then
        local cached = ip_cache:get(ip)
        if cached and cached ~= "" then
            return cached
        end
    end
    local code = unknown_country
    if ensure_mmdb() then
        local ok, result = pcall(maxminddb.lookup, ip)
        if ok and result and result.country and result.country.iso_code then
            code = normalize_country_code(result.country.iso_code)
        elseif ok and result and result.registered_country and result.registered_country.iso_code then
            code = normalize_country_code(result.registered_country.iso_code)
        end
    end
    if code == unknown_country then
        local api_code = lookup_country_with_api(ip)
        if api_code ~= "" then
            code = api_code
        end
    end
    code = normalize_country_code(code)
    if code == "" then
        code = unknown_country
    end
    if ip_cache then
        local ttl = 86400
        if code == unknown_country then
            ttl = 300
        end
        ip_cache:set(ip, code, ttl)
    end
    return code
end

local function load_region_config()
    if not ok_cjson or not cjson or not region_config_dict then
        return
    end
    local config_paths = {
        "__DUSHENGCDN_RUNTIME_CONFIG_DIR__/region_config.json",
        "/etc/nginx/dushengcdn-lua/region_config.json",
        "/usr/local/openresty/nginx/conf/region_config.json"
    }
    for _, config_path in ipairs(config_paths) do
        local f = io.open(config_path, "r")
        if f then
            local content = f:read("*a")
            f:close()
            local current_hash = ngx.md5(content or "")
            if current_hash == region_config_dict:get("_config_hash") then
                return
            end
            local old_keys = region_config_dict:get("_domain_keys")
            if old_keys then
                for domain in string.gmatch(old_keys, "[^\n]+") do
                    region_config_dict:delete(domain)
                end
            end
            local domain_keys = {}
            if content and content ~= "" and content ~= "{}" then
                local ok, entries = pcall(cjson.decode, content)
                if ok and entries and type(entries) == "table" then
                    for _, entry in ipairs(entries) do
                        if entry.domains then
                            for _, domain in ipairs(entry.domains) do
                                region_config_dict:set(domain, cjson.encode(entry), 0)
                                domain_keys[#domain_keys+1] = domain
                            end
                        end
                    end
                end
            end
            region_config_dict:set("_domain_keys", table.concat(domain_keys, "\n"), 0)
            region_config_dict:set("_config_hash", current_hash, 0)
            return
        end
    end
end

local function contains_country(countries, code)
    if not countries or not code or code == "" then
        return false
    end
    for _, country in ipairs(countries) do
        if string.upper(tostring(country)) == code then
            return true
        end
    end
    return false
end

function M.check_region()
    load_region_config()
    if not region_config_dict then
        return
    end
    local host = ngx.var.host
    if not host or host == "" then
        return
    end
    local raw = region_config_dict:get(host)
    if not raw then
        return
    end
    local ok, config = pcall(cjson.decode, raw)
    if not ok or not config or not config.enabled then
        return
    end
    local countries = config.countries or {}
    if #countries == 0 then
        return
    end
    local code = M.country_code(client_ip())
    local matched = contains_country(countries, code)
    local mode = config.mode or "block"
    if mode == "allow" and not matched then
        return ngx.exit(ngx.HTTP_FORBIDDEN)
    end
    if mode ~= "allow" and matched then
        return ngx.exit(ngx.HTTP_FORBIDDEN)
    end
end

function M.run_access()
    M.check_region()
    local ok_waf, waf = pcall(require, "waf.check")
    if ok_waf and waf and waf.run then
        waf.run()
    end
    local ok_pow, pow = pcall(require, "pow.check")
    if ok_pow and pow and pow.run then
        return pow.run()
    end
end

return M
`

const openRestyAccessLua = `local source = debug.getinfo(1, "S").source or ""
if string.sub(source, 1, 1) == "@" then
    local script_path = string.sub(source, 2)
    local base_dir = string.match(script_path, "^(.*)/[^/]+%.lua$")
    if base_dir and base_dir ~= "" then
        package.path = base_dir .. "/?.lua;" .. base_dir .. "/?/init.lua;" .. package.path
    end
end

local access = require "geoip.access"
return access.run_access()
`

func ManagedGeoIPLuaFiles() []protocol.SupportFile {
	return []protocol.SupportFile{
		{Path: "access.lua", Content: openRestyAccessLua},
		{Path: "geoip/access.lua", Content: openRestyGeoIPAccessLua},
	}
}
