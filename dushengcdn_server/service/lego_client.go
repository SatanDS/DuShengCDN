package service

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"dushengcdn/model"

	"github.com/go-acme/lego/v4/acme"
	"github.com/go-acme/lego/v4/certcrypto"
	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/challenge"
	"github.com/go-acme/lego/v4/challenge/dns01"
	"github.com/go-acme/lego/v4/lego"
	"github.com/go-acme/lego/v4/providers/dns/cloudflare"
	"github.com/go-acme/lego/v4/registration"
)

type AcmeUser struct {
	Email        string
	Registration *registration.Resource
	key          crypto.PrivateKey
}

func (u *AcmeUser) GetEmail() string {
	return u.Email
}

func (u *AcmeUser) GetRegistration() *registration.Resource {
	return u.Registration
}

func (u *AcmeUser) GetPrivateKey() crypto.PrivateKey {
	return u.key
}

func parsePrivateKey(pemData string) (crypto.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemData))
	if block == nil {
		return nil, errors.New("failed to parse PEM block containing the key")
	}

	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	if key, err := x509.ParseECPrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	return nil, errors.New("failed to parse private key")
}

func encodePrivateKey(key crypto.PrivateKey) (string, error) {
	var pemBlock *pem.Block
	switch k := key.(type) {
	case *rsa.PrivateKey:
		pemBlock = &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(k)}
	case *ecdsa.PrivateKey:
		b, err := x509.MarshalECPrivateKey(k)
		if err != nil {
			return "", err
		}
		pemBlock = &pem.Block{Type: "EC PRIVATE KEY", Bytes: b}
	default:
		return "", errors.New("unsupported key type")
	}
	return string(pem.EncodeToMemory(pemBlock)), nil
}

func GetOrCreateLegoClient(account *model.AcmeAccount, keyAlgorithm string) (*lego.Client, *AcmeUser, error) {
	var privateKey crypto.PrivateKey
	var err error

	if account.PrivateKey == "" {
		privateKey, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return nil, nil, err
		}
		pemStr, err := encodePrivateKey(privateKey)
		if err != nil {
			return nil, nil, err
		}
		account.PrivateKey = pemStr
		// Don't save it to DB yet, wait for successful registration.
	} else {
		privateKey, err = parsePrivateKey(account.PrivateKey)
		if err != nil {
			return nil, nil, err
		}
	}

	user := &AcmeUser{
		Email: account.Email,
		key:   privateKey,
	}

	if account.URL != "" {
		user.Registration = &registration.Resource{
			Body: acme.Account{
				Status:  "valid",
				Contact: []string{"mailto:" + account.Email},
			},
			URI: account.URL,
		}
	}

	config := lego.NewConfig(user)
	// Use Let's Encrypt production environment by default
	config.CADirURL = lego.LEDirectoryProduction

	switch keyAlgorithm {
	case "RSA2048":
		config.Certificate.KeyType = certcrypto.RSA2048
	case "RSA4096":
		config.Certificate.KeyType = certcrypto.RSA4096
	case "EC256":
		config.Certificate.KeyType = certcrypto.EC256
	case "EC384":
		config.Certificate.KeyType = certcrypto.EC384
	default:
		config.Certificate.KeyType = certcrypto.RSA2048
	}

	client, err := lego.NewClient(config)
	if err != nil {
		return nil, nil, err
	}

	if account.URL == "" {
		reg, err := client.Registration.Register(registration.RegisterOptions{TermsOfServiceAgreed: true})
		if err != nil {
			return nil, nil, err
		}
		user.Registration = reg
		account.URL = reg.URI
		if account.ID == 0 {
			err = model.DB.Create(account).Error
		} else {
			err = model.DB.Save(account).Error
		}
		if err != nil {
			return nil, nil, err
		}
	}

	return client, user, nil
}

