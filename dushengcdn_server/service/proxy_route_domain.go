package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"dushengcdn/model"

	"gorm.io/gorm"
)

func normalizeProxyRouteSiteNameInput(route *model.ProxyRoute, raw string, primaryDomain string) string {
	siteName := strings.TrimSpace(raw)
	if siteName != "" {
		return siteName
	}
	if route != nil && strings.TrimSpace(route.SiteName) != "" {
		return strings.TrimSpace(route.SiteName)
	}
	return primaryDomain
}

func normalizeProxyRouteDomainValue(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func normalizeProxyRouteDomainsInput(route *model.ProxyRoute, rawDomain string, rawDomains []string) ([]string, error) {
	if len(rawDomains) > 0 {
		domains, err := normalizeProxyRouteDomains(rawDomains)
		if err != nil {
			return nil, err
		}
		domain := normalizeProxyRouteDomainValue(rawDomain)
		if domain != "" && domain != domains[0] {
			return nil, errors.New("domain must match domains[0]")
		}
		return domains, nil
	}

	if route != nil {
		existingDomains, err := decodeStoredDomains(route.Domains, route.Domain)
		if err == nil && len(existingDomains) > 0 {
			domain := normalizeProxyRouteDomainValue(rawDomain)
			if domain == "" || domain == existingDomains[0] {
				return existingDomains, nil
			}
		}
	}

	return normalizeProxyRouteDomains([]string{rawDomain})
}

func normalizeProxyRouteDomains(rawDomains []string) ([]string, error) {
	normalized := make([]string, 0, len(rawDomains))
	seen := make(map[string]struct{}, len(rawDomains))
	for _, rawDomain := range rawDomains {
		domain := normalizeProxyRouteDomainValue(rawDomain)
		if domain == "" {
			continue
		}
		if !isValidProxyRouteDomain(domain) {
			return nil, errors.New("domain format is invalid")
		}
		if _, ok := seen[domain]; ok {
			continue
		}
		seen[domain] = struct{}{}
		normalized = append(normalized, domain)
	}
	if len(normalized) == 0 {
		return nil, errors.New("at least one domain is required")
	}
	return normalized, nil
}

func isValidProxyRouteDomain(domain string) bool {
	if domain == "" || len(domain) > 253 {
		return false
	}
	if strings.ContainsAny(domain, " \t\r\n;{}\"'`$:\\/") || strings.Contains(domain, "://") {
		return false
	}
	if strings.HasSuffix(domain, ".") {
		domain = strings.TrimSuffix(domain, ".")
	}
	if strings.HasPrefix(domain, "*.") {
		domain = strings.TrimPrefix(domain, "*.")
		if domain == "" {
			return false
		}
	} else if strings.Contains(domain, "*") {
		return false
	}
	labels := strings.Split(domain, ".")
	if len(labels) < 2 {
		return false
	}
	for _, label := range labels {
		if !proxyRouteDomainLabelPattern.MatchString(label) {
			return false
		}
	}
	return true
}

func validateProxyRouteSiteName(siteName string) error {
	if strings.TrimSpace(siteName) == "" {
		return errors.New("site_name cannot be empty")
	}
	return nil
}

func validateProxyRouteIdentityUniqueness(route *model.ProxyRoute, siteName string, domains []string) error {
	currentID := uint(0)
	if route != nil {
		currentID = route.ID
	}

	for _, domain := range domains {
		binding, err := model.GetProxySiteDomainByDomain(domain)
		if err == nil && binding != nil && binding.ProxyRouteID != currentID {
			return fmt.Errorf("domain %s already exists", domain)
		}
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
	}

	routes, err := model.ListProxyRouteIdentityCandidates(siteName, domains)
	if err != nil {
		return err
	}

	for _, item := range routes {
		if item == nil || item.ID == currentID {
			continue
		}
		existingSiteName := normalizeProxyRouteSiteNameInput(item, item.SiteName, item.Domain)
		if existingSiteName == siteName {
			return errors.New("site_name already exists")
		}

		existingDomains, err := decodeStoredDomains(item.Domains, item.Domain)
		if err != nil {
			return fmt.Errorf("existing route %d domains are invalid: %w", item.ID, err)
		}
		existingSet := make(map[string]struct{}, len(existingDomains))
		for _, existingDomain := range existingDomains {
			existingSet[existingDomain] = struct{}{}
		}
		for _, domain := range domains {
			if _, ok := existingSet[domain]; ok {
				return fmt.Errorf("domain %s already exists", domain)
			}
		}
	}

	return nil
}

func decodeStoredDomains(raw string, fallbackDomain string) ([]string, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return normalizeProxyRouteDomains([]string{fallbackDomain})
	}
	var domains []string
	if err := json.Unmarshal([]byte(text), &domains); err != nil {
		return nil, errors.New("domains payload is invalid")
	}
	return normalizeProxyRouteDomains(domains)
}
