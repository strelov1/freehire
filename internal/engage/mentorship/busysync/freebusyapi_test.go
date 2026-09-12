package busysync

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/application/gmailsync"
)

// The response shape Google actually returns for a freeBusy query: one calendar
// ("primary") carrying a busy[] array of {start, end}. No title, no attendee — the
// endpoint has nothing else to offer.
const freeBusyJSON = `{"calendars":{"primary":{"busy":[
  {"start":"2026-08-13T09:00:00Z","end":"2026-08-13T10:00:00Z"},
  {"start":"2026-08-14T14:00:00Z","end":"2026-08-14T15:30:00Z"}
]}}}`

// readerAgainst returns the reader plus pointers the caller reads AFTER making the
// request — the request itself only happens inside reader.ListBusy, so a plain returned
// value would always read as the zero value.
func readerAgainst(t *testing.T, handler http.HandlerFunc) (reader *APIReader, gotReq **http.Request, gotBody *[]byte) {
	t.Helper()
	gotReq = new(*http.Request)
	gotBody = new([]byte)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*gotReq = r
		*gotBody, _ = io.ReadAll(r.Body)
		handler(w, r)
	}))
	t.Cleanup(srv.Close)

	reader = NewAPIReader(srv.Client())
	reader.client = &http.Client{Transport: rewriteHost{to: srv.URL, base: srv.Client().Transport}}
	return reader, gotReq, gotBody
}

type rewriteHost struct {
	to   string
	base http.RoundTripper
}

func (t rewriteHost) RoundTrip(r *http.Request) (*http.Response, error) {
	stub, err := http.NewRequestWithContext(r.Context(), r.Method, t.to, r.Body)
	if err != nil {
		return nil, err
	}
	stub.Header = r.Header
	return http.DefaultTransport.RoundTrip(stub)
}

func TestListBusyReadsThePeriodsGoogleReturns(t *testing.T) {
	reader, _, _ := readerAgainst(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(freeBusyJSON))
	})

	got, err := reader.ListBusy(context.Background(), time.Now(), time.Now().AddDate(0, 0, 60))
	if err != nil {
		t.Fatalf("ListBusy: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("read %d periods, want 2", len(got))
	}
	wantStart := time.Date(2026, 8, 13, 9, 0, 0, 0, time.UTC)
	wantEnd := time.Date(2026, 8, 13, 10, 0, 0, 0, time.UTC)
	if !got[0].Start.Equal(wantStart) || !got[0].End.Equal(wantEnd) {
		t.Errorf("period 0 = %+v, want [%v, %v]", got[0], wantStart, wantEnd)
	}
}

// The request itself: a POST naming exactly the primary calendar and the window asked
// for, since a caller giving the wrong window would silently get the wrong answer.
func TestListBusyAsksForThePrimaryCalendarAndTheWindow(t *testing.T) {
	from := time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC)
	reader, req, body := readerAgainst(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"calendars":{"primary":{"busy":[]}}}`))
	})

	if _, err := reader.ListBusy(context.Background(), from, to); err != nil {
		t.Fatalf("ListBusy: %v", err)
	}
	if (*req).Method != http.MethodPost {
		t.Errorf("method = %s, want POST", (*req).Method)
	}
	var sent struct {
		TimeMin string `json:"timeMin"`
		TimeMax string `json:"timeMax"`
		Items   []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(*body, &sent); err != nil {
		t.Fatalf("decode sent body %q: %v", *body, err)
	}
	if sent.TimeMin != from.Format(time.RFC3339) {
		t.Errorf("timeMin = %q, want %q", sent.TimeMin, from.Format(time.RFC3339))
	}
	if sent.TimeMax != to.Format(time.RFC3339) {
		t.Errorf("timeMax = %q, want %q", sent.TimeMax, to.Format(time.RFC3339))
	}
	if len(sent.Items) != 1 || sent.Items[0].ID != "primary" {
		t.Errorf("items = %+v, want exactly [{id: primary}]", sent.Items)
	}
}

// A period Google returns in a shape ListBusy cannot parse must not fail the whole read
// or silently vanish without a trace — it is dropped, and every OTHER period in the same
// response still comes through, matching how the codebase already treats an unexpected
// API shape elsewhere (meetAPI's pending-conference case).
func TestListBusyDropsAnUnparseablePeriodButKeepsTheRest(t *testing.T) {
	reader, _, _ := readerAgainst(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"calendars":{"primary":{"busy":[
		  {"start":"not-a-time","end":"2026-08-13T10:00:00Z"},
		  {"start":"2026-08-14T14:00:00Z","end":"2026-08-14T15:30:00Z"}
		]}}}`))
	})

	got, err := reader.ListBusy(context.Background(), time.Now(), time.Now().AddDate(0, 0, 60))
	if err != nil {
		t.Fatalf("ListBusy: %v, want no error for one malformed period among valid ones", err)
	}
	if len(got) != 1 {
		t.Fatalf("read %d periods, want the 1 well-formed one", len(got))
	}
	if !got[0].Start.Equal(time.Date(2026, 8, 14, 14, 0, 0, 0, time.UTC)) {
		t.Errorf("kept period = %+v, want the well-formed one", got[0])
	}
}

// A non-2xx response wraps as gmailsync.APIError so RevokedGrant can classify it —
// exactly the shape calsync's own reader uses, since the two consents share one grant
// and one status flag.
func TestListBusyWrapsAFailingResponse(t *testing.T) {
	reader, _, _ := readerAgainst(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})

	_, err := reader.ListBusy(context.Background(), time.Now(), time.Now().AddDate(0, 0, 60))
	if err == nil {
		t.Fatal("ListBusy succeeded against a 403")
	}
	var apiErr *gmailsync.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error %v is not a *gmailsync.APIError", err)
	}
	if apiErr.StatusCode != http.StatusForbidden {
		t.Errorf("StatusCode = %d, want 403", apiErr.StatusCode)
	}
	if !gmailsync.RevokedGrant(err) {
		t.Error("a 403 must be classified as a revoked grant")
	}
}
