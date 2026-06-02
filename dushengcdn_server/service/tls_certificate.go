package service

import (
	"crypto/tls"
	"dushengcdn/model"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"strings"
)

type TLSCertificateInput struct {
	Name    string `json:"name"`
	CertPEM string `json:"cert_pem"`
	KeyPEM  string `json:"key_pem"`
	Remark  string `json:"remark"`
}

type TLSCertificateContent struct {
	ID              uint   `json:"id"`
	Name            string `json:"name"`
	CertPEM         string `json:"cert_pem"`
	KeyPEM          string `json:"key_pem"`
	Remark          string `json:"remark"`
	Provider        string `json:"provider"`
	AcmeAccountID   uint   `json:"acme_account_id"`
	DnsAccountID    uint   `json:"dns_account_id"`
	DNSProviderMode string `json:"dns_provider_mode"`
	DNSZoneIDRef    *uint  `json:"dns_zone_id_ref"`
	KeyAlgorithm    string `json:"key_algorithm"`
	AutoRenew       bool   `json:"auto_renew"`
	PrimaryDomain   string `json:"primary_domain"`
	OtherDomains    string `json:"other_domains"`
	DisableCNAME    bool   `json:"disable_cname"`
	SkipDNS         bool   `json:"skip_dns"`
	DNS1            string `json:"dns1"`
	DNS2            string `json:"dns2"`
	ApplyStatus     string `json:"apply_status"`
	ApplyMessage    string `json:"apply_message"`
}

type TLSApplyInput struct {
	Name            string `json:"name"`
	Remark          string `json:"remark"`
	AcmeAccountID   uint   `json:"acme_account_id"`
	DnsAccountID    uint   `json:"dns_account_id"`
	DNSProviderMode string `json:"dns_provider_mode"`
	DNSZoneIDRef    *uint  `json:"dns_zone_id_ref"`
	KeyAlgorithm    string `json:"key_algorithm"`
	AutoRenew       bool   `json:"auto_renew"`
	PrimaryDomain   string `json:"primary_domain"`
	OtherDomains    string `json:"other_domains"`
	DisableCNAME    bool   `json:"disable_cname"`
	SkipDNS         bool   `json:"skip_dns"`
	DNS1            string `json:"dns1"`
	DNS2            string `json:"dns2"`
}

var obtainTLSCertificate = ObtainSSL

func SetTLSCertificateObtainFuncForTest(fn func(*model.TLSCertificate) error) func() {
	previous := obtainTLSCertificate
	obtainTLSCertificate = fn
	return func() {
		obtainTLSCertificate = previous
	}
}

func ListTLSCertificates() ([]*model.TLSCertificate, error) {
	return model.ListTLSCertificates()
}

func GetTLSCertificate(id uint) (*model.TLSCertificate, error) {
	return model.GetTLSCertificateByID(id)
}

func GetTLSCertificateContent(id uint) (*TLSCertificateContent, error) {
	certificate, err := model.GetTLSCertificateByID(id)
	if err != nil {
		return nil, err
	}

	return &TLSCertificateContent{
		ID:              certificate.ID,
		Name:            certificate.Name,
		CertPEM:         certificate.CertPEM,
		KeyPEM:          certificate.KeyPEM,
		Remark:          certificate.Remark,
		Provider:        certificate.Provider,
		AcmeAccountID:   certificate.AcmeAccountID,
		DnsAccountID:    certificate.DnsAccountID,
		DNSProviderMode: normalizeTLSCertificateDNSProviderMode(certificate.DNSProviderMode),
		DNSZoneIDRef:    certificate.DNSZoneIDRef,
		KeyAlgorithm:    certificate.KeyAlgorithm,
		AutoRenew:       certificate.AutoRenew,
		PrimaryDomain:   certificate.PrimaryDomain,
		OtherDomains:    certificate.OtherDomains,
		DisableCNAME:    certificate.DisableCNAME,
		SkipDNS:         certificate.SkipDNS,
		DNS1:            certificate.DNS1,
		DNS2:            certificate.DNS2,
		ApplyStatus:     certificate.ApplyStatus,
		ApplyMessage:    certificate.ApplyMessage,
	}, nil
}

func CreateTLSCertificate(input TLSCertificateInput) (*model.TLSCertificate, error) {
	certificate, err := buildTLSCertificate(nil, input)
	if err != nil {
		return nil, err
	}
	if err = certificate.Insert(); err != nil {
		if isUniqueConstraintError(err) {
			return nil, errors.New("certificate name already exists")
		}
		return nil, err
	}
	return certificate, nil
}

