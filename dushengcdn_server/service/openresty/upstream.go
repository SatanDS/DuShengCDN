package openresty

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"

	"dushengcdn/utils/security"
)

const multiOriginShapeError = "multi-origin upstreams must be scheme://host:port without path/query/fragment"

func buildRouteUpstreamConfig(route Route, upstreams []string, options RenderOptions) (routeUpstreamConfig, error) {
	if len(upstreams) == 0 {
		return routeUpstreamConfig{}, nil
	}
	resolveMode := normalizeOriginResolveMode(route.OriginResolveMode)
	if len(upstreams) == 1 {
		parsed, err := url.Parse(strings.TrimSpace(upstreams[0]))
		if err != nil || parsed.Host == "" || parsed.Scheme == "" {
			return routeUpstreamConfig{}, nil
		}
		if resolveMode == OriginResolveModeRuntimeDNS {
			if err := validateRuntimeDNSOrigin(parsed.Scheme, parsed.Host); err != nil {
				return routeUpstreamConfig{}, err
			}
			return routeUpstreamConfig{
				Scheme:           parsed.Scheme,
				ProxyPassURI:     buildUpstreamProxyPassURI(parsed),
				RuntimeProxyPass: buildRuntimeProxyPassOrigin(parsed),
				UsesRuntimeDNS:   true,
			}, nil
		}
		servers, err := buildResolvedUpstreamServers(context.Background(), parsed.Scheme, parsed.Host, resolveMode, options)
		if err != nil {
			return routeUpstreamConfig{}, err
		}
		return routeUpstreamConfig{
			Name:              buildRouteUpstreamName(route),
			Scheme:            parsed.Scheme,
			ProxyPassURI:      buildUpstreamProxyPassURI(parsed),
			Servers:           servers,
			UsesNamedUpstream: true,
		}, nil
	}
	servers := make([]string, 0, len(upstreams))
	var scheme string
	usesRuntimeDNS := resolveMode == OriginResolveModeRuntimeDNS
	for _, upstream := range upstreams {
		parsed, err := url.Parse(strings.TrimSpace(upstream))
		if err != nil || !isValidMultiOriginUpstream(parsed) {
			return routeUpstreamConfig{}, errors.New(multiOriginShapeError)
		}
		if scheme == "" {
			scheme = parsed.Scheme
		} else if scheme != parsed.Scheme {
			return routeUpstreamConfig{}, errors.New("multi-origin upstreams must use the same scheme")
		}
		if usesRuntimeDNS {
			server, err := buildRuntimeDNSUpstreamServer(parsed.Scheme, parsed.Host)
			if err != nil {
				return routeUpstreamConfig{}, err
			}
			servers = append(servers, server)
		} else {
			resolvedServers, err := buildResolvedUpstreamServers(context.Background(), parsed.Scheme, parsed.Host, resolveMode, options)
			if err != nil {
				return routeUpstreamConfig{}, err
			}
			servers = append(servers, resolvedServers...)
		}
	}
	return routeUpstreamConfig{
		Name:              buildRouteUpstreamName(route),
		Scheme:            scheme,
		Servers:           servers,
		UsesNamedUpstream: true,
		UsesRuntimeDNS:    usesRuntimeDNS,
	}, nil
}

func normalizeOriginResolveMode(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case OriginResolveModeRuntimeDNS:
		return OriginResolveModeRuntimeDNS
	case OriginResolveModeStaticIP:
		return OriginResolveModeStaticIP
	case OriginResolveModeOriginGroup:
		return OriginResolveModeOriginGroup
	case OriginResolveModePublishResolve, "":
		return OriginResolveModePublishResolve
	default:
		return OriginResolveModePublishResolve
	}
}

func isValidMultiOriginUpstream(parsed *url.URL) bool {
	if parsed == nil || parsed.Host == "" || parsed.Scheme == "" {
		return false
	}
	path := strings.TrimSpace(parsed.EscapedPath())
	return (path == "" || path == "/") && parsed.RawQuery == "" && parsed.Fragment == ""
}

func buildUpstreamProxyPassURI(parsed *url.URL) string {
	if parsed == nil {
		return ""
	}
	path := parsed.EscapedPath()
	if path == "/" {
		path = ""
	}
	if parsed.RawQuery == "" {
		return path
	}
	return fmt.Sprintf("%s?%s", path, parsed.RawQuery)
}

func buildRuntimeProxyPassOrigin(parsed *url.URL) string {
	if parsed == nil {
		return ""
	}
	host, port := splitUpstreamHostPort(parsed.Scheme, parsed.Host)
	if host == "" || port == "" {
		return fmt.Sprintf("%s://%s", parsed.Scheme, parsed.Host)
	}
	return fmt.Sprintf("%s://%s", parsed.Scheme, formatUpstreamServer(host, port))
}

func buildRuntimeDNSUpstreamServer(scheme string, hostPort string) (string, error) {
	host, port := splitUpstreamHostPort(scheme, hostPort)
	normalizedPort, err := normalizeOriginPort(port)
	if err != nil {
		return "", err
	}
	if err := validatePublicOriginHost(host); err != nil {
		return "", err
	}
	return formatUpstreamServer(host, normalizedPort), nil
}

