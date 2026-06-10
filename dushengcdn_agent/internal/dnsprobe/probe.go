package dnsprobe

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"time"

	"dushengcdn-agent/internal/protocol"
	"dushengcdn-agent/internal/security"

	"github.com/miekg/dns"
)

const defaultTimeout = 3 * time.Second

var lookupIPAddr = net.DefaultResolver.LookupIPAddr

func ProbeTargets(ctx context.Context, targets []protocol.DNSProbeTarget) []protocol.DNSProbeReport {
	if len(targets) == 0 {
		return nil
	}
	slots := make([]probeTargetResult, len(targets))
	var wg sync.WaitGroup
	for index, target := range targets {
		wg.Add(1)
		go func(index int, target protocol.DNSProbeTarget) {
			defer wg.Done()
			report, ok := probeTarget(ctx, target)
			slots[index] = probeTargetResult{report: report, ok: ok}
		}(index, target)
	}
	wg.Wait()
	reports := make([]protocol.DNSProbeReport, 0, len(targets))
	for _, slot := range slots {
		if !slot.ok {
			continue
		}
		reports = append(reports, slot.report)
	}
	return reports
}

type probeTargetResult struct {
	report protocol.DNSProbeReport
	ok     bool
}

func probeTarget(ctx context.Context, target protocol.DNSProbeTarget) (protocol.DNSProbeReport, bool) {
	workerID := strings.TrimSpace(target.WorkerID)
	address, err := normalizeAddress(target.PublicAddress)
	report := protocol.DNSProbeReport{
		WorkerID:      workerID,
		Name:          strings.TrimSpace(target.Name),
		PublicAddress: strings.TrimSpace(target.PublicAddress),
		QueryName:     dns.Fqdn(strings.TrimSpace(target.QueryName)),
		QueryType:     normalizeQueryType(target.QueryType),
		CheckedAtUnix: time.Now().UTC().Unix(),
		Results:       make([]protocol.DNSProbeResult, 0, 2),
	}
	if workerID == "" || report.QueryName == "." {
		return protocol.DNSProbeReport{}, false
	}
	if err != nil {
		report.PublicAddress = strings.TrimSpace(target.PublicAddress)
		report.Results = append(report.Results, protocol.DNSProbeResult{
			Network: "UDP",
			Error:   err.Error(),
		})
		report.Results = append(report.Results, protocol.DNSProbeResult{
			Network: "TCP",
			Error:   err.Error(),
		})
		return report, true
	}
	report.Results = probeAddress(ctx, address, report.QueryName, report.QueryType)
	return report, true
}

func probeAddress(ctx context.Context, address string, qname string, qtype string) []protocol.DNSProbeResult {
	results := make([]protocol.DNSProbeResult, 2)
	var wg sync.WaitGroup
	for index, network := range []string{"udp", "tcp"} {
		wg.Add(1)
		go func(index int, network string) {
			defer wg.Done()
			results[index] = exchange(ctx, address, network, qname, qtype)
		}(index, network)
	}
	wg.Wait()
	return results
}

func normalizeAddress(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", errors.New("public address is empty")
	}
	if host, port, err := net.SplitHostPort(value); err == nil {
		return normalizeHostPort(host, port)
	}
	host := value
	if parsed := net.ParseIP(strings.Trim(value, "[]")); parsed != nil {
		host = parsed.String()
	}
	return normalizeHostPort(host, "53")
}

func normalizeHostPort(host string, port string) (string, error) {
	host = strings.TrimSpace(strings.Trim(host, "[]"))
	port = strings.TrimSpace(port)
	if host == "" {
		return "", errors.New("public address host is empty")
	}
	if err := security.ValidatePublicHostname(host); err != nil {
		return "", err
	}
	if port == "" {
		return "", errors.New("public address port is empty")
	}
	if port != "53" {
		return "", errors.New("public address port must be 53")
	}
	if ip := net.ParseIP(host); ip != nil {
		host = ip.String()
	}
	return net.JoinHostPort(host, port), nil
}

func normalizeQueryType(value string) string {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "A", "AAAA", "NS":
		return strings.ToUpper(strings.TrimSpace(value))
	default:
		return "SOA"
	}
}

func queryTypeCode(value string) uint16 {
	switch normalizeQueryType(value) {
	case "A":
		return dns.TypeA
	case "AAAA":
		return dns.TypeAAAA
	case "NS":
		return dns.TypeNS
	default:
		return dns.TypeSOA
	}
}

func exchange(ctx context.Context, address string, network string, qname string, qtype string) protocol.DNSProbeResult {
	result := protocol.DNSProbeResult{
		Network: strings.ToUpper(network),
	}
	targets, err := resolvePublicProbeTargets(ctx, address)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	message := new(dns.Msg)
	message.SetQuestion(dns.Fqdn(qname), queryTypeCode(qtype))
	client := &dns.Client{
		Net:     network,
		Timeout: defaultTimeout,
	}
	startedAt := time.Now()
	var lastErr error
	for _, target := range targets {
		response, _, err := client.ExchangeContext(ctx, message, target)
		if err != nil {
			lastErr = err
			continue
		}
		result.DurationMs = time.Since(startedAt).Milliseconds()
		result.Reachable = true
		result.RCode = dns.RcodeToString[response.Rcode]
		result.AnswerCount = len(response.Answer)
		return result
	}
	result.DurationMs = time.Since(startedAt).Milliseconds()
	if lastErr != nil {
		result.Error = lastErr.Error()
		return result
	}
	result.Error = "public address has no dialable addresses"
	return result
}

func resolvePublicProbeTargets(ctx context.Context, address string) ([]string, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	if port != "53" {
		return nil, errors.New("public address port must be 53")
	}
	if err := security.ValidatePublicHostname(host); err != nil {
		return nil, err
	}
	if ip := net.ParseIP(strings.Trim(host, "[]")); ip != nil {
		if err := security.ValidatePublicIP(ip); err != nil {
			return nil, err
		}
		return []string{net.JoinHostPort(ip.String(), port)}, nil
	}
	lookupCtx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()
	addresses, err := lookupIPAddr(lookupCtx, host)
	if err != nil {
		return nil, err
	}
	if len(addresses) == 0 {
		return nil, errors.New("public address host has no addresses")
	}
	targets := make([]string, 0, len(addresses))
	for _, address := range addresses {
		if err := security.ValidatePublicIP(address.IP); err != nil {
			return nil, err
		}
		targets = append(targets, net.JoinHostPort(address.IP.String(), port))
	}
	return targets, nil
}
