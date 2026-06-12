package openresty

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
	"sync"
)

func CertFileName(id uint) string {
	return fmt.Sprintf("%d.crt", id)
}

func KeyFileName(id uint) string {
	return fmt.Sprintf("%d.key", id)
}

func NormalizePEM(content string) string {
	return strings.TrimSpace(content) + "\n"
}

func validateCertificateCoverage(certificate TLSCertificate, domains []string) error {
	if certificate.ID == 0 {
		return errors.New("certificate is nil")
	}
	leaf, err := parseCachedLeafCertificate(certificate.CertPEM)
	if err != nil {
		return err
	}
	for _, domain := range domains {
		if err := leaf.VerifyHostname(domain); err != nil {
			return fmt.Errorf("certificate does not cover domain %s", domain)
		}
	}
	return nil
}

func ValidateCertificateCoverage(certificate TLSCertificate, domains []string) error {
	return validateCertificateCoverage(certificate, domains)
}

var (
	leafCertificateParseCache sync.Map
	leafCertificateParser     = parseLeafCertificate
)

type leafCertificateCacheEntry struct {
	once        sync.Once
	certPEM     string
	certificate *x509.Certificate
	err         error
}

func parseCachedLeafCertificate(certPEM string) (*x509.Certificate, error) {
	cacheKey := leafCertificateCacheKey(certPEM)
	value, _ := leafCertificateParseCache.LoadOrStore(cacheKey, &leafCertificateCacheEntry{certPEM: certPEM})
	entry := value.(*leafCertificateCacheEntry)
	entry.once.Do(func() {
		entry.certificate, entry.err = leafCertificateParser(entry.certPEM)
	})
	return entry.certificate, entry.err
}

func leafCertificateCacheKey(certPEM string) string {
	sum := sha256.Sum256([]byte(certPEM))
	return hex.EncodeToString(sum[:])
}

func parseLeafCertificate(certPEM string) (*x509.Certificate, error) {
	certPEMBlock, _ := pem.Decode([]byte(certPEM))
	if certPEMBlock == nil {
		return nil, errors.New("证书 PEM 内容不合法")
	}
	leaf, err := x509.ParseCertificate(certPEMBlock.Bytes)
	if err != nil {
		return nil, err
	}
	return leaf, nil
}
