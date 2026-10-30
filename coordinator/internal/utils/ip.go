package utils

import (
	"net"
	"strings"
)

func SameIP(a, b string) bool {
	ipA := parseIP(a)
	ipB := parseIP(b)
	if ipA == nil || ipB == nil {
		return false
	}

	return ipA.Equal(ipB)
}

func parseIP(s string) net.IP {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}

	if host, _, err := net.SplitHostPort(s); err == nil {
		s = host
	}

	return net.ParseIP(strings.Trim(s, "[]"))
}
