package jobderive

import (
	"reflect"
	"slices"
	"testing"
)

// A bare-"Remote" location resolves to the global bucket, but a hard US-only
// eligibility statement in the description pins the job to the US instead of leaving
// it Global/Worldwide (the reported bug: a US-citizen/clearance role labeled global).
func TestDerive_USOnlyDescriptionPinsBareRemoteToUS(t *testing.T) {
	got := Derive(Input{
		Title:       "Senior Full-Stack Engineer",
		Company:     "Redhorse",
		Source:      "lever",
		ExternalID:  "1",
		Location:    "Remote", // → global bucket, no country
		Description: "Must be a U.S. Citizen and eligible for a U.S. SECRET clearance.",
	})
	if !reflect.DeepEqual(got.Countries, []string{"us"}) {
		t.Errorf("Countries = %v, want [us]", got.Countries)
	}
	if !reflect.DeepEqual(got.Regions, []string{"north_america"}) {
		t.Errorf("Regions = %v, want [north_america] (not global)", got.Regions)
	}
	// The override touches only geography — remoteness stays on the work-mode facet.
	if got.WorkMode != "remote" {
		t.Errorf("WorkMode = %q, want remote (unchanged)", got.WorkMode)
	}
}

// The US-only override fires ONLY when the location left the geography unpinned. A
// location that resolved a specific place ("Remote - Germany" → de/eu) is never
// overridden, even if the description mentions a US eligibility phrase.
func TestDerive_USOnlyDoesNotOverrideResolvedPlace(t *testing.T) {
	got := Derive(Input{
		Title:       "Engineer",
		Company:     "Acme",
		Source:      "greenhouse",
		ExternalID:  "board:1",
		Location:    "Remote - Germany", // resolves de/eu
		Description: "US citizenship preferred but we hire across the EU.",
	})
	if !reflect.DeepEqual(got.Countries, []string{"de"}) {
		t.Errorf("Countries = %v, want [de] (resolved place not overridden)", got.Countries)
	}
}

// A title-embedded restriction suffix resolves a bare-remote posting — the quanata
// report (freehire job_reports #29): the location string alone ("remote") carries no
// country, and the ATS-convention restriction lives only in the title.
func TestDerive_TitleRestrictionPinsBareRemote(t *testing.T) {
	got := Derive(Input{
		Title:      "Senior Back End Engineer [Remote-US]",
		Company:    "Quanata",
		Source:     "greenhouse",
		ExternalID: "1",
		Location:   "remote",
	})
	if !reflect.DeepEqual(got.Countries, []string{"us"}) {
		t.Errorf("Countries = %v, want [us]", got.Countries)
	}
	if !reflect.DeepEqual(got.Regions, []string{"north_america"}) {
		t.Errorf("Regions = %v, want [north_america]", got.Regions)
	}
}

// The creative-fabrica report (freehire job_reports #31/#33): a parenthetical title
// suffix names two countries.
func TestDerive_TitleRestrictionResolvesMultipleCountries(t *testing.T) {
	got := Derive(Input{
		Title:      "Senior Backend Engineer (Go) (Location - Australia or New Zealand)",
		Company:    "Creative Fabrica",
		Source:     "greenhouse",
		ExternalID: "2",
		Location:   "Remote",
	})
	if !reflect.DeepEqual(got.Countries, []string{"au", "nz"}) {
		t.Errorf("Countries = %v, want [au nz]", got.Countries)
	}
	if !reflect.DeepEqual(got.Regions, []string{"apac"}) {
		t.Errorf("Regions = %v, want [apac]", got.Regions)
	}
}

// A location that already resolved a place is never overridden by the title, even
// when the title carries what looks like a restriction marker.
func TestDerive_TitleRestrictionNeverOverridesResolvedLocation(t *testing.T) {
	got := Derive(Input{
		Title:      "Engineer [Remote-US]",
		Company:    "Acme",
		Source:     "greenhouse",
		ExternalID: "3",
		Location:   "Remote - Germany",
	})
	if !reflect.DeepEqual(got.Countries, []string{"de"}) {
		t.Errorf("Countries = %v, want [de] (title never consulted)", got.Countries)
	}
}

// The title check runs before the description checks: when both carry a signal, the
// title's (the more literal, ATS-authored one) wins.
func TestDerive_TitleRestrictionTakesPrecedenceOverDescription(t *testing.T) {
	got := Derive(Input{
		Title:       "Engineer [Remote-US]",
		Company:     "Acme",
		Source:      "greenhouse",
		ExternalID:  "4",
		Location:    "Remote",
		Description: "This role is based in Canada.",
	})
	if !reflect.DeepEqual(got.Countries, []string{"us"}) {
		t.Errorf("Countries = %v, want [us] (title wins over description)", got.Countries)
	}
}

