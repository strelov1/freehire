package calsync

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// The response shapes Google actually returns: a timed meeting, an all-day entry that
// carries `date` instead of `dateTime`, a cancellation, and two rows the reader must drop
// because there is nothing to key or place them by.
const eventsJSON = `{"items":[
  {"iCalUID":"timed@google.com","summary":"Technical screen","status":"confirmed",
   "hangoutLink":"https://meet.google.com/abc",
   "organizer":{"email":"recruiter@derq.com"},
   "start":{"dateTime":"2026-08-13T09:00:00Z"},"end":{"dateTime":"2026-08-13T10:00:00Z"}},
  {"iCalUID":"allday@google.com","summary":"Onsite day","status":"confirmed",
   "start":{"date":"2026-08-20"},"end":{"date":"2026-08-21"}},
  {"id":"evt-gone","iCalUID":"gone@google.com","summary":"Screen","status":"cancelled",
   "start":{"dateTime":"2026-08-14T09:00:00Z"}},
  {"id":"evt-minimal","status":"cancelled"},
  {"iCalUID":"","summary":"No identifier","start":{"dateTime":"2026-08-15T09:00:00Z"}},
  {"iCalUID":"nostart@google.com","summary":"No start"}
]}`

func readerAgainst(t *testing.T, body string) (*APIReader, *string) {
	t.Helper()
	var gotQuery string
	reader := readerAgainstFunc(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	})
	return reader, &gotQuery
}

// readerAgainstFunc is readerAgainst generalized to a per-request handler, for tests where
// the response must depend on which request is being served — pagination, chiefly, where a
// fixed body (readerAgainst) cannot distinguish the first page's request from the second's.
func readerAgainstFunc(t *testing.T, handler http.HandlerFunc) *APIReader {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	reader := NewAPIReader(srv.Client())
	// Point the reader at the stub by rewriting the request host through a transport,
	// so the production URL constant stays the one under test.
	reader.client = &http.Client{Transport: rewriteHost{to: srv.URL, base: srv.Client().Transport}}
	return reader
}

type rewriteHost struct {
	to   string
	base http.RoundTripper
}

func (t rewriteHost) RoundTrip(r *http.Request) (*http.Response, error) {
	stub, err := http.NewRequestWithContext(r.Context(), r.Method, t.to+"?"+r.URL.RawQuery, nil)
	if err != nil {
		return nil, err
	}
	return http.DefaultTransport.RoundTrip(stub)
}

func TestListEventsReadsTheShapesGoogleReturns(t *testing.T) {
	reader, _ := readerAgainst(t, eventsJSON)

	got, err := reader.ListEvents(context.Background(), time.Now(), time.Now().AddDate(0, 3, 0))
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}

	if len(got) != 4 {
		t.Fatalf("read %d meetings, want 4 — the two LIVE rows without an identifier or a start are unusable", len(got))
	}
	timed := got[0]
	if timed.UID != "timed@google.com" || timed.JoinURL != "https://meet.google.com/abc" {
		t.Errorf("timed meeting = %+v", timed)
	}
	if want := time.Date(2026, 8, 13, 9, 0, 0, 0, time.UTC); !timed.StartsAt.Equal(want) {
		t.Errorf("starts_at = %v, want %v", timed.StartsAt, want)
	}
	// An all-day entry carries `date` and no clock. Reading only dateTime would drop it
	// silently, and an onsite day is exactly the kind of thing booked that way.
	// Midday, not midnight: the calendar groups by the reader's local day, and midnight
	// UTC is the previous day for everyone west of Greenwich — which is the onsite-day
	// case this branch exists for.
	if want := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC); !got[1].StartsAt.Equal(want) {
		t.Errorf("all-day starts_at = %v, want midday %v", got[1].StartsAt, want)
	}
	if !got[2].Cancelled {
		t.Error("a cancelled event did not report itself cancelled; the worker would store it as current")
	}
	// The shape Google documents for a deleted event: `id` and nothing else. The live
	// rules would discard it, and the cancellation would never reach the store — which is
	// how a called-off interview stands on a calendar forever.
	minimal := got[3]
	if !minimal.Cancelled || minimal.ProviderID != "evt-minimal" || minimal.UID != "" {
		t.Errorf("a minimal cancellation read as %+v, want it cancelled under the provider's id alone", minimal)
	}
}

// Cancellations are the reason showDeleted is asked for: they are facts about meetings
// already stored, and a window that omitted them would leave a called-off interview
// standing on the calendar forever.
func TestListEventsAsksForTheWindowItNeeds(t *testing.T) {
	reader, query := readerAgainst(t, `{"items":[]}`)

	if _, err := reader.ListEvents(context.Background(), time.Now(), time.Now().AddDate(0, 3, 0)); err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	for _, want := range []string{"singleEvents=true", "showDeleted=true", "timeMin=", "timeMax="} {
		if !strings.Contains(*query, want) {
			t.Errorf("request query %q is missing %q", *query, want)
		}
	}
}

// TestListEventsAssemblesMultiplePages guards the reason maxEvents bounds one page, not the
// window (see its doc comment): a page that came back full must trigger a follow-up request
// carrying NextPageToken, and the assembled result must hold every page's items, in order —
// not just the first page's, which would silently truncate the FUTURE end of the window.
func TestListEventsAssemblesMultiplePages(t *testing.T) {
	pages := map[string]string{
		"": `{"items":[{"iCalUID":"page1@google.com","summary":"First","status":"confirmed",
		      "start":{"dateTime":"2026-08-13T09:00:00Z"},"end":{"dateTime":"2026-08-13T10:00:00Z"}}],
		     "nextPageToken":"page2"}`,
		"page2": `{"items":[{"iCalUID":"page2@google.com","summary":"Second","status":"confirmed",
		      "start":{"dateTime":"2026-08-14T09:00:00Z"},"end":{"dateTime":"2026-08-14T10:00:00Z"}}]}`,
	}
	var requestedTokens []string
	reader := readerAgainstFunc(t, func(w http.ResponseWriter, r *http.Request) {
		token := r.URL.Query().Get("pageToken")
		requestedTokens = append(requestedTokens, token)
		body, ok := pages[token]
		if !ok {
			t.Fatalf("unexpected pageToken %q", token)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	})

	got, err := reader.ListEvents(context.Background(), time.Now(), time.Now().AddDate(0, 3, 0))
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if len(requestedTokens) != 2 || requestedTokens[1] != "page2" {
		t.Fatalf("requested pageTokens = %v, want [\"\", \"page2\"] (one request per page)", requestedTokens)
	}
	if len(got) != 2 {
		t.Fatalf("read %d meetings across pages, want 2 (one per page)", len(got))
	}
	if got[0].UID != "page1@google.com" || got[1].UID != "page2@google.com" {
		t.Errorf("meetings = %+v, want page1's event then page2's, in order", got)
	}
}

// TestListEventsStopsAtMaxPages guards the maxPages cap: a calendar (or a broken server)
// that keeps returning a NextPageToken forever must not spin ListEvents forever — the walk
// stops after exactly maxPages requests and returns what it has, rather than erroring or
// hanging.
func TestListEventsStopsAtMaxPages(t *testing.T) {
	var requests int
	reader := readerAgainstFunc(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[],"nextPageToken":"more"}`))
	})

	if _, err := reader.ListEvents(context.Background(), time.Now(), time.Now().AddDate(0, 3, 0)); err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if requests != maxPages {
		t.Errorf("made %d requests, want exactly maxPages=%d", requests, maxPages)
	}
}
