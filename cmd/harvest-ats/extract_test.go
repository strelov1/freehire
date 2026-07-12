package main

import (
	"reflect"
	"testing"
)

func TestParseUniversitySites(t *testing.T) {
	got, err := parseUniversitySites([]byte(`[
		{"name":"Alpha University","web_pages":["https://alpha.edu/"],"domains":["alpha.edu"]},
		{"name":"Beta College","web_pages":[],"domains":["beta.ac.uk"]},
		{"name":"Ghost","web_pages":[],"domains":[]}
	]`))
	if err != nil {
		t.Fatalf("parseUniversitySites: %v", err)
	}
	want := []companySite{
		{Name: "Alpha University", Website: "https://alpha.edu/"}, // web_pages wins
		{Name: "Beta College", Website: "https://beta.ac.uk"},     // built from domain
		// Ghost dropped: no web page and no domain
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

func TestParseYCSites(t *testing.T) {
	got, err := parseYCSites([]byte(`[
		{"name":"Acme","website":"https://acme.com"},
		{"name":"NoSite","website":""}
	]`))
	if err != nil {
		t.Fatalf("parseYCSites: %v", err)
	}
	want := []companySite{{Name: "Acme", Website: "https://acme.com"}} // empty website skipped
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

func TestParseEUSites(t *testing.T) {
	got, err := parseEUSites([]byte(`[{"Name":"Revolut","Website URL":"https://revolut.com"}]`))
	if err != nil {
		t.Fatalf("parseEUSites: %v", err)
	}
	want := []companySite{{Name: "Revolut", Website: "https://revolut.com"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

func TestParseCSVSites_CommaCompanyWebsite(t *testing.T) {
	csv := "Rank,Company,Website\n1,Walmart,www.walmart.com\n2,NoSite,\n"
	got, err := parseCSVSites([]byte(csv), ',', "Company", "Website")
	if err != nil {
		t.Fatalf("parseCSVSites: %v", err)
	}
	want := []companySite{{Name: "Walmart", Website: "www.walmart.com"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

func TestParseTechstarsSites_FirstURL(t *testing.T) {
	// Techstars CSV is semicolon-separated; the urls column is a comma list whose
	// first entry is the homepage.
	csv := "name;urls;description\n" +
		"Sentry;https://sentry.io, https://twitter.com/x;monitoring\n" +
		"NoUrl;;x"
	got, err := parseTechstarsSites([]byte(csv))
	if err != nil {
		t.Fatalf("parseTechstarsSites: %v", err)
	}
	want := []companySite{{Name: "Sentry", Website: "https://sentry.io"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

func TestFilterUnmatched(t *testing.T) {
	sites := []companySite{
		{Name: "Stripe", Website: "https://stripe.com"},  // slug stripe in set → drop
		{Name: "Unknown Co", Website: "https://unk.com"}, // not in set → keep
	}
	existing := map[string]bool{"stripe": true}
	got := filterUnmatched(sites, existing)
	want := []companySite{{Name: "Unknown Co", Website: "https://unk.com"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}
