package serviceuri

import (
	"errors"
	"strings"
	"testing"

	"github.com/free5gc/openapi/models"
)

const testServiceName = models.Nrf_NFMgmt_ServiceName_NAMF_COMM

func registered(s models.Nrf_NFDisc_NFService) models.Nrf_NFDisc_NFService {
	s.ServiceName = testServiceName
	s.NfServiceStatus = models.Nrf_NFMgmt_NFServiceStatus_REGISTERED
	return s
}

func ipv4Service(ip string, port int32) models.Nrf_NFDisc_NFService {
	return registered(models.Nrf_NFDisc_NFService{
		Scheme:      models.UriScheme_HTTP,
		IpEndPoints: []models.Nrf_NFMgmt_IpEndPoint{{Ipv4Address: ip, Port: port}},
	})
}

func TestResolveNFServiceURISelection(t *testing.T) {
	const want = "http://10.101.200.28:8000"

	valid := ipv4Service("10.101.200.28", 8000)
	wrongName := valid
	wrongName.ServiceName = models.Nrf_NFMgmt_ServiceName_NUDR_DR
	notRegistered := valid
	notRegistered.NfServiceStatus = models.Nrf_NFMgmt_NFServiceStatus_SUSPENDED
	noAddress := registered(models.Nrf_NFDisc_NFService{Scheme: models.UriScheme_HTTP})

	tests := []struct {
		name     string
		services []models.Nrf_NFDisc_NFService
		want     string
		wantErr  error
	}{
		{"No_services", nil, "", ErrServiceNotFound},
		{"Wrong_service_name", []models.Nrf_NFDisc_NFService{wrongName}, "", ErrServiceNotFound},
		{"Not_registered", []models.Nrf_NFDisc_NFService{notRegistered}, "", ErrServiceNotFound},
		{"No_usable_address", []models.Nrf_NFDisc_NFService{noAddress}, "", ErrAddressUnavailable},
		{"Skip_wrong_name", []models.Nrf_NFDisc_NFService{wrongName, valid}, want, nil},
		{"Skip_not_registered", []models.Nrf_NFDisc_NFService{notRegistered, valid}, want, nil},
		{"Skip_no_usable_address", []models.Nrf_NFDisc_NFService{noAddress, valid}, want, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile := &models.Nrf_NFDisc_NFProfile{NfServices: tt.services}
			got, err := ResolveNFServiceURI(profile, testServiceName, Options{})
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil || got.APIRoot != tt.want {
				t.Fatalf("got %q, error %v; want %q", got.APIRoot, err, tt.want)
			}
		})
	}
}

