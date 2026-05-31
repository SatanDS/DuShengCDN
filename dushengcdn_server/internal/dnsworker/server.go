package dnsworker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"time"

	"github.com/miekg/dns"
)

type DNSServer struct {
	Store      *SnapshotStore
	Scheduler  *Scheduler
	Rollups    *RollupAggregator
	Resolver   SourceLookup
	ListenAddr string
	Limiter    *QueryRateLimiter
	UDPSize    int
	udpServer  *dns.Server
	tcpServer  *dns.Server
}

func NewDNSServer(store *SnapshotStore, scheduler *Scheduler, rollups *RollupAggregator, resolver SourceLookup, listenAddr string) *DNSServer {
	return NewDNSServerWithLimits(store, scheduler, rollups, resolver, listenAddr, DefaultQueryRateLimit, DefaultUDPResponseSize)
}

func NewDNSServerWithLimits(store *SnapshotStore, scheduler *Scheduler, rollups *RollupAggregator, resolver SourceLookup, listenAddr string, queryRateLimit int, udpSize int) *DNSServer {
	if scheduler == nil {
		scheduler = NewScheduler()
	}
	if rollups == nil {
		rollups = NewRollupAggregator(time.Minute)
	}
	if strings.TrimSpace(listenAddr) == "" {
		listenAddr = DefaultListenAddr
	}
	if udpSize <= 0 {
		udpSize = DefaultUDPResponseSize
	}
	if udpSize < dns.MinMsgSize {
		udpSize = dns.MinMsgSize
	}
	if udpSize > dns.MaxMsgSize {
		udpSize = dns.MaxMsgSize
	}
	server := &DNSServer{
		Store:      store,
		Scheduler:  scheduler,
		Rollups:    rollups,
		Resolver:   resolver,
		ListenAddr: listenAddr,
		Limiter:    NewQueryRateLimiter(queryRateLimit),
		UDPSize:    udpSize,
	}
	mux := dns.NewServeMux()
	mux.HandleFunc(".", server.handleDNS)
	server.udpServer = &dns.Server{Addr: listenAddr, Net: "udp", Handler: mux, UDPSize: udpSize}
	server.tcpServer = &dns.Server{Addr: listenAddr, Net: "tcp", Handler: mux}
	return server
}

func (s *DNSServer) Run(ctx context.Context) error {
	if s == nil {
		return errors.New("dns server is nil")
	}
	errCh := make(chan error, 2)
	go func() {
		slog.Info("dns worker udp listening", "address", s.ListenAddr)
		errCh <- s.udpServer.ListenAndServe()
	}()
	go func() {
		slog.Info("dns worker tcp listening", "address", s.ListenAddr)
		errCh <- s.tcpServer.ListenAndServe()
	}()
	select {
	case <-ctx.Done():
		s.shutdown()
		return ctx.Err()
	case err := <-errCh:
		s.shutdown()
		return err
	}
}

func (s *DNSServer) shutdown() {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if s.udpServer != nil {
		_ = s.udpServer.ShutdownContext(shutdownCtx)
	}
	if s.tcpServer != nil {
		_ = s.tcpServer.ShutdownContext(shutdownCtx)
	}
}

func (s *DNSServer) handleDNS(w dns.ResponseWriter, request *dns.Msg) {
	remoteAddr := w.RemoteAddr()
	response := s.Resolve(request, remoteAddr)
	if isUDPResponse(w) {
		response = s.truncateUDPResponse(request, response)
	}
	_ = w.WriteMsg(response)
}

