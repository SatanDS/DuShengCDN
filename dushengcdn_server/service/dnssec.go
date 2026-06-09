package service

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"strconv"
	"strings"
	"time"

	"dushengcdn/model"

	"github.com/miekg/dns"
	"gorm.io/gorm"
)

const (
	dnssecDenialModeNSEC      = "nsec"
	dnssecDenialModeNSEC3     = "nsec3"
	dnssecKeyRoleKSK          = "ksk"
	dnssecKeyRoleZSK          = "zsk"
	dnssecKeyStatusActive     = "active"
	dnssecKeyStatusRetired    = "retired"
	defaultDNSSECAlgorithm    = dns.ECDSAP256SHA256
	defaultDNSSECValiditySecs = 7 * 24 * 3600
)

type DNSSECEnableInput struct {
	DenialMode      string `json:"denial_mode"`
	NSEC3Iterations int    `json:"nsec3_iterations"`
}

type DNSSECView struct {
	ZoneID                   uint            `json:"zone_id"`
	Enabled                  bool            `json:"enabled"`
	DenialMode               string          `json:"denial_mode"`
	NSEC3Salt                string          `json:"nsec3_salt,omitempty"`
	NSEC3Iterations          int             `json:"nsec3_iterations"`
	SignatureValiditySeconds int             `json:"signature_validity_seconds"`
	Algorithm                uint8           `json:"algorithm"`
	AlgorithmName            string          `json:"algorithm_name"`
	Keys                     []DNSSECKeyView `json:"keys"`
	KeyEncryptionConfigured  bool            `json:"key_encryption_configured"`
	DSRecords                []DNSSECDSView  `json:"ds_records"`
}

