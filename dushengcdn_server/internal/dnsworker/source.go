package dnsworker

import (
	"net"
	"strconv"
	"strings"

	"github.com/miekg/dns"
	"github.com/oschwald/maxminddb-golang"
)

type SourceResolver struct {
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
}

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

func NewSourceResolver(mmdbPath string, extraPaths ...string) (*SourceResolver, error) {
	resolver := &SourceResolver{
		databasePath: strings.TrimSpace(mmdbPath),
	}
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
	sourceIP := clientSubnetIP(request)
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
		return subnet.Address
	}
	return nil
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