func TestResolveNFServiceURIStandardFields(t *testing.T) {
	tests := []struct {
		name    string
		profile models.Nrf_NFDisc_NFProfile
		want    string
	}{
		{
			name: "HTTP_service_FQDN_default_port",
			profile: models.Nrf_NFDisc_NFProfile{NfServices: []models.Nrf_NFDisc_NFService{
				registered(models.Nrf_NFDisc_NFService{Scheme: models.UriScheme_HTTP, Fqdn: "amf.example.org"}),
			}},
			want: "http://amf.example.org:80",
		},
		{
			name: "HTTPS_service_FQDN_port_and_prefix",
			profile: models.Nrf_NFDisc_NFProfile{NfServices: []models.Nrf_NFDisc_NFService{
				registered(models.Nrf_NFDisc_NFService{
					Scheme:      models.UriScheme_HTTPS,
					Fqdn:        "amf.example.org",
					ApiPrefix:   "/sbi",
					IpEndPoints: []models.Nrf_NFMgmt_IpEndPoint{{Port: 8443}},
				}),
			}},
			want: "https://amf.example.org:8443/sbi",
		},
		{
			// free5gc/free5gc#1173: OAI UDR registers a profile-level FQDN and a
			// service-level IP endpoint. With HTTP the service IP takes precedence.
			name: "OAI_profile_FQDN_with_service_IP",
			profile: models.Nrf_NFDisc_NFProfile{
				Fqdn:       "oai-udr",
				NfServices: []models.Nrf_NFDisc_NFService{ipv4Service("192.168.70.136", 8080)},
			},
			want: "http://192.168.70.136:8080",
		},
		{
			name: "Port_only_with_profile_FQDN",
			profile: models.Nrf_NFDisc_NFProfile{
				Fqdn: "profile.example.org",
				NfServices: []models.Nrf_NFDisc_NFService{
					registered(models.Nrf_NFDisc_NFService{
						Scheme:      models.UriScheme_HTTP,
						IpEndPoints: []models.Nrf_NFMgmt_IpEndPoint{{Port: 8080}},
					}),
				},
			},
			want: "http://profile.example.org:8080",
		},
		{
			name: "HTTPS_profile_FQDN_default_port",
			profile: models.Nrf_NFDisc_NFProfile{
				Fqdn: "profile.example.org",
				NfServices: []models.Nrf_NFDisc_NFService{
					registered(models.Nrf_NFDisc_NFService{Scheme: models.UriScheme_HTTPS}),
				},
			},
			want: "https://profile.example.org:443",
		},
		{
			name: "HTTP_IPv6_with_prefix",
			profile: models.Nrf_NFDisc_NFProfile{NfServices: []models.Nrf_NFDisc_NFService{
				registered(models.Nrf_NFDisc_NFService{
					Scheme:      models.UriScheme_HTTP,
					ApiPrefix:   "/api",
					IpEndPoints: []models.Nrf_NFMgmt_IpEndPoint{{Ipv6Address: "2001:db8::1", Port: 8000}},
				}),
			}},
			want: "http://[2001:db8::1]:8000/api",
		},
		{
			name: "NFServiceList_only",
			profile: models.Nrf_NFDisc_NFProfile{NfServiceList: map[string]models.Nrf_NFDisc_NFService{
				"comm": registered(models.Nrf_NFDisc_NFService{Scheme: models.UriScheme_HTTP, Fqdn: "amf.example.org"}),
			}},
			want: "http://amf.example.org:80",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveNFServiceURI(&tt.profile, testServiceName, Options{})
			if err != nil || got.APIRoot != tt.want {
				t.Fatalf("got %q, error %v; want %q", got.APIRoot, err, tt.want)
			}
		})
	}
}

func TestResolveNFServiceURIInvalidInput(t *testing.T) {
	tests := []struct {
		name   string
		scheme models.UriScheme
		prefix string
		fqdn   string
	}{
		{"Missing_scheme", "", "/x", "amf.example.org"},
		{"Bad_scheme", "ftp", "/x", "amf.example.org"},
		{"Relative_prefix", models.UriScheme_HTTP, "bad", "amf.example.org"},
		{"Query_prefix", models.UriScheme_HTTP, "/x?admin=1", "amf.example.org"},
		{"Authority_prefix", models.UriScheme_HTTP, "//evil.example.org", "amf.example.org"},
		{"Absolute_prefix", models.UriScheme_HTTP, "http://evil.example.org", "amf.example.org"},
		{"Missing_FQDN_for_HTTPS", models.UriScheme_HTTPS, "/x", ""},
		{"URL_as_FQDN", models.UriScheme_HTTP, "/x", "http://evil.example.org"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile := &models.Nrf_NFDisc_NFProfile{NfServices: []models.Nrf_NFDisc_NFService{
				registered(models.Nrf_NFDisc_NFService{Scheme: tt.scheme, Fqdn: tt.fqdn, ApiPrefix: tt.prefix}),
			}}
			if got, err := ResolveNFServiceURI(profile, testServiceName, Options{}); err == nil {
				t.Fatalf("accepted invalid input: %q", got.APIRoot)
			}
		})
	}
}

