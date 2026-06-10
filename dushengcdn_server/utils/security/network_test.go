package security

import (
	"net"
	"testing"
)

func TestValidatePublicIPRejectsSpecialUseRanges(t *testing.T) {
	blocked := []string{
		"0.0.0.1",
		"100.64.0.1",
		"127.0.0.1",
		"169.254.169.254",
		"192.0.2.1",
		"198.18.0.1",
		"198.51.100.1",
		"203.0.113.1",
		"240.0.0.1",
		"100.100.100.200",
		"::1",
		"64:ff9b::808:808",
		"64:ff9b:1::1",
		"100::1",
		"2001:2::1",
		"2001:20::1",
		"2001:db8::1",
		"2002::1",
		"fc00::1",
		"fe80::1",
		"fd00:ec2::254",
	}
	for _, ip := range blocked {
		t.Run(ip, func(t *testing.T) {
			if err := ValidatePublicIP(net.ParseIP(ip)); err == nil {
				t.Fatalf("expected %s to be rejected", ip)
			}
		})
	}
}

func TestValidatePublicIPAllowsPublicResolvers(t *testing.T) {
	for _, ip := range []string{"8.8.8.8", "1.1.1.1", "2606:4700:4700::1111"} {
		t.Run(ip, func(t *testing.T) {
			if err := ValidatePublicIP(net.ParseIP(ip)); err != nil {
				t.Fatalf("expected %s to be accepted: %v", ip, err)
			}
		})
	}
}