// The kard-financial report (freehire job_reports #30): no title marker, but the
// description states an explicit multi-country restriction.
func TestDerive_DescriptionRegionScopePinsBareRemote(t *testing.T) {
	got := Derive(Input{
		Title:       "Senior Software Engineer II, Customer Experience",
		Company:     "Kard Financial",
		Source:      "greenhouse",
		ExternalID:  "5",
		Location:    "Remote",
		Description: "We are a fully remote company hiring in the US, Canada, Argentina, or Brazil only.",
	})
	if !reflect.DeepEqual(got.Countries, []string{"ar", "br", "ca", "us"}) {
		t.Errorf("Countries = %v, want [ar br ca us]", got.Countries)
	}
	if !reflect.DeepEqual(got.Regions, []string{"latam", "north_america"}) {
		t.Errorf("Regions = %v, want [latam north_america]", got.Regions)
	}
}

// A bare-"Remote" job whose title and description carry no restriction stays in the
// "region not specified" bucket, not global — the override never guesses, so
// genuinely open-anywhere remote jobs are unaffected.
func TestDerive_BareRemoteWithoutUSOnlyStaysUnspecified(t *testing.T) {
	got := Derive(Input{
		Title:       "Engineer",
		Company:     "Acme",
		Source:      "greenhouse",
		ExternalID:  "board:1",
		Location:    "Remote",
		Description: "We are a fully distributed team hiring worldwide.",
	})
	if len(got.Countries) != 0 {
		t.Errorf("Countries = %v, want [] (no US-only signal)", got.Countries)
	}
	// "distributed"/"worldwide" here are incidental prose, not the anchored open-anywhere
	// location marker or a region-scoping restriction statement, so they resolve nothing —
	// the posting states no geography and stays in the "region not specified" bucket.
	if len(got.Regions) != 0 {
		t.Errorf("Regions = %v, want [] (no geography stated)", got.Regions)
	}
}

// FallbackCountries (Greenhouse's office) is the LAST step of the geography chain: it
// fills a posting that stated nothing, pulling its region along, and loses to every
// stated place — the location, the title marker, a description restriction, an explicit
// open-anywhere marker, and the authoritative Countries signal.
func TestDerive_FallbackCountriesFillOnlyWhatNothingStated(t *testing.T) {
	cases := []struct {
		name        string
		title       string
		location    string
		description string
		countries   []string
		wantC       []string
		wantR       []string
	}{
		{"fills a bare remote posting", "Engineer", "Remote", "Build things.", nil,
			[]string{"us"}, []string{"north_america"}},
		{"the location wins", "Engineer", "Remote - Germany", "", nil,
			[]string{"de"}, []string{"eu"}},
		{"the title marker wins", "Engineer [Remote-Canada]", "Remote", "", nil,
			[]string{"ca"}, []string{"north_america"}},
		{"a description restriction wins", "Engineer", "Remote",
			"This is a remote position open to candidates based in Singapore.", nil,
			[]string{"sg"}, []string{"apac"}},
		{"an open-anywhere location is never narrowed", "Engineer", "Remote - Worldwide", "", nil,
			nil, []string{"global"}},
		{"the authoritative Countries signal wins", "Engineer", "Remote", "", []string{"mx"},
			[]string{"mx"}, []string{"latam"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Derive(Input{
				Title: tc.title, Company: "Acme", Source: "greenhouse", ExternalID: "board:1",
				Location: tc.location, Description: tc.description,
				Countries: tc.countries, FallbackCountries: []string{"us"},
			})
			if len(tc.wantC) == 0 && len(got.Countries) != 0 {
				t.Errorf("Countries = %v, want none", got.Countries)
			} else if len(tc.wantC) != 0 && !reflect.DeepEqual(got.Countries, tc.wantC) {
				t.Errorf("Countries = %v, want %v", got.Countries, tc.wantC)
			}
			if !reflect.DeepEqual(got.Regions, tc.wantR) {
				t.Errorf("Regions = %v, want %v", got.Regions, tc.wantR)
			}
		})
	}
}

// Explicit region/city signals supplied on the Input win over the location-dictionary
// derivation, mirroring the precedence work-mode already has: a recruiter (or moderator)
// who states the geography by hand overrides what the free-text location would derive,
// while an unsupplied facet still derives.
func TestDerive_ExplicitRegionCityOverride(t *testing.T) {
	got := Derive(Input{
		Title:       "Senior Go Developer",
		Company:     "Acme",
		Source:      "manual",
		ExternalID:  "https://acme.example/jobs/1",
		Location:    "Remote - Germany",
		Description: "We use Golang.",
		Regions:     []string{"north_america"},
		Cities:      []string{"Austin"},
	})
	if !reflect.DeepEqual(got.Regions, []string{"north_america"}) {
		t.Errorf("Regions = %v, want [north_america] (explicit wins over derived)", got.Regions)
	}
	if !reflect.DeepEqual(got.Cities, []string{"Austin"}) {
		t.Errorf("Cities = %v, want [Austin] (explicit wins over derived)", got.Cities)
	}
}