func (s *DNSServer) Resolve(request *dns.Msg, remoteAddr net.Addr) *dns.Msg {
	startedAt := time.Now()
	response := new(dns.Msg)
	if request == nil {
		response.SetRcode(&dns.Msg{}, dns.RcodeFormatError)
		return response
	}
	response.SetReply(request)
	response.Authoritative = true
	response.RecursionAvailable = false
	if len(request.Question) == 0 {
		response.Rcode = dns.RcodeFormatError
		return response
	}
	question := request.Question[0]
	qname := normalizeDomain(question.Name)
	qtype := dns.TypeToString[question.Qtype]
	if qtype == "" {
		qtype = fmt.Sprintf("TYPE%d", question.Qtype)
	}
	zoneID, routeID := uint(0), uint(0)
	rcodeLabel := "NOERROR"
	targets := []string{}
	sourceScope := "global"
	defer func() {
		if s.Rollups != nil {
			s.Rollups.Record(zoneID, routeID, sourceScope, qname, qtype, rcodeLabel, targets, time.Since(startedAt))
		}
	}()
	if s.Limiter != nil && !s.Limiter.Allow(remoteAddr) {
		response.Rcode = dns.RcodeRefused
		rcodeLabel = "REFUSED"
		return response
	}
	if question.Qtype == dns.TypeAXFR || question.Qtype == dns.TypeIXFR {
		response.Rcode = dns.RcodeRefused
		rcodeLabel = "REFUSED"
		return response
	}
	snapshot, index, _, _ := s.Store.Current()
	if snapshot == nil {
		response.Rcode = dns.RcodeServerFailure
		rcodeLabel = "SERVFAIL"
		return response
	}
	zone := findBestZone(qname, index.zonesByName)
	if zone == nil {
		response.Rcode = dns.RcodeNameError
		rcodeLabel = "NXDOMAIN"
		return response
	}
	zoneID = zone.ID
	response.Ns = append(response.Ns, soaRecord(zone))
	source := SourceContext{ScopeKey: "global"}
	if s.Resolver != nil {
		source = s.Resolver.Resolve(request, remoteAddr)
	}
	sourceScope = sourceScopeKey(source)
	fresh := s.Store.IsFresh(time.Now().UTC())

	switch question.Qtype {
	case dns.TypeSOA:
		if qname == zone.Name {
			response.Answer = append(response.Answer, soaRecord(zone))
		} else {
			records := index.recordsByNameType[recordKey{ZoneID: zone.ID, Name: qname, Type: "SOA"}]
			response.Answer = append(response.Answer, recordsToRR(zone, records)...)
			targets = recordValues(records)
		}
	case dns.TypeNS:
		if qname == zone.Name {
			response.Answer = append(response.Answer, nsRecords(zone)...)
		}
		records := index.recordsByNameType[recordKey{ZoneID: zone.ID, Name: qname, Type: "NS"}]
		response.Answer = append(response.Answer, recordsToRR(zone, records)...)
		if len(records) > 0 {
			targets = recordValues(records)
		} else {
			targets = append([]string(nil), zone.NameServers...)
		}
	case dns.TypeA, dns.TypeAAAA:
		route := matchRoute(qname, qtype, index.routesByDomain[qname])
		if route != nil {
			routeID = route.ID
			selected, ttl, selectedScope, err := s.Scheduler.Select(snapshot, route, qtype, source, fresh)
			sourceScope = selectedScope
			if err != nil {
				response.Rcode = dns.RcodeServerFailure
				rcodeLabel = "SERVFAIL"
				return response
			}
			answers := addressRecords(qname, qtype, selected, ttl)
			response.Answer = append(response.Answer, answers...)
			targets = append(targets, selected...)
		} else {
			records := index.recordsByNameType[recordKey{ZoneID: zone.ID, Name: qname, Type: qtype}]
			response.Answer = append(response.Answer, recordsToRR(zone, records)...)
			targets = recordValues(records)
			if len(response.Answer) == 0 {
				cnameAnswers, cnameTargets := cnameFallback(zone, index, qname, qtype)
				response.Answer = append(response.Answer, cnameAnswers...)
				targets = append(targets, cnameTargets...)
			}
		}
	default:
		records := index.recordsByNameType[recordKey{ZoneID: zone.ID, Name: qname, Type: qtype}]
		response.Answer = append(response.Answer, recordsToRR(zone, records)...)
		targets = recordValues(records)
		if len(response.Answer) == 0 && qtype != "CNAME" {
			cnameAnswers, cnameTargets := cnameFallback(zone, index, qname, qtype)
			response.Answer = append(response.Answer, cnameAnswers...)
			targets = append(targets, cnameTargets...)
		}
	}
	if len(response.Answer) == 0 {
		if nameExists(zone.ID, qname, index.namesByZone) {
			rcodeLabel = "NODATA"
			return response
		}
		response.Rcode = dns.RcodeNameError
		rcodeLabel = "NXDOMAIN"
	}
	return response
}