type DNSSECKeyView struct {
	ID             uint       `json:"id"`
	ZoneID         uint       `json:"zone_id"`
	Role           string     `json:"role"`
	Flags          uint16     `json:"flags"`
	Algorithm      uint8      `json:"algorithm"`
	AlgorithmName  string     `json:"algorithm_name"`
	PublicKey      string     `json:"public_key"`
	KeyTag         uint16     `json:"key_tag"`
	DSDigestSHA256 string     `json:"ds_digest_sha256"`
	Status         string     `json:"status"`
	ActivatedAt    *time.Time `json:"activated_at,omitempty"`
	RetiredAt      *time.Time `json:"retired_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type DNSSECDSView struct {
	KeyTag     uint16 `json:"key_tag"`
	Algorithm  uint8  `json:"algorithm"`
	DigestType uint8  `json:"digest_type"`
	Digest     string `json:"digest"`
	Record     string `json:"record"`
}

func GetAuthoritativeDNSSEC(zoneID uint) (*DNSSECView, error) {
	if err := EnsureCommercialFeatureEnabled(CommercialFeatureAuthoritativeDNS); err != nil {
		return nil, err
	}
	zone, err := model.GetDNSZoneByID(zoneID)
	if err != nil {
		return nil, err
	}
	return buildDNSSECView(zone)
}

func EnableAuthoritativeDNSSEC(zoneID uint, input DNSSECEnableInput) (*DNSSECView, error) {
	if err := EnsureCommercialFeatureEnabled(CommercialFeatureAuthoritativeDNS); err != nil {
		return nil, err
	}
	if _, err := dnssecEncryptionKeyFromEnv(); err != nil {
		return nil, err
	}
	zone, err := model.GetDNSZoneByID(zoneID)
	if err != nil {
		return nil, err
	}
	zone.DNSSECDenialMode = normalizeDNSSECDenialMode(input.DenialMode)
	zone.DNSSECNSEC3Iterations = normalizeDNSSECNSEC3Iterations(input.NSEC3Iterations)
	if zone.DNSSECSignatureValidity <= 0 {
		zone.DNSSECSignatureValidity = defaultDNSSECValiditySecs
	}
	if zone.DNSSECDenialMode == dnssecDenialModeNSEC3 && strings.TrimSpace(zone.DNSSECNSEC3Salt) == "" {
		salt, err := randomDNSSECNSEC3Salt()
		if err != nil {
			return nil, err
		}
		zone.DNSSECNSEC3Salt = salt
	}
	if zone.DNSSECDenialMode != dnssecDenialModeNSEC3 {
		zone.DNSSECNSEC3Salt = ""
		zone.DNSSECNSEC3Iterations = 0
	}
	now := time.Now().UTC()
	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		if err := ensureDNSSECActiveKeyWithDB(tx, zone.ID, dnssecKeyRoleKSK, now); err != nil {
			return err
		}
		if err := ensureDNSSECActiveKeyWithDB(tx, zone.ID, dnssecKeyRoleZSK, now); err != nil {
			return err
		}
		zone.DNSSECEnabled = true
		zone.Serial = nextDNSZoneSerial(zone.Serial)
		if err := tx.Save(zone).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return buildDNSSECView(zone)
}

func DisableAuthoritativeDNSSEC(zoneID uint) (*DNSSECView, error) {
	if err := EnsureCommercialFeatureEnabled(CommercialFeatureAuthoritativeDNS); err != nil {
		return nil, err
	}
	zone, err := model.GetDNSZoneByID(zoneID)
	if err != nil {
		return nil, err
	}
	zone.DNSSECEnabled = false
	zone.Serial = nextDNSZoneSerial(zone.Serial)
	if err := zone.Update(); err != nil {
		return nil, err
	}
	return buildDNSSECView(zone)
}

func RotateAuthoritativeDNSSECZSK(zoneID uint) (*DNSSECView, error) {
	return rotateAuthoritativeDNSSECKey(zoneID, dnssecKeyRoleZSK)
}

func RotateAuthoritativeDNSSECKSK(zoneID uint) (*DNSSECView, error) {
	return rotateAuthoritativeDNSSECKey(zoneID, dnssecKeyRoleKSK)
}

func ListAuthoritativeDNSSECDS(zoneID uint) ([]DNSSECDSView, error) {
	if err := EnsureCommercialFeatureEnabled(CommercialFeatureAuthoritativeDNS); err != nil {
		return nil, err
	}
	zone, err := model.GetDNSZoneByID(zoneID)
	if err != nil {
		return nil, err
	}
	view, err := buildDNSSECView(zone)
	if err != nil {
		return nil, err
	}
	return view.DSRecords, nil
}

func rotateAuthoritativeDNSSECKey(zoneID uint, role string) (*DNSSECView, error) {
	if _, err := dnssecEncryptionKeyFromEnv(); err != nil {
		return nil, err
	}
	zone, err := model.GetDNSZoneByID(zoneID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		if err := retireDNSSECActiveKeysWithDB(tx, zoneID, role, now); err != nil {
			return err
		}
		if err := createDNSSECKeyWithDB(tx, zone, role, now); err != nil {
			return err
		}
		zone.DNSSECEnabled = true
		if zone.DNSSECSignatureValidity <= 0 {
			zone.DNSSECSignatureValidity = defaultDNSSECValiditySecs
		}
		zone.Serial = nextDNSZoneSerial(zone.Serial)
		return tx.Save(zone).Error
	}); err != nil {
		return nil, err
	}
	return buildDNSSECView(zone)
}

func buildDNSSECView(zone *model.DNSZone) (*DNSSECView, error) {
	if zone == nil {
		return nil, errors.New("DNS zone is nil")
	}
	keys, err := model.ListDNSSECKeysByZoneID(zone.ID)
	if err != nil {
		return nil, err
	}
	view := &DNSSECView{
		ZoneID:                   zone.ID,
		Enabled:                  zone.DNSSECEnabled,
		DenialMode:               normalizeDNSSECDenialMode(zone.DNSSECDenialMode),
		NSEC3Salt:                strings.TrimSpace(zone.DNSSECNSEC3Salt),
		NSEC3Iterations:          normalizeDNSSECNSEC3Iterations(zone.DNSSECNSEC3Iterations),
		SignatureValiditySeconds: normalizeDNSSECSignatureValidity(zone.DNSSECSignatureValidity),
		Algorithm:                defaultDNSSECAlgorithm,
		AlgorithmName:            dns.AlgorithmToString[defaultDNSSECAlgorithm],
		KeyEncryptionConfigured:  dnssecEncryptionConfigured(),
		Keys:                     make([]DNSSECKeyView, 0, len(keys)),
		DSRecords:                make([]DNSSECDSView, 0),
	}
	for _, key := range keys {
		keyView := buildDNSSECKeyView(key)
		view.Keys = append(view.Keys, keyView)
		if keyView.Role == dnssecKeyRoleKSK && keyView.Status == dnssecKeyStatusActive && keyView.DSDigestSHA256 != "" {
			view.DSRecords = append(view.DSRecords, DNSSECDSView{
				KeyTag:     keyView.KeyTag,
				Algorithm:  keyView.Algorithm,
				DigestType: dns.SHA256,
				Digest:     keyView.DSDigestSHA256,
				Record:     dns.Fqdn(zone.Name) + " IN DS " + keyView.dsRDATA(),
			})
		}
	}
	return view, nil
}

func buildDNSSECKeyView(key *model.DNSSECKey) DNSSECKeyView {
	if key == nil {
		return DNSSECKeyView{}
	}
	return DNSSECKeyView{
		ID:             key.ID,
		ZoneID:         key.ZoneID,
		Role:           normalizeDNSSECKeyRole(key.Role),
		Flags:          key.Flags,
		Algorithm:      key.Algorithm,
		AlgorithmName:  dns.AlgorithmToString[key.Algorithm],
		PublicKey:      key.PublicKey,
		KeyTag:         key.KeyTag,
		DSDigestSHA256: strings.ToUpper(strings.TrimSpace(key.DSDigestSHA256)),
		Status:         normalizeDNSSECKeyStatus(key.Status),
		ActivatedAt:    key.ActivatedAt,
		RetiredAt:      key.RetiredAt,
		CreatedAt:      key.CreatedAt,
		UpdatedAt:      key.UpdatedAt,
	}
}

func (view DNSSECKeyView) dsRDATA() string {
	return strings.TrimSpace(strings.Join([]string{
		strconv.FormatUint(uint64(view.KeyTag), 10),
		strconv.FormatUint(uint64(view.Algorithm), 10),
		strconv.FormatUint(uint64(dns.SHA256), 10),
		view.DSDigestSHA256,
	}, " "))
}

func ensureDNSSECActiveKeyWithDB(tx *gorm.DB, zoneID uint, role string, now time.Time) error {
	var count int64
	if err := tx.Model(&model.DNSSECKey{}).
		Where("zone_id = ? AND role = ? AND status = ?", zoneID, role, dnssecKeyStatusActive).
		Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	zone := &model.DNSZone{}
	if err := tx.First(zone, zoneID).Error; err != nil {
		return err
	}
	return createDNSSECKeyWithDB(tx, zone, role, now)
}

func retireDNSSECActiveKeysWithDB(tx *gorm.DB, zoneID uint, role string, now time.Time) error {
	return tx.Model(&model.DNSSECKey{}).
		Where("zone_id = ? AND role = ? AND status = ?", zoneID, role, dnssecKeyStatusActive).
		Updates(map[string]any{
			"status":     dnssecKeyStatusRetired,
			"retired_at": now,
		}).Error
}

func createDNSSECKeyWithDB(tx *gorm.DB, zone *model.DNSZone, role string, now time.Time) error {
	if zone == nil {
		return errors.New("DNS zone is nil")
	}
	role = normalizeDNSSECKeyRole(role)
	dnskey := &dns.DNSKEY{
		Hdr:       dns.RR_Header{Name: dns.Fqdn(zone.Name), Rrtype: dns.TypeDNSKEY, Class: dns.ClassINET, Ttl: uint32(normalizeDNSZoneTTL(zone.DefaultTTL))},
		Flags:     dnssecKeyFlags(role),
		Protocol:  3,
		Algorithm: defaultDNSSECAlgorithm,
	}
	privateKey, err := dnskey.Generate(256)
	if err != nil {
		return err
	}
	privateText := dnskey.PrivateKeyString(privateKey)
	if strings.TrimSpace(privateText) == "" {
		return errors.New("failed to serialize DNSSEC private key")
	}
	encrypted, err := encryptDNSSECPrivateKey(privateText)
	if err != nil {
		return err
	}
	ds := dnskey.ToDS(dns.SHA256)
	key := &model.DNSSECKey{
		ZoneID:              zone.ID,
		Role:                role,
		Flags:               dnskey.Flags,
		Algorithm:           dnskey.Algorithm,
		PublicKey:           dnskey.PublicKey,
		EncryptedPrivateKey: encrypted,
		KeyTag:              dnskey.KeyTag(),
		Status:              dnssecKeyStatusActive,
		ActivatedAt:         &now,
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	if ds != nil {
		key.DSDigestSHA256 = strings.ToUpper(ds.Digest)
	}
	return tx.Create(key).Error
}

func dnssecKeyFlags(role string) uint16 {
	if normalizeDNSSECKeyRole(role) == dnssecKeyRoleKSK {
		return 257
	}
	return 256
}

func normalizeDNSSECDenialMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case dnssecDenialModeNSEC3:
		return dnssecDenialModeNSEC3
	default:
		return dnssecDenialModeNSEC
	}
}

func normalizeDNSSECKeyRole(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case dnssecKeyRoleKSK:
		return dnssecKeyRoleKSK
	default:
		return dnssecKeyRoleZSK
	}
}

func normalizeDNSSECKeyStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case dnssecKeyStatusRetired:
		return dnssecKeyStatusRetired
	default:
		return dnssecKeyStatusActive
	}
}

func normalizeDNSSECNSEC3Iterations(value int) int {
	if value < 0 {
		return 0
	}
	if value > 50 {
		return 50
	}
	return value
}

func normalizeDNSSECSignatureValidity(value int) int {
	if value <= 0 {
		return defaultDNSSECValiditySecs
	}
	if value < 3600 {
		return 3600
	}
	return value
}

func randomDNSSECNSEC3Salt() (string, error) {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return strings.ToUpper(hex.EncodeToString(raw[:])), nil
}

func dnssecEncryptionConfigured() bool {
	return strings.TrimSpace(os.Getenv("DUSHENGCDN_DNSSEC_KEY_ENCRYPTION_KEY")) != ""
}
