package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

type GSLBSourceContext struct {
	IP       string
	Country  string
	Operator string
	ASN      uint32
}

type GSLBSourceResolver interface {
	Resolve(ctx context.Context, sourceIP string, provider ProxyRouteGSLBSourceIPProvider) (GSLBSourceContext, error)
}

type HTTPGSLBSourceResolver struct {
	Client *http.Client
}

func (resolver HTTPGSLBSourceResolver) Resolve(ctx context.Context, sourceIP string, provider ProxyRouteGSLBSourceIPProvider) (GSLBSourceContext, error) {
	ip := strings.TrimSpace(sourceIP)
	if net.ParseIP(ip) == nil {
		return GSLBSourceContext{}, errors.New("source IP format is invalid")
	}
	if strings.ToLower(strings.TrimSpace(provider.Provider)) != gslbSourceProviderHTTP {
		return GSLBSourceContext{IP: ip}, nil
	}
	apiURL := strings.TrimSpace(provider.APIURL)
	if apiURL == "" {
		return GSLBSourceContext{}, errors.New("source IP provider api_url is empty")
	}
	if strings.Contains(apiURL, "{ip}") {
		apiURL = strings.ReplaceAll(apiURL, "{ip}", ip)
	} else {
		separator := "?"
		if strings.Contains(apiURL, "?") {
			separator = "&"
		}
		apiURL += separator + "ip=" + ip
	}

	client := resolver.Client
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return GSLBSourceContext{}, err
	}
	if token := strings.TrimSpace(provider.APIToken); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return GSLBSourceContext{}, fmt.Errorf("source IP provider request failed: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return GSLBSourceContext{}, fmt.Errorf("read source IP provider response failed: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return GSLBSourceContext{}, fmt.Errorf("source IP provider returned %s", resp.Status)
	}

	var payload struct {
		Country     string `json:"country"`
		CountryCode string `json:"country_code"`
		Code        string `json:"code"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return GSLBSourceContext{}, fmt.Errorf("parse source IP provider response failed: %w", err)
	}
	country := strings.ToUpper(strings.TrimSpace(firstNonEmptyCloudflareCredential(payload.CountryCode, payload.Country, payload.Code)))
	if len(country) > 2 {
		country = country[:2]
	}
	return GSLBSourceContext{
		IP:      ip,
		Country: country,
	}, nil
}
