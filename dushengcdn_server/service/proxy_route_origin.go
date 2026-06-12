package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"dushengcdn/model"

	"gorm.io/gorm"
)

func normalizeProxyRouteLimitConnValue(value int, field string) (int, error) {
	if value < 0 {
		return 0, fmt.Errorf("%s must be greater than or equal to 0", field)
	}
	return value, nil
}

func normalizeProxyRouteLimitRate(raw string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	if normalized == "" || normalized == "0" {
		return "", nil
	}
	if !proxyRouteLimitRatePattern.MatchString(normalized) {
		return "", errors.New("limit_rate must be a number or use the 512k / 1m format")
	}
	if strings.TrimRight(normalized, "km") == "" {
		return "", nil
	}
	return normalized, nil
}

func normalizeProxyRouteProxyBufferingMode(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case proxyRouteProxyBufferingModeOff:
		return proxyRouteProxyBufferingModeOff
	default:
		return proxyRouteProxyBufferingModeDefault
	}
}

func resolveProxyRoutePrimaryOrigin(input ProxyRouteInput) (string, *uint, error) {
	if hasStructuredOriginInput(input) {
		scheme, err := normalizeOriginScheme(input.OriginScheme)
		if err != nil {
			return "", nil, err
		}
		port, err := normalizeOriginPort(input.OriginPort)
		if err != nil {
			return "", nil, err
		}
		uri, err := normalizeOriginURI(input.OriginURI)
		if err != nil {
			return "", nil, err
		}
		if strings.Contains(uri, "$") {
			return "", nil, errors.New("origin URI cannot contain $")
		}
		if input.OriginID != nil && *input.OriginID != 0 {
			origin, err := model.GetOriginByID(*input.OriginID)
			if err != nil {
				return "", nil, errors.New("selected origin does not exist")
			}
			originURL, err := buildOriginURLFromParts(
				scheme,
				origin.Address,
				port,
				uri,
			)
			if err != nil {
				return "", nil, err
			}
			return originURL, &origin.ID, nil
		}

		address := normalizeOriginAddress(input.OriginAddress)
		if err := validateOriginAddress(address); err != nil {
			return "", nil, err
		}
		originURL, err := buildOriginURLFromParts(scheme, address, port, uri)
		if err != nil {
			return "", nil, err
		}
		origin, err := getOrCreateOriginByAddress(address)
		if err != nil {
			return "", nil, err
		}
		return originURL, &origin.ID, nil
	}

	originURL := strings.TrimSpace(input.OriginURL)
	if originURL == "" {
		return "", nil, errors.New("origin_url cannot be empty")
	}
	address, err := extractOriginAddress(originURL)
	if err != nil {
		return "", nil, err
	}
	origin, findErr := model.GetOriginByAddress(address)
	if findErr == nil {
		return originURL, &origin.ID, nil
	}
	if !errors.Is(findErr, gorm.ErrRecordNotFound) {
		return "", nil, findErr
	}
	return originURL, nil, nil
}

func hasStructuredOriginInput(input ProxyRouteInput) bool {
	return (input.OriginID != nil && *input.OriginID != 0) ||
		strings.TrimSpace(input.OriginScheme) != "" ||
		strings.TrimSpace(input.OriginAddress) != "" ||
		strings.TrimSpace(input.OriginPort) != "" ||
		strings.TrimSpace(input.OriginURI) != ""
}

func normalizeCustomHeaders(headers []ProxyRouteCustomHeaderInput) ([]ProxyRouteCustomHeaderInput, error) {
	if len(headers) == 0 {
		return []ProxyRouteCustomHeaderInput{}, nil
	}
	normalized := make([]ProxyRouteCustomHeaderInput, 0, len(headers))
	for _, header := range headers {
		key := strings.TrimSpace(header.Key)
		value := strings.TrimSpace(header.Value)
		if key == "" && value == "" {
			continue
		}
		if key == "" {
			return nil, errors.New("custom header key cannot be empty")
		}
		if !proxyHeaderKeyPattern.MatchString(key) {
			return nil, errors.New("custom header key format is invalid")
		}
		if strings.EqualFold(key, "Host") {
			return nil, errors.New("custom header Host is managed by origin_host")
		}
		if strings.ContainsAny(key, "\r\n") || strings.ContainsAny(value, "\r\n") {
			return nil, errors.New("custom headers cannot contain newlines")
		}
		if err := validateCustomHeaderValue(value); err != nil {
			return nil, err
		}
		normalized = append(normalized, ProxyRouteCustomHeaderInput{
			Key:   key,
			Value: value,
		})
	}
	return normalized, nil
}

