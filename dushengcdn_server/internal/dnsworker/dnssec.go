package dnsworker

import (
	"crypto"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/miekg/dns"
)

const (
	dnssecKeyRoleKSK       = "ksk"
	dnssecKeyRoleZSK       = "zsk"
	dnssecKeyStatusActive  = "active"
	dnssecDenialModeNSEC3  = "nsec3"
	dnssecDefaultAlgorithm = dns.ECDSAP256SHA256
)

type dnssecRuntimeKey struct {
	role       string
	key        *dns.DNSKEY
	privateKey crypto.Signer
}

func dnssecRequested(request *dns.Msg) bool {
	if request == nil {
		return false
	}
	opt := request.IsEdns0()
	return opt != nil && opt.Do()
}

func signDNSSECResponse(response *dns.Msg, request *dns.Msg, zone *SnapshotZone, index snapshotIndex) {
	if response == nil || zone == nil || !zone.DNSSEC.Enabled || !dnssecRequested(request) {
		return
	}
	keys := loadDNSSECKeys(zone)
	if len(keys) == 0 {
		return
	}
	if response.Rcode == dns.RcodeNameError || isDNSSECNoDataResponse(response) {
		addDNSSECDenialProof(response, request, zone, index)
	}
	signDNSSECRRsets(response, zone, keys)
}

func loadDNSSECKeys(zone *SnapshotZone) []dnssecRuntimeKey {
	result := make([]dnssecRuntimeKey, 0, len(zone.DNSSECKeys))
	for _, item := range zone.DNSSECKeys {
		if strings.ToLower(strings.TrimSpace(item.Status)) != dnssecKeyStatusActive {
			continue
		}
		if strings.TrimSpace(item.EncryptedPrivateKey) == "" || strings.TrimSpace(item.PublicKey) == "" {
			continue
		}
		dnskey := &dns.DNSKEY{
			Hdr:       dns.RR_Header{Name: dnsName(zone.Name), Rrtype: dns.TypeDNSKEY, Class: dns.ClassINET, Ttl: uint32(normalizeStaticTTL(zone.DefaultTTL, DefaultZoneTTL))},
			Flags:     item.Flags,
			Protocol:  3,
			Algorithm: item.Algorithm,
			PublicKey: strings.TrimSpace(item.PublicKey),
		}
		privateText, err := decryptDNSSECPrivateKey(item.EncryptedPrivateKey)
		if err != nil {
			continue
		}
		privateKey, err := dnskey.NewPrivateKey(privateText)
		if err != nil {
			continue
		}
		signer, ok := privateKey.(crypto.Signer)
		if !ok {
			continue
		}
		result = append(result, dnssecRuntimeKey{
			role:       strings.ToLower(strings.TrimSpace(item.Role)),
			key:        dnskey,
			privateKey: signer,
		})
	}
	return result
}

func signDNSSECRRsets(response *dns.Msg, zone *SnapshotZone, keys []dnssecRuntimeKey) {
	zsk := firstDNSSECKey(keys, dnssecKeyRoleZSK)
	if zsk.key == nil {
		zsk = firstDNSSECKey(keys, dnssecKeyRoleKSK)
	}
	ksk := firstDNSSECKey(keys, dnssecKeyRoleKSK)
	if ksk.key == nil {
		ksk = zsk
	}
	response.Answer = append(response.Answer, signRRsets(response.Answer, zone, zsk, ksk)...)
	response.Ns = append(response.Ns, signRRsets(response.Ns, zone, zsk, ksk)...)
}

func firstDNSSECKey(keys []dnssecRuntimeKey, role string) dnssecRuntimeKey {
	role = strings.ToLower(strings.TrimSpace(role))
	for _, key := range keys {
		if key.key != nil && key.privateKey != nil && key.role == role {
			return key
		}
	}
	return dnssecRuntimeKey{}
}

func signRRsets(records []dns.RR, zone *SnapshotZone, zsk dnssecRuntimeKey, ksk dnssecRuntimeKey) []dns.RR {
	groups := make(map[string][]dns.RR)
	order := make([]string, 0)
	for _, rr := range records {
		if rr == nil {
			continue
		}
		if rr.Header().Rrtype == dns.TypeRRSIG {
			continue
		}
		key := strings.ToLower(rr.Header().Name) + "|" + dns.TypeToString[rr.Header().Rrtype]
		if _, ok := groups[key]; !ok {
			order = append(order, key)
		}
		groups[key] = append(groups[key], rr)
	}
	sort.Strings(order)
	sigs := make([]dns.RR, 0, len(order))
	for _, key := range order {
		rrset := groups[key]
		if len(rrset) == 0 {
			continue
		}
		signer := zsk
		if rrset[0].Header().Rrtype == dns.TypeDNSKEY {
			signer = ksk
		}
		sig, err := signDNSSECRRSet(zone, signer, rrset)
		if err == nil && sig != nil {
			sigs = append(sigs, sig)
		}
	}
	return sigs
}

