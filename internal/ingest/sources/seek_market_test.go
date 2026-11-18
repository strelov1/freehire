package sources

import (
	"context"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"
)

// seek re-lists vacancies employers also post on their own ATS, so the cross-source dedup pass must
// be able to prefer the first-party copy. It still needs a board to bound the crawl, so unlike the
// boardless aggregators it must NOT carry that marker.
func TestSeekIsAnAggregatorButNotBoardless(t *testing.T) {
	src := NewSeek(&seekFake{}, newSeekDetailNetwork(&seekFake{}, nil))
	if _, ok := src.(aggregator); !ok {
		t.Error("seek must be an aggregator so its copies lose to first-party ATS postings")
	}
	if _, ok := src.(boardless); ok {
		t.Error("seek must not be boardless: the board selects the slice to crawl")
	}
	if !slices.Contains(AggregatorProviders(Taxonomy()), "seek") {
		t.Error("seek missing from AggregatorProviders")
	}
}

// SEEK stops serving results past ~550, so the busiest slices have a tail no crawl reaches. On the
// default window that tail would be closed and reopened as it drifts, writing a phantom removal
// each cycle — and SEEK's job pages are interstitial-gated, so liveness cannot settle it instead.
func TestSeekDeclaresAWiderSweepGrace(t *testing.T) {
	got, ok := SweepGraceWindows(Taxonomy())["seek"]
	if !ok {
		t.Fatal("seek must declare a sweep-grace window")
	}
	if got <= DefaultSweepGrace {
		t.Errorf("sweep grace = %v, want wider than the %v default", got, DefaultSweepGrace)
	}
	if want := 14 * 24 * time.Hour; got != want {
		t.Errorf("sweep grace = %v, want %v", got, want)
	}
}

func TestSeekIsRegisteredAndItsBoardsValidate(t *testing.T) {
	src, ok := All(nil)["seek"]
	if !ok {
		t.Fatal(`All(nil)["seek"] missing`)
	}
	if src.Provider() != "seek" {
		t.Errorf("Provider() = %q, want seek", src.Provider())
	}
	cfg := Config{Sources: []CompanyEntry{
		{Company: "SEEK Australia — Engineering - Software", Provider: "seek", Region: "au", Board: "6290"},
		{Company: "SEEK New Zealand — Engineering - Software", Provider: "seek", Region: "nz", Board: "6290"},
	}}
	if err := cfg.Validate(All(nil)); err != nil {
		t.Fatalf("board entries must validate against the registry: %v", err)
	}
	// Region is part of the board dedupe key, so the same slice in two markets is two crawl targets.
	au, _ := BoardDedupeKey(cfg.Sources[0])
	nz, _ := BoardDedupeKey(cfg.Sources[1])
	if au == nz {
		t.Errorf("same board id across markets must not collide: both key to %q", au)
	}
}

func TestSeekNewZealandMarketUsesItsOwnHostAndScope(t *testing.T) {
	fake := &seekFake{searchByPage: map[int][]seekPosting{1: {seekPost("1", "Dev", "Co")}}}
	if _, err := NewSeek(fake, newSeekDetailNetwork(fake, nil)).Fetch(context.Background(), CompanyEntry{Region: "nz", Board: "6287"}); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	pu, _ := url.Parse(fake.searchURLs[0])
	if pu.Host != "www.seek.co.nz" {
		t.Errorf("host = %q, want www.seek.co.nz", pu.Host)
	}
	if got := pu.Query().Get("siteKey"); got != "NZ-Main" {
		t.Errorf("siteKey = %q, want NZ-Main", got)
	}
	// where is load-bearing: omitting it does not mean "everywhere", it collapses the result set.
	if got := pu.Query().Get("where"); got != "All New Zealand" {
		t.Errorf("where = %q, want All New Zealand", got)
	}
}

// SEEK AU and NZ share subclassification ids and external_id carries no market, so the
// slice close narrows each board to its market by the exact host its postings live on.
func TestSeekSlicesCloseOnTheGraceWindowScopedToTheirMarket(t *testing.T) {
	src := NewSeek(&seekFake{}, newSeekDetailNetwork(&seekFake{}, nil))
	if !SliceBoardListingProviders(Taxonomy())["seek"] {
		t.Error("seek must carry sliceBoardListing")
	}
	if FullBoardListingProviders(Taxonomy())["seek"] {
		t.Error("seek must not carry fullBoardListing")
	}
	if _, ok := SweepGraceWindows(Taxonomy())["seek"]; !ok {
		t.Error("seek must declare its sweep grace")
	}
	if !ScopesBoardsByRegion(src) {
		t.Fatal("seek must scope its boards by region")
	}
	prefixes := map[string]string{}
	for region, m := range seekMarkets {
		prefix, ok := BoardURLPrefix(src, region)
		if !ok {
			t.Fatalf("no URL prefix for market %q", region)
		}
		job, ok := seekPost("123", "Engineer", "Co").toJob(m)
		if !ok || !strings.HasPrefix(job.URL, prefix) {
			t.Errorf("market %q: posting URL %q does not start with the prefix %q", region, job.URL, prefix)
		}
		prefixes[prefix] = region
	}
	if len(prefixes) != len(seekMarkets) {
		t.Errorf("markets share a prefix: %v", prefixes)
	}
	for prefix, region := range prefixes {
		for other, m := range seekMarkets {
			if other == region {
				continue
			}
			job, _ := seekPost("123", "Engineer", "Co").toJob(m)
			if strings.HasPrefix(job.URL, prefix) {
				t.Errorf("prefix of %q matches market %q posting %q", region, other, job.URL)
			}
		}
	}
	if _, ok := BoardURLPrefix(src, "sg"); ok {
		t.Error("a market seek does not serve must have no prefix")
	}
}

// shiftedTotalFake states a total one higher than the listing on its first walk only — a
// posting listed while that walk was under way.
type shiftedTotalFake struct {
	*seekFake
	walks int
}

func (f *shiftedTotalFake) GetJSON(ctx context.Context, u string, v any) error {
	if strings.Contains(u, "page=1&") {
		f.walks++
	}
	if err := f.seekFake.GetJSON(ctx, u, v); err != nil {
		return err
	}
	if f.walks == 1 {
		v.(*seekSearchPage).TotalCount++
	}
	return nil
}
