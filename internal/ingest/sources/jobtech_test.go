package sources

import (
	"context"
	"errors"
	"fmt"
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

	// Removals are emitted before live ads (TestJobtechAppliesRemovalsBeforeLiveAds).
	removed, live := jobs[0], jobs[1]
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

func TestJobtechKeepsPartialButReportsATruncatedBody(t *testing.T) {
	// The body ends mid-array: the first element decodes, the rest is cut off. The decoded job
	// is still emitted, but the run did not read its window — and the removals the feed lists
	// LAST are what a cut-short read loses — so it must not pass for a complete read.
	body := `[
{"id":"1","removed":false,"webpage_url":"u","headline":"h","publication_date":"2026-06-10T12:00:00","employer":{"name":"C"}},
{"id":"2","removed":fal`
	fake := &fakeStream{body: body}
	jobs, err := NewJobtech(fake).Fetch(context.Background(), CompanyEntry{})
	if err == nil {
		t.Fatal("Fetch should report a body cut short mid-array")
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 (the element decoded before the cut)", len(jobs))
	}
}

// droppingStream serves body and then fails the read, the way a connection reset or the
// stream client's timeout ends a body part-way.
type droppingStream struct{ body string }

func (d droppingStream) GetStream(_ context.Context, _, _ string, fn func(io.Reader) error) error {
	return fn(io.MultiReader(strings.NewReader(d.body), failingReader{}))
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("connection reset") }

func TestJobtechReportsADroppedConnection(t *testing.T) {
	body := `[
{"id":"1","removed":false,"webpage_url":"u","headline":"h","publication_date":"2026-06-10T12:00:00","employer":{"name":"C"}},
`
	jobs, err := NewJobtech(droppingStream{body: body}).Fetch(context.Background(), CompanyEntry{})
	if err == nil || !strings.Contains(err.Error(), "connection reset") {
		t.Fatalf("Fetch error = %v, want the dropped read reported", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 (what arrived before the drop)", len(jobs))
	}
}

// JobStream lists a window's live ads first and its removals after them (measured 2026-10-31:
// 6,692 live ads, then 11,053 removals). Emitting in that order put every removal behind
// thousands of database writes, so a run that ran out of time closed nothing.
func TestJobtechAppliesRemovalsBeforeLiveAds(t *testing.T) {
	body := `[
{"id":"1","removed":false,"webpage_url":"u","headline":"h","publication_date":"2026-06-10T12:00:00","employer":{"name":"C"}},
{"id":"2","removed":false,"webpage_url":"u","headline":"h","publication_date":"2026-06-10T12:00:00","employer":{"name":"C"}},
{"id":"3","removed":true},
{"id":"4","removed":true}
]`
	jobs, err := NewJobtech(&fakeStream{body: body}).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	var got []string
	for _, j := range jobs {
		got = append(got, fmt.Sprintf("%s:%v", j.ExternalID, j.Removed))
	}
	want := []string{"3:true", "4:true", "1:false", "2:false"}
	if !slices.Equal(got, want) {
		t.Errorf("emit order = %v, want removals first %v", got, want)
	}
}

// Emitting removals first must not let a later live emit of the same ad reopen what it closed.
func TestJobtechDoesNotEmitARemovedAdLive(t *testing.T) {
	body := `[
{"id":"7","removed":false,"webpage_url":"u","headline":"h","publication_date":"2026-06-10T12:00:00","employer":{"name":"C"}},
{"id":"7","removed":true}
]`
	jobs, err := NewJobtech(&fakeStream{body: body}).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || !jobs[0].Removed {
		t.Errorf("got %+v, want only the removal of ad 7", jobs)
	}
}

// eofSignallingStream serves body and records when the body has been read to its end.
type eofSignallingStream struct {
	body string
	done *bool
}

func (s eofSignallingStream) GetStream(_ context.Context, _, _ string, fn func(io.Reader) error) error {
	return fn(eofSignallingReader{r: strings.NewReader(s.body), done: s.done})
}

type eofSignallingReader struct {
	r    io.Reader
	done *bool
}

func (s eofSignallingReader) Read(p []byte) (int, error) {
	n, err := s.r.Read(p)
	if err == io.EOF {
		*s.done = true
	}
	return n, err
}

// Every emit is a database write from the crawl host, a few a second, while the feed's 72h
// window runs to ~18k items. Decoding while writing held the download open behind the writes
// until the stream client's timeout ended it a few thousand items in, short of every removal.
// The window must be read to its end before the first emit.
func TestJobtechReadsTheWholeWindowBeforeEmitting(t *testing.T) {
	body := `[
{"id":"1","removed":false,"webpage_url":"u","headline":"h","publication_date":"2026-06-10T12:00:00","employer":{"name":"C"}},
{"id":"2","removed":true}
]`
	done := false
	src := NewJobtech(eofSignallingStream{body: body, done: &done}).(StreamingSource)
	emitted := 0
	err := src.FetchStream(context.Background(), CompanyEntry{}, func(j Job) {
		emitted++
		if !done {
			t.Errorf("emitted %s before the stream was read to its end", j.ExternalID)
		}
	})
	if err != nil {
		t.Fatalf("FetchStream: %v", err)
	}
	if emitted != 2 {
		t.Errorf("emitted %d, want 2", emitted)
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
