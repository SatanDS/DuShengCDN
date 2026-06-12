package service

import (
	"errors"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"

	"dushengcdn/common"
	"dushengcdn/model"

	"github.com/miekg/dns"
)

var dnsLookupNS = net.LookupNS

func CheckAuthoritativeDNSZoneDelegation(id uint) (*DNSZoneDelegationCheckView, error) {
	zone, err := model.GetDNSZoneByID(id)
	if err != nil {
		return nil, err
	}
	expected := normalizeDNSNameServerSet(decodeStoredStringList(zone.NameServers))
	view := &DNSZoneDelegationCheckView{
		ZoneID:              zone.ID,
		ZoneName:            zone.Name,
		ExpectedNameServers: expected,
		GlueNameServers:     glueNameServersForZone(zone.Name, expected),
		CheckedAt:           time.Now().UTC(),
	}
	view.GlueRequired = len(view.GlueNameServers) > 0
	if len(expected) == 0 {
		view.Status = dnsDelegationNotConfig
		return view, nil
	}

	records, err := dnsLookupNS(zone.Name)
	if err != nil {
		view.Status = dnsDelegationFailed
		view.Error = err.Error()
		return view, nil
	}
	actual := make([]string, 0, len(records))
	for _, record := range records {
		if record == nil {
			continue
		}
		actual = append(actual, record.Host)
	}
	view.ActualNameServers = normalizeDNSNameServerSet(actual)
	view.MatchedNameServers, view.MissingNameServers, view.ExtraNameServers = compareNameServerSets(expected, view.ActualNameServers)
	view.Status = dnsDelegationStatus(expected, view.ActualNameServers, view.MatchedNameServers, view.MissingNameServers, view.ExtraNameServers)
	return view, nil
}

func normalizeDNSNameServerSet(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		ns := normalizeDNSRecordName(value)
		if ns == "" {
			continue
		}
		if _, ok := seen[ns]; ok {
			continue
		}
		seen[ns] = struct{}{}
		result = append(result, ns)
	}
	sort.Strings(result)
	return result
}

func compareNameServerSets(expected []string, actual []string) ([]string, []string, []string) {
	expected = normalizeDNSNameServerSet(expected)
	actual = normalizeDNSNameServerSet(actual)
	actualSet := make(map[string]struct{}, len(actual))
	for _, ns := range actual {
		actualSet[ns] = struct{}{}
	}
	expectedSet := make(map[string]struct{}, len(expected))
	for _, ns := range expected {
		expectedSet[ns] = struct{}{}
	}
	matched := make([]string, 0)
	missing := make([]string, 0)
	extra := make([]string, 0)
	for _, ns := range expected {
		if _, ok := actualSet[ns]; ok {
			matched = append(matched, ns)
		} else {
			missing = append(missing, ns)
		}
	}
	for _, ns := range actual {
		if _, ok := expectedSet[ns]; !ok {
			extra = append(extra, ns)
		}
	}
	return matched, missing, extra
}

func dnsDelegationStatus(expected []string, actual []string, matched []string, missing []string, extra []string) string {
	if len(expected) == 0 {
		return dnsDelegationNotConfig
	}
	if len(actual) == 0 {
		return dnsDelegationMismatch
	}
	if len(missing) == 0 && len(extra) == 0 {
		return dnsDelegationMatched
	}
	if len(matched) > 0 {
		return dnsDelegationPartial
	}
	return dnsDelegationMismatch
}

func glueNameServersForZone(zoneName string, nameServers []string) []string {
	zoneName = normalizeDNSRecordName(zoneName)
	result := make([]string, 0, len(nameServers))
	for _, nameServer := range normalizeDNSNameServerSet(nameServers) {
		if domainBelongsToZone(nameServer, zoneName) {
			result = append(result, nameServer)
		}
	}
	return result
}

func normalizeDNSZoneName(raw string) (string, error) {
	name := normalizeDNSRecordName(raw)
	if name == "" {
		return "", errors.New("DNS zone name cannot be empty")
	}
	if strings.HasPrefix(name, "*.") || !isValidProxyRouteDomain(name) {
		return "", errors.New("DNS zone name format is invalid")
	}
	return name, nil
}

func normalizeNameServers(values []string) ([]string, error) {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		ns := normalizeDNSRecordName(value)
		if ns == "" {
			continue
		}
		if !isValidProxyRouteDomain(ns) {
			return nil, errors.New("name_servers contains invalid domain")
		}
		if _, ok := seen[ns]; ok {
			continue
		}
		seen[ns] = struct{}{}
		result = append(result, ns)
	}
	return result, nil
}

func normalizeSOAEmail(raw string, zoneName string) string {
	email := strings.TrimSpace(raw)
	if email != "" {
		return email
	}
	return "hostmaster@" + zoneName
}

func normalizeDNSZoneTTL(value int) int {
	if value <= 0 {
		return defaultDNSZoneTTL
	}
	if value > 86400 {
		return 86400
	}
	return value
}

func normalizeAuthoritativeTTL(value int, fallback int) int {
	if fallback <= 0 {
		fallback = defaultDNSZoneTTL
	}
	if value <= 0 {
		return fallback
	}
	if value > 86400 {
		return 86400
	}
	return value
}

func normalizeAuthoritativeRouteTTL(value int) int {
	defaultTTL := authoritativeDNSDefaultTTL()
	if value <= 1 {
		return defaultTTL
	}
	if value < defaultTTL {
		return defaultTTL
	}
	if value > 86400 {
		return 86400
	}
	return value
}

