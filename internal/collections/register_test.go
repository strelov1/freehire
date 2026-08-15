package collections

import (
	"reflect"
	"testing"
)

func TestRegistry_HasEverySponsorCredential(t *testing.T) {
	for _, slug := range []string{"uk-skilled-worker-sponsor", "nl-recognised-sponsor", "us-h1b-sponsor"} {
		c, ok := Lookup(slug)
		if !ok {
			t.Errorf("registry missing %q", slug)
			continue
		}
		if c.Kind != KindCredential {
			t.Errorf("%s kind = %q, want %q", slug, c.Kind, KindCredential)
		}
		if c.Title == "" || c.Description == "" {
			t.Errorf("%s missing display copy: %+v", slug, c)
		}
		if c.Dataset == nil || !c.Dataset.Valid() {
			t.Errorf("%s has no single dataset source", slug)
		}
		if c.Gate == nil {
			t.Errorf("%s has no gate — a credential must never tag on a bare name match", slug)
		}
	}
}

func TestUKSponsorEntry_GateRejectsATemporaryWorkerFarm(t *testing.T) {
	c, ok := Lookup("uk-skilled-worker-sponsor")
	if !ok {
		t.Fatal("uk-skilled-worker-sponsor missing")
	}
	farm := Company{Slug: "green-fields-farm", Countries: []string{"GB"}, HQCountry: "GB"}
	seasonal := Record{Name: "Green Fields Farm Ltd", Meta: map[string]string{"route": "Temporary Worker - Seasonal Worker"}}
	if c.Admits(farm, seasonal) {
		t.Error("a seasonal-worker licence earned the skilled-worker credential")
	}
	skilled := Record{Name: "Green Fields Farm Ltd", Meta: map[string]string{"route": "Skilled Worker"}}
	if !c.Admits(farm, skilled) {
		t.Error("a skilled-worker row was rejected")
	}
}

func TestNLSponsorEntry_GateIsGeographyOnly(t *testing.T) {
	c, ok := Lookup("nl-recognised-sponsor")
	if !ok {
		t.Fatal("nl-recognised-sponsor missing")
	}
	// The IND register publishes no route breakdown, so recognition plus presence is
	// the whole test — but presence still has to hold.
	adyen := Company{Slug: "adyen", Countries: []string{"NL"}, HQCountry: "NL"}
	if !c.Admits(adyen, Record{Name: "Adyen N.V."}) {
		t.Error("a Dutch-headquartered recognised sponsor was rejected")
	}
	elsewhere := Company{Slug: "adyen", Countries: []string{"US"}, HQCountry: "US"}
	if c.Admits(elsewhere, Record{Name: "Adyen N.V."}) {
		t.Error("a company with no Dutch presence earned the Dutch credential")
	}
}

func TestUSH1BSponsorEntry_GateIsGeographyOnly(t *testing.T) {
	c, ok := Lookup("us-h1b-sponsor")
	if !ok {
		t.Fatal("us-h1b-sponsor missing")
	}
	// The USCIS Data Hub is H-1B-specific already, so — like the Dutch register and
	// unlike the UK register — there is no route to gate on: presence is the whole
	// test, but presence still has to hold.
	stripe := Company{Slug: "stripe", Countries: []string{"US"}, HQCountry: "US"}
	if !c.Admits(stripe, Record{Name: "STRIPE"}) {
		t.Error("a US-headquartered approved sponsor was rejected")
	}
	elsewhere := Company{Slug: "stripe", Countries: []string{"NL"}, HQCountry: "NL"}
	if c.Admits(elsewhere, Record{Name: "STRIPE"}) {
		t.Error("a company with no US presence earned the US credential")
	}
}

func TestRequireCountry_MultiTokenNameMatchesOnTheCountryFacet(t *testing.T) {
	gate := RequireCountry("GB")
	co := Company{Slug: "acme-robotics", Countries: []string{"GB", "US"}}
	if !gate(co, Record{Name: "ACME ROBOTICS LIMITED"}) {
		t.Error("a two-token name with GB jobs was rejected")
	}
}

func TestRequireCountry_RejectsACompanyWithNoJobsInTheRegistersCountry(t *testing.T) {
	gate := RequireCountry("GB")
	co := Company{Slug: "acme-robotics", Countries: []string{"US"}, HQCountry: "US"}
	if gate(co, Record{Name: "ACME ROBOTICS LIMITED"}) {
		t.Error("a company with no GB presence earned a UK credential")
	}
}