func (s *DNSServer) truncateUDPResponse(request *dns.Msg, response *dns.Msg) *dns.Msg {
	if response == nil {
		return response
	}
	size := s.udpResponseSize(request)
	if response.Len() <= size {
		return response
	}
	copy := response.Copy()
	copy.Truncate(size)
	return copy
}

func (s *DNSServer) udpResponseSize(request *dns.Msg) int {
	size := dns.MinMsgSize
	limit := DefaultUDPResponseSize
	if s != nil && s.UDPSize > 0 {
		limit = s.UDPSize
	}
	if request != nil {
		if opt := request.IsEdns0(); opt != nil {
			size = int(opt.UDPSize())
		}
	}
	if size > limit {
		size = limit
	}
	if size < dns.MinMsgSize {
		return dns.MinMsgSize
	}
	if size > dns.MaxMsgSize {
		return dns.MaxMsgSize
	}
	return size
}

func isUDPResponse(w dns.ResponseWriter) bool {
	if w == nil {
		return false
	}
	switch w.LocalAddr().(type) {
	case *net.UDPAddr:
		return true
	default:
		return false
	}
}

func cnameFallback(zone *SnapshotZone, index snapshotIndex, qname string, qtype string) ([]dns.RR, []string) {
	cnameRecords := index.recordsByNameType[recordKey{ZoneID: zone.ID, Name: qname, Type: "CNAME"}]
	if len(cnameRecords) == 0 {
		return nil, nil
	}
	answers := recordsToRR(zone, cnameRecords)
	targets := recordValues(cnameRecords)
	if qtype == "A" || qtype == "AAAA" {
		targetName := normalizeDomain(cnameRecords[0].Value)
		targetRecords := index.recordsByNameType[recordKey{ZoneID: zone.ID, Name: targetName, Type: qtype}]
		answers = append(answers, recordsToRR(zone, targetRecords)...)
		targets = append(targets, recordValues(targetRecords)...)
	}
	return answers, targets
}

func findBestZone(qname string, zones map[string]*SnapshotZone) *SnapshotZone {
	qname = normalizeDomain(qname)
	var best *SnapshotZone
	for zoneName, zone := range zones {
		if qname == zoneName || strings.HasSuffix(qname, "."+zoneName) {
			if best == nil || len(zoneName) > len(best.Name) {
				best = zone
			}
		}
	}
	return best
}

func matchRoute(qname string, qtype string, routes []*SnapshotRoute) *SnapshotRoute {
	for _, route := range routes {
		if route == nil {
			continue
		}
		if normalizeAddressRecordType(route.RecordType) != normalizeAddressRecordType(qtype) {
			continue
		}
		return route
	}
	return nil
}

func nameExists(zoneID uint, qname string, namesByZone map[uint]map[string]struct{}) bool {
	names := namesByZone[zoneID]
	if len(names) == 0 {
		return false
	}
	_, ok := names[normalizeDomain(qname)]
	return ok
}

func soaRecord(zone *SnapshotZone) dns.RR {
	ttl := uint32(normalizeStaticTTL(zone.DefaultTTL, DefaultZoneTTL))
	primaryNS := zone.PrimaryNS
	if primaryNS == "" && len(zone.NameServers) > 0 {
		primaryNS = zone.NameServers[0]
	}
	if primaryNS == "" {
		primaryNS = "ns1." + zone.Name
	}
	return &dns.SOA{
		Hdr:     dns.RR_Header{Name: dnsName(zone.Name), Rrtype: dns.TypeSOA, Class: dns.ClassINET, Ttl: ttl},
		Ns:      dnsName(primaryNS),
		Mbox:    dnsName(soaMbox(zone.SOAEmail, zone.Name)),
		Serial:  uint32(zone.Serial),
		Refresh: 3600,
		Retry:   600,
		Expire:  86400,
		Minttl:  ttl,
	}
}

