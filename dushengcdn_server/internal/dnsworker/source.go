package dnsworker

import (
	"net"
	"strconv"
	"strings"
	"sync"

	"github.com/miekg/dns"
	"github.com/oschwald/maxminddb-golang"
)

type SourceResolver struct {
	mu                       sync.RWMutex
	reader                   *maxminddb.Reader
	asnReader                *maxminddb.Reader
	databasePath             string
	asnDatabasePath          string
	lastError                string
	asnLastError             string
	databaseType             string
	asnDatabaseType          string
	countryEnabled           bool
	asnEnabled               bool
	operatorEnabled          bool
	operatorCIDRs            *OperatorCIDRMatcher
	operatorCIDRDatabasePath string
	operatorCIDRLastError    string
	ecsConfigured            bool
	ecsEnabled               bool
	ecsIPv4Prefix            int
	ecsIPv6Prefix            int
}

type SourceResolverOption func(*SourceResolver)

type SourceLookup interface {
	Resolve(request *dns.Msg, remoteAddr net.Addr) SourceContext
}

type SourceResolverStatus struct {
	Enabled                  bool
	DatabasePath             string
	ASNDatabasePath          string
	LastError                string
	ASNLastError             string
	DatabaseType             string
	ASNDatabaseType          string
	CountryEnabled           bool
	ASNEnabled               bool
	OperatorEnabled          bool
	OperatorCIDRDatabasePath string
	OperatorCIDRLastError    string
}

type geoRecord struct {
	Country struct {
		ISOCode string `maxminddb:"iso_code"`
	} `maxminddb:"country"`
	ASN                    uint32 `maxminddb:"autonomous_system_number"`
	AutonomousOrganization string `maxminddb:"autonomous_system_organization"`
	ISP                    string `maxminddb:"isp"`
	Organization           string `maxminddb:"organization"`
}

func WithECS(enabled bool, ipv4Prefix int, ipv6Prefix int) SourceResolverOption {
	return func(r *SourceResolver) {
		r.ApplyECSPolicy(enabled, ipv4Prefix, ipv6Prefix)
	}
}

func NewSourceResolver(mmdbPath string, extraPaths ...string) (*SourceResolver, error) {
	resolver := &SourceResolver{
		databasePath:  strings.TrimSpace(mmdbPath),
		ecsEnabled:    true,
		ecsIPv4Prefix: DefaultECSIPv4Prefix,
		ecsIPv6Prefix: DefaultECSIPv6Prefix,
	}
	filteredExtraPaths := make([]string, 0, len(extraPaths))
	for _, item := range extraPaths {
		if strings.HasPrefix(item, "ecs:") {
			continue
		}
		filteredExtraPaths = append(filteredExtraPaths, item)
	}
	extraPaths = filteredExtraPaths
	if len(extraPaths) > 0 {
		resolver.asnDatabasePath = strings.TrimSpace(extraPaths[0])
	}
	if len(extraPaths) > 1 {
		resolver.operatorCIDRDatabasePath = strings.TrimSpace(extraPaths[1])
	}
	var firstErr error
	if resolver.databasePath != "" {
		reader, err := maxminddb.Open(resolver.databasePath)
		if err != nil {
			resolver.lastError = err.Error()
			firstErr = err
		} else {
			resolver.reader = reader
			resolver.detectMMDBCapabilities(reader, false)
		}
	}
	if resolver.asnDatabasePath != "" && resolver.asnDatabasePath != resolver.databasePath {
		reader, err := maxminddb.Open(resolver.asnDatabasePath)
		if err != nil {
			resolver.asnLastError = err.Error()
			if firstErr == nil {
				firstErr = err
			}
		} else {
			resolver.asnReader = reader
			resolver.detectMMDBCapabilities(reader, true)
		}
	}
	if resolver.operatorCIDRDatabasePath != "" {
		matcher, err := LoadOperatorCIDRMatcher(resolver.operatorCIDRDatabasePath)
		if err != nil {
			resolver.operatorCIDRLastError = err.Error()
			if firstErr == nil {
				firstErr = err
			}
		} else {
			resolver.operatorCIDRs = matcher
			if matcher != nil && matcher.Count() > 0 {
				resolver.operatorEnabled = true
			}
		}
	}
	return resolver, firstErr
}

func (r *SourceResolver) Close() error {
	if r == nil || r.reader == nil {
		if r != nil && r.asnReader != nil {
			return r.asnReader.Close()
		}
		return nil
	}
	err := r.reader.Close()
	if r.asnReader != nil {
		if asnErr := r.asnReader.Close(); err == nil {
			err = asnErr
		}
	}
	return err
}

