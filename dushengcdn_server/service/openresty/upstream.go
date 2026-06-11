package openresty

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"

	"dushengcdn/utils/security"
)

func buildRouteUpstreamConfig(route Route, upstreams []string, options RenderOptions) (routeUpstreamConfig, error) {
	if len(upstreams) == 0 {
		return routeUpstreamConfig{}, nil
	}
	if len(upstreams) == 1 {
		parsed, err := url.Parse(strings.TrimSpace(upstreams[0]))
		if err != nil || parsed.Host == "" || parsed.Scheme == "" {
			return routeUpstreamConfig{}, nil
		}
		servers, err := resolvePublicUpstreamServers(context.Background(), parsed.Scheme, parsed.Host, options)
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
	for _, upstream := range upstreams {
		parsed, err := url.Parse(strings.TrimSpace(upstream))
		if err != nil || parsed.Host == "" || parsed.Scheme == "" {
			return routeUpstreamConfig{}, nil
		}
		if strings.TrimSpace(parsed.EscapedPath()) != "" && strings.TrimSpace(parsed.EscapedPath()) != "/" {
			return routeUpstreamConfig{}, nil
		}
		if parsed.RawQuery != "" {
			return routeUpstreamConfig{}, nil
		}
		if scheme == "" {
			scheme = parsed.Scheme
		} else if scheme != parsed.Scheme {
			return routeUpstreamConfig{}, nil
		}
		resolvedServers, err := resolvePublicUpstreamServers(context.Background(), parsed.Scheme, parsed.Host, options)
		if err != nil {
			return routeUpstreamConfig{}, err
		}
		servers = append(servers, resolvedServers...)
	}
	return routeUpstreamConfig{
		Name:              buildRouteUpstreamName(route),
		Scheme:            scheme,
		Servers:           servers,
		UsesNamedUpstream: true,
	}, nil
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
		builder.WriteString(fmt.Sprintf("    server %s max_fails=3 fail_timeout=10s;\n", server))
	}
	builder.WriteString("    keepalive 128;\n}\n\n")
	return builder.String()
}

func resolveUpstreamServerName(originURL string, originHost string) string {
	parsed, err := url.Parse(originURL)
	if err != nil || !strings.EqualFold(parsed.Scheme, "https") {
		return ""
	}
	if strings.TrimSpace(originHost) != "" {
		parsedHost, err := url.Parse("//" + originHost)
		if err == nil && parsedHost.Hostname() != "" {
			return parsedHost.Hostname()
		}
		return originHost
	}
	return parsed.Hostname()
}
