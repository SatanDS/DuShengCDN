package dnsworker

import (
	"net"
	"strconv"
	"strings"

	"github.com/miekg/dns"
	"github.com/oschwald/maxminddb-golang"
)

type SourceResolver struct {
	reader       *maxminddb.Reader
	databasePath string
	lastError    string
}

type SourceLookup interface {
	Resolve(request *dns.Msg, remoteAddr net.Addr) SourceContext
}

type SourceResolverStatus struct {
	Enabled      bool
	DatabasePath string
	LastError    string
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

func NewSourceResolver(mmdbPath string) (*SourceResolver, error) {
	resolver := &SourceResolver{
		databasePath: strings.TrimSpace(mmdbPath),
	}
	if resolver.databasePath == "" {
		return resolver, nil
	}
	reader, err := maxminddb.Open(resolver.databasePath)
	if err != nil {
		resolver.lastError = err.Error()
		return resolver, err
	}
	resolver.reader = reader
	return resolver, nil
}

func (r *SourceResolver) Close() error {
	if r == nil || r.reader == nil {
		return nil
	}
	return r.reader.Close()
}

func (r *SourceResolver) Status() SourceResolverStatus {
	if r == nil {
		return SourceResolverStatus{}
	}
	return SourceResolverStatus{
		Enabled:      r.reader != nil,
		DatabasePath: r.databasePath,
		LastError:    r.lastError,
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
	ctx.ScopeKey = sourceScopeKey(ctx)
	return ctx
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