func TestRequireCountry_SingleTokenNameNeedsAMatchingHeadquarters(t *testing.T) {
	gate := RequireCountry("GB")

	// Apple hires in London, so the country facet alone would admit it — and hand it
	// a licence belonging to some unrelated "APPLE LTD". The headquarters rule is
	// what stops that.
	apple := Company{Slug: "apple", Countries: []string{"GB", "US"}, HQCountry: "US"}
	if gate(apple, Record{Name: "APPLE LTD"}) {
		t.Error("a single-token match was admitted on the country facet alone")
	}

	// Monzo is single-token and genuinely British; it must still pass.
	monzo := Company{Slug: "monzo", Countries: []string{"GB"}, HQCountry: "GB"}
	if !gate(monzo, Record{Name: "MONZO BANK LTD"}) {
		t.Error("a UK-headquartered single-token company was rejected")
	}
}

func TestRequireCountry_PunctuatedSingleTokenNameNeedsAMatchingHeadquarters(t *testing.T) {
	// "T-Mobile Inc" has two whitespace tokens. After significantFields strips the
	// trailing legal form, "T-Mobile" is the one significant token left; its
	// internal hyphen must not make RequireCountry treat it as multi-token and
	// skip the headquarters check.
	gate := RequireCountry("GB")

	elsewhere := Company{Slug: "t-mobile", Countries: []string{"GB", "US"}, HQCountry: "US"}
	if gate(elsewhere, Record{Name: "T-Mobile Inc"}) {
		t.Error("a punctuated single-token name was admitted on the country facet alone")
	}

	uk := Company{Slug: "t-mobile", Countries: []string{"GB"}, HQCountry: "GB"}
	if !gate(uk, Record{Name: "T-Mobile Inc"}) {
		t.Error("a UK-headquartered punctuated single-token company was rejected")
	}
}

func TestRequireCountry_SingleTokenWithNoHeadquartersIsRejected(t *testing.T) {
	// An unknown hq_country is not evidence of a UK headquarters. Never guess.
	gate := RequireCountry("GB")
	co := Company{Slug: "monzo", Countries: []string{"GB"}}
	if gate(co, Record{Name: "MONZO LTD"}) {
		t.Error("a single-token match was admitted with an unknown headquarters")
	}
}

func TestRequireCountry_IsCaseInsensitiveOnCountryCodes(t *testing.T) {
	gate := RequireCountry("GB")
	co := Company{Slug: "acme-robotics", Countries: []string{"gb"}}
	if !gate(co, Record{Name: "ACME ROBOTICS LTD"}) {
		t.Error("a lowercase country code was not recognised")
	}
}

func TestRequireRoute_AdmitsOnAWorkRoute(t *testing.T) {
	gate := RequireRoute(ukWorkRoutes...)
	for _, route := range []string{
		"Skilled Worker",
		"Global Business Mobility - Senior or Specialist Worker",
		"Scale-up",
	} {
		if !gate(Company{}, Record{Meta: map[string]string{"route": route}}) {
			t.Errorf("work route %q was rejected", route)
		}
	}
}

func TestRequireRoute_RejectsATemporaryOrStudyRoute(t *testing.T) {
	gate := RequireRoute(ukWorkRoutes...)
	for _, route := range []string{
		"Temporary Worker - Seasonal Worker",
		"Temporary Worker - Creative Worker",
		"Temporary Worker - Charity Worker",
		"Student",
		"",
	} {
		if gate(Company{}, Record{Meta: map[string]string{"route": route}}) {
			t.Errorf("non-work route %q was admitted", route)
		}
	}
}

func TestAllOf_RequiresEveryGate(t *testing.T) {
	yes := func(Company, Record) bool { return true }
	no := func(Company, Record) bool { return false }

	if !AllOf(yes, yes)(Company{}, Record{}) {
		t.Error("AllOf rejected when every gate admitted")
	}
	if AllOf(yes, no)(Company{}, Record{}) {
		t.Error("AllOf admitted when a gate rejected")
	}
	if !AllOf()(Company{}, Record{}) {
		t.Error("AllOf with no gates should admit")
	}
}