func CreateTLSCertificateFromFiles(name string, certFile *multipart.FileHeader, keyFile *multipart.FileHeader, remark string) (*model.TLSCertificate, error) {
	if certFile == nil || keyFile == nil {
		return nil, errors.New("certificate file and key file cannot be empty")
	}
	certContent, err := readMultipartFile(certFile)
	if err != nil {
		return nil, err
	}
	keyContent, err := readMultipartFile(keyFile)
	if err != nil {
		return nil, err
	}
	return CreateTLSCertificate(TLSCertificateInput{
		Name:    name,
		CertPEM: certContent,
		KeyPEM:  keyContent,
		Remark:  remark,
	})
}

func UpdateTLSCertificate(id uint, input TLSCertificateInput) (*model.TLSCertificate, error) {
	existing, err := model.GetTLSCertificateByID(id)
	if err != nil {
		return nil, err
	}

	certificate, err := buildTLSCertificate(existing, input)
	if err != nil {
		return nil, err
	}
	if err = certificate.Update(); err != nil {
		if isUniqueConstraintError(err) {
			return nil, errors.New("certificate name already exists")
		}
		return nil, err
	}
	return certificate, nil
}

func DeleteTLSCertificate(id uint) error {
	routes, err := model.ListProxyRoutes()
	if err != nil {
		return err
	}
	for _, route := range routes {
		if route == nil {
			continue
		}
		if route.CertID != nil && *route.CertID == id {
			return errors.New("certificate is still referenced by proxy routes")
		}
		if strings.TrimSpace(route.CertIDs) == "" {
			continue
		}
		var certIDs []uint
		if err := json.Unmarshal([]byte(route.CertIDs), &certIDs); err != nil {
			return fmt.Errorf("proxy route %d cert_ids payload is invalid: %w", route.ID, err)
		}
		for _, certID := range certIDs {
			if certID == id {
				return errors.New("certificate is still referenced by proxy routes")
			}
		}
		domainCertIDs, err := decodeStoredDomainCertIDs(route.DomainCertIDs, 0)
		if err != nil {
			return fmt.Errorf("proxy route %d domain_cert_ids payload is invalid: %w", route.ID, err)
		}
		for _, certID := range domainCertIDs {
			if certID == id {
				return errors.New("certificate is still referenced by proxy routes")
			}
		}
	}

	certificate, err := model.GetTLSCertificateByID(id)
	if err != nil {
		return err
	}
	if err := certificate.Delete(); err != nil {
		return err
	}
	return ClearManagedDomainCertificateReferences(id)
}

func ApplyTLSCertificate(input TLSApplyInput) (*model.TLSCertificate, error) {
	dnsProviderMode, dnsAccountID, dnsZoneIDRef, err := normalizeTLSCertificateDNSSettings(input)
	if err != nil {
		return nil, err
	}

	cert := &model.TLSCertificate{
		Name:            strings.TrimSpace(input.Name),
		Remark:          strings.TrimSpace(input.Remark),
		Provider:        "acme",
		AcmeAccountID:   input.AcmeAccountID,
		DnsAccountID:    dnsAccountID,
		DNSProviderMode: dnsProviderMode,
		DNSZoneIDRef:    dnsZoneIDRef,
		KeyAlgorithm:    input.KeyAlgorithm,
		AutoRenew:       input.AutoRenew,
		PrimaryDomain:   strings.TrimSpace(input.PrimaryDomain),
		OtherDomains:    strings.TrimSpace(input.OtherDomains),
		DisableCNAME:    input.DisableCNAME,
		SkipDNS:         input.SkipDNS,
		DNS1:            strings.TrimSpace(input.DNS1),
		DNS2:            strings.TrimSpace(input.DNS2),
		ApplyStatus:     "applying",
		CertPEM:         " ", // Temporary empty value, since gorm may prevent empty insert
		KeyPEM:          " ", // Temporary empty value
	}

	if cert.Name == "" {
		return nil, errors.New("certificate name cannot be empty")
	}

	if err := cert.Insert(); err != nil {
		if isUniqueConstraintError(err) {
			return nil, errors.New("certificate name already exists")
		}
		return nil, err
	}

	// Async obtain SSL
	go func(c *model.TLSCertificate) {
		_ = obtainTLSCertificate(c)
	}(cert)

	return cert, nil
}

