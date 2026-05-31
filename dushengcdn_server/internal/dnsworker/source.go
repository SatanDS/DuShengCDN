package dnsworker

import (
	"net"
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
		}
	}
	ctx.ScopeKey = sourceScopeKey(ctx)
	return ctx
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
	country := strings.ToUpper(strings.TrimSpace(source.Country))
	if country != "" {
		return "country:" + country
	}
	return "global"
}