func (r *SourceResolver) Status() SourceResolverStatus {
	if r == nil {
		return SourceResolverStatus{}
	}
	return SourceResolverStatus{
		Enabled:                  r.reader != nil,
		DatabasePath:             r.databasePath,
		ASNDatabasePath:          r.asnDatabasePath,
		LastError:                r.lastError,
		ASNLastError:             r.asnLastError,
		DatabaseType:             r.databaseType,
		ASNDatabaseType:          r.asnDatabaseType,
		CountryEnabled:           r.countryEnabled,
		ASNEnabled:               r.asnEnabled,
		OperatorEnabled:          r.operatorEnabled,
		OperatorCIDRDatabasePath: r.operatorCIDRDatabasePath,
		OperatorCIDRLastError:    r.operatorCIDRLastError,
	}
}

func (r *SourceResolver) Resolve(request *dns.Msg, remoteAddr net.Addr) SourceContext {
	ecsEnabled, ecsIPv4Prefix, ecsIPv6Prefix := sourceResolverECSPolicy(r)
	sourceIP := clientSubnetIPWithPolicy(request, ecsEnabled, ecsIPv4Prefix, ecsIPv6Prefix)
	if sourceIP == nil {
		sourceIP = remoteIP(remoteAddr)
	}
	ctx := SourceContext{}
	if sourceIP != nil {
		ctx.IP = sourceIP.String()
	}
	if r != nil && r.reader != nil && sourceIP != nil {
		var record geoRecord
		if err := r.reader.Lookup(sourceIP, &record); err == nil {
			ctx.Country = strings.ToUpper(strings.TrimSpace(record.Country.ISOCode))
			ctx.ASN = record.ASN
			ctx.Operator = classifySourceOperator(record.ISP, record.Organization, record.AutonomousOrganization)
		}
	}
	if r != nil && r.asnReader != nil && sourceIP != nil && ctx.ASN == 0 {
		var record geoRecord
		if err := r.asnReader.Lookup(sourceIP, &record); err == nil {
			ctx.ASN = record.ASN
			if ctx.Operator == "" {
				ctx.Operator = classifySourceOperator(record.ISP, record.Organization, record.AutonomousOrganization)
			}
		}
	}
	if r != nil && r.operatorCIDRs != nil && sourceIP != nil {
		if operator := r.operatorCIDRs.Lookup(sourceIP); operator != "" {
			ctx.Operator = operator
			if ctx.Country == "" {
				ctx.Country = "CN"
			}
		}
	}
	ctx.ScopeKey = sourceScopeKey(ctx)
	return ctx
}

func (r *SourceResolver) ApplyECSPolicy(enabled bool, ipv4Prefix int, ipv6Prefix int) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.ecsConfigured = true
	r.ecsEnabled = enabled
	r.ecsIPv4Prefix = normalizePrefix(ipv4Prefix, 32, DefaultECSIPv4Prefix)
	r.ecsIPv6Prefix = normalizePrefix(ipv6Prefix, 128, DefaultECSIPv6Prefix)
	r.mu.Unlock()
}

func sourceResolverECSPolicy(r *SourceResolver) (bool, int, int) {
	if r == nil {
		return true, DefaultECSIPv4Prefix, DefaultECSIPv6Prefix
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if !r.ecsConfigured {
		return true, DefaultECSIPv4Prefix, DefaultECSIPv6Prefix
	}
	return r.ecsEnabled,
		normalizePrefix(r.ecsIPv4Prefix, 32, DefaultECSIPv4Prefix),
		normalizePrefix(r.ecsIPv6Prefix, 128, DefaultECSIPv6Prefix)
}

func (r *SourceResolver) detectMMDBCapabilities(reader *maxminddb.Reader, asnOnly bool) {
	if r == nil || reader == nil {
		return
	}
	databaseTypeName := strings.TrimSpace(reader.Metadata.DatabaseType)
	if asnOnly {
		r.asnDatabaseType = databaseTypeName
	} else {
		r.databaseType = databaseTypeName
	}
	databaseType := strings.ToLower(databaseTypeName)
	switch {
	case strings.Contains(databaseType, "country") ||
		strings.Contains(databaseType, "city") ||
		strings.Contains(databaseType, "enterprise"):
		if !asnOnly {
			r.countryEnabled = true
		}
	}
	switch {
	case strings.Contains(databaseType, "asn") ||
		strings.Contains(databaseType, "isp") ||
		strings.Contains(databaseType, "enterprise"):
		r.asnEnabled = true
	}
	switch {
	case strings.Contains(databaseType, "isp") ||
		strings.Contains(databaseType, "enterprise") ||
		strings.Contains(databaseType, "asn"):
		r.operatorEnabled = true
	}
	if databaseType == "" && !asnOnly {
		r.countryEnabled = true
	}
}

func classifySourceOperator(values ...string) string {
	for _, value := range values {
		operator := classifySourceOperatorValue(value)
		if operator != "" {
			return operator
		}
	}
	return ""
}

func classifySourceOperatorValue(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return ""
	}
	compact := strings.NewReplacer(" ", "", "-", "", "_", "", ".", "").Replace(normalized)
	switch {
	case strings.Contains(normalized, "电信") ||
		strings.Contains(compact, "chinatelecom") ||
		strings.Contains(compact, "chinanet") ||
		strings.Contains(compact, "ctcc"):
		return "cn-telecom"
	case strings.Contains(normalized, "联通") ||
		strings.Contains(compact, "chinaunicom") ||
		strings.Contains(compact, "unicom"):
		return "cn-unicom"
	case strings.Contains(normalized, "移动") ||
		strings.Contains(compact, "chinamobile") ||
		strings.Contains(compact, "cmcc"):
		return "cn-mobile"
	case strings.Contains(normalized, "广电") ||
		strings.Contains(compact, "chinabroadcast") ||
		strings.Contains(compact, "cbn"):
		return "cn-broadcast"
	case strings.Contains(compact, "cernet") ||
		strings.Contains(normalized, "教育网"):
		return "cernet"
	default:
		return normalizeOperator(value)
	}
}