func authoritativeDNSDefaultTTL() int {
	raw := strings.TrimSpace(common.GetOptionValue("AuthoritativeDNSDefaultTTL"))
	if raw == "" {
		return common.AuthoritativeDNSDefaultTTL
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return common.AuthoritativeDNSDefaultTTL
	}
	if value > 86400 {
		return 86400
	}
	return value
}

func authoritativeDNSSnapshotMaxAge() time.Duration {
	raw := strings.TrimSpace(common.GetOptionValue("AuthoritativeDNSSnapshotMaxAge"))
	if raw == "" {
		if common.AuthoritativeDNSSnapshotMaxAge > 0 {
			return time.Duration(common.AuthoritativeDNSSnapshotMaxAge) * time.Second
		}
		return defaultDNSSnapshotMaxAge
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return defaultDNSSnapshotMaxAge
	}
	return time.Duration(value) * time.Second
}

func normalizeAuthoritativeDNSRecordType(raw string) (string, error) {
	recordType := strings.ToUpper(strings.TrimSpace(raw))
	switch recordType {
	case "A", "AAAA", "CNAME", "TXT", "MX", "NS", "SOA", "CAA", "SRV", "HTTPS", "SVCB", "TLSA":
		return recordType, nil
	default:
		return "", errors.New("unsupported DNS record type")
	}
}

func normalizeAuthoritativeDNSRecordTypeOrDefault(raw string) string {
	recordType, err := normalizeAuthoritativeDNSRecordType(raw)
	if err != nil {
		return "A"
	}
	return recordType
}

func normalizeAuthoritativeDNSRecordName(zoneName string, raw string) (string, error) {
	zoneName = normalizeDNSRecordName(zoneName)
	name := normalizeDNSRecordName(raw)
	if name == "" || name == "@" {
		name = zoneName
	} else if !strings.Contains(name, ".") {
		name += "." + zoneName
	}
	if !isValidAuthoritativeDNSRecordName(name) {
		return "", errors.New("DNS record name format is invalid")
	}
	if !domainBelongsToZone(name, zoneName) {
		return "", errors.New("DNS record name is outside the zone")
	}
	return name, nil
}

func isValidAuthoritativeDNSRecordName(name string) bool {
	name = normalizeDNSRecordName(name)
	if name == "" || len(name) > 253 || strings.ContainsAny(name, " \t\r\n;{}\"'`$:\\/") || strings.Contains(name, "://") {
		return false
	}
	labels := strings.Split(name, ".")
	if len(labels) < 2 {
		return false
	}
	for _, label := range labels {
		if label == "" || len(label) > 63 {
			return false
		}
		for _, r := range label {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
				continue
			}
			return false
		}
	}
	return true
}

func normalizeAuthoritativeDNSRecordValue(recordType string, raw string, priority int) (string, int, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", 0, errors.New("DNS record value cannot be empty")
	}
	switch recordType {
	case "A":
		ip := net.ParseIP(value)
		if ip == nil || ip.To4() == nil {
			return "", 0, errors.New("A record value must be an IPv4 address")
		}
		return ip.String(), 0, nil
	case "AAAA":
		ip := net.ParseIP(value)
		if ip == nil || ip.To4() != nil {
			return "", 0, errors.New("AAAA record value must be an IPv6 address")
		}
		return ip.String(), 0, nil
	case "CNAME", "NS":
		target := normalizeDNSRecordName(value)
		if target == "" || !isValidProxyRouteDomain(target) || net.ParseIP(target) != nil {
			return "", 0, fmt.Errorf("%s record value must be a domain name", recordType)
		}
		return target, 0, nil
	case "MX":
		target := normalizeDNSRecordName(value)
		if target == "" || !isValidProxyRouteDomain(target) || net.ParseIP(target) != nil {
			return "", 0, errors.New("MX record value must be a mail server domain name")
		}
		if priority < 0 {
			priority = 0
		}
		return target, priority, nil
	case "TXT", "SOA":
		return value, priority, nil
	case "CAA", "TLSA":
		if err := validateAuthoritativeDNSPresentationRDATA(recordType, "example.com", value); err != nil {
			return "", 0, err
		}
		return value, 0, nil
	case "SRV", "HTTPS", "SVCB":
		if priority < 0 {
			priority = 0
		}
		rdata := fmt.Sprintf("%d %s", priority, value)
		if err := validateAuthoritativeDNSPresentationRDATA(recordType, "example.com", rdata); err != nil {
			return "", 0, err
		}
		return value, priority, nil
	default:
		return "", 0, errors.New("unsupported DNS record type")
	}
}

func validateAuthoritativeDNSPresentationRDATA(recordType string, owner string, rdata string) error {
	recordType = strings.ToUpper(strings.TrimSpace(recordType))
	owner = normalizeDNSRecordName(owner)
	if owner == "" {
		owner = "example.com"
	}
	_, err := dns.NewRR(fmt.Sprintf("%s. 300 IN %s %s", owner, recordType, strings.TrimSpace(rdata)))
	if err != nil {
		return fmt.Errorf("%s record value is invalid: %w", recordType, err)
	}
	return nil
}

func normalizeDNSRollupWindow(value int) int {
	if value <= 0 {
		return 1
	}
	if value > defaultDNSMaxRollupWindowMinutes {
		return defaultDNSMaxRollupWindowMinutes
	}
	return value
}

func normalizeDNSRCode(raw string) string {
	rcode := strings.ToUpper(strings.TrimSpace(raw))
	switch rcode {
	case "NOERROR", "NXDOMAIN", "NODATA", "SERVFAIL", "REFUSED":
		return rcode
	default:
		return "NOERROR"
	}
}