func TestDropAmbiguous_RemovesEveryRowOfANameTwoOrganisationsShare(t *testing.T) {
	// Two unrelated organisations normalize alike — different towns give them away.
	// The name identifies neither, so both go; granting to one would be a coin flip.
	in := []Record{
		{Name: "SPARK LTD", Meta: map[string]string{"town": "Liverpool"}},
		{Name: "Spark Limited", Meta: map[string]string{"town": "Leeds"}},
		{Name: "ACME ROBOTICS LTD", Meta: map[string]string{"town": "London"}},
	}
	got := DropAmbiguous(in, "town")
	want := []Record{{Name: "ACME ROBOTICS LTD", Meta: map[string]string{"town": "London"}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("DropAmbiguous = %+v, want %+v", got, want)
	}
}

func TestDropAmbiguous_KeepsOneOrganisationsRowsAcrossRoutes(t *testing.T) {
	// The UK register lists an organisation once per route it holds. Those rows share
	// a name AND a town, so they are one body: they must all survive, because the
	// route gate needs to see every one of them.
	in := []Record{
		{Name: "ACME ROBOTICS LTD", Meta: map[string]string{"town": "London", "route": "Skilled Worker"}},
		{Name: "ACME ROBOTICS LTD", Meta: map[string]string{"town": "London", "route": "Scale-up"}},
	}
	if got := DropAmbiguous(in, "town"); len(got) != 2 {
		t.Errorf("DropAmbiguous dropped an organisation's own per-route rows: %+v", got)
	}
}

func TestDropAmbiguous_WithNoIdentityKeyTreatsARepeatedNameAsOneOrganisation(t *testing.T) {
	// A register with no disambiguator (or rows missing it) cannot tell a duplicate
	// from a collision. Treating them as one body keeps the guard from firing on
	// every multi-route row; the geography rules remain the real defence.
	in := []Record{{Name: "ACME LTD"}, {Name: "Acme Limited"}}
	if got := DropAmbiguous(in, ""); len(got) != 2 {
		t.Errorf("DropAmbiguous with no identity key dropped rows: %+v", got)
	}
}

func TestDropAmbiguous_KeepsAnEmptyInputEmpty(t *testing.T) {
	if got := DropAmbiguous(nil, "town"); len(got) != 0 {
		t.Errorf("DropAmbiguous(nil) = %+v, want empty", got)
	}
}

func TestRequireCountry_AcceptsSpelledOutCountryNames(t *testing.T) {
	// cmd/import-yc, hq_country's writer, runs values through location.Parse, which
	// already emits ISO codes — but a retired one-time importer left a small number
	// of rows with a spelled-out country verbatim, and those rows outlive it. The
	// gate must not depend on a third party's formatting choice, because the
	// failure is silent: a company simply never earns a credential it qualifies for.
	gate := RequireCountry("GB")
	for _, hq := range []string{"GB", "gb", "uk", "UK", "United Kingdom", "united kingdom", "Great Britain"} {
		co := Company{Slug: "monzo", Countries: []string{"GB"}, HQCountry: hq}
		if !gate(co, Record{Name: "MONZO LTD"}) {
			t.Errorf("hq_country %q was not recognised as the UK", hq)
		}
	}
	gate = RequireCountry("NL")
	for _, hq := range []string{"NL", "nl", "Netherlands", "the netherlands"} {
		co := Company{Slug: "adyen", Countries: []string{"NL"}, HQCountry: hq}
		if !gate(co, Record{Name: "ADYEN N.V."}) {
			t.Errorf("hq_country %q was not recognised as the Netherlands", hq)
		}
	}
	gate = RequireCountry("US")
	for _, hq := range []string{"US", "us", "USA", "U.S.A.", "United States", "united states", "United States of America", "U.S.", "America"} {
		co := Company{Slug: "stripe", Countries: []string{"US"}, HQCountry: hq}
		if !gate(co, Record{Name: "STRIPE"}) {
			t.Errorf("hq_country %q was not recognised as the United States", hq)
		}
	}
}

func TestRequireCountry_StillRejectsAnotherCountry(t *testing.T) {
	gate := RequireCountry("GB")
	for _, hq := range []string{"US", "United States", "IE", "Ireland", ""} {
		co := Company{Slug: "apple", Countries: []string{"GB", "US"}, HQCountry: hq}
		if gate(co, Record{Name: "APPLE LTD"}) {
			t.Errorf("hq_country %q was accepted as the UK", hq)
		}
	}
}

func TestRequireCountry_DoesNotMatchOnASubstring(t *testing.T) {
	// "Great Britain" must not make every string containing "britain" a match, and a
	// country facet value is compared whole, not by prefix.
	gate := RequireCountry("GB")
	for _, hq := range []string{"New Great Britain Holdings", "gbr-something", "ukraine"} {
		co := Company{Slug: "x", Countries: []string{"GB"}, HQCountry: hq}
		if gate(co, Record{Name: "X LTD"}) {
			t.Errorf("hq_country %q matched by substring", hq)
		}
	}
}
