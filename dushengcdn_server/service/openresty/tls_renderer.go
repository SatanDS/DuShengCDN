package openresty

import (
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
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
	leaf, err := parseLeafCertificate(certificate.CertPEM)
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

func parseLeafCertificate(certPEM string) (*x509.Certificate, error) {
	certPEMBlock, _ := pem.Decode([]byte(certPEM))
	if certPEMBlock == nil {
		return nil, errors.New("璇佷功 PEM 鍐呭涓嶅悎娉?")
	}
	leaf, err := x509.ParseCertificate(certPEMBlock.Bytes)
	if err != nil {
		return nil, err
	}
	return leaf, nil
}
