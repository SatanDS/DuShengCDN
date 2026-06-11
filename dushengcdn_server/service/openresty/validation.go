package openresty

import (
	"dushengcdn/utils/security"
	"errors"
	"net"
	"strconv"
	"strings"
)

func normalizeOriginPort(raw string) (string, error) {
	port := strings.TrimSpace(raw)
	if port == "" {
		return "", errors.New("绔彛涓嶈兘涓虹┖")
	}
	value, err := strconv.Atoi(port)
	if err != nil || value < 1 || value > 65535 {
		return "", errors.New("绔彛鏍煎紡涓嶅悎娉?")
	}
	return strconv.Itoa(value), nil
}

func validatePublicOriginHost(host string) error {
	if err := security.ValidatePublicHostname(host); err != nil {
		return err
	}
	if ip := net.ParseIP(strings.Trim(host, "[]")); ip != nil {
		return security.ValidatePublicIP(ip)
	}
	return nil
}

func normalizeProxyBufferingMode(raw string) string {
	if strings.EqualFold(strings.TrimSpace(raw), ProxyBufferingModeOff) {
		return ProxyBufferingModeOff
	}
	return ""
}