// Metros follow an adapter's city override: they are re-resolved from the adapter's
// cities under the final country rather than read off the free-text line.
func TestDerive_MetrosFollowCityOverride(t *testing.T) {
	got := Derive(Input{Title: "Engineer", Company: "Acme", Source: "manual", ExternalID: "1",
		Location: "San Diego, CA", Cities: []string{"Folsom", "Elk Grove"}})
	if want := []string{"Sacramento (US)"}; !reflect.DeepEqual(got.Metros, want) {
		t.Errorf("Metros = %v, want %v (from the adapter's cities)", got.Metros, want)
	}

	// The adapter's cities with an adapter country: resolved under that country.
	got = Derive(Input{Title: "Engineer", Company: "Acme", Source: "manual", ExternalID: "1",
		Location: "Office", Cities: []string{"Khimki"}, Countries: []string{"ru"}})
	if want := []string{"Moscow (RU)"}; !reflect.DeepEqual(got.Metros, want) {
		t.Errorf("Metros = %v, want %v", got.Metros, want)
	}

	// Two final countries cannot place a bare city in either.
	got = Derive(Input{Title: "Engineer", Company: "Acme", Source: "manual", ExternalID: "1",
		Cities: []string{"Folsom"}, Countries: []string{"us", "ca"}})
	if len(got.Metros) != 0 {
		t.Errorf("Metros = %v, want none under two countries", got.Metros)
	}
}

// An adapter country alone drops any metro the line resolved in a country the posting
// no longer states, and keeps one it still does.
func TestDerive_MetrosFollowCountryOverride(t *testing.T) {
	got := Derive(Input{Title: "Engineer", Company: "Acme", Source: "lever", ExternalID: "1",
		Location: "Folsom, CA", Countries: []string{"de"}})
	if len(got.Metros) != 0 {
		t.Errorf("Metros = %v, want none (Sacramento is not in de)", got.Metros)
	}

	got = Derive(Input{Title: "Engineer", Company: "Acme", Source: "lever", ExternalID: "1",
		Location: "Folsom, CA", Countries: []string{"us"}})
	if want := []string{"Sacramento (US)"}; !reflect.DeepEqual(got.Metros, want) {
		t.Errorf("Metros = %v, want %v", got.Metros, want)
	}
}

// A structured country signal (Ashby/Lever/Workday's own country field) overrides the
// location-dictionary derivation AND pulls its region along with it, so a job whose
// free-text location the dictionary cannot parse ("UAE" as a bare name) still lands
// under both facets instead of just the country.
func TestDerive_ExplicitCountryOverrideAlsoDerivesRegion(t *testing.T) {
	got := Derive(Input{
		Title:      "Ops Manager",
		Company:    "Acme",
		Source:     "lever",
		ExternalID: "1",
		Location:   "Some Office", // resolves no geography on its own
		Countries:  []string{"ae"},
	})
	if !reflect.DeepEqual(got.Countries, []string{"ae"}) {
		t.Errorf("Countries = %v, want [ae] (explicit wins over derived)", got.Countries)
	}
	if !reflect.DeepEqual(got.Regions, []string{"mena"}) {
		t.Errorf("Regions = %v, want [mena] (paired from the explicit country)", got.Regions)
	}
}

// A location line whose scope is at least as broad outranks the adapter's country: Ashby
// states the employer's postal address as the country, beside a line that names the hiring
// scope (Chromatic, freehire report 2026-11-10). A line with a city, one naming other
// countries, or one that names only "Anywhere" still yields to the country.
func TestDerive_RegionLineOutranksAdapterCountry(t *testing.T) {
	cases := []struct {
		name, location string
		countries      []string
		regions        []string // a stored row's regions, as rederive feeds them back
		wantC, wantR   []string
	}{
		{"line names regions outright", "Remote | North America or Europe", []string{"us"}, nil, []string{}, []string{"eu", "north_america"}},
		{"line names the region the office is in", "Remote | Europe", []string{"nl"}, nil, []string{}, []string{"eu"}},
		{"a line of more countries outranks", "Remote | US or Canada", []string{"us"}, nil, []string{"ca", "us"}, []string{"north_america"}},
		{"a line of other countries yields", "Canada", []string{"us"}, nil, []string{"us"}, []string{"north_america"}},
		{"a line with a city yields", "London, UK or Berlin", []string{"gb"}, nil, []string{"gb"}, []string{"uk"}},
		{"a country the line misreads yields", "Porto Alegre, RS, Brasil", []string{"br"}, nil, []string{"br"}, []string{"latam"}},
		{"rederive's stored regions do not narrow it back", "Remote | North America or Europe", []string{"us"}, []string{"north_america"}, []string{}, []string{"eu", "north_america"}},
		{"a contradicting line yields", "Berlin", []string{"at"}, nil, []string{"at"}, []string{"eu"}},
		{"an open-anywhere line yields", "Anywhere", []string{"us"}, nil, []string{"us"}, []string{"north_america"}},
		{"a bare remote line yields", "Remote", []string{"nl"}, nil, []string{"nl"}, []string{"eu"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Derive(Input{Title: "Engineer", Company: "Acme", Source: "ashby", ExternalID: "1",
				Location: tc.location, Countries: tc.countries, Regions: tc.regions})
			if !slices.Equal(got.Countries, tc.wantC) || !slices.Equal(got.Regions, tc.wantR) {
				t.Errorf("Countries, Regions = %v, %v; want %v, %v", got.Countries, got.Regions, tc.wantC, tc.wantR)
			}
		})
	}
}