func validateRuntimeDNSOrigin(scheme string, hostPort string) error {
	_, err := buildRuntimeDNSUpstreamServer(scheme, hostPort)
	return err
}

func buildResolvedUpstreamServers(ctx context.Context, scheme string, hostPort string, resolveMode string, options RenderOptions) ([]string, error) {
	if resolveMode == OriginResolveModeStaticIP {
		return buildStaticIPUpstreamServer(scheme, hostPort)
	}
	return resolvePublicUpstreamServers(ctx, scheme, hostPort, options)
}

func buildStaticIPUpstreamServer(scheme string, hostPort string) ([]string, error) {
	host, port := splitUpstreamHostPort(scheme, hostPort)
	normalizedPort, err := normalizeOriginPort(port)
	if err != nil {
		return nil, err
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	if ip == nil {
		return nil, fmt.Errorf("static_ip origin host %s must be a public IP address", host)
	}
	if err := security.ValidatePublicIP(ip); err != nil {
		return nil, err
	}
	return []string{formatUpstreamServer(ip.String(), normalizedPort)}, nil
}

func resolvePublicUpstreamServers(ctx context.Context, scheme string, hostPort string, options RenderOptions) ([]string, error) {
	host, port := splitUpstreamHostPort(scheme, hostPort)
	normalizedPort, err := normalizeOriginPort(port)
	if err != nil {
		return nil, err
	}
	port = normalizedPort
	if err := validatePublicOriginHost(host); err != nil {
		return nil, err
	}
	if ip := net.ParseIP(strings.Trim(host, "[]")); ip != nil {
		if err := security.ValidatePublicIP(ip); err != nil {
			return nil, err
		}
		return []string{formatUpstreamServer(ip.String(), port)}, nil
	}
	lookupIPAddr := options.LookupIPAddr
	if lookupIPAddr == nil {
		lookupIPAddr = net.DefaultResolver.LookupIPAddr
	}
	addresses, err := lookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	if len(addresses) == 0 {
		return nil, fmt.Errorf("origin host %s has no addresses", host)
	}
	servers := make([]string, 0, len(addresses))
	seen := make(map[string]struct{}, len(addresses))
	for _, address := range addresses {
		if err := security.ValidatePublicIP(address.IP); err != nil {
			return nil, fmt.Errorf("origin host %s resolved to unsafe ip: %w", host, err)
		}
		server := formatUpstreamServer(address.IP.String(), port)
		if _, ok := seen[server]; ok {
			continue
		}
		seen[server] = struct{}{}
		servers = append(servers, server)
	}
	return servers, nil
}

func splitUpstreamHostPort(scheme string, hostPort string) (string, string) {
	host, port, err := net.SplitHostPort(hostPort)
	if err == nil {
		return host, port
	}
	parsed := &url.URL{Scheme: scheme, Host: hostPort}
	host = parsed.Hostname()
	port = parsed.Port()
	if port == "" {
		if strings.EqualFold(scheme, "https") {
			port = "443"
		} else {
			port = "80"
		}
	}
	return host, port
}

func formatUpstreamServer(host string, port string) string {
	return net.JoinHostPort(strings.Trim(host, "[]"), port)
}

func buildRouteUpstreamName(route Route) string {
	identity := strings.TrimSpace(route.SiteName)
	if identity == "" {
		identity = route.Domain
	}
	sanitized := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= 'A' && r <= 'Z':
			return r + ('a' - 'A')
		case r >= '0' && r <= '9':
			return r
		default:
			return '_'
		}
	}, identity)
	sanitized = strings.Trim(sanitized, "_")
	if sanitized == "" {
		sanitized = "backend"
	}
	return fmt.Sprintf("backend_%s_%d", sanitized, route.ID)
}

func renderNamedUpstreamBlock(upstreamConfig routeUpstreamConfig) string {
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("upstream %s {\n", upstreamConfig.Name))
	for _, server := range upstreamConfig.Servers {
		resolveParam := ""
		if upstreamConfig.UsesRuntimeDNS && upstreamServerNeedsResolve(server) {
			resolveParam = " resolve"
		}
		builder.WriteString(fmt.Sprintf("    server %s%s max_fails=3 fail_timeout=10s;\n", server, resolveParam))
	}
	builder.WriteString("    keepalive 128;\n}\n\n")
	return builder.String()
}

func upstreamServerNeedsResolve(server string) bool {
	host, _, err := net.SplitHostPort(server)
	if err != nil {
		host = server
	}
	return net.ParseIP(strings.Trim(host, "[]")) == nil
}

func resolveUpstreamServerName(origin routeOriginConfig) string {
	parsed, err := url.Parse(origin.URL)
	if err != nil || !strings.EqualFold(parsed.Scheme, "https") {
		return ""
	}
	if strings.TrimSpace(origin.SNI) != "" {
		parsedHost, err := url.Parse("//" + origin.SNI)
		if err == nil && parsedHost.Hostname() != "" {
			return parsedHost.Hostname()
		}
		return origin.SNI
	}
	if strings.TrimSpace(origin.HostHeader) != "" {
		parsedHost, err := url.Parse("//" + origin.HostHeader)
		if err == nil && parsedHost.Hostname() != "" {
			return parsedHost.Hostname()
		}
		return origin.HostHeader
	}
	return parsed.Hostname()
}
