package sources

import (
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
)

// fakeStream is a StreamGetter that serves a fixed body (or error) to the decode callback
// and records the requested URL.
type fakeStream struct {
	body   string
	err    error
	gotURL string
}

func (f *fakeStream) GetStream(_ context.Context, url, _ string, fn func(io.Reader) error) error {
	f.gotURL = url
	if f.err != nil {
		return f.err
	}
	return fn(strings.NewReader(f.body))
}

func TestJobtechProvider(t *testing.T) {
	if got := NewJobtech(nil).Provider(); got != "jobtech" {
		t.Errorf("Provider() = %q, want jobtech", got)
	}
}

func TestJobtechMarkers(t *testing.T) {
	s := NewJobtech(nil)
	if _, ok := s.(boardless); !ok {
		t.Error("jobtech should implement the boardless marker")
	}
	if _, ok := s.(aggregator); !ok {
		t.Error("jobtech should implement the aggregator marker")
	}
	if _, ok := s.(selfClosing); !ok {
		t.Error("jobtech should implement the selfClosing marker (it closes removed ads itself)")
	}
}

func TestJobtechRegisteredAndFilterable(t *testing.T) {
	if _, ok := All(nil)["jobtech"]; !ok {
		t.Error("All() should register provider jobtech")
	}
	if !slices.Contains(FilterableProviders(), "jobtech") {
		t.Error("FilterableProviders() should include jobtech")
	}
	if !slices.Contains(SelfClosingProviders(All(nil)), "jobtech") {
		t.Error("SelfClosingProviders() should include jobtech")
	}
}

func TestJobtechBoardFileValidates(t *testing.T) {
	cfg, err := LoadConfig("../../sources/jobtech.yml")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if err := cfg.Validate(All(nil)); err != nil {
		t.Fatalf("sources/jobtech.yml fails validation: %v", err)
	}
}

func TestJobtechStreamMapsLiveClosesRemovedDropsInvalid(t *testing.T) {
	// One live ad (mapped + sanitized), one removed ad (emitted as a close), one ad with no
	// employer (dropped — it would break the company slug).
	body := `[
{"id":"31223446","removed":false,"webpage_url":"https://arbetsformedlingen.se/platsbanken/annonser/31223446","headline":"Junior Test Automation Engineer","publication_date":"2026-06-10T12:00:00","description":{"text":"plain","text_formatted":"<p>About the <b>Role</b></p><script>evil()</script>"},"employer":{"name":"Tatvanord AB"},"workplace_address":{"municipality":"Göteborg","region":"Västra Götalands län","country":"Sverige"}},
{"id":"555","removed":true,"employer":{"name":""}},
{"id":"999","removed":false,"headline":"No employer drop","publication_date":"2026-06-10T11:00:00","employer":{"name":""}}
]`
	fake := &fakeStream{body: body}
	jobs, err := NewJobtech(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2 (1 live + 1 removed close; no-employer dropped)", len(jobs))
	}

	live := jobs[0]
	if live.ExternalID != "31223446" || live.Removed {
		t.Errorf("live ad: ExternalID=%q Removed=%v, want id + not removed", live.ExternalID, live.Removed)
	}
	if live.URL != "https://arbetsformedlingen.se/platsbanken/annonser/31223446" {
		t.Errorf("URL = %q, want webpage_url", live.URL)
	}
	if live.Company != "Tatvanord AB" || live.Title != "Junior Test Automation Engineer" {
		t.Errorf("bad mapping: %+v", live)
	}
	if live.Location != "Göteborg, Västra Götalands län, Sverige" {
		t.Errorf("Location = %q, want joined workplace_address", live.Location)
	}
	if !strings.Contains(live.Description, "Role") || strings.Contains(live.Description, "script") || strings.Contains(live.Description, "evil") {
		t.Errorf("Description not sanitized as expected: %q", live.Description)
	}
	if live.PostedAt == nil {
		t.Error("PostedAt nil, want parsed publication_date")
	}
	if live.Remote || live.WorkMode != "" {
		t.Errorf("Remote=%v WorkMode=%q, want unset (left to the location dictionary)", live.Remote, live.WorkMode)
	}

	removed := jobs[1]
	if removed.ExternalID != "555" || !removed.Removed {
		t.Errorf("removed ad: ExternalID=%q Removed=%v, want id 555 + removed", removed.ExternalID, removed.Removed)
	}
}

func TestJobtechPrefersWorkplaceOverLegalName(t *testing.T) {
	// Swedish ads often register under a numbered shell AB (employer.name, e.g. "Miro
	// 461704 AB") while the real trading name sits in employer.workplace ("Direkten Nöje
	// Casablanca"). The workplace is the meaningful company display — and using the shell
	// name would let logo.dev fuzzy-match a well-known brand ("Miro") onto the wrong firm.
	// Prefer workplace, fall back to name when the ad carries no workplace.
	body := `[
{"id":"1","removed":false,"webpage_url":"u","headline":"Butikssäljare","publication_date":"2026-06-10T12:00:00","employer":{"name":"Miro 461704 AB","workplace":"Direkten Nöje Casablanca"}},
{"id":"2","removed":false,"webpage_url":"u","headline":"h","publication_date":"2026-06-10T12:00:00","employer":{"name":"Tatvanord AB"}}
]`
	jobs, err := NewJobtech(&fakeStream{body: body}).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2", len(jobs))
	}
	if jobs[0].Company != "Direkten Nöje Casablanca" {
		t.Errorf("Company = %q, want the workplace trading name over the shell AB", jobs[0].Company)
	}
	if jobs[1].Company != "Tatvanord AB" {
		t.Errorf("Company = %q, want employer.name fallback when workplace is empty", jobs[1].Company)
	}
}

func TestJobtechRequestsTrailingWindow(t *testing.T) {
	fake := &fakeStream{body: `[]`}
	if _, err := NewJobtech(fake).Fetch(context.Background(), CompanyEntry{}); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !strings.Contains(fake.gotURL, "jobstream.api.jobtechdev.se/stream") || !strings.Contains(fake.gotURL, "date=") {
		t.Errorf("request URL = %q, want the JobStream /stream endpoint with a date window", fake.gotURL)
	}
}

func TestJobtechKeepsPartialOnMidStreamDrop(t *testing.T) {
	// The throttled feed drops mid-array: the first element decodes, then the body is
	// truncated. The already-emitted job must survive (partial result), not error.
	body := `[
{"id":"1","removed":false,"webpage_url":"u","headline":"h","publication_date":"2026-06-10T12:00:00","employer":{"name":"C"}},
{"id":"2","removed":fal`
	fake := &fakeStream{body: body}
	jobs, err := NewJobtech(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch should swallow a mid-stream drop after progress, got: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 (the element decoded before the drop)", len(jobs))
	}
}

func TestJobtechErrorsWhenStreamFailsImmediately(t *testing.T) {
	if _, err := NewJobtech(&fakeStream{err: errors.New("boom")}).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Error("Fetch should return an error when the stream request fails")
	}
	// A body that is not a JSON array fails before any progress → a board error.
	if _, err := NewJobtech(&fakeStream{body: `{"oops":1}`}).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Error("Fetch should error when the body is not a JSON array")
	}
}