// An explicit Regions signal still wins over the region an explicit Countries signal
// would otherwise pair in — the same "most specific wins" precedence Regions already
// has over the dictionary.
func TestDerive_ExplicitRegionOverridesCountryPairedRegion(t *testing.T) {
	got := Derive(Input{
		Title:      "Ops Manager",
		Company:    "Acme",
		Source:     "manual",
		ExternalID: "1",
		Countries:  []string{"ae"},
		Regions:    []string{"apac"},
	})
	if !reflect.DeepEqual(got.Regions, []string{"apac"}) {
		t.Errorf("Regions = %v, want [apac] (explicit Regions beats the country-paired one)", got.Regions)
	}
}

// A Regions signal outside the vocabulary is folded before it wins: a stored
// regions=['us'] fed back by backfill-derive must come out as north_america, and a
// signal that folds to nothing must leave the dictionary's derivation standing.
func TestDerive_RegionSignalFoldsIntoVocabulary(t *testing.T) {
	got := Derive(Input{Title: "Engineer", Company: "Acme", Source: "greenhouse", ExternalID: "1",
		Location: "Berlin", Regions: []string{"us", "emea"}})
	if want := []string{"africa", "eu", "mena", "north_america", "uk"}; !reflect.DeepEqual(got.Regions, want) {
		t.Errorf("Regions = %v, want %v", got.Regions, want)
	}

	got = Derive(Input{Title: "Engineer", Company: "Acme", Source: "greenhouse", ExternalID: "1",
		Location: "Berlin", Regions: []string{"gmt+1 to gmt+5"}})
	if want := []string{"eu"}; !reflect.DeepEqual(got.Regions, want) {
		t.Errorf("Regions = %v, want %v (an unresolvable signal is no signal)", got.Regions, want)
	}
}
func TestDeriveCarriesMetros(t *testing.T) {
	d := Derive(Input{Title: "Backend Engineer", Location: "Folsom, CA"})
	if !slices.Equal(d.Metros, []string{"Sacramento (US)"}) {
		t.Errorf("Metros = %v", d.Metros)
	}
}

// An adapter that writes the place alone and the country beside it (pandapelatam's
// "Palermo", a Buenos Aires barrio) gets the city and the metro the country implies.
func TestDerive_AdapterCountryResolvesABarePlace(t *testing.T) {
	got := Derive(Input{Title: "Analista", Company: "Acme", Source: "pandapelatam", ExternalID: "1",
		Location: "Palermo", Countries: []string{"ar"}})
	if !reflect.DeepEqual(got.Cities, []string{"Buenos Aires"}) || !reflect.DeepEqual(got.Metros, []string{"Buenos Aires (AR)"}) {
		t.Errorf("cities %v metros %v, want [Buenos Aires] [Buenos Aires (AR)]", got.Cities, got.Metros)
	}
	got = Derive(Input{Title: "Analista", Company: "Acme", Source: "whatjobs-ar", ExternalID: "1",
		Location: "Rosario", FallbackCountries: []string{"ar"}})
	if !reflect.DeepEqual(got.Metros, []string{"Rosario (AR)"}) {
		t.Errorf("metros %v, want [Rosario (AR)]", got.Metros)
	}
	// A fallback country the title overrules places nothing.
	got = Derive(Input{Title: "Engineer [Remote-US]", Company: "Acme", Source: "whatjobs-ar", ExternalID: "1",
		Location: "Palermo", FallbackCountries: []string{"ar"}})
	if !reflect.DeepEqual(got.Countries, []string{"us"}) || len(got.Metros) != 0 || reflect.DeepEqual(got.Cities, []string{"Buenos Aires"}) {
		t.Errorf("countries %v cities %v metros %v, want [us], no Buenos Aires", got.Countries, got.Cities, got.Metros)
	}
}