func signDNSSECRRSet(zone *SnapshotZone, signer dnssecRuntimeKey, rrset []dns.RR) (*dns.RRSIG, error) {
	if zone == nil || signer.key == nil || signer.privateKey == nil || len(rrset) == 0 {
		return nil, errors.New("DNSSEC signer is incomplete")
	}
	now := time.Now().UTC()
	validity := normalizeDNSSECSignatureValidity(zone.DNSSEC.SignatureValiditySeconds)
	sig := &dns.RRSIG{
		Hdr:         dns.RR_Header{Name: rrset[0].Header().Name, Rrtype: dns.TypeRRSIG, Class: dns.ClassINET, Ttl: rrset[0].Header().Ttl},
		TypeCovered: rrset[0].Header().Rrtype,
		Algorithm:   signer.key.Algorithm,
		Labels:      uint8(dns.CountLabel(rrset[0].Header().Name)),
		OrigTtl:     rrset[0].Header().Ttl,
		Expiration:  uint32(now.Add(time.Duration(validity) * time.Second).Unix()),
		Inception:   uint32(now.Add(-time.Hour).Unix()),
		KeyTag:      signer.key.KeyTag(),
		SignerName:  dnsName(zone.Name),
	}
	if err := sig.Sign(signer.privateKey, rrset); err != nil {
		return nil, err
	}
	return sig, nil
}

func dnssecDNSKEYRecords(zone *SnapshotZone) []dns.RR {
	records := make([]dns.RR, 0, len(zone.DNSSECKeys))
	for _, item := range zone.DNSSECKeys {
		if strings.ToLower(strings.TrimSpace(item.Status)) != dnssecKeyStatusActive || strings.TrimSpace(item.PublicKey) == "" {
			continue
		}
		records = append(records, &dns.DNSKEY{
			Hdr:       dns.RR_Header{Name: dnsName(zone.Name), Rrtype: dns.TypeDNSKEY, Class: dns.ClassINET, Ttl: uint32(normalizeStaticTTL(zone.DefaultTTL, DefaultZoneTTL))},
			Flags:     item.Flags,
			Protocol:  3,
			Algorithm: item.Algorithm,
			PublicKey: strings.TrimSpace(item.PublicKey),
		})
	}
	return records
}

func dnssecCoveredRRsets(zone *SnapshotZone, index snapshotIndex, qname string) []dns.RR {
	keys := loadDNSSECKeys(zone)
	if len(keys) == 0 {
		return nil
	}
	rrsets := make([]dns.RR, 0)
	if normalizeDomain(qname) == zone.Name {
		rrsets = append(rrsets, soaRecord(zone))
		rrsets = append(rrsets, nsRecords(zone)...)
		rrsets = append(rrsets, dnssecDNSKEYRecords(zone)...)
		if normalizeDNSSECDenialMode(zone.DNSSEC.DenialMode) == dnssecDenialModeNSEC3 {
			rrsets = append(rrsets, dnssecNSEC3PARAMRecord(zone))
		}
	}
	for key, records := range index.recordsByNameType {
		if key.ZoneID != zone.ID || key.Name != normalizeDomain(qname) {
			continue
		}
		rrsets = append(rrsets, recordsToRR(zone, records)...)
	}
	if len(rrsets) == 0 {
		return nil
	}
	zsk := firstDNSSECKey(keys, dnssecKeyRoleZSK)
	if zsk.key == nil {
		zsk = firstDNSSECKey(keys, dnssecKeyRoleKSK)
	}
	ksk := firstDNSSECKey(keys, dnssecKeyRoleKSK)
	if ksk.key == nil {
		ksk = zsk
	}
	return signRRsets(rrsets, zone, zsk, ksk)
}

func dnssecNSEC3PARAMRecord(zone *SnapshotZone) dns.RR {
	return &dns.NSEC3PARAM{
		Hdr:        dns.RR_Header{Name: dnsName(zone.Name), Rrtype: dns.TypeNSEC3PARAM, Class: dns.ClassINET, Ttl: uint32(normalizeStaticTTL(zone.DefaultTTL, DefaultZoneTTL))},
		Hash:       dns.SHA1,
		Flags:      0,
		Iterations: uint16(normalizeDNSSECNSEC3Iterations(zone.DNSSEC.NSEC3Iterations)),
		SaltLength: uint8(len(strings.TrimSpace(zone.DNSSEC.NSEC3Salt)) / 2),
		Salt:       strings.TrimSpace(zone.DNSSEC.NSEC3Salt),
	}
}