func TestResolveNFServiceURIHTTPSNeverUsesBareIP(t *testing.T) {
	profile := &models.Nrf_NFDisc_NFProfile{NfServices: []models.Nrf_NFDisc_NFService{
		registered(models.Nrf_NFDisc_NFService{
			Scheme:      models.UriScheme_HTTPS,
			IpEndPoints: []models.Nrf_NFMgmt_IpEndPoint{{Ipv4Address: "192.0.2.10", Port: 8443}},
		}),
	}}
	got, err := ResolveNFServiceURI(profile, testServiceName, Options{})
	if err == nil || strings.Contains(got.APIRoot, "192.0.2.10") {
		t.Fatalf("HTTPS must not fall back to a bare IP: got %q, error %v", got.APIRoot, err)
	}
}

func TestResolveNFServiceURILegacyAbsolutePrefix(t *testing.T) {
	profile := &models.Nrf_NFDisc_NFProfile{NfServices: []models.Nrf_NFDisc_NFService{
		registered(models.Nrf_NFDisc_NFService{
			Scheme:      models.UriScheme_HTTP,
			ApiPrefix:   "http://amf.free5gc.org:8000",
			IpEndPoints: []models.Nrf_NFMgmt_IpEndPoint{{Ipv4Address: "192.0.2.11", Port: 8000}},
		}),
	}}

	if _, err := ResolveNFServiceURI(profile, testServiceName, Options{}); err == nil {
		t.Fatal("strict mode must reject a non-standard absolute apiPrefix")
	}

	got, err := ResolveNFServiceURI(profile, testServiceName, Options{AllowLegacyAbsoluteAPIPrefix: true})
	if err != nil || got.APIRoot != "http://amf.free5gc.org:8000" || !got.UsedLegacyAbsoluteAPIPrefix {
		t.Fatalf("legacy mode returned %+v, error %v", got, err)
	}
}

func TestResolveNFServiceURINilProfile(t *testing.T) {
	if _, err := ResolveNFServiceURI(nil, testServiceName, Options{}); err == nil {
		t.Fatal("nil profile must return an error")
	}
}

func TestResolveFromSearchResult(t *testing.T) {
	withoutService := models.Nrf_NFDisc_NFProfile{NfInstanceId: "no-service"}
	invalid := models.Nrf_NFDisc_NFProfile{
		NfInstanceId: "invalid",
		NfServices: []models.Nrf_NFDisc_NFService{
			registered(models.Nrf_NFDisc_NFService{Scheme: "ftp", Fqdn: "amf.example.org"}),
		},
	}
	valid := models.Nrf_NFDisc_NFProfile{
		NfInstanceId: "valid",
		NfServices:   []models.Nrf_NFDisc_NFService{ipv4Service("10.0.0.1", 8000)},
	}

	t.Run("Skips_unusable_instances", func(t *testing.T) {
		instances := []models.Nrf_NFDisc_NFProfile{withoutService, invalid, valid}
		profile, got, err := ResolveFromSearchResult(instances, testServiceName, Options{})
		if err != nil {
			t.Fatalf("error = %v", err)
		}
		if profile == nil || profile.NfInstanceId != "valid" {
			t.Fatalf("selected profile = %+v, want instance %q", profile, "valid")
		}
		if profile != &instances[2] {
			t.Fatal("returned profile must point into the given slice")
		}
		if got.APIRoot != "http://10.0.0.1:8000" {
			t.Fatalf("APIRoot = %q", got.APIRoot)
		}
	})

	t.Run("Empty_result", func(t *testing.T) {
		if _, _, err := ResolveFromSearchResult(nil, testServiceName, Options{}); !errors.Is(err, ErrServiceNotFound) {
			t.Fatalf("error = %v, want ErrServiceNotFound", err)
		}
	})

	t.Run("No_usable_instance", func(t *testing.T) {
		instances := []models.Nrf_NFDisc_NFProfile{withoutService, invalid}
		if _, _, err := ResolveFromSearchResult(instances, testServiceName, Options{}); err == nil {
			t.Fatal("expected an error when no instance is usable")
		}
	})
}
