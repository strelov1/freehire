package linksource

import (
	"context"
	"testing"
)

// resolveReg builds the real registry over a fake client routing the habr short link to a
// vacancy, a second short link to a failing fetch, and a third to the non-vacancy index.
func resolveReg() []Source {
	c := (&fakeClient{}).
		route("u.habr.com/PnBO7", habrVacancyHTML, "https://career.habr.com/vacancies/1000166712?utm_source=telegram").
		route("u.habr.com/fq3n5", habrListingHTML, "https://career.habr.com/vacancies")
	// u.habr.com/BROKEN is intentionally unrouted → fakeClient returns an error.
	return All(c)
}

func TestResolveLinksReturnsMatchedVacanciesAndSkipsRest(t *testing.T) {
	urls := []string{
		"https://t.me/habr_career/75410", // not an outbound link — no adapter
		"https://u.habr.com/PnBO7",       // → vacancy
		"https://example.com/whatever",   // unknown domain
		"https://u.habr.com/fq3n5",       // → vacancies index (skip, ok=false)
	}
	got, err := ResolveLinks(context.Background(), resolveReg(), urls)
	if err != nil {
		t.Fatalf("ResolveLinks: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("resolved = %d (%+v), want 1", len(got), got)
	}
	if got[0].Source != "habr_career" || got[0].Job.ExternalID != ":1000166712" {
		t.Errorf("resolved = %+v, want habr_career/:1000166712", got[0])
	}
}

func TestResolveLinksErrorsWhenAllMatchedLinksFail(t *testing.T) {
	// Only a matched-but-failing link: the fetch errors, nothing resolves → retry signal.
	urls := []string{"https://u.habr.com/BROKEN"}
	got, err := ResolveLinks(context.Background(), resolveReg(), urls)
	if err == nil {
		t.Fatalf("want an error so the post is retried; got jobs %+v", got)
	}
}

func TestResolveLinksNoMatchIsNotAnError(t *testing.T) {
	// Links present but no destination adapter matches → fall back to the LLM (nil, nil).
	urls := []string{"https://example.com/a", "https://t.me/x/1"}
	got, err := ResolveLinks(context.Background(), resolveReg(), urls)
	if err != nil || got != nil {
		t.Fatalf("got (%+v, %v), want (nil, nil)", got, err)
	}
}

func TestMatchesAny(t *testing.T) {
	reg := resolveReg()
	if !MatchesAny(reg, []string{"https://t.me/x/1", "https://u.habr.com/PnBO7"}) {
		t.Error("MatchesAny = false, want true (a habr link is present)")
	}
	if MatchesAny(reg, []string{"https://example.com/a", "https://t.me/x/1"}) {
		t.Error("MatchesAny = true, want false (no destination link)")
	}
}
