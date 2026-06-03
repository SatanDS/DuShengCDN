package service

import (
	"dushengcdn/utils/geoip"
	"log/slog"
	"net"
	"strings"
)

var accessLogGeoProviderFactory = func() (geoip.GeoIPService, error) {
	return geoip.NewMaxMindGeoIPService()
}

type accessLogRegionResolver struct {
	provider geoip.GeoIPService
	cache    map[string]accessLogGeoResult
}

type accessLogGeoResult struct {
	region   string
	operator string
}

func newAccessLogRegionResolver() (*accessLogRegionResolver, error) {
	provider, err := accessLogGeoProviderFactory()
	if err != nil {
		return nil, err
	}
	return &accessLogRegionResolver{
		provider: provider,
		cache:    make(map[string]accessLogGeoResult),
	}, nil
}

func (r *accessLogRegionResolver) Close() {
	if r == nil || r.provider == nil {
		return
	}
	if err := r.provider.Close(); err != nil {
		slog.Warn("close access log geo provider failed", "error", err)
	}
}

func (r *accessLogRegionResolver) Resolve(rawIP string) string {
	return r.ResolveInfo(rawIP).region
}

func (r *accessLogRegionResolver) ResolveInfo(rawIP string) accessLogGeoResult {
	if r == nil || r.provider == nil {
		return accessLogGeoResult{}
	}
	normalizedIP := normalizeAccessLogIP(rawIP)
	if normalizedIP == "" {
		return accessLogGeoResult{}
	}
	if cached, ok := r.cache[normalizedIP]; ok {
		return cached
	}

	info, err := r.provider.GetGeoInfo(net.ParseIP(normalizedIP))
	if err != nil || info == nil {
		r.cache[normalizedIP] = accessLogGeoResult{}
		return accessLogGeoResult{}
	}

	region := strings.TrimSpace(info.Name)
	if region == "" {
		region = strings.TrimSpace(info.ISOCode)
	}
	result := accessLogGeoResult{
		region:   region,
		operator: strings.TrimSpace(info.Operator),
	}
	r.cache[normalizedIP] = result
	return result
}

func normalizeAccessLogIP(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}

	if ip := net.ParseIP(trimmed); ip != nil {
		return ip.String()
	}

	trimmed = strings.TrimPrefix(trimmed, "[")
	trimmed = strings.TrimSuffix(trimmed, "]")
	if ip := net.ParseIP(trimmed); ip != nil {
		return ip.String()
	}

	host, _, err := net.SplitHostPort(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	host = strings.TrimPrefix(host, "[")
	host = strings.TrimSuffix(host, "]")
	if ip := net.ParseIP(host); ip != nil {
		return ip.String()
	}
	return ""
}
