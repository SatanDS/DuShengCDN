package dnsprobe

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"time"

	"dushengcdn-agent/internal/protocol"

	"github.com/miekg/dns"
)

const defaultTimeout = 3 * time.Second

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
		if strings.TrimSpace(host) == "" {
			return "", errors.New("public address host is empty")
		}
		if strings.TrimSpace(port) == "" {
			return "", errors.New("public address port is empty")
		}
		return net.JoinHostPort(host, port), nil
	}
	host := value
	if parsed := net.ParseIP(strings.Trim(value, "[]")); parsed != nil {
		host = parsed.String()
	}
	return net.JoinHostPort(host, "53"), nil
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
	message := new(dns.Msg)
	message.SetQuestion(dns.Fqdn(qname), queryTypeCode(qtype))
	client := &dns.Client{
		Net:     network,
		Timeout: defaultTimeout,
	}
	startedAt := time.Now()
	response, _, err := client.ExchangeContext(ctx, message, address)
	result.DurationMs = time.Since(startedAt).Milliseconds()
	if err != nil {
		result.Error = err.Error()
		return result
	}
	result.Reachable = true
	result.RCode = dns.RcodeToString[response.Rcode]
	result.AnswerCount = len(response.Answer)
	return result
}