func validateCustomHeaderValues(headers []ProxyRouteCustomHeaderInput) error {
	for _, header := range headers {
		if err := validateCustomHeaderValue(header.Value); err != nil {
			return err
		}
	}
	return nil
}

func validateCustomHeaderValue(value string) error {
	if strings.Contains(value, "$") {
		return errors.New("custom header value cannot contain $")
	}
	return nil
}

func restoreRedactedCustomHeaders(headers []ProxyRouteCustomHeaderInput, route *model.ProxyRoute) ([]ProxyRouteCustomHeaderInput, error) {
	if len(headers) == 0 || route == nil {
		return headers, nil
	}
	existing, err := decodeStoredCustomHeaders(route.CustomHeaders)
	if err != nil {
		return nil, err
	}
	existingByKey := make(map[string]string, len(existing))
	for _, header := range existing {
		existingByKey[strings.ToLower(header.Key)] = header.Value
	}
	for index := range headers {
		if !isSensitiveProxyRouteCustomHeader(headers[index].Key) {
			continue
		}
		if headers[index].Value != redactedProxyRouteCustomHeaderValue {
			continue
		}
		if value, ok := existingByKey[strings.ToLower(headers[index].Key)]; ok {
			headers[index].Value = value
		}
	}
	return headers, nil
}

func marshalCustomHeadersForView(headers []ProxyRouteCustomHeaderInput) string {
	viewHeaders := redactSensitiveCustomHeaders(headers)
	raw, err := json.Marshal(viewHeaders)
	if err != nil {
		return "[]"
	}
	return string(raw)
}

func redactSensitiveCustomHeaders(headers []ProxyRouteCustomHeaderInput) []ProxyRouteCustomHeaderInput {
	if len(headers) == 0 {
		return []ProxyRouteCustomHeaderInput{}
	}
	redacted := make([]ProxyRouteCustomHeaderInput, 0, len(headers))
	for _, header := range headers {
		if isSensitiveProxyRouteCustomHeader(header.Key) && header.Value != "" {
			header.Value = redactedProxyRouteCustomHeaderValue
		}
		redacted = append(redacted, header)
	}
	return redacted
}

func isSensitiveProxyRouteCustomHeader(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	if normalized == "" {
		return false
	}
	switch normalized {
	case "authorization", "proxy-authorization", "cookie", "set-cookie", "x-api-key", "x-api-token", "x-auth-token", "x-access-token", "x-csrf-token":
		return true
	}
	for _, marker := range []string{"token", "secret", "credential", "password", "passwd", "apikey", "api-key", "session", "cookie", "authorization"} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

func normalizeUpstreams(originURL string, upstreams []string) ([]string, error) {
	candidates := make([]string, 0, len(upstreams)+1)
	if strings.TrimSpace(originURL) != "" {
		candidates = append(candidates, originURL)
	}
	candidates = append(candidates, upstreams...)
	trimmed := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		item := strings.TrimSpace(candidate)
		if item == "" {
			continue
		}
		trimmed = append(trimmed, item)
	}
	unique := make([]string, 0, len(trimmed))
	seen := make(map[string]struct{}, len(trimmed))
	for _, item := range trimmed {
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		unique = append(unique, item)
	}
	normalized := make([]string, 0, len(unique))
	var scheme string
	multiUpstream := len(unique) > 1
	for _, item := range unique {
		if err := validateOriginURL(item); err != nil {
			return nil, err
		}
		parsed, err := url.ParseRequestURI(item)
		if err != nil {
			return nil, errors.New("origin URL format is invalid")
		}
		if multiUpstream && parsed.Path != "" && parsed.Path != "/" {
			return nil, errors.New("multi-upstream mode does not support origin paths")
		}
		if multiUpstream && parsed.RawQuery != "" {
			return nil, errors.New("multi-upstream mode does not support origin query strings")
		}
		if scheme == "" {
			scheme = parsed.Scheme
		} else if scheme != parsed.Scheme {
			return nil, errors.New("all upstreams must use the same scheme")
		}
		normalized = append(normalized, item)
	}
	if len(normalized) == 0 {
		return nil, errors.New("at least one upstream is required")
	}
	return normalized, nil
}