func isDNSSECNoDataResponse(response *dns.Msg) bool {
	return response != nil && response.Rcode == dns.RcodeSuccess && len(response.Answer) == 0
}

func addDNSSECDenialProof(response *dns.Msg, request *dns.Msg, zone *SnapshotZone, index snapshotIndex) {
	if response == nil || request == nil || len(request.Question) == 0 || zone == nil {
		return
	}
	if normalizeDNSSECDenialMode(zone.DNSSEC.DenialMode) == dnssecDenialModeNSEC3 {
		response.Ns = append(response.Ns, dnssecNSEC3ForName(zone, request.Question[0].Name, index))
		return
	}
	response.Ns = append(response.Ns, dnssecNSECForName(zone, request.Question[0].Name, index))
}

func dnssecNSECForName(zone *SnapshotZone, qname string, index snapshotIndex) dns.RR {
	name := normalizeDomain(qname)
	if name == "" || !strings.HasSuffix(name, zone.Name) {
		name = zone.Name
	}
	nextName := dnssecNextName(zone, name, index)
	return &dns.NSEC{
		Hdr:        dns.RR_Header{Name: dnsName(name), Rrtype: dns.TypeNSEC, Class: dns.ClassINET, Ttl: uint32(normalizeStaticTTL(zone.DefaultTTL, DefaultZoneTTL))},
		NextDomain: dnsName(nextName),
		TypeBitMap: []uint16{dns.TypeA, dns.TypeNS, dns.TypeCNAME, dns.TypeSOA, dns.TypeMX, dns.TypeTXT, dns.TypeAAAA, dns.TypeRRSIG, dns.TypeNSEC, dns.TypeDNSKEY, dns.TypeCAA},
	}
}

func dnssecNSEC3ForName(zone *SnapshotZone, qname string, index snapshotIndex) dns.RR {
	name := normalizeDomain(qname)
	if name == "" || !strings.HasSuffix(name, zone.Name) {
		name = zone.Name
	}
	salt := strings.TrimSpace(zone.DNSSEC.NSEC3Salt)
	iterations := uint16(normalizeDNSSECNSEC3Iterations(zone.DNSSEC.NSEC3Iterations))
	hash := dns.HashName(dnsName(name), dns.SHA1, iterations, salt)
	next := dnssecNextNSEC3Hash(zone, name, index, iterations, salt)
	return &dns.NSEC3{
		Hdr:        dns.RR_Header{Name: strings.ToLower(hash) + "." + dnsName(zone.Name), Rrtype: dns.TypeNSEC3, Class: dns.ClassINET, Ttl: uint32(normalizeStaticTTL(zone.DefaultTTL, DefaultZoneTTL))},
		Hash:       dns.SHA1,
		Flags:      0,
		Iterations: iterations,
		SaltLength: uint8(len(salt) / 2),
		Salt:       salt,
		HashLength: 20,
		NextDomain: next,
		TypeBitMap: []uint16{dns.TypeNS, dns.TypeSOA, dns.TypeRRSIG, dns.TypeDNSKEY, dns.TypeNSEC3PARAM},
	}
}

func dnssecNextName(zone *SnapshotZone, name string, index snapshotIndex) string {
	names := make([]string, 0, len(index.namesByZone[zone.ID])+1)
	for item := range index.namesByZone[zone.ID] {
		if item != "" {
			names = append(names, item)
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		return zone.Name
	}
	for _, item := range names {
		if item > name {
			return item
		}
	}
	return names[0]
}

func dnssecNextNSEC3Hash(zone *SnapshotZone, name string, index snapshotIndex, iterations uint16, salt string) string {
	names := make([]string, 0, len(index.namesByZone[zone.ID])+1)
	for item := range index.namesByZone[zone.ID] {
		if item != "" {
			names = append(names, item)
		}
	}
	if len(names) == 0 {
		return dns.HashName(dnsName(zone.Name), dns.SHA1, iterations, salt)
	}
	hashes := make([]string, 0, len(names))
	for _, item := range names {
		hashes = append(hashes, dns.HashName(dnsName(item), dns.SHA1, iterations, salt))
	}
	sort.Strings(hashes)
	current := dns.HashName(dnsName(name), dns.SHA1, iterations, salt)
	for _, item := range hashes {
		if item > current {
			return item
		}
	}
	return hashes[0]
}
