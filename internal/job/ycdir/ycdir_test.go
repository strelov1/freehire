package ycdir

import (
	"reflect"
	"testing"

	"github.com/strelov1/freehire/internal/dict/normalize"
)

func TestMapFullEntry(t *testing.T) {
	e := Entry{
		Name:            "CircuitHub",
		OneLiner:        "On-Demand Electronics Manufacturing",
		LongDescription: "CircuitHub offers on-demand electronics manufacturing.",
		Batch:           "Winter 2012",
		Status:          "Active",
		Stage:           "Early",
		Industry:        "Industrials",
		Industries:      []string{"Manufacturing"},
		Subindustry:     "Industrials -> Manufacturing and Robotics",
		Tags:            []string{"Industrials", "Robotics", "Hardware"},
		FormerNames:     []string{"Old CircuitHub Inc"},
		TeamSize:        58,
		LaunchedAt:      1322045523, // 2011-11-23 UTC
		Website:         "https://circuithub.com",
		AllLocations:    "San Francisco, CA, USA",
		TopCompany:      true,
		IsHiring:        true,
		URL:             "https://www.ycombinator.com/companies/circuithub",
	}
	r, ok := Map(e)
	if !ok {
		t.Fatal("ok = false, want true for a named entry")
	}
	if r.Slug != "circuithub" {
		t.Errorf("slug = %q, want circuithub", r.Slug)
	}
	if r.Tagline != "On-Demand Electronics Manufacturing" {
		t.Errorf("tagline = %q", r.Tagline)
	}
	// industry + industries[] + subindustry leaf + tags, de-duplicated, in that order.
	if !reflect.DeepEqual(r.Industries, []string{"Industrials", "Manufacturing", "Manufacturing and Robotics", "Robotics", "Hardware"}) {
		t.Errorf("industries = %v", r.Industries)
	}
	// subindustry is the clean leaf of the YC subindustry path, stored separately from
	// the tag-inclusive industries bag.
	if r.Subindustry != "Manufacturing and Robotics" {
		t.Errorf("subindustry = %q, want %q", r.Subindustry, "Manufacturing and Robotics")
	}
	if r.EmployeeCount != 58 {
		t.Errorf("employee_count = %d, want 58", r.EmployeeCount)
	}
	if r.YearFounded != 2011 {
		t.Errorf("year_founded = %d, want 2011 (from launched_at)", r.YearFounded)
	}
	if r.HQCountry != "us" {
		t.Errorf("hq_country = %q, want us (from all_locations)", r.HQCountry)
	}
	if r.Batch != "Winter 2012" || r.Status != "Active" {
		t.Errorf("batch/status = %q/%q", r.Batch, r.Status)
	}
	if r.Info["description"] != "CircuitHub offers on-demand electronics manufacturing." {
		t.Errorf("info.description = %v", r.Info["description"])
	}
	if r.Info["website"] != "https://circuithub.com" || r.Info["stage"] != "Early" {
		t.Errorf("info website/stage = %v/%v", r.Info["website"], r.Info["stage"])
	}
	if r.Stage != "Early" {
		t.Errorf("stage = %q, want Early", r.Stage)
	}
	// The corporate form is not part of who the employer is, so it is not part of the
	// slug either — the catalogue never stores "…-inc", and a former slug carrying one
	// matches nothing.
	if !reflect.DeepEqual(r.FormerSlugs, []string{"old-circuithub"}) {
		t.Errorf("former slugs = %v, want [old-circuithub]", r.FormerSlugs)
	}
	// flags sorted: hiring, top_company.
	if !reflect.DeepEqual(r.Flags, []string{"hiring", "top_company"}) {
		t.Errorf("flags = %v, want [hiring top_company]", r.Flags)
	}
}

func TestMapNoFlagsWhenNeither(t *testing.T) {
	r, _ := Map(Entry{Name: "Plain Co"})
	if len(r.Flags) != 0 {
		t.Errorf("flags = %v, want empty", r.Flags)
	}
}

func TestMapBlankNameSkipped(t *testing.T) {
	if _, ok := Map(Entry{Name: "   "}); ok {
		t.Error("ok = true for a blank name, want false")
	}
}

