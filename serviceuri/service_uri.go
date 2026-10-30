// Package serviceuri resolves the {apiRoot} of an NF service (TS 29.501 4.4.1)
// from the NFProfile / NFService information returned by NRF discovery
// (TS 29.510). Build works on plain values; ResolveNFServiceURI and
// ResolveFromSearchResult accept the openapi discovery models directly, so
// every NF can share one resolution implementation.
package serviceuri

import (
	"errors"
	"fmt"
	"math/rand"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/free5gc/util/validator"
)

// ErrAddressUnavailable indicates that another matching NFService may be tried.
var ErrAddressUnavailable = errors.New("NFService has no usable FQDN or IP address")

type Endpoint struct {
	IPv4Address string
	IPv6Address string
	Port        int32
}

type Profile struct {
	FQDN          string
	IPv4Addresses []string
	IPv6Addresses []string
}

type Service struct {
	Scheme    string
	FQDN      string
	APIPrefix string
	Endpoints []Endpoint
}

type Options struct {
	InterPLMN   bool
	IndirectSCP bool
	// Legacy compatibility only; 3GPP apiPrefix must be a path, not a URL.
	AllowLegacyAbsoluteAPIPrefix bool
}

type Result struct {
	APIRoot                     string
	UsedLegacyAbsoluteAPIPrefix bool
}

func Build(profile Profile, service Service, opts Options) (Result, error) {
	scheme := strings.ToLower(service.Scheme)
	if scheme != "http" && scheme != "https" {
		return Result{}, fmt.Errorf("invalid NFService.scheme %q", service.Scheme)
	}
	prefix := service.APIPrefix
	if strings.HasPrefix(prefix, "http://") || strings.HasPrefix(prefix, "https://") {
		if !opts.AllowLegacyAbsoluteAPIPrefix {
			return Result{}, fmt.Errorf("non-standard absolute NFService.apiPrefix %q", prefix)
		}
		u, err := url.Parse(prefix)
		if err != nil || u.Scheme != scheme || u.Hostname() == "" || u.User != nil ||
			u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
			return Result{}, fmt.Errorf("invalid legacy apiPrefix URL %q", prefix)
		}
		return Result{APIRoot: strings.TrimSuffix(u.String(), "/"), UsedLegacyAbsoluteAPIPrefix: true}, nil
	}
	if err := validator.ValidateApiPrefix(prefix); err != nil {
		return Result{}, err
	}

	requireFQDN := scheme == "https" || opts.InterPLMN || opts.IndirectSCP
	if requireFQDN || service.FQDN != "" {
		host := service.FQDN
		if host == "" {
			host = profile.FQDN
		}
		if host == "" {
			return Result{}, fmt.Errorf("FQDN is required for scheme %q and selected communication mode", scheme)
		}
		port, err := PortForFQDN(service.Endpoints)
		if err != nil {
			return Result{}, err
		}
		root, err := BuildAPIRoot(scheme, host, port, prefix)
		return Result{APIRoot: root}, err
	}

	candidates := make([]Endpoint, 0, len(service.Endpoints))
	for _, ep := range service.Endpoints {
		if ep.IPv4Address != "" || ep.IPv6Address != "" {
			if err := validator.ValidateIpEndPoint(ep.IPv4Address, ep.IPv6Address, ep.Port); err != nil {
				return Result{}, err
			}
			candidates = append(candidates, ep)
		}
	}
	if len(candidates) != 0 {
		ep := candidates[rand.Intn(len(candidates))]
		host := ep.IPv4Address
		if host == "" {
			host = ep.IPv6Address
		}
		root, err := BuildAPIRoot(scheme, host, ep.Port, prefix)
		return Result{APIRoot: root}, err
	}
	port, err := PortForFQDN(service.Endpoints)
	if err != nil {
		return Result{}, err
	}
	if profile.FQDN != "" {
		root, err := BuildAPIRoot(scheme, profile.FQDN, port, prefix)
		return Result{APIRoot: root}, err
	}
	addresses, err := profileAddresses(profile)
	if err != nil {
		return Result{}, err
	}
	if len(addresses) == 0 {
		return Result{}, ErrAddressUnavailable
	}
	root, err := BuildAPIRoot(scheme, addresses[rand.Intn(len(addresses))], port, prefix)
	return Result{APIRoot: root}, err
}

// profileAddresses returns the NFProfile IPv4 and IPv6 addresses after
// validating that each one is a literal address of the expected family.
func profileAddresses(profile Profile) ([]string, error) {
	addresses := make([]string, 0, len(profile.IPv4Addresses)+len(profile.IPv6Addresses))
	for _, addr := range profile.IPv4Addresses {
		if err := validator.ValidateIpEndPoint(addr, "", 0); err != nil {
			return nil, fmt.Errorf("invalid NFProfile IPv4 address: %w", err)
		}
		addresses = append(addresses, addr)
	}
	for _, addr := range profile.IPv6Addresses {
		if err := validator.ValidateIpEndPoint("", addr, 0); err != nil {
			return nil, fmt.Errorf("invalid NFProfile IPv6 address: %w", err)
		}
		addresses = append(addresses, addr)
	}
	return addresses, nil
}

func PortForFQDN(points []Endpoint) (int32, error) {
	var result int32
	for _, ep := range points {
		if !validator.IsValidPort(ep.Port) {
			return 0, fmt.Errorf("invalid IpEndPoint port: %d", ep.Port)
		}
		if ep.Port == 0 {
			continue
		}
		if result != 0 && result != ep.Port {
			return 0, fmt.Errorf("ambiguous ports for FQDN: %d and %d", result, ep.Port)
		}
		result = ep.Port
	}
	return result, nil
}

func BuildAPIRoot(scheme, host string, port int32, prefix string) (string, error) {
	if !validator.IsValidPort(port) {
		return "", fmt.Errorf("invalid port %d", port)
	}
	if port == 0 {
		port = 80
		if scheme == "https" {
			port = 443
		}
	}
	if host == "" || strings.ContainsAny(host, "/?#@\\ \t\n\r") {
		return "", fmt.Errorf("invalid target host %q", host)
	}
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		host = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
	}
	if strings.Contains(host, ":") && net.ParseIP(host) == nil {
		return "", fmt.Errorf("invalid FQDN/IP host %q", host)
	}
	if strings.Contains(host, "%") {
		return "", fmt.Errorf("zone identifiers are not valid here: %q", host)
	}
	return scheme + "://" + net.JoinHostPort(host, strconv.Itoa(int(port))) + strings.TrimSuffix(prefix, "/"), nil
}