func clientSubnetIP(request *dns.Msg) net.IP {
	return clientSubnetIPWithPolicy(request, true, DefaultECSIPv4Prefix, DefaultECSIPv6Prefix)
}

func clientSubnetIPWithPolicy(request *dns.Msg, enabled bool, ipv4Prefix int, ipv6Prefix int) net.IP {
	if !enabled {
		return nil
	}
	if request == nil {
		return nil
	}
	opt := request.IsEdns0()
	if opt == nil {
		return nil
	}
	for _, option := range opt.Option {
		subnet, ok := option.(*dns.EDNS0_SUBNET)
		if !ok || subnet.Address == nil {
			continue
		}
		return normalizeECSAddress(subnet.Address, subnet.Family, subnet.SourceNetmask, ipv4Prefix, ipv6Prefix)
	}
	return nil
}

func normalizeECSAddress(ip net.IP, family uint16, sourcePrefix uint8, ipv4Prefix int, ipv6Prefix int) net.IP {
	if ip == nil {
		return nil
	}
	switch family {
	case 1:
		ipv4 := ip.To4()
		if ipv4 == nil {
			return nil
		}
		prefix := minInt(int(sourcePrefix), normalizePrefix(ipv4Prefix, 32, DefaultECSIPv4Prefix))
		if prefix < 0 {
			prefix = 0
		}
		return ipv4.Mask(net.CIDRMask(prefix, 32))
	case 2:
		if ip.To4() != nil {
			return nil
		}
		ipv6 := ip.To16()
		if ipv6 == nil {
			return nil
		}
		prefix := minInt(int(sourcePrefix), normalizePrefix(ipv6Prefix, 128, DefaultECSIPv6Prefix))
		if prefix < 0 {
			prefix = 0
		}
		return ipv6.Mask(net.CIDRMask(prefix, 128))
	default:
		return nil
	}
}

func ecsIPv4Prefix(r *SourceResolver) int {
	if r == nil || !r.ecsConfigured {
		return DefaultECSIPv4Prefix
	}
	return normalizePrefix(r.ecsIPv4Prefix, 32, DefaultECSIPv4Prefix)
}

func ecsIPv6Prefix(r *SourceResolver) int {
	if r == nil || !r.ecsConfigured {
		return DefaultECSIPv6Prefix
	}
	return normalizePrefix(r.ecsIPv6Prefix, 128, DefaultECSIPv6Prefix)
}

func ecsEnabled(r *SourceResolver) bool {
	if r == nil || !r.ecsConfigured {
		return true
	}
	return r.ecsEnabled
}

func normalizePrefix(value int, max int, fallback int) int {
	if value < 0 {
		return fallback
	}
	if value > max {
		return max
	}
	return value
}

func minInt(left int, right int) int {
	if left < right {
		return left
	}
	return right
}

func remoteIP(remoteAddr net.Addr) net.IP {
	if remoteAddr == nil {
		return nil
	}
	host := remoteAddr.String()
	if splitHost, _, err := net.SplitHostPort(host); err == nil {
		host = splitHost
	}
	return net.ParseIP(strings.TrimSpace(host))
}

func sourceScopeKey(source SourceContext) string {
	if source.ASN > 0 {
		return "asn:" + strconv.FormatUint(uint64(source.ASN), 10)
	}
	operator := normalizeOperator(source.Operator)
	if operator != "" {
		return "operator:" + operator
	}
	country := strings.ToUpper(strings.TrimSpace(source.Country))
	if country != "" {
		return "country:" + country
	}
	return "global"
}
