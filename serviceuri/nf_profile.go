package serviceuri

import (
	"errors"
	"fmt"
	"sort"

	"github.com/free5gc/openapi/models"
)

// ErrServiceNotFound indicates that no REGISTERED NFService with the requested
// service name exists in the NF profile.
var ErrServiceNotFound = errors.New("NFService not found")

// ResolveNFServiceURI returns the API root of the requested service from an
// NF profile obtained through NRF discovery (TS 29.510 6.1.6.2.2 / 6.1.6.2.3).
//
// Only NFService instances in REGISTERED status are considered. Services in
// nfServices are checked first, then nfServiceList in key order so the result
// is reproducible. If a matching service has no usable address
// (ErrAddressUnavailable), the next matching service is tried; any other
// error is returned immediately.
func ResolveNFServiceURI(
	profile *models.Nrf_NFDisc_NFProfile,
	name models.Nrf_NFMgmt_ServiceName,
	opts Options,
) (Result, error) {
	if profile == nil {
		return Result{}, errors.New("NF profile is nil")
	}

	var lastAddrErr error
	for _, service := range matchingServices(profile, name) {
		result, err := Build(toProfile(profile), toService(service), opts)
		if errors.Is(err, ErrAddressUnavailable) {
			lastAddrErr = err
			continue
		}
		if err != nil {
			return Result{}, fmt.Errorf("NFService %q of NF %s: %w", name, profile.NfInstanceId, err)
		}
		return result, nil
	}
	if lastAddrErr != nil {
		return Result{}, fmt.Errorf("NFService %q of NF %s: %w", name, profile.NfInstanceId, lastAddrErr)
	}
	return Result{}, fmt.Errorf("%w: %q in NF %s", ErrServiceNotFound, name, profile.NfInstanceId)
}

// ResolveFromSearchResult returns the first NF instance in an NRF discovery
// result that provides a resolvable API root for the requested service,
// together with that API root. Instances that cannot be resolved are skipped.
func ResolveFromSearchResult(
	instances []models.Nrf_NFDisc_NFProfile,
	name models.Nrf_NFMgmt_ServiceName,
	opts Options,
) (*models.Nrf_NFDisc_NFProfile, Result, error) {
	var lastErr error
	for i := range instances {
		result, err := ResolveNFServiceURI(&instances[i], name, opts)
		if err != nil {
			lastErr = err
			continue
		}
		return &instances[i], result, nil
	}
	if lastErr == nil {
		return nil, Result{}, fmt.Errorf("%w: no NF instance in search result", ErrServiceNotFound)
	}
	return nil, Result{}, fmt.Errorf("no NF instance provides a usable %q API root: %w", name, lastErr)
}

func matchingServices(
	profile *models.Nrf_NFDisc_NFProfile,
	name models.Nrf_NFMgmt_ServiceName,
) []models.Nrf_NFDisc_NFService {
	matches := make([]models.Nrf_NFDisc_NFService, 0, len(profile.NfServices)+len(profile.NfServiceList))
	isMatch := func(s models.Nrf_NFDisc_NFService) bool {
		return s.ServiceName == name && s.NfServiceStatus == models.Nrf_NFMgmt_NFServiceStatus_REGISTERED
	}
	for _, s := range profile.NfServices {
		if isMatch(s) {
			matches = append(matches, s)
		}
	}
	keys := make([]string, 0, len(profile.NfServiceList))
	for k := range profile.NfServiceList {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if s := profile.NfServiceList[k]; isMatch(s) {
			matches = append(matches, s)
		}
	}
	return matches
}

func toProfile(p *models.Nrf_NFDisc_NFProfile) Profile {
	return Profile{
		FQDN:          p.Fqdn,
		IPv4Addresses: p.Ipv4Addresses,
		IPv6Addresses: p.Ipv6Addresses,
	}
}

func toService(s models.Nrf_NFDisc_NFService) Service {
	endpoints := make([]Endpoint, 0, len(s.IpEndPoints))
	for _, ep := range s.IpEndPoints {
		endpoints = append(endpoints, Endpoint{
			IPv4Address: ep.Ipv4Address,
			IPv6Address: ep.Ipv6Address,
			Port:        ep.Port,
		})
	}
	return Service{
		Scheme:    string(s.Scheme),
		FQDN:      s.Fqdn,
		APIPrefix: s.ApiPrefix,
		Endpoints: endpoints,
	}
}
