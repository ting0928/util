package validator

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// IsValidPort checks whether port is within the TCP port range.
// 0 is accepted and means the port is not specified (scheme default applies).
func IsValidPort(port int32) bool {
	return port >= 0 && port <= 65535
}

// ValidateIpEndPoint validates the address and port of an NFService IpEndPoint
// (TS 29.510 6.1.6.2.5). At most one of ipv4 and ipv6 may be set.
func ValidateIpEndPoint(ipv4, ipv6 string, port int32) error {
	if ipv4 != "" && ipv6 != "" {
		return fmt.Errorf("IpEndPoint cannot contain both IPv4 and IPv6")
	}
	if ipv4 != "" {
		ip := net.ParseIP(ipv4)
		if ip == nil || ip.To4() == nil {
			return fmt.Errorf("invalid IpEndPoint IPv4: %q", ipv4)
		}
	}
	if ipv6 != "" {
		ip := net.ParseIP(ipv6)
		if ip == nil || ip.To4() != nil {
			return fmt.Errorf("invalid IpEndPoint IPv6: %q", ipv6)
		}
	}
	if !IsValidPort(port) {
		return fmt.Errorf("invalid IpEndPoint port: %d", port)
	}
	return nil
}

// ValidateApiPrefix validates NFService.apiPrefix (TS 29.510 6.1.6.2.3).
// An empty prefix is valid; otherwise it must be an absolute path such as
// "/operator/edge", not a URL.
func ValidateApiPrefix(prefix string) error {
	if prefix == "" {
		return nil
	}
	if !strings.HasPrefix(prefix, "/") || strings.HasPrefix(prefix, "//") || strings.ContainsAny(prefix, "?#\\") {
		return fmt.Errorf("invalid NFService.apiPrefix path %q", prefix)
	}
	for _, r := range prefix {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("control character in NFService.apiPrefix")
		}
	}
	u, err := url.Parse(prefix)
	if err != nil || u.IsAbs() || u.Host != "" || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("invalid NFService.apiPrefix %q", prefix)
	}
	// The prefix is copied into the API root as-is, so it must already be a
	// correctly escaped path (e.g. "/a%20b", not "/a b").
	if u.EscapedPath() != prefix {
		return fmt.Errorf("NFService.apiPrefix %q is not a properly escaped path", prefix)
	}
	return nil
}
