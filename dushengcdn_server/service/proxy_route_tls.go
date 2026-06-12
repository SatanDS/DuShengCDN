package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"dushengcdn/model"

	"gorm.io/gorm"
)

func normalizeProxyRouteCertificateIDs(enableHTTPS bool, certID *uint, certIDs []uint) ([]uint, error) {
	if !enableHTTPS {
		return []uint{}, nil
	}

	candidates := make([]uint, 0, len(certIDs)+1)
	if certID != nil && *certID != 0 {
		candidates = append(candidates, *certID)
	}
	candidates = append(candidates, certIDs...)

	normalized := make([]uint, 0, len(candidates))
	seen := make(map[uint]struct{}, len(candidates))
	for _, item := range candidates {
		if item == 0 {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		normalized = append(normalized, item)
	}
	if len(normalized) == 0 {
		return nil, errors.New("must select a certificate when HTTPS is enabled")
	}
	return normalized, nil
}

func normalizeProxyRouteDomainCertificateIDs(
	domains []string,
	enableHTTPS bool,
	rawDomainCertIDs []uint,
	certID *uint,
	certIDs []uint,
) ([]uint, []uint, *uint, bool, error) {
	if !enableHTTPS {
		return []uint{}, []uint{}, nil, true, nil
	}

	if len(rawDomainCertIDs) > 0 {
		if len(rawDomainCertIDs) != len(domains) {
			return nil, nil, nil, false, errors.New("domain_cert_ids must match domains length")
		}

		normalizedDomainCertIDs := make([]uint, len(rawDomainCertIDs))
		uniqueCertIDs := make([]uint, 0, len(rawDomainCertIDs))
		seen := make(map[uint]struct{}, len(rawDomainCertIDs))
		hasAssignedCertificate := false
		for index, item := range rawDomainCertIDs {
			if item == 0 {
				continue
			}
			normalizedDomainCertIDs[index] = item
			hasAssignedCertificate = true
			if _, ok := seen[item]; ok {
				continue
			}
			seen[item] = struct{}{}
			uniqueCertIDs = append(uniqueCertIDs, item)
		}
		if !hasAssignedCertificate {
			return nil, nil, nil, false, errors.New("must select a certificate when HTTPS is enabled")
		}

		primaryCertID := &uniqueCertIDs[0]
		return normalizedDomainCertIDs, uniqueCertIDs, primaryCertID, false, nil
	}

	normalizedCertIDs, err := normalizeProxyRouteCertificateIDs(
		enableHTTPS,
		certID,
		certIDs,
	)
	if err != nil {
		return nil, nil, nil, false, err
	}

	switch {
	case len(normalizedCertIDs) == 0:
		return nil, nil, nil, false, errors.New("must select a certificate when HTTPS is enabled")
	case len(normalizedCertIDs) == 1:
		domainCertIDs := make([]uint, len(domains))
		for index := range domainCertIDs {
			domainCertIDs[index] = normalizedCertIDs[0]
		}
		primaryCertID := &normalizedCertIDs[0]
		return domainCertIDs, normalizedCertIDs, primaryCertID, false, nil
	case len(normalizedCertIDs) == len(domains):
		domainCertIDs := make([]uint, len(normalizedCertIDs))
		copy(domainCertIDs, normalizedCertIDs)
		primaryCertID := &normalizedCertIDs[0]
		return domainCertIDs, normalizedCertIDs, primaryCertID, false, nil
	default:
		domainCertIDs, err := deriveDomainCertIDsFromCertificateSet(
			domains,
			normalizedCertIDs,
		)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, nil, nil, false, errors.New("selected certificate does not exist")
			}
			return nil, nil, nil, false, err
		}
		primaryCertID := &normalizedCertIDs[0]
		return domainCertIDs, normalizedCertIDs, primaryCertID, true, nil
	}
}

