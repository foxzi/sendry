package handlers

import (
	"context"
	"errors"
	"net"
	"reflect"
	"testing"

	"github.com/foxzi/sendry/internal/web/dnsprovider"
)

type fakeZoneProvider struct {
	name string
	zone *dnsprovider.Zone
	err  error
}

func (f fakeZoneProvider) Name() string { return f.name }
func (f fakeZoneProvider) ResolveZone(context.Context, string) (*dnsprovider.Zone, error) {
	return f.zone, f.err
}
func (fakeZoneProvider) ListRecords(context.Context, string, string, string) ([]dnsprovider.Record, error) {
	return nil, nil
}
func (fakeZoneProvider) CreateRecord(context.Context, string, dnsprovider.Record) error { return nil }
func (fakeZoneProvider) UpdateRecord(context.Context, string, string, dnsprovider.Record) error {
	return nil
}

func fakeNS(zones map[string][]string) nsLookupFunc {
	return func(_ context.Context, name string) ([]*net.NS, error) {
		hosts, ok := zones[name]
		if !ok {
			return nil, &net.DNSError{Err: "no such host", Name: name, IsNotFound: true}
		}
		var out []*net.NS
		for _, h := range hosts {
			out = append(out, &net.NS{Host: h})
		}
		return out, nil
	}
}

func TestCheckDNSHosting(t *testing.T) {
	providers := []dnsprovider.Provider{
		fakeZoneProvider{name: "cloudflare", zone: &dnsprovider.Zone{ID: "1", Name: "example.com"}},
		fakeZoneProvider{name: "namedot", err: dnsprovider.ErrZoneNotFound},
		fakeZoneProvider{name: "other", err: errors.New("403 forbidden")},
	}
	lookup := fakeNS(map[string][]string{"example.com": {"Lisa.NS.Cloudflare.com.", "bob.ns.cloudflare.com."}})

	got := checkDNSHosting(context.Background(), "Mail.Example.com.", providers, nil, lookup)

	if got.NSError != "" || got.NSZone != "example.com" {
		t.Fatalf("NS zone = %q, err = %q", got.NSZone, got.NSError)
	}
	if want := []string{"bob.ns.cloudflare.com", "lisa.ns.cloudflare.com"}; !reflect.DeepEqual(got.NS, want) {
		t.Errorf("NS = %v, want %v", got.NS, want)
	}
	want := []providerZone{
		{Key: "cloudflare", Provider: "Cloudflare", Zone: "example.com", Found: true},
		{Key: "namedot", Provider: "namedot"},
		{Key: "other", Provider: "other", Error: "403 forbidden"},
	}
	if !reflect.DeepEqual(got.Providers, want) {
		t.Errorf("providers = %+v, want %+v", got.Providers, want)
	}
}

func TestFindNS(t *testing.T) {
	if _, _, msg := findNS(context.Background(), "example.org", fakeNS(nil)); msg == "" {
		t.Error("missing NS should be reported")
	}

	failing := func(context.Context, string) ([]*net.NS, error) {
		return nil, &net.DNSError{Err: "i/o timeout", IsTimeout: true}
	}
	if _, _, msg := findNS(context.Background(), "example.org", failing); msg == "" {
		t.Error("lookup failure should be reported")
	}
}
