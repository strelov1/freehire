package companyname

import (
	"context"
	"encoding/json"
	"testing"
)

type fakeText map[string]string

func (f fakeText) GetText(_ context.Context, url string) (string, error) { return f[url], nil }

// GetJSON lets fakeText double as an httpGetter for tests that never need it —
// NewRegistry takes one combined interface for both title-scrape and API resolvers.
func (f fakeText) GetJSON(context.Context, string, any) error { return nil }

// fakeJSON serves canned JSON bodies keyed by URL, for resolvers that call GetJSON.
type fakeJSON map[string]string

func (f fakeJSON) GetText(context.Context, string) (string, error) { return "", nil }

func (f fakeJSON) GetJSON(_ context.Context, url string, v any) error {
	body, ok := f[url]
	if !ok {
		return nil
	}
	return json.Unmarshal([]byte(body), v)
}

func TestTitleResolver(t *testing.T) {
	getter := fakeText{
		"https://lbresearch.pinpointhq.com": `<html><head><title>Jobs at Centellic | Centellic Careers</title></head></html>`,
		"https://empty.pinpointhq.com":      `<html><head><title>Just a moment...</title></head></html>`,
	}
	r := newTitleResolver(getter, "https://%s.pinpointhq.com", ExtractTitleName)

	if got, _ := r.Name(context.Background(), "lbresearch"); got != "Centellic" {
		t.Errorf("Name(lbresearch) = %q, want Centellic", got)
	}
	if got, _ := r.Name(context.Background(), "empty"); got != "" {
		t.Errorf("Name(empty) = %q, want empty", got)
	}
}

// Lever's storefront titles the page with the bare company name — no lead-in, no suffix.
// Live-verified against jobs.lever.co/binance, whose <title> is literally "Binance".
func TestLeverResolverUsesTheBareTitle(t *testing.T) {
	getter := fakeText{
		"https://jobs.lever.co/binance": `<html><head><title>Binance</title></head></html>`,
	}
	reg := NewRegistry(getter)
	if got, _ := reg["lever"].Name(context.Background(), "binance"); got != "Binance" {
		t.Errorf("Name(binance) = %q, want Binance", got)
	}
}

// Ashby's storefront titles itself "{Name} Jobs" — not the Pinpoint "Careers" suffix.
// Live-verified against jobs.ashbyhq.com/airgarage, whose <title> is "AirGarage Jobs".
func TestAshbyResolverUsesTheJobsSuffix(t *testing.T) {
	getter := fakeText{
		"https://jobs.ashbyhq.com/airgarage": `<html><head><title>AirGarage Jobs</title></head></html>`,
	}
	reg := NewRegistry(getter)
	if got, _ := reg["ashby"].Name(context.Background(), "airgarage"); got != "AirGarage" {
		t.Errorf("Name(airgarage) = %q, want AirGarage", got)
	}
}

// Live-verified against join.com's public company-profile endpoint:
// GET https://join.com/api/public/companies/175014 -> {"name":"Goodweek",...}.
func TestJoinResolverUsesTheCompanyProfileAPI(t *testing.T) {
	getter := fakeJSON{
		"https://join.com/api/public/companies/175014": `{"id":175014,"name":"Goodweek","domain":"goodweekcom"}`,
	}
	reg := NewRegistry(getter)
	if got, _ := reg["join"].Name(context.Background(), "175014"); got != "Goodweek" {
		t.Errorf("Name(175014) = %q, want Goodweek", got)
	}
}

func TestRegistryLookup(t *testing.T) {
	reg := NewRegistry(fakeText{})
	for _, src := range []string{"pinpoint", "lever", "ashby", "join"} {
		if _, ok := reg[src]; !ok {
			t.Errorf("registry missing %s resolver", src)
		}
	}
	// Greenhouse job URLs are vanity domains, so it has no URL-derivable board.
	if _, ok := reg["greenhouse"]; ok {
		t.Error("registry should not have a greenhouse resolver")
	}
	// BambooHR's careers page is a client-rendered SPA: the static <title> is always the
	// platform's own boilerplate, never the tenant's name, so no resolver can work here.
	if _, ok := reg["bamboohr"]; ok {
		t.Error("registry should not have a bamboohr resolver — its static title never carries the name")
	}
	if _, ok := reg["nonexistent-ats"]; ok {
		t.Error("registry should not have a resolver for an unknown source")
	}
}