func SetupDNSProvider(client *lego.Client, dnsAccount *model.DnsAccount, dns1, dns2 string, disableCNAME, skipDNS bool) error {
	var provider challengeProvider

	switch dnsAccount.Type {
	case "cloudflare":
		token := parseCloudflareAPIToken(dnsAccount.Authorization)
		if token == "" {
			return errors.New("Cloudflare DNS account is missing api_token")
		}

		config := cloudflare.NewDefaultConfig()
		config.AuthToken = token

		p, err := cloudflare.NewDNSProviderConfig(config)
		if err != nil {
			return err
		}
		provider = p
	default:
		return fmt.Errorf("unsupported DNS provider: %s", dnsAccount.Type)
	}

	return setDNS01Provider(client, provider, dns1, dns2, disableCNAME, skipDNS)
}

func SetupAuthoritativeDNSProvider(client *lego.Client, zoneID uint, dns1, dns2 string, disableCNAME, skipDNS bool) error {
	if zoneID == 0 {
		return errors.New("本地自建解析验证需要选择托管域名")
	}
	if _, err := model.GetDNSZoneByID(zoneID); err != nil {
		return errors.New("选择的托管域名不存在")
	}
	return setDNS01Provider(client, newAuthoritativeDNSChallengeProvider(zoneID), dns1, dns2, disableCNAME, skipDNS)
}

func setDNS01Provider(client *lego.Client, provider challengeProvider, dns1, dns2 string, disableCNAME, skipDNS bool) error {
	// We can use custom DNS servers to verify challenges if provided
	var resolvers []string
	if dns1 != "" {
		resolvers = append(resolvers, dns1+":53")
	}
	if dns2 != "" {
		resolvers = append(resolvers, dns2+":53")
	}

	var opts []dns01.ChallengeOption

	if len(resolvers) > 0 {
		opts = append(opts, dns01.AddRecursiveNameservers(resolvers))
	}

	if disableCNAME {
		opts = append(opts, dns01.DisableCompletePropagationRequirement())
	}

	if skipDNS {
		opts = append(opts, dns01.WrapPreCheck(func(domain, fqdn, value string, check dns01.PreCheckFunc) (bool, error) {
			// If we skip the local DNS check entirely, we might trigger Let's Encrypt to verify
			// BEFORE Cloudflare's edge servers have actually synced the TXT record (which takes 5-15 seconds).
			// So we add a safe 20-second artificial delay before forcing the true return.
			time.Sleep(20 * time.Second)
			return true, nil
		}))
	}

	return client.Challenge.SetDNS01Provider(provider, opts...)
}

// challengeProvider interface helps to bypass the strict type definition of SetDNS01Provider
type challengeProvider interface {
	Present(domain, token, keyAuth string) error
	CleanUp(domain, token, keyAuth string) error
}

type authoritativeDNSChallengeProvider struct {
	zoneID  uint
	mu      sync.Mutex
	records map[string]uint
}

func newAuthoritativeDNSChallengeProvider(zoneID uint) *authoritativeDNSChallengeProvider {
	return &authoritativeDNSChallengeProvider{
		zoneID:  zoneID,
		records: make(map[string]uint),
	}
}

func (provider *authoritativeDNSChallengeProvider) Present(domain, token, keyAuth string) error {
	info := dns01.GetChallengeInfo(domain, keyAuth)
	name := dns01.UnFqdn(info.EffectiveFQDN)
	record, err := CreateAuthoritativeDNSRecord(provider.zoneID, DNSRecordInput{
		Name:    name,
		Type:    "TXT",
		Value:   info.Value,
		TTL:     120,
		Enabled: boolPtr(true),
	})
	if err != nil {
		return err
	}
	provider.mu.Lock()
	provider.records[token] = record.ID
	provider.mu.Unlock()
	return nil
}

func (provider *authoritativeDNSChallengeProvider) CleanUp(domain, token, keyAuth string) error {
	info := dns01.GetChallengeInfo(domain, keyAuth)
	name := normalizeDNSRecordName(dns01.UnFqdn(info.EffectiveFQDN))
	value := strings.TrimSpace(info.Value)

	provider.mu.Lock()
	recordID := provider.records[token]
	delete(provider.records, token)
	provider.mu.Unlock()

	if recordID != 0 {
		err := DeleteAuthoritativeDNSRecord(recordID)
		if err == nil {
			return nil
		}
	}

	records, err := model.ListDNSRecordsByZoneID(provider.zoneID)
	if err != nil {
		return err
	}
	for _, record := range records {
		if record == nil {
			continue
		}
		if strings.EqualFold(record.Type, "TXT") &&
			normalizeDNSRecordName(record.Name) == name &&
			strings.TrimSpace(record.Value) == value {
			return DeleteAuthoritativeDNSRecord(record.ID)
		}
	}
	return nil
}

