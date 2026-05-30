package nginx

import "openflare-agent/internal/protocol"

const openRestyWAFCheckLua = `local M = {}

local ok_cjson, cjson = pcall(require, "cjson.safe")
local waf_config_dict = ngx.shared.openflare_waf_config

local function first_header_ip(value)
    if not value or value == "" then
        return ""
    end
    local ip = string.match(value, "^%s*([^,%s]+)")
    return ip or ""
end

local function client_ip()
    local cf_ip = first_header_ip(ngx.var.http_cf_connecting_ip)
    if cf_ip ~= "" then
        return cf_ip
    end
    local real_ip = first_header_ip(ngx.var.http_x_real_ip)
    if real_ip ~= "" then
        return real_ip
    end
    local forwarded_ip = first_header_ip(ngx.var.http_x_forwarded_for)
    if forwarded_ip ~= "" then
        return forwarded_ip
    end
    return ngx.var.remote_addr or ""
end

local function ip_to_number(ip)
    local a, b, c, d = string.match(ip or "", "^(%d+)%.(%d+)%.(%d+)%.(%d+)$")
    if not a then
        return nil
    end
    a, b, c, d = tonumber(a), tonumber(b), tonumber(c), tonumber(d)
    if not a or not b or not c or not d then
        return nil
    end
    if a > 255 or b > 255 or c > 255 or d > 255 then
        return nil
    end
    return a * 16777216 + b * 65536 + c * 256 + d
end

local function cidr_match(ip, cidr)
    local base, bits = string.match(cidr or "", "^([^/]+)/(%d+)$")
    if not base or not bits then
        return false
    end
    bits = tonumber(bits)
    if not bits or bits < 0 or bits > 32 then
        return false
    end
    local ip_num = ip_to_number(ip)
    local base_num = ip_to_number(base)
    if not ip_num or not base_num then
        return false
    end
    if bits == 0 then
        return true
    end
    local mask = 2 ^ 32 - 2 ^ (32 - bits)
    return (ip_num - (ip_num % (2 ^ (32 - bits)))) == (base_num - (base_num % (2 ^ (32 - bits))))
end

local function contains_text(haystack, needle)
    if not haystack or not needle or needle == "" then
        return false
    end
    return string.find(string.lower(tostring(haystack)), string.lower(tostring(needle)), 1, true) ~= nil
end

local function path_matches(pattern, uri)
    if not pattern or pattern == "" then
        return false
    end
    if string.sub(pattern, -1) == "*" then
        local prefix = string.sub(pattern, 1, -2)
        return string.sub(uri, 1, #prefix) == prefix
    end
    return uri == pattern
end

local function request_headers_text()
    local headers = ngx.req.get_headers(100, true)
    local parts = {}
    for key, value in pairs(headers or {}) do
        if type(value) == "table" then
            value = table.concat(value, " ")
        end
        parts[#parts + 1] = tostring(key) .. ": " .. tostring(value)
    end
    return table.concat(parts, "\n")
end

local function has_builtin_rule(config, name)
    local rules = config and config.builtin_rules or {}
    for _, rule in ipairs(rules) do
        if rule == name then
            return true
        end
    end
    return false
end

local function load_waf_config()
    if not ok_cjson or not cjson or not waf_config_dict then
        return
    end
    local config_paths = {
        "__OPENFLARE_RUNTIME_CONFIG_DIR__/waf_config.json",
        "/etc/nginx/openflare-lua/waf_config.json",
        "/usr/local/openresty/nginx/conf/waf_config.json"
    }
    for _, config_path in ipairs(config_paths) do
        local f = io.open(config_path, "r")
        if f then
            local content = f:read("*a")
            f:close()
            local current_hash = ngx.md5(content or "")
            if current_hash == waf_config_dict:get("_config_hash") then
                return
            end
            local old_keys = waf_config_dict:get("_domain_keys")
            if old_keys then
                for domain in string.gmatch(old_keys, "[^\n]+") do
                    waf_config_dict:delete(domain)
                end
            end
            local domain_keys = {}
            if content and content ~= "" and content ~= "{}" then
                local ok, entries = pcall(cjson.decode, content)
                if ok and entries and type(entries) == "table" then
                    for _, entry in ipairs(entries) do
                        if entry.domains then
                            for _, domain in ipairs(entry.domains) do
                                waf_config_dict:set(domain, cjson.encode(entry), 0)
                                domain_keys[#domain_keys + 1] = domain
                            end
                        end
                    end
                end
            end
            waf_config_dict:set("_domain_keys", table.concat(domain_keys, "\n"), 0)
            waf_config_dict:set("_config_hash", current_hash, 0)
            return
        end
    end
end

local function matched_whitelist(config)
    local whitelist = config.whitelist or {}
    local ip = client_ip()
    for _, item in ipairs(whitelist.ips or {}) do
        if item == ip then
            return true
        end
    end
    for _, item in ipairs(whitelist.ip_cidrs or {}) do
        if cidr_match(ip, item) then
            return true
        end
    end
    local uri = ngx.var.uri or ""
    for _, item in ipairs(whitelist.paths or {}) do
        if path_matches(item, uri) then
            return true
        end
    end
    return false
end

local function builtin_match(config, uri, query, user_agent, headers_text)
    local target = uri .. "?" .. query .. "\n" .. headers_text
    if has_builtin_rule(config, "path_traversal") then
        local lower_uri = string.lower(uri .. "?" .. query)
        if string.find(lower_uri, "../", 1, true) or string.find(lower_uri, "..\\", 1, true) or string.find(lower_uri, "%2e%2e", 1, true) then
            return "path_traversal"
        end
    end
    if has_builtin_rule(config, "sensitive_paths") then
        local sensitive = {"/.git", "/.env", "/wp-config.php", "/phpmyadmin", "/adminer.php", "/.svn", "/.hg"}
        local lower_uri = string.lower(uri)
        for _, item in ipairs(sensitive) do
            if string.sub(lower_uri, 1, #item) == item then
                return "sensitive_paths"
            end
        end
    end
    if has_builtin_rule(config, "bad_bots") then
        local bots = {"sqlmap", "nikto", "acunetix", "masscan", "nessus", "nmap", "zgrab", "gobuster", "dirbuster"}
        for _, bot in ipairs(bots) do
            if contains_text(user_agent, bot) then
                return "bad_bots"
            end
        end
    end
    if has_builtin_rule(config, "xss") then
        local xss = {"<script", "javascript:", "onerror=", "onload=", "document.cookie", "alert("}
        for _, item in ipairs(xss) do
            if contains_text(target, item) then
                return "xss"
            end
        end
    end
    if has_builtin_rule(config, "sqli") then
        local sqli = {" union select ", " or 1=1", "' or '1'='1", "\" or \"1\"=\"1", "information_schema", "sleep(", "benchmark(", "load_file("}
        local lower_target = " " .. string.lower(target)
        for _, item in ipairs(sqli) do
            if string.find(lower_target, item, 1, true) then
                return "sqli"
            end
        end
    end
    return nil
end

local function custom_match(config, uri, query, user_agent, headers_text)
    local rules = config.block_rules or {}
    for _, item in ipairs(rules.path_contains or {}) do
        if contains_text(uri, item) then
            return "custom_path_contains"
        end
    end
    for _, pattern in ipairs(rules.path_regexes or {}) do
        local ok, matched = pcall(ngx.re.find, uri, pattern, "ijo")
        if ok and matched then
            return "custom_path_regex"
        end
    end
    for _, item in ipairs(rules.query_contains or {}) do
        if contains_text(query, item) then
            return "custom_query_contains"
        end
    end
    for _, item in ipairs(rules.header_contains or {}) do
        if contains_text(headers_text, item) then
            return "custom_header_contains"
        end
    end
    for _, item in ipairs(rules.user_agents or {}) do
        if contains_text(user_agent, item) then
            return "custom_user_agent"
        end
    end
    return nil
end

function M.run()
    load_waf_config()
    if not waf_config_dict or not ok_cjson or not cjson then
        return
    end
    local host = ngx.var.host
    if not host or host == "" then
        return
    end
    local raw = waf_config_dict:get(host)
    if not raw then
        return
    end
    local ok, entry = pcall(cjson.decode, raw)
    if not ok or not entry or not entry.enabled then
        return
    end
    local config = entry.config or {}
    if matched_whitelist(config) then
        return
    end
    local uri = ngx.var.uri or ""
    local query = ngx.var.args or ""
    local user_agent = ngx.var.http_user_agent or ""
    local headers_text = request_headers_text()
    local reason = builtin_match(config, uri, query, user_agent, headers_text) or custom_match(config, uri, query, user_agent, headers_text)
    if not reason then
        return
    end
    ngx.log(ngx.WARN, "openflare waf matched ", reason, " host=", host, " ip=", client_ip(), " uri=", ngx.var.request_uri or uri)
    if (entry.mode or "block") == "log" then
        ngx.header["X-OpenFlare-WAF"] = "matched; mode=log; rule=" .. reason
        return
    end
    ngx.header["X-OpenFlare-WAF"] = "blocked; rule=" .. reason
    return ngx.exit(ngx.HTTP_FORBIDDEN)
end

return M
`

func ManagedWAFLuaFiles() []protocol.SupportFile {
	return []protocol.SupportFile{
		{Path: "waf/check.lua", Content: openRestyWAFCheckLua},
	}
}
