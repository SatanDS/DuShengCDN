package security

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

var blockedPublicValidationPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("169.254.169.254/32"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("100.100.100.200/32"),
	netip.MustParsePrefix("::/128"),
	netip.MustParsePrefix("::/96"),
	netip.MustParsePrefix("::1/128"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001:2::/48"),
	netip.MustParsePrefix("2001:20::/28"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
	netip.MustParsePrefix("fc00::/7"),
	netip.MustParsePrefix("fe80::/10"),
	netip.MustParsePrefix("fd00:ec2::254/128"),
}

func ValidatePublicHostname(hostname string) error {
	hostname = strings.Trim(strings.ToLower(strings.TrimSpace(hostname)), "[]")
	if hostname == "" {
		return errors.New("host is empty")
	}
	if strings.Contains(hostname, "%") {
		return fmt.Errorf("host %s is not allowed", hostname)
	}
	if ip := net.ParseIP(hostname); ip != nil {
		return ValidatePublicIP(ip)
	}
	if strings.EqualFold(hostname, "localhost") || strings.HasSuffix(hostname, ".localhost") {
		return fmt.Errorf("host %s is not allowed", hostname)
	}
	return nil
}

func ValidatePublicIP(ip net.IP) error {
	if ip == nil {
		return errors.New("ip is empty")
	}
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return errors.New("ip is invalid")
	}
	addr = addr.Unmap()
	if !addr.IsGlobalUnicast() ||
		addr.IsPrivate() ||
		addr.IsLoopback() ||
		addr.IsLinkLocalUnicast() ||
		addr.IsLinkLocalMulticast() ||
		addr.IsMulticast() ||
		addr.IsUnspecified() {
		return fmt.Errorf("ip %s is not allowed", addr.String())
	}
	for _, prefix := range blockedPublicValidationPrefixes {
		if prefix.Contains(addr) {
			return fmt.Errorf("ip %s is not allowed", addr.String())
		}
	}
	return nil
}

func ValidatePublicHTTPURL(rawURL string, requireHTTPS bool) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return nil, err
	}
	if parsed == nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, errors.New("url must include scheme and host")
	}
	scheme := strings.ToLower(parsed.Scheme)
	if requireHTTPS {
		if scheme != "https" {
			return nil, errors.New("url must use https")
		}
	} else if scheme != "http" && scheme != "https" {
		return nil, errors.New("url must use http or https")
	}
	if parsed.User != nil {
		return nil, errors.New("url must not include userinfo")
	}
	if err := ValidatePublicHostname(parsed.Hostname()); err != nil {
		return nil, err
	}
	return parsed, nil
}

func NewPublicHTTPClient(timeout time.Duration, requireHTTPS bool) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Proxy: nil,
			DialContext: func(ctx context.Context, network string, address string) (net.Conn, error) {
				host, port, err := net.SplitHostPort(address)
				if err != nil {
					return nil, err
				}
				targets, err := resolvePublicDialTargets(ctx, host, port)
				if err != nil {
					return nil, err
				}
				dialer := &net.Dialer{}
				var lastErr error
				for _, target := range targets {
					conn, err := dialer.DialContext(ctx, network, target)
					if err == nil {
						return conn, nil
					}
					lastErr = err
				}
				if lastErr != nil {
					return nil, lastErr
				}
				return nil, fmt.Errorf("host %s has no dialable addresses", host)
			},
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many redirects")
			}
			if _, err := ValidatePublicHTTPURL(req.URL.String(), requireHTTPS); err != nil {
				return err
			}
			return nil
		},
	}
}

func resolvePublicDialTargets(ctx context.Context, host string, port string) ([]string, error) {
	if err := ValidatePublicHostname(host); err != nil {
		return nil, err
	}
	if ip := net.ParseIP(strings.Trim(host, "[]")); ip != nil {
		if err := ValidatePublicIP(ip); err != nil {
			return nil, err
		}
		return []string{net.JoinHostPort(ip.String(), port)}, nil
	}
	addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	if len(addresses) == 0 {
		return nil, fmt.Errorf("host %s has no addresses", host)
	}
	targets := make([]string, 0, len(addresses))
	for _, address := range addresses {
		if err := ValidatePublicIP(address.IP); err != nil {
			return nil, err
		}
		targets = append(targets, net.JoinHostPort(address.IP.String(), port))
	}
	return targets, nil
}
