package sources

import (
	"context"
	"testing"
)

// emptyBoardListingAdapters is every adapter granted emptyBoardListing, with the answer each
// source gives for an emptied board (measured live 2026-10-29) and the shapes that are NOT that
// answer. The registry test below fails if an adapter carries the marker without a row here.
var emptyBoardListingAdapters = map[string]struct {
	newSource func(*fakeHTTP) Source
	empty     string   // the source's own answer for a board with no postings
	notEmpty  []string // decodable answers that carry no posting collection
}{
	"join":           {func(f *fakeHTTP) Source { return NewJoin(f) }, `{"items":[],"pagination":{"pageCount":0,"rowCount":0}}`, []string{`{}`, `{"items":null,"pagination":{"pageCount":0,"rowCount":0}}`, `{"items":[],"pagination":{"pageCount":0}}`}},
	"personio":       {func(f *fakeHTTP) Source { return NewPersonio(f) }, `<workzag-jobs/>`, []string{`<html></html>`, `<error></error>`, `<workzag-jobs><error>temporarily unavailable</error></workzag-jobs>`, `<workzag-jobs>temporarily unavailable</workzag-jobs>`}},
	"hiringthing":    {func(f *fakeHTTP) Source { return NewHiringThing(f) }, htListingHTML(), []string{`<html></html>`, `<div data-react-class="HiringThing.Components.JobFiltersContainer" data-react-props="{}"></div>`}},
	"idealtraits":    {func(f *fakeHTTP) Source { return NewIdealTraits(f) }, `{"joblist":"","jobcount":0}`, []string{`{}`, `{"joblist":""}`, `{"jobcount":0}`, `{"joblist":null,"jobcount":0}`}},
	"greenhouse":     {func(f *fakeHTTP) Source { return NewGreenhouse(f) }, `{"jobs":[],"meta":{"total":0}}`, []string{`{}`, `{"jobs":null}`, `{"status":404}`}},
	"ashby":          {func(f *fakeHTTP) Source { return NewAshby(f) }, `{"jobs":[],"apiVersion":"1"}`, []string{`{}`, `{"jobs":null}`, `{"apiVersion":"1"}`}},
	"lever":          {func(f *fakeHTTP) Source { return NewLever(f) }, `[]`, []string{`null`}},
	"workable":       {func(f *fakeHTTP) Source { return NewWorkable(f) }, `{"name":"Acme","description":"","jobs":[]}`, []string{`{}`, `{"name":"Acme"}`, `{"jobs":null}`}},
	"recruitee":      {func(f *fakeHTTP) Source { return NewRecruitee(f) }, `{"offers":[]}`, []string{`{}`, `{"offers":null}`}},
	"breezy":         {func(f *fakeHTTP) Source { return NewBreezy(f) }, `[]`, []string{`null`}},
	"bamboohr":       {func(f *fakeHTTP) Source { return NewBambooHR(f) }, `{"meta":{"totalCount":0},"result":[]}`, []string{`{}`, `{"meta":{"totalCount":0}}`, `{"result":null}`}},
	"workday":        {func(f *fakeHTTP) Source { return NewWorkday(f) }, `{"total":0,"jobPostings":[],"facets":[]}`, []string{`{}`, `{"total":0}`, `{"jobPostings":null}`, `{"total":5,"jobPostings":[]}`}},
	"beehire":        {func(f *fakeHTTP) Source { return NewBeehire(f) }, `{"campaigns":[]}`, []string{`{}`, `{"campaigns":null}`}},
	"seamlesshiring": {func(f *fakeHTTP) Source { return NewSeamlessHiring(f) }, `{"status":"success","status_code":200,"data":{"jobs":{"data":[],"total":0,"current_page":1,"last_page":1,"per_page":100}}}`, []string{`{}`, `{"status":"success","status_code":200,"data":null}`, `{"status":"success","status_code":200,"data":{"jobs":null}}`, `{"status":"success","status_code":200,"data":{"jobs":{"total":0,"current_page":1,"last_page":1,"per_page":100,"data":null}}}`}},
}

// An emptied board is the source's own answer, with its posting collection present and empty:
// Fetch reports it as an empty listing, never as a failure.
func TestEmptyBoardListingAdaptersReturnAnEmptiedBoardAsEmpty(t *testing.T) {
	for provider, a := range emptyBoardListingAdapters {
		t.Run(provider, func(t *testing.T) {
			jobs, err := a.newSource(&fakeHTTP{body: a.empty}).Fetch(context.Background(), emptyBoardEntry(provider))
			if err != nil {
				t.Fatalf("Fetch(%s) = %v, want an empty listing", a.empty, err)
			}
			if len(jobs) != 0 {
				t.Errorf("Fetch(%s) = %d jobs, want 0", a.empty, len(jobs))
			}
		})
	}
}

// The trap the marker exists to keep out: an answer that decodes but carries no posting
// collection is not an emptied board. Reading it as one would close the board's whole
// catalogue, so it must fail the Fetch.
func TestEmptyBoardListingAdaptersFailAnAnswerWithoutTheCollection(t *testing.T) {
	for provider, a := range emptyBoardListingAdapters {
		for _, body := range a.notEmpty {
			t.Run(provider+" "+body, func(t *testing.T) {
				if jobs, err := a.newSource(&fakeHTTP{body: body}).Fetch(context.Background(), emptyBoardEntry(provider)); err == nil {
					t.Errorf("Fetch(%s) = %d jobs and no error, want a failed fetch", body, len(jobs))
				}
			})
		}
	}
}

// The marker is granted per adapter after the check above, and only on top of a complete
// listing read on the default window: a slice or a widened window says nothing about the rest
// of a board that came back empty.
func TestEmptyBoardListingProvidersAreCheckedCompleteListings(t *testing.T) {
	reg := Taxonomy()
	full := FullBoardListingProviders(reg)
	grace := SweepGraceWindows(reg)
	got := EmptyBoardListingProviders(reg)
	for provider := range got {
		if !full[provider] {
			t.Errorf("%q carries emptyBoardListing but not fullBoardListing", provider)
		}
		if _, ok := grace[provider]; ok {
			t.Errorf("%q carries emptyBoardListing but declares a sweepGrace", provider)
		}
		if _, ok := emptyBoardListingAdapters[provider]; !ok {
			t.Errorf("%q carries emptyBoardListing without a row in emptyBoardListingAdapters proving its empty answer", provider)
		}
	}
	for provider := range emptyBoardListingAdapters {
		if !got[provider] {
			t.Errorf("EmptyBoardListingProviders() lacks %q", provider)
		}
	}
	// SmartRecruiters answers an unknown company identifier with totalFound=0 and an empty
	// content array, so its empty answer is also what a failed lookup looks like.
	if got["smartrecruiters"] {
		t.Error("smartrecruiters must not carry emptyBoardListing: an unknown company answers empty")
	}
}

// emptyBoardEntry names a board in the shape the provider's own parser accepts.
func emptyBoardEntry(provider string) CompanyEntry {
	if provider == "idealtraits" {
		return CompanyEntry{Company: "Acme", Board: "8739"}
	}
	if provider == "workday" {
		return CompanyEntry{Company: "Acme", Board: "acme.wd1.myworkdayjobs.com/Careers"}
	}
	return CompanyEntry{Company: "Acme", Board: "acme"}
}
