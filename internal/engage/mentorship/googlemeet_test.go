package mentorship

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/application/gmailsync"
	"github.com/strelov1/freehire/internal/platform/tokencrypt"
)

// fakeGrantReader stands in for the mentorship Repository's slice GoogleCalendarLinker
// needs, without a database.
type fakeGrantReader struct {
	refreshTokenEnc string
	scopes          []string
	found           bool
	err             error
}

func (f *fakeGrantReader) GetMentorCalendarGrant(context.Context, int64) (string, []string, bool, error) {
	return f.refreshTokenEnc, f.scopes, f.found, f.err
}

// A mentor with no qualifying grant — no row, or one that never covers calendar.events —
// refuses BOTH calls the same way, before any network call is attempted.
func TestGoogleCalendarLinkerNotConnected(t *testing.T) {
	cipher := mustTokenCipher(t)
	linker := NewGoogleCalendarLinker(&fakeGrantReader{found: false}, gmailsync.NewConnector("id", "secret", "https://freehire.me"), cipher)

	if _, _, err := linker.CreateMeetEvent(context.Background(), 1, MeetEventInput{}); err != ErrCalendarNotConnected {
		t.Errorf("CreateMeetEvent err = %v, want ErrCalendarNotConnected", err)
	}
	if err := linker.DeleteMeetEvent(context.Background(), 1, "evt-1"); err != ErrCalendarNotConnected {
		t.Errorf("DeleteMeetEvent err = %v, want ErrCalendarNotConnected", err)
	}
}

// A grant that exists but never covers the write scope — a candidate's read-only calendar
// grant, say — must not be treated as a mentor calendar connection.
func TestGoogleCalendarLinkerWrongScope(t *testing.T) {
	cipher := mustTokenCipher(t)
	reader := &fakeGrantReader{found: true, scopes: []string{gmailsync.CalendarScope}, refreshTokenEnc: mustEncrypt(t, cipher, "refresh")}
	linker := NewGoogleCalendarLinker(reader, gmailsync.NewConnector("id", "secret", "https://freehire.me"), cipher)

	if _, _, err := linker.CreateMeetEvent(context.Background(), 1, MeetEventInput{}); err != ErrCalendarNotConnected {
		t.Errorf("err = %v, want ErrCalendarNotConnected", err)
	}
}

func mustTokenCipher(t *testing.T) *tokencrypt.Cipher {
	t.Helper()
	c, err := tokencrypt.New([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("tokencrypt.New: %v", err)
	}
	return c
}

func mustEncrypt(t *testing.T, c *tokencrypt.Cipher, plaintext string) string {
	t.Helper()
	enc, err := c.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	return enc
}

// reqCapture holds the last request a stub server received. A pointer to it, not a bare
// *http.Request, because apiAgainst must return before the test issues the call the
// capture records.
type reqCapture struct{ req *http.Request }

// apiAgainst builds a meetAPI against a stub server, mirroring
// calsync.readerAgainstFunc's pattern: a rewriting transport keeps the production URL
// constant under test rather than parameterising it for tests alone.
func apiAgainst(t *testing.T, handler http.HandlerFunc) (*meetAPI, *reqCapture) {
	t.Helper()
	rc := &reqCapture{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rc.req = r
		handler(w, r)
	}))
	t.Cleanup(srv.Close)

	api := newMeetAPI(&http.Client{Transport: rewriteToTestServer{base: srv.URL}})
	return api, rc
}

// rewriteToTestServer sends every request to the stub server while preserving the
// original request's method, path and body — meetEventsURL's own path segment (an event
// id appended for DELETE) must survive the rewrite, unlike a query-only GET.
type rewriteToTestServer struct{ base string }

func (t rewriteToTestServer) RoundTrip(r *http.Request) (*http.Response, error) {
	stubURL := t.base + r.URL.Path
	if r.URL.RawQuery != "" {
		stubURL += "?" + r.URL.RawQuery
	}
	stub, err := http.NewRequestWithContext(r.Context(), r.Method, stubURL, r.Body)
	if err != nil {
		return nil, err
	}
	stub.Header = r.Header
	return http.DefaultTransport.RoundTrip(stub)
}

func TestMeetAPICreateEventParsesTheCanonicalResponse(t *testing.T) {
	api, req := apiAgainst(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"evt-123","hangoutLink":"https://meet.google.com/abc-defg-hij"}`))
	})

	id, link, err := api.createEvent(context.Background(), MeetEventInput{
		StartsAt:    time.Date(2026, 9, 10, 15, 0, 0, 0, time.UTC),
		EndsAt:      time.Date(2026, 9, 10, 15, 30, 0, 0, time.UTC),
		SeekerEmail: "seeker@example.com",
		Summary:     "freehire mentorship session",
	})
	if err != nil {
		t.Fatalf("createEvent: %v", err)
	}
	if id != "evt-123" {
		t.Errorf("id = %q, want evt-123", id)
	}
	if link != "https://meet.google.com/abc-defg-hij" {
		t.Errorf("link = %q", link)
	}
	if req.req.URL.Query().Get("conferenceDataVersion") != "1" {
		t.Errorf("conferenceDataVersion missing: %s", req.req.URL.RawQuery)
	}
}

// A 401/403 must come back as gmailsync.APIError so RevokedGrant recognises it — the
// signal Book() uses to mark the grant needs_reconsent.
func TestMeetAPICreateEventRevokedGrant(t *testing.T) {
	api, _ := apiAgainst(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})

	_, _, err := api.createEvent(context.Background(), MeetEventInput{})
	if err == nil {
		t.Fatal("createEvent: want an error on 403")
	}
	if !gmailsync.RevokedGrant(err) {
		t.Errorf("RevokedGrant(%v) = false, want true", err)
	}
}

// A 500 is Google having a bad day, not a revocation — RevokedGrant must say so, or a
// transient failure would cost a mentor their calendar connection.
func TestMeetAPICreateEventTransientFailureIsNotRevocation(t *testing.T) {
	api, _ := apiAgainst(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	_, _, err := api.createEvent(context.Background(), MeetEventInput{})
	if err == nil {
		t.Fatal("createEvent: want an error on 500")
	}
	if gmailsync.RevokedGrant(err) {
		t.Errorf("RevokedGrant(%v) = true, want false", err)
	}
}

func TestMeetAPIDeleteEventSuccess(t *testing.T) {
	api, req := apiAgainst(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	if err := api.deleteEvent(context.Background(), "evt-123"); err != nil {
		t.Fatalf("deleteEvent: %v", err)
	}
	if req.req.Method != http.MethodDelete {
		t.Errorf("method = %s, want DELETE", req.req.Method)
	}
}

// An event already gone (deleted by hand, say) is success: Cancel()'s best-effort cleanup
// must not treat "already achieved the outcome" as a failure.
func TestMeetAPIDeleteEventAlreadyGoneIsSuccess(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusGone} {
		api, _ := apiAgainst(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
		})
		if err := api.deleteEvent(context.Background(), "evt-123"); err != nil {
			t.Errorf("status %d: deleteEvent err = %v, want nil", status, err)
		}
	}
}