func soaMbox(email string, zoneName string) string {
	email = strings.TrimSpace(email)
	if email == "" {
		return "hostmaster." + normalizeDomain(zoneName)
	}
	if strings.Contains(email, "@") {
		parts := strings.SplitN(email, "@", 2)
		return normalizeDomain(parts[0] + "." + parts[1])
	}
	return normalizeDomain(email)
}

func nsRecords(zone *SnapshotZone) []dns.RR {
	ttl := uint32(normalizeStaticTTL(zone.DefaultTTL, DefaultZoneTTL))
	nameServers := zone.NameServers
	if len(nameServers) == 0 && zone.PrimaryNS != "" {
		nameServers = []string{zone.PrimaryNS}
	}
	records := make([]dns.RR, 0, len(nameServers))
	for _, ns := range nameServers {
		records = append(records, &dns.NS{
			Hdr: dns.RR_Header{Name: dnsName(zone.Name), Rrtype: dns.TypeNS, Class: dns.ClassINET, Ttl: ttl},
			Ns:  dnsName(ns),
		})
	}
	return records
}

func recordsToRR(zone *SnapshotZone, records []SnapshotRecord) []dns.RR {
	answers := make([]dns.RR, 0, len(records))
	for _, record := range records {
		if rr := recordToRR(zone, record); rr != nil {
			answers = append(answers, rr)
		}
	}
	return answers
}

func recordToRR(zone *SnapshotZone, record SnapshotRecord) dns.RR {
	ttl := uint32(normalizeStaticTTL(record.TTL, zone.DefaultTTL))
	header := dns.RR_Header{Name: dnsName(record.Name), Class: dns.ClassINET, Ttl: ttl}
	switch normalizeRecordType(record.Type) {
	case "A":
		ip := net.ParseIP(record.Value)
		if ip == nil || ip.To4() == nil {
			return nil
		}
		header.Rrtype = dns.TypeA
		return &dns.A{Hdr: header, A: ip.To4()}
	case "AAAA":
		ip := net.ParseIP(record.Value)
		if ip == nil || ip.To4() != nil {
			return nil
		}
		header.Rrtype = dns.TypeAAAA
		return &dns.AAAA{Hdr: header, AAAA: ip}
	case "CNAME":
		header.Rrtype = dns.TypeCNAME
		return &dns.CNAME{Hdr: header, Target: dnsName(record.Value)}
	case "TXT":
		header.Rrtype = dns.TypeTXT
		return &dns.TXT{Hdr: header, Txt: []string{record.Value}}
	case "MX":
		header.Rrtype = dns.TypeMX
		return &dns.MX{Hdr: header, Preference: uint16(record.Priority), Mx: dnsName(record.Value)}
	case "NS":
		header.Rrtype = dns.TypeNS
		return &dns.NS{Hdr: header, Ns: dnsName(record.Value)}
	case "SOA":
		return soaRecord(zone)
	default:
		return nil
	}
}

func addressRecords(qname string, recordType string, targets []string, ttl int) []dns.RR {
	answers := make([]dns.RR, 0, len(targets))
	for _, target := range targets {
		ip := net.ParseIP(target)
		if ip == nil {
			continue
		}
		header := dns.RR_Header{Name: dnsName(qname), Class: dns.ClassINET, Ttl: uint32(normalizeAuthoritativeTTL(ttl))}
		if recordType == "A" {
			if ip.To4() == nil {
				continue
			}
			header.Rrtype = dns.TypeA
			answers = append(answers, &dns.A{Hdr: header, A: ip.To4()})
			continue
		}
		if ip.To4() != nil {
			continue
		}
		header.Rrtype = dns.TypeAAAA
		answers = append(answers, &dns.AAAA{Hdr: header, AAAA: ip})
	}
	return answers
}

func recordValues(records []SnapshotRecord) []string {
	result := make([]string, 0, len(records))
	for _, record := range records {
		result = append(result, strings.TrimSpace(record.Value))
	}
	return result
}