func UpdateAcmeCertificate(id uint, input TLSApplyInput) (*model.TLSCertificate, error) {
	cert, err := model.GetTLSCertificateByID(id)
	if err != nil {
		return nil, err
	}
	if cert.Provider != "acme" {
		return nil, errors.New("only acme certificates can be updated via this endpoint")
	}

	cert.Name = strings.TrimSpace(input.Name)
	if cert.Name == "" {
		return nil, errors.New("certificate name cannot be empty")
	}
	dnsProviderMode, dnsAccountID, dnsZoneIDRef, err := normalizeTLSCertificateDNSSettings(input)
	if err != nil {
		return nil, err
	}

	cert.Remark = strings.TrimSpace(input.Remark)
	cert.AcmeAccountID = input.AcmeAccountID
	cert.DnsAccountID = dnsAccountID
	cert.DNSProviderMode = dnsProviderMode
	cert.DNSZoneIDRef = dnsZoneIDRef
	cert.KeyAlgorithm = input.KeyAlgorithm
	cert.AutoRenew = input.AutoRenew
	cert.PrimaryDomain = strings.TrimSpace(input.PrimaryDomain)
	cert.OtherDomains = strings.TrimSpace(input.OtherDomains)
	cert.DisableCNAME = input.DisableCNAME
	cert.SkipDNS = input.SkipDNS
	cert.DNS1 = strings.TrimSpace(input.DNS1)
	cert.DNS2 = strings.TrimSpace(input.DNS2)
	cert.ApplyStatus = "applying"

	if err := cert.Update(); err != nil {
		if isUniqueConstraintError(err) {
			return nil, errors.New("certificate name already exists")
		}
		return nil, err
	}

	// Async obtain SSL with updated config
	go func(c *model.TLSCertificate) {
		_ = obtainTLSCertificate(c)
	}(cert)

	return cert, nil
}

func ConvertTLSCertificateToAcme(id uint, input TLSApplyInput) (*model.TLSCertificate, error) {
	cert, err := model.GetTLSCertificateByID(id)
	if err != nil {
		return nil, err
	}
	if cert.Provider != "upload" {
		return nil, errors.New("only uploaded certificates can be converted to acme")
	}
	if cert.ApplyStatus == "applying" {
		return nil, errors.New("certificate is already applying")
	}

	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, errors.New("certificate name cannot be empty")
	}
	dnsProviderMode, dnsAccountID, dnsZoneIDRef, err := normalizeTLSCertificateDNSSettings(input)
	if err != nil {
		return nil, err
	}

	cert.Name = name
	cert.Remark = strings.TrimSpace(input.Remark)
	cert.AcmeAccountID = input.AcmeAccountID
	cert.DnsAccountID = dnsAccountID
	cert.DNSProviderMode = dnsProviderMode
	cert.DNSZoneIDRef = dnsZoneIDRef
	cert.KeyAlgorithm = input.KeyAlgorithm
	cert.AutoRenew = input.AutoRenew
	cert.PrimaryDomain = strings.TrimSpace(input.PrimaryDomain)
	cert.OtherDomains = strings.TrimSpace(input.OtherDomains)
	cert.DisableCNAME = input.DisableCNAME
	cert.SkipDNS = input.SkipDNS
	cert.DNS1 = strings.TrimSpace(input.DNS1)
	cert.DNS2 = strings.TrimSpace(input.DNS2)
	cert.ApplyStatus = "applying"
	cert.ApplyMessage = ""

	if err := cert.Update(); err != nil {
		if isUniqueConstraintError(err) {
			return nil, errors.New("certificate name already exists")
		}
		return nil, err
	}

	go func(c *model.TLSCertificate) {
		if err := obtainTLSCertificate(c); err != nil {
			return
		}

		latest, err := model.GetTLSCertificateByID(c.ID)
		if err != nil {
			return
		}
		latest.Provider = "acme"
		latest.ApplyStatus = "ready"
		latest.ApplyMessage = ""
		if err := latest.Update(); err != nil {
			return
		}
		if err := SyncManagedDomainForCertificate(latest); err != nil {
			latest.ApplyMessage = fmt.Sprintf("证书已签发，但同步域名资产失败：%v", err)
			_ = latest.Update()
		}
	}(cert)

	return cert, nil
}

