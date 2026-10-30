package serviceuri

import (
	"errors"
	"fmt"
	"math/rand"
	"sort"

	"github.com/free5gc/openapi/models"
)

// ErrServiceNotFound indicates that no REGISTERED NFService with the requested
// service name exists in the NF profile(s).
var ErrServiceNotFound = errors.New("NFService not found")

// randInt63n and shuffle are the random sources used for NF (service)
// selection. Tests replace them to make the selection deterministic.
var (
	randInt63n = rand.Int63n
	shuffle    = rand.Shuffle
)

// candidate is one REGISTERED NFService (with its NF profile) that offers the
// requested service.
type candidate struct {
	profile  *models.Nrf_NFDisc_NFProfile
	service  models.Nrf_NFDisc_NFService
	priority int32
	capacity int32
}

// ResolveNFServiceURI returns the API root of the requested service from an
// NF profile obtained through NRF discovery (TS 29.510 6.1.6.2.2 / 6.1.6.2.3).
//
// Only NFService instances in REGISTERED status are considered. When several
// services match, they are tried in the order given by their priority and
// capacity (see orderCandidates). If a service has no usable address
// (ErrAddressUnavailable), the next one is tried; any other error is returned.
func ResolveNFServiceURI(
	profile *models.Nrf_NFDisc_NFProfile,
	name models.Nrf_NFMgmt_ServiceName,
	opts Options,
) (Result, error) {
	if profile == nil {
		return Result{}, errors.New("NF profile is nil")
	}

	var lastAddrErr error
	for _, c := range orderCandidates(matchingCandidates(profile, name)) {
		result, err := Build(toProfile(profile), toService(c.service), opts)
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

// ResolveFromSearchResult selects an NF (service) instance from an NRF
// discovery result and returns the selected NF profile and its API root.
//
// All REGISTERED NFService instances offering the requested service, across
// all NF profiles, are tried in priority / capacity order (see
// orderCandidates). Candidates that cannot be resolved are skipped.
func ResolveFromSearchResult(
	instances []models.Nrf_NFDisc_NFProfile,
	name models.Nrf_NFMgmt_ServiceName,
	opts Options,
) (*models.Nrf_NFDisc_NFProfile, Result, error) {
	var all []candidate
	for i := range instances {
		all = append(all, matchingCandidates(&instances[i], name)...)
	}
	if len(all) == 0 {
		return nil, Result{}, fmt.Errorf("%w: %q in NRF search result", ErrServiceNotFound, name)
	}

	var lastErr error
	for _, c := range orderCandidates(all) {
		result, err := Build(toProfile(c.profile), toService(c.service), opts)
		if err != nil {
			lastErr = fmt.Errorf("NFService %q of NF %s: %w", name, c.profile.NfInstanceId, err)
			continue
		}
		return c.profile, result, nil
	}
	return nil, Result{}, fmt.Errorf("no NF instance provides a usable %q API root: %w", name, lastErr)
}

// matchingCandidates returns the REGISTERED services named name in profile,
// nfServices first and then nfServiceList in key order.
//
// TS 29.510 6.1.6.2.2: priority and capacity in the NFService have precedence
// over those in the NFProfile. The generated models cannot tell an absent
// value from 0, so a service value of 0 falls back to the NFProfile value.
func matchingCandidates(profile *models.Nrf_NFDisc_NFProfile, name models.Nrf_NFMgmt_ServiceName) []candidate {
	var out []candidate
	add := func(s models.Nrf_NFDisc_NFService) {
		if s.ServiceName != name || s.NfServiceStatus != models.Nrf_NFMgmt_NFServiceStatus_REGISTERED {
			return
		}
		c := candidate{profile: profile, service: s, priority: s.Priority, capacity: s.Capacity}
		if c.priority == 0 {
			c.priority = profile.Priority
		}
		if c.capacity == 0 {
			c.capacity = profile.Capacity
		}
		out = append(out, c)
	}
	for _, s := range profile.NfServices {
		add(s)
	}
	keys := make([]string, 0, len(profile.NfServiceList))
	for k := range profile.NfServiceList {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		add(profile.NfServiceList[k])
	}
	return out
}

// orderCandidates orders candidates for selection as required by TS 29.510
// 6.1.6.2.2 NOTE 4 / 6.1.6.2.3 NOTE 2: priority and capacity are used the same
// way as priority and weight for server selection in IETF RFC 2782.
//
// Lower priority values are tried first. Within one priority, the order is
// built with the RFC 2782 weight algorithm: entries with weight 0 are placed
// first, a uniform random number between 0 and the sum of the weights
// (inclusive) is chosen, and the first entry whose running sum is greater
// than or equal to it is selected; this is repeated for the remaining entries.
func orderCandidates(cands []candidate) []candidate {
	sort.SliceStable(cands, func(i, j int) bool { return cands[i].priority < cands[j].priority })

	ordered := make([]candidate, 0, len(cands))
	for start := 0; start < len(cands); {
		end := start
		for end < len(cands) && cands[end].priority == cands[start].priority {
			end++
		}
		ordered = append(ordered, orderByWeight(cands[start:end])...)
		start = end
	}
	return ordered
}

func orderByWeight(group []candidate) []candidate {
	remaining := append([]candidate(nil), group...)
	shuffle(len(remaining), func(i, j int) { remaining[i], remaining[j] = remaining[j], remaining[i] })

	ordered := make([]candidate, 0, len(group))
	for len(remaining) > 0 {
		sort.SliceStable(remaining, func(i, j int) bool {
			return remaining[i].capacity == 0 && remaining[j].capacity != 0
		})
		var sum int64
		for _, c := range remaining {
			sum += int64(c.capacity)
		}
		r := randInt63n(sum + 1)
		var running int64
		pick := len(remaining) - 1
		for i, c := range remaining {
			running += int64(c.capacity)
			if running >= r {
				pick = i
				break
			}
		}
		ordered = append(ordered, remaining[pick])
		remaining = append(remaining[:pick], remaining[pick+1:]...)
	}
	return ordered
}

func toProfile(p *models.Nrf_NFDisc_NFProfile) Profile {
	return Profile{
		FQDN:          p.Fqdn,
		InterPlmnFQDN: p.InterPlmnFqdn,
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
		Scheme:        string(s.Scheme),
		FQDN:          s.Fqdn,
		InterPlmnFQDN: s.InterPlmnFqdn,
		APIPrefix:     s.ApiPrefix,
		Endpoints:     endpoints,
	}
}
