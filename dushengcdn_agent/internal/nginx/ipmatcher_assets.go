package nginx

import "dushengcdn-agent/internal/protocol"

const openRestyIPMatcherLua = `local M = {}

local bit = require "bit"

local function trim(value)
    return tostring(value or ""):match("^%s*(.-)%s*$")
end

local function clean_ip(value)
    local ip = trim(value)
    if ip == "" then
        return ""
    end
    if string.sub(ip, 1, 1) == "[" then
        local closing = string.find(ip, "]", 2, true)
        if closing then
            ip = string.sub(ip, 2, closing - 1)
        end
    end
    local zone = string.find(ip, "%%", 1, true)
    if zone then
        ip = string.sub(ip, 1, zone - 1)
    end
    return ip
end

local function parse_ipv4(ip)
    ip = clean_ip(ip)
    local host = string.match(ip, "^(%d+%.%d+%.%d+%.%d+):%d+$")
    if host then
        ip = host
    end
    local a, b, c, d = string.match(ip, "^(%d+)%.(%d+)%.(%d+)%.(%d+)$")
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
    return { family = 4, bits = 32, bytes = { a, b, c, d } }
end

local function split_colon(text)
    if text == "" then
        return {}
    end
    if string.sub(text, 1, 1) == ":" or string.sub(text, -1) == ":" then
        return nil
    end
    local parts = {}
    for part in string.gmatch(text, "[^:]+") do
        parts[#parts + 1] = part
    end
    return parts
end

local function parse_hextets(parts)
    if not parts then
        return nil
    end
    local words = {}
    for _, part in ipairs(parts) do
        if part == "" or #part > 4 or not string.match(part, "^[0-9a-fA-F]+$") then
            return nil
        end
        local number = tonumber(part, 16)
        if not number or number < 0 or number > 65535 then
            return nil
        end
        words[#words + 1] = number
    end
    return words
end

local function append_words(target, words)
    for _, word in ipairs(words or {}) do
        target[#target + 1] = word
    end
end

local function parse_ipv6(ip)
    ip = string.lower(clean_ip(ip))
    if ip == "" then
        return nil
    end
    if string.find(ip, ".", 1, true) then
        local prefix, tail = string.match(ip, "^(.*:)([^:]+)$")
        if not prefix or not tail then
            return nil
        end
        local parsed_tail = parse_ipv4(tail)
        if not parsed_tail then
            return nil
        end
        local b = parsed_tail.bytes
        ip = prefix .. string.format("%x:%x", b[1] * 256 + b[2], b[3] * 256 + b[4])
    end

    local words = {}
    local double_pos = string.find(ip, "::", 1, true)
    if double_pos then
        if string.find(ip, "::", double_pos + 2, true) then
            return nil
        end
        local left_text = string.sub(ip, 1, double_pos - 1)
        local right_text = string.sub(ip, double_pos + 2)
        local left = parse_hextets(split_colon(left_text))
        local right = parse_hextets(split_colon(right_text))
        if not left or not right then
            return nil
        end
        local zero_count = 8 - #left - #right
        if zero_count < 1 then
            return nil
        end
        append_words(words, left)
        for _ = 1, zero_count do
            words[#words + 1] = 0
        end
        append_words(words, right)
    else
        words = parse_hextets(split_colon(ip))
        if not words or #words ~= 8 then
            return nil
        end
    end

    if #words ~= 8 then
        return nil
    end
    local bytes = {}
    for _, word in ipairs(words) do
        bytes[#bytes + 1] = math.floor(word / 256)
        bytes[#bytes + 1] = word % 256
    end
    return { family = 6, bits = 128, bytes = bytes }
end

local function parse_ip(ip)
    ip = clean_ip(ip)
    if ip == "" then
        return nil
    end
    if string.find(ip, ":", 1, true) then
        return parse_ipv6(ip)
    end
    return parse_ipv4(ip)
end

local function parse_cidr(cidr)
    cidr = trim(cidr)
    if cidr == "" then
        return nil
    end
    local base, prefix_text = string.match(cidr, "^([^/]+)/(%d+)$")
    if not base then
        local parsed = parse_ip(cidr)
        if not parsed then
            return nil
        end
        return parsed, parsed.bits
    end
    local parsed = parse_ip(base)
    local prefix = tonumber(prefix_text)
    if not parsed or not prefix or prefix < 0 or prefix > parsed.bits then
        return nil
    end
    return parsed, prefix
end

local function bytes_match(ip_bytes, network_bytes, prefix)
    if #ip_bytes ~= #network_bytes then
        return false
    end
    local full_bytes = math.floor(prefix / 8)
    for i = 1, full_bytes do
        if ip_bytes[i] ~= network_bytes[i] then
            return false
        end
    end
    local remaining_bits = prefix % 8
    if remaining_bits == 0 then
        return true
    end
    local mask = bit.band(bit.lshift(0xff, 8 - remaining_bits), 0xff)
    local index = full_bytes + 1
    return bit.band(ip_bytes[index], mask) == bit.band(network_bytes[index], mask)
end

function M.cidr_match(ip, cidr)
    local parsed_ip = parse_ip(ip)
    local parsed_network, prefix = parse_cidr(cidr)
    if not parsed_ip or not parsed_network or not prefix then
        return false
    end
    if parsed_ip.family ~= parsed_network.family then
        return false
    end
    return bytes_match(parsed_ip.bytes, parsed_network.bytes, prefix)
end

function M.match_cidrs(ip, cidrs)
    if not cidrs or #cidrs == 0 then
        return false
    end
    for _, cidr in ipairs(cidrs) do
        if M.cidr_match(ip, cidr) then
            return true
        end
    end
    return false
end

function M.ip_equal(left, right)
    local parsed_left = parse_ip(left)
    local parsed_right = parse_ip(right)
    if not parsed_left or not parsed_right or parsed_left.family ~= parsed_right.family then
        return false
    end
    for i = 1, #parsed_left.bytes do
        if parsed_left.bytes[i] ~= parsed_right.bytes[i] then
            return false
        end
    end
    return true
end

function M.match_ips(ip, ips)
    if not ips or #ips == 0 then
        return false
    end
    for _, candidate in ipairs(ips) do
        if M.ip_equal(ip, candidate) then
            return true
        end
    end
    return false
end

return M
`

func ManagedSharedLuaFiles() []protocol.SupportFile {
	return []protocol.SupportFile{
		{Path: "shared/ipmatcher.lua", Content: openRestyIPMatcherLua},
	}
}
