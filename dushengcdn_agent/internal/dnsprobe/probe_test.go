package dnsprobe

import (
	"context"
	"net"
	"testing"

	"dushengcdn-agent/internal/protocol"

	"github.com/miekg/dns"
)

func TestProbeTargetsReportsUDPAndTCPReachability(t *testing.T) {
	server := &dns.Server{
		Addr: "127.0.0.1:0",
		Net:  "udp",
		Handler: dns.HandlerFunc(func(w dns.ResponseWriter, r *dns.Msg) {
			response := new(dns.Msg)
			response.SetReply(r)
			response.Authoritative = true
			response.Answer = []dns.RR{
				&dns.SOA{
					Hdr:     dns.RR_Header{Name: "example.com.", Rrtype: dns.TypeSOA, Class: dns.ClassINET, Ttl: 60},
					Ns:      "ns1.example.com.",
					Mbox:    "hostmaster.example.com.",
					Serial:  2026060101,
					Refresh: 3600,
					Retry:   600,
					Expire:  86400,
					Minttl:  60,
				},
			}
			_ = w.WriteMsg(response)
		}),
	}
	packetConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen udp: %v", err)
	}
	server.PacketConn = packetConn
	go func() {
		_ = server.ActivateAndServe()
	}()
	t.Cleanup(func() {
		_ = server.Shutdown()
	})

	reports := ProbeTargets(context.Background(), []protocol.DNSProbeTarget{
		{
			WorkerID:      "dns-worker-1",
			Name:          "ns1",
			PublicAddress: packetConn.LocalAddr().String(),
			QueryName:     "example.com",
			QueryType:     "SOA",
		},
	})
	if len(reports) != 1 {
		t.Fatalf("expected one report, got %+v", reports)
	}
	report := reports[0]
	if report.WorkerID != "dns-worker-1" || report.QueryName != "example.com." || report.QueryType != "SOA" {
		t.Fatalf("unexpected report metadata: %+v", report)
	}
	if len(report.Results) != 2 {
		t.Fatalf("expected UDP and TCP results, got %+v", report.Results)
	}
	if report.Results[0].Network != "UDP" || !report.Results[0].Reachable || report.Results[0].RCode != "NOERROR" {
		t.Fatalf("unexpected udp probe result: %+v", report.Results[0])
	}
	if report.Results[1].Network != "TCP" || report.Results[1].Reachable {
		t.Fatalf("expected tcp to fail against udp-only test server, got %+v", report.Results[1])
	}
}

func TestProbeTargetsReportsInvalidAddress(t *testing.T) {
	reports := ProbeTargets(context.Background(), []protocol.DNSProbeTarget{
		{
			WorkerID:      "dns-worker-1",
			PublicAddress: "",
			QueryName:     "example.com",
			QueryType:     "SOA",
		},
	})
	if len(reports) != 1 || len(reports[0].Results) != 2 {
		t.Fatalf("expected invalid address report, got %+v", reports)
	}
	for _, result := range reports[0].Results {
		if result.Reachable || result.Error == "" {
			t.Fatalf("expected failed result with error, got %+v", result)
		}
	}
}