func (provider *authoritativeDNSChallengeProvider) Timeout() (timeout, interval time.Duration) {
	return 60 * time.Second, 2 * time.Second
}

var _ challenge.ProviderTimeout = (*authoritativeDNSChallengeProvider)(nil)

func boolPtr(value bool) *bool {
	return &value
}

func ObtainSSL(cert *model.TLSCertificate) error {
	cert.ApplyStatus = "applying"
	model.DB.Save(cert)

	acmeAccount, err := model.GetAcmeAccountByID(cert.AcmeAccountID)
	if err != nil {
		// Fallback to default ACME account if the specified one is not found (e.g. ID 0 during testing)
		acmeAccount, err = model.GetDefaultAcmeAccount()
		if err != nil {
			updateCertError(cert, fmt.Sprintf("Failed to get ACME account: %v", err))
			return err
		}
		// Self-heal the certificate
		cert.AcmeAccountID = acmeAccount.ID
		model.DB.Save(cert)
	}

	client, _, err := GetOrCreateLegoClient(acmeAccount, cert.KeyAlgorithm)
	if err != nil {
		updateCertError(cert, fmt.Sprintf("Failed to create ACME client: %v", err))
		return err
	}

	if normalizeTLSCertificateDNSProviderMode(cert.DNSProviderMode) == DNSProviderModeAuthoritative {
		if cert.DNSZoneIDRef == nil || *cert.DNSZoneIDRef == 0 {
			err = errors.New("本地自建解析验证需要选择托管域名")
			updateCertError(cert, err.Error())
			return err
		}
		err = SetupAuthoritativeDNSProvider(client, *cert.DNSZoneIDRef, cert.DNS1, cert.DNS2, cert.DisableCNAME, cert.SkipDNS)
		if err != nil {
			updateCertError(cert, fmt.Sprintf("Failed to setup local DNS challenge: %v", err))
			return err
		}
	} else {
		dnsAccount, err := model.GetDnsAccountByID(cert.DnsAccountID)
		if err != nil {
			updateCertError(cert, fmt.Sprintf("Failed to get DNS account: %v", err))
			return err
		}
		err = SetupDNSProvider(client, dnsAccount, cert.DNS1, cert.DNS2, cert.DisableCNAME, cert.SkipDNS)
		if err != nil {
			updateCertError(cert, fmt.Sprintf("Failed to setup DNS provider: %v", err))
			return err
		}
	}

	domains := []string{cert.PrimaryDomain}
	if cert.OtherDomains != "" {
		for _, d := range strings.Split(cert.OtherDomains, "\n") {
			d = strings.TrimSpace(d)
			if d != "" {
				domains = append(domains, d)
			}
		}
	}

	request := certificate.ObtainRequest{
		Domains: domains,
		Bundle:  true,
	}

	certificates, err := client.Certificate.Obtain(request)
	if err != nil {
		updateCertError(cert, fmt.Sprintf("Failed to obtain certificate: %v", err))
		return err
	}

	cert.CertPEM = string(certificates.Certificate)
	cert.KeyPEM = string(certificates.PrivateKey)

	// Parse validity dates
	certBlock, _ := pem.Decode(certificates.Certificate)
	if certBlock != nil {
		parsedCert, err := x509.ParseCertificate(certBlock.Bytes)
		if err == nil {
			cert.NotBefore = parsedCert.NotBefore
			cert.NotAfter = parsedCert.NotAfter
		}
	}

	cert.ApplyStatus = "ready"
	cert.ApplyMessage = ""
	return model.DB.Save(cert).Error
}

func updateCertError(cert *model.TLSCertificate, message string) {
	cert.ApplyStatus = "error"
	cert.ApplyMessage = message
	model.DB.Save(cert)
}