func validateProxyRouteDomainCertificateCoverage(
	domains []string,
	domainCertIDs []uint,
) error {
	if len(domainCertIDs) == 0 {
		return nil
	}

	domainsByCertID := make(map[uint][]string)
	certIDs := make([]uint, 0, len(domainCertIDs))
	seenCertIDs := make(map[uint]struct{}, len(domainCertIDs))
	for index, certID := range domainCertIDs {
		if certID == 0 {
			continue
		}
		domainsByCertID[certID] = append(domainsByCertID[certID], domains[index])
		if _, ok := seenCertIDs[certID]; ok {
			continue
		}
		seenCertIDs[certID] = struct{}{}
		certIDs = append(certIDs, certID)
	}

	if len(certIDs) == 0 {
		return nil
	}
	certificates, err := loadTLSCertificates(certIDs)
	if err != nil {
		return errors.New("selected certificate does not exist")
	}
	for _, certificate := range certificates {
		assignedDomains := domainsByCertID[certificate.ID]
		if err := validateCertificateCoverage(certificate, assignedDomains); err != nil {
			return err
		}
	}
	return nil
}

func deriveDomainCertIDsFromCertificateSet(
	domains []string,
	certIDs []uint,
) ([]uint, error) {
	return deriveDomainCertIDsFromCertificateSetWithContext(domains, certIDs, nil)
}

func deriveDomainCertIDsFromCertificateSetWithContext(
	domains []string,
	certIDs []uint,
	context proxyRouteTLSCertificateLoader,
) ([]uint, error) {
	certificates, err := loadTLSCertificatesWithContext(context, certIDs)
	if err != nil {
		return nil, err
	}

	result := make([]uint, len(domains))
	for domainIndex, domain := range domains {
		if domainIndex < len(certificates) &&
			certificates[domainIndex] != nil &&
			validateCertificateCoverage(certificates[domainIndex], []string{domain}) == nil {
			result[domainIndex] = certificates[domainIndex].ID
			continue
		}

		assigned := uint(0)
		for _, certificate := range certificates {
			if certificate != nil &&
				validateCertificateCoverage(certificate, []string{domain}) == nil {
				assigned = certificate.ID
				break
			}
		}
		if assigned == 0 {
			return nil, fmt.Errorf("certificate does not cover domain %s", domain)
		}
		result[domainIndex] = assigned
	}
	return result, nil
}

func decodeStoredDomainCertIDs(raw string, domainCount int) ([]uint, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return []uint{}, nil
	}

	var domainCertIDs []uint
	if err := json.Unmarshal([]byte(text), &domainCertIDs); err != nil {
		return nil, errors.New("domain_cert_ids payload is invalid")
	}
	if len(domainCertIDs) == 0 {
		return []uint{}, nil
	}
	if domainCount > 0 && len(domainCertIDs) != domainCount {
		return nil, errors.New("domain_cert_ids length does not match domains")
	}

	normalized := make([]uint, len(domainCertIDs))
	copy(normalized, domainCertIDs)
	return normalized, nil
}

func resolveProxyRouteDomainCertIDsWithContext(
	route *model.ProxyRoute,
	domains []string,
	certIDs []uint,
	context proxyRouteTLSCertificateLoader,
) ([]uint, error) {
	domainCertIDs, err := decodeStoredDomainCertIDs(route.DomainCertIDs, len(domains))
	if err != nil {
		return nil, err
	}
	if len(domainCertIDs) > 0 || len(certIDs) == 0 {
		return domainCertIDs, nil
	}
	return deriveDomainCertIDsFromCertificateSetWithContext(domains, certIDs, context)
}

func loadTLSCertificatesWithContext(
	context proxyRouteTLSCertificateLoader,
	certIDs []uint,
) ([]*model.TLSCertificate, error) {
	if context == nil {
		return loadTLSCertificates(certIDs)
	}
	return context.loadTLSCertificates(certIDs)
}

func decodeStoredCertIDs(raw string, fallbackCertID *uint) ([]uint, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		if fallbackCertID == nil || *fallbackCertID == 0 {
			return []uint{}, nil
		}
		return []uint{*fallbackCertID}, nil
	}
	var certIDs []uint
	if err := json.Unmarshal([]byte(text), &certIDs); err != nil {
		return nil, errors.New("cert_ids payload is invalid")
	}
	normalized := make([]uint, 0, len(certIDs))
	seen := make(map[uint]struct{}, len(certIDs))
	for _, certID := range certIDs {
		if certID == 0 {
			continue
		}
		if _, ok := seen[certID]; ok {
			continue
		}
		seen[certID] = struct{}{}
		normalized = append(normalized, certID)
	}
	if len(normalized) == 0 && fallbackCertID != nil && *fallbackCertID != 0 {
		return []uint{*fallbackCertID}, nil
	}
	return normalized, nil
}