func decodeStoredCustomHeaders(raw string) ([]ProxyRouteCustomHeaderInput, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return []ProxyRouteCustomHeaderInput{}, nil
	}
	var headers []ProxyRouteCustomHeaderInput
	if err := json.Unmarshal([]byte(text), &headers); err != nil {
		return nil, errors.New("custom_headers payload is invalid")
	}
	return normalizeCustomHeaders(headers)
}

func decodeStoredUpstreams(raw string, fallbackOriginURL string) ([]string, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return normalizeUpstreams(fallbackOriginURL, nil)
	}
	var upstreams []string
	if err := json.Unmarshal([]byte(text), &upstreams); err != nil {
		return nil, errors.New("upstreams payload is invalid")
	}
	return normalizeUpstreams(fallbackOriginURL, upstreams)
}

func validateOriginURL(raw string) error {
	if raw == "" {
		return errors.New("origin URL cannot be empty")
	}
	parsed, err := url.ParseRequestURI(raw)
	if err != nil {
		return errors.New("origin URL format is invalid")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("origin URL must start with http:// or https://")
	}
	if parsed.User != nil {
		return errors.New("origin URL must not include userinfo")
	}
	if parsed.Host == "" {
		return errors.New("origin URL format is invalid")
	}
	if err := validateOriginAddress(normalizeOriginAddress(parsed.Hostname())); err != nil {
		return err
	}
	if parsed.Port() != "" {
		if _, err := normalizeOriginPort(parsed.Port()); err != nil {
			return err
		}
	}
	if strings.ContainsAny(parsed.EscapedPath(), ";\r\n{}$") || strings.ContainsAny(parsed.RawQuery, ";\r\n{}$") {
		return errors.New("origin URL path or query contains unsafe characters")
	}
	return nil
}

func validateOriginHost(raw string) error {
	if raw == "" {
		return nil
	}
	if strings.ContainsAny(raw, "/\\ \t\r\n;$`{}") || strings.Contains(raw, "://") {
		return errors.New("origin_host format is invalid")
	}
	parsed, err := url.Parse("//" + raw)
	if err != nil || parsed.Host == "" || parsed.Host != raw {
		return errors.New("origin_host format is invalid")
	}
	if parsed.Hostname() == "" {
		return errors.New("origin_host format is invalid")
	}
	if parsed.Port() != "" {
		if _, err := normalizeOriginPort(parsed.Port()); err != nil {
			return errors.New("origin_host format is invalid")
		}
	}
	return nil
}

func validateOriginHostHeader(raw string) error {
	if err := validateOriginHost(raw); err != nil {
		return errors.New(strings.Replace(err.Error(), "origin_host", "origin_host_header", 1))
	}
	return nil
}

func validateOriginSNI(raw string) error {
	if err := validateOriginHost(raw); err != nil {
		return errors.New(strings.Replace(err.Error(), "origin_host", "origin_sni", 1))
	}
	return nil
}

func normalizeOriginTLSVerify(value *bool) bool {
	if value == nil {
		return true
	}
	return *value
}

func normalizeOriginResolveMode(raw string) (string, error) {
	mode := strings.ToLower(strings.TrimSpace(raw))
	if mode == "" {
		return proxyRouteOriginResolvePublish, nil
	}
	switch mode {
	case proxyRouteOriginResolveRuntimeDNS,
		proxyRouteOriginResolvePublish,
		proxyRouteOriginResolveStaticIP,
		proxyRouteOriginResolveOriginGroup:
		return mode, nil
	default:
		return "", errors.New("origin_resolve_mode must be one of runtime_dns, publish_resolve, static_ip, origin_group")
	}
}

func normalizeStoredOriginHostHeader(route *model.ProxyRoute) string {
	if route == nil {
		return ""
	}
	if host := strings.TrimSpace(route.OriginHostHeader); host != "" {
		return host
	}
	return strings.TrimSpace(route.OriginHost)
}

func normalizeStoredOriginTLSVerify(route *model.ProxyRoute) bool {
	if route == nil {
		return true
	}
	return route.OriginTLSVerify
}

func normalizeStoredOriginResolveMode(raw string) string {
	mode, err := normalizeOriginResolveMode(raw)
	if err != nil {
		return proxyRouteOriginResolvePublish
	}
	return mode
}