func RenewTLSCertificate(id uint) (*model.TLSCertificate, error) {
	cert, err := model.GetTLSCertificateByID(id)
	if err != nil {
		return nil, err
	}
	if cert.Provider != "acme" {
		return nil, errors.New("only acme certificates can be renewed")
	}

	// Async obtain SSL
	go func(c *model.TLSCertificate) {
		_ = obtainTLSCertificate(c)
	}(cert)

	cert.ApplyStatus = "applying"
	cert.Update()

	return cert, nil
}

func normalizeTLSCertificateDNSProviderMode(raw string) string {
	return normalizeDNSProviderMode(raw)
}

func normalizeTLSCertificateDNSSettings(input TLSApplyInput) (string, uint, *uint, error) {
	providerMode := normalizeTLSCertificateDNSProviderMode(input.DNSProviderMode)
	domains, err := parseTLSCertificateDomains(input.PrimaryDomain, input.OtherDomains)
	if err != nil {
		return "", 0, nil, err
	}
	switch providerMode {
	case DNSProviderModeAuthoritative:
		if input.DNSZoneIDRef == nil || *input.DNSZoneIDRef == 0 {
			return "", 0, nil, errors.New("本地自建解析验证需要选择托管域名")
		}
		zone, err := model.GetDNSZoneByID(*input.DNSZoneIDRef)
		if err != nil {
			return "", 0, nil, errors.New("选择的托管域名不存在")
		}
		if !zone.Enabled {
			return "", 0, nil, errors.New("选择的托管域名已停用")
		}
		for _, domain := range domains {
			checkDomain := strings.TrimPrefix(domain, "*.")
			if !domainBelongsToZone(checkDomain, zone.Name) {
				return "", 0, nil, fmt.Errorf("证书域名 %s 不属于托管域名 %s", domain, zone.Name)
			}
		}
		zoneID := *input.DNSZoneIDRef
		return providerMode, 0, &zoneID, nil
	default:
		if input.DnsAccountID == 0 {
			return "", 0, nil, errors.New("Cloudflare 验证需要选择解析账号")
		}
		return DNSProviderModeCloudflare, input.DnsAccountID, nil, nil
	}
}

func parseTLSCertificateDomains(primaryDomain string, otherDomains string) ([]string, error) {
	rawDomains := []string{primaryDomain}
	rawDomains = append(rawDomains, strings.FieldsFunc(otherDomains, func(r rune) bool {
		return r == '\n' || r == '\r' || r == '\t' || r == ' ' || r == ',' || r == '，' || r == ';' || r == '；'
	})...)
	domains := make([]string, 0, len(rawDomains))
	seen := make(map[string]struct{}, len(rawDomains))
	for _, raw := range rawDomains {
		domain := normalizeDNSRecordName(raw)
		if domain == "" {
			continue
		}
		if !isValidProxyRouteDomain(domain) {
			return nil, fmt.Errorf("证书域名格式不正确：%s", strings.TrimSpace(raw))
		}
		if _, ok := seen[domain]; ok {
			continue
		}
		seen[domain] = struct{}{}
		domains = append(domains, domain)
	}
	if len(domains) == 0 {
		return nil, errors.New("证书域名不能为空")
	}
	return domains, nil
}

func buildTLSCertificate(existing *model.TLSCertificate, input TLSCertificateInput) (*model.TLSCertificate, error) {
	name := strings.TrimSpace(input.Name)
	certPEM := strings.TrimSpace(input.CertPEM)
	keyPEM := strings.TrimSpace(input.KeyPEM)
	remark := strings.TrimSpace(input.Remark)
	if name == "" {
		return nil, errors.New("certificate name cannot be empty")
	}
	if certPEM == "" || keyPEM == "" {
		return nil, errors.New("certificate content and key content cannot be empty")
	}
	parsed, err := tls.X509KeyPair([]byte(certPEM), []byte(keyPEM))
	if err != nil {
		return nil, fmt.Errorf("certificate or key format is invalid: %w", err)
	}
	if len(parsed.Certificate) == 0 {
		return nil, errors.New("certificate content is invalid")
	}
	leaf, err := parseLeafCertificate(certPEM)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		existing = &model.TLSCertificate{}
	}
	existing.Name = name
	existing.CertPEM = certPEM
	existing.KeyPEM = keyPEM
	existing.NotBefore = leaf.NotBefore
	existing.NotAfter = leaf.NotAfter
	existing.Remark = remark
	return existing, nil
}
