package openresty

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"
)

func TestRenderRouteConfigCachesLeafCertificateParsingByPEM(t *testing.T) {
	certPEM, keyPEM := generateOpenRestyCertificatePair(t, []string{"a.example.com", "b.example.com"})
	leafCertificateParseCache.Clear()
	previousParser := leafCertificateParser
	parseCalls := 0
	leafCertificateParser = func(certPEM string) (*x509.Certificate, error) {
		parseCalls++
		return previousParser(certPEM)
	}
	t.Cleanup(func() {
		leafCertificateParser = previousParser
		leafCertificateParseCache.Clear()
	})

	certID := uint(100)
	config, supportFiles, err := RenderRouteConfig([]Route{
		{
			ID:                1,
			SiteName:          "a",
			Domain:            "a.example.com",
			Domains:           []string{"a.example.com"},
			OriginURL:         "https://origin.example.net",
			Upstreams:         []string{"https://origin.example.net"},
			EnableHTTPS:       true,
			CertID:            &certID,
			CertIDs:           []uint{certID},
			DomainCertIDs:     []uint{certID},
			RedirectHTTP:      true,
			OriginResolveMode: OriginResolveModePublishResolve,
			Certificates: []TLSCertificate{
				{ID: certID, CertPEM: certPEM, KeyPEM: keyPEM},
			},
		},
		{
			ID:                2,
			SiteName:          "b",
			Domain:            "b.example.com",
			Domains:           []string{"b.example.com"},
			OriginURL:         "https://origin.example.net",
			Upstreams:         []string{"https://origin.example.net"},
			EnableHTTPS:       true,
			CertID:            &certID,
			CertIDs:           []uint{certID},
			DomainCertIDs:     []uint{certID},
			RedirectHTTP:      true,
			OriginResolveMode: OriginResolveModePublishResolve,
			Certificates: []TLSCertificate{
				{ID: certID, CertPEM: certPEM, KeyPEM: keyPEM},
			},
		},
	}, ConfigSnapshot{}, RenderOptions{
		LookupIPAddr: func(_ context.Context, _ string) ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
		},
	})
	if err != nil {
		t.Fatalf("RenderRouteConfig failed: %v", err)
	}
	if parseCalls != 1 {
		t.Fatalf("expected shared PEM to be parsed once, got %d", parseCalls)
	}
	if strings.Count(config, "ssl_certificate __DUSHENGCDN_CERT_DIR__/100.crt") != 2 {
		t.Fatalf("expected both HTTPS servers to reference the certificate, got %s", config)
	}
	if len(supportFiles) != 2 {
		t.Fatalf("expected deduped certificate support files, got %#v", supportFiles)
	}
}

func generateOpenRestyCertificatePair(t *testing.T, dnsNames []string) (string, string) {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}
	template := &x509.Certificate{
		Subject:      pkix.Name{CommonName: dnsNames[0]},
		DNSNames:     dnsNames,
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		SerialNumber: big.NewInt(time.Now().UnixNano()),
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		t.Fatalf("CreateCertificate failed: %v", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})
	return string(certPEM), string(keyPEM)
}
