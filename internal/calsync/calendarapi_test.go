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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	reader := NewAPIReader(srv.Client())
	// Point the reader at the stub by rewriting the request host through a transport,
	// so the production URL constant stays the one under test.
	reader.client = &http.Client{Transport: rewriteHost{to: srv.URL, base: srv.Client().Transport}}
	return reader, &gotQuery
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
