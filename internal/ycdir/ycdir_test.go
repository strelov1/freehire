package ycdir

import (
	"reflect"
	"testing"
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
	if !reflect.DeepEqual(r.FormerSlugs, []string{"old-circuithub-inc"}) {
		t.Errorf("former slugs = %v, want [old-circuithub-inc]", r.FormerSlugs)
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