func TestMapMissingOptionalsOmitted(t *testing.T) {
	r, ok := Map(Entry{Name: "Ghost Co", AllLocations: "Nowhereland", Industry: "Fintech"})
	if !ok {
		t.Fatal("ok = false")
	}
	if r.EmployeeCount != 0 {
		t.Errorf("employee_count = %d, want 0 (unknown)", r.EmployeeCount)
	}
	if r.YearFounded != 0 {
		t.Errorf("year_founded = %d, want 0 (unknown)", r.YearFounded)
	}
	if r.HQCountry != "" {
		t.Errorf("hq_country = %q, want empty (unresolved location)", r.HQCountry)
	}
	if _, has := r.Info["description"]; has {
		t.Error("info.description present for empty long_description")
	}
	if !reflect.DeepEqual(r.Industries, []string{"Fintech"}) {
		t.Errorf("industries = %v, want [Fintech]", r.Industries)
	}
	if r.Subindustry != "" {
		t.Errorf("subindustry = %q, want empty (no subindustry given)", r.Subindustry)
	}
}

// TestMappedSlugsAreCompanySlugStable guards the import against the slug rule it silently
// depends on, in the manner of collections' TestHandListSlugsAreCompanySlugStable.
//
// cmd/import-yc looks every slug this mapper produces up against the catalogue's
// company_slug, which is normalize.CompanySlug. A slug that rule would never produce —
// one still carrying a corporate form — matches nothing, and NOTHING SAYS SO: the import
// files the entry as a fresh reference row and counts it as inserted, so the real
// employer stays un-enriched while the run reports success. Measured on live yc-oss
// before this was fixed: 76 current and 369 former names.
//
// The test is that every produced slug is a fixed point of the rule.
func TestMappedSlugsAreCompanySlugStable(t *testing.T) {
	// The shapes yc-oss actually carries: bare names, every common corporate form, and
	// the punctuated spellings of them.
	entries := []Entry{
		{Name: "CircuitHub", FormerNames: []string{"Old CircuitHub Inc"}},
		{Name: "Stripe, Inc.", FormerNames: []string{"/dev/payments"}},
		{Name: "Rippling Inc", FormerNames: []string{"EnterpriseOS, Inc."}},
		{Name: "Zapier LLC"},
		{Name: "Adyen N.V.", FormerNames: []string{"Adyen B.V."}},
		{Name: "Deel Ltd.", FormerNames: []string{"Deel Limited"}},
		{Name: "Monzo Bank Ltd", FormerNames: []string{"Mondo Bank Limited"}},
		{Name: "Klarna Bank AB", FormerNames: []string{"Kreditor Europe AB"}},
		{Name: "Wise plc"},
	}
	var checked int
	for _, e := range entries {
		r, ok := Map(e)
		if !ok {
			t.Errorf("Map(%q) = ok false, want a record", e.Name)
			continue
		}
		for _, slug := range append([]string{r.Slug}, r.FormerSlugs...) {
			checked++
			if got := normalize.CompanySlug(slug); got != slug {
				t.Errorf("%q yielded slug %q, which is not what the slug rule produces (%q) — "+
					"the catalogue keys companies by normalize.CompanySlug, so this matches nothing",
					e.Name, slug, got)
			}
		}
	}
	// A detector that has stopped seeing the entries passes for the same reason a clean
	// table does; count the population so the two are told apart.
	if checked != 16 {
		t.Errorf("checked %d slugs, want 16 — has the table been edited without this count?", checked)
	}
}

// A multi-office entry lists all_locations HQ first, semicolon-separated. hqCountry must
// resolve the FIRST office's country, not the alphabetically-first one location.Parse's
// sorted, whole-string result would otherwise surface for a later office.
func TestMapMultiOfficeUsesTheFirstListedLocation(t *testing.T) {
	r, ok := Map(Entry{Name: "CleverDeck", AllLocations: "San Francisco, CA, USA; Istanbul, Istanbul, Turkey"})
	if !ok {
		t.Fatal("ok = false")
	}
	if r.HQCountry != "us" {
		t.Errorf("hq_country = %q, want us (the first-listed office, not alphabetically-first tr)", r.HQCountry)
	}
}
