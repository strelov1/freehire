package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/apptimeline"
	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/db"
)

// timelineStore answers with fixed rows so the envelope is asserted without a pool.
type timelineStore struct {
	rows       []db.ListApplicationEventsInRangeRow
	interviews []db.ListApplicationInterviewsInRangeRow
}

func (s timelineStore) ListApplicationEventsInRange(context.Context, db.ListApplicationEventsInRangeParams) ([]db.ListApplicationEventsInRangeRow, error) {
	return s.rows, nil
}

// The single-application read. These cases exercise the range endpoint, which never calls it.
func (s timelineStore) ListApplicationEvents(context.Context, db.ListApplicationEventsParams) ([]db.ListApplicationEventsRow, error) {
	return nil, nil
}

// meTimelineApp mounts the range read behind RequireAuth. The store is nil unless a case
// supplies one: the auth and range cases must refuse before the service reaches it, and a
// nil dereference here is how a regression that queried first would announce itself.
func meTimelineApp(t *testing.T, store apptimeline.Queries) (*fiber.App, string) {
	t.Helper()
	iss := auth.NewIssuer("test-secret", time.Hour)
	token, err := iss.Issue(1, testTokenVersion)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	h := &timelineHandlers{timeline: apptimeline.New(store)}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/me/timeline", auth.RequireAuth(iss, testVersions), h.Timeline)
	return app, token
}

func getTimeline(t *testing.T, app *fiber.App, path, token string) (int, []byte) {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, path, nil)
	if token != "" {
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp.StatusCode, body
}

func TestTimeline_RequiresAuth(t *testing.T) {
	app, _ := meTimelineApp(t, nil)
	if got, _ := getTimeline(t, app, "/me/timeline?from=2026-08-01T00:00:00Z&to=2026-08-31T00:00:00Z", ""); got != fiber.StatusUnauthorized {
		t.Errorf("status = %d, want 401", got)
	}
}

// Each of these is refused before the store is consulted — the nil store proves it.
func TestTimeline_RefusesABadRangeBeforeReachingTheStore(t *testing.T) {
	cases := map[string]string{
		"no bounds":       "/me/timeline",
		"only a lower":    "/me/timeline?from=2026-08-01T00:00:00Z",
		"unparseable":     "/me/timeline?from=August&to=2026-08-31T00:00:00Z",
		"inverted":        "/me/timeline?from=2026-08-31T00:00:00Z&to=2026-08-01T00:00:00Z",
		"beyond the span": "/me/timeline?from=2020-01-01T00:00:00Z&to=2026-08-01T00:00:00Z",
	}
	for name, path := range cases {
		t.Run(name, func(t *testing.T) {
			app, token := meTimelineApp(t, nil)
			if got, _ := getTimeline(t, app, path, token); got != fiber.StatusBadRequest {
				t.Errorf("status = %d, want 400", got)
			}
		})
	}
}

func TestTimeline_RendersTheDocumentedEnvelope(t *testing.T) {
	at := time.Date(2026, 8, 13, 9, 41, 0, 0, time.UTC)
	app, token := meTimelineApp(t, timelineStore{rows: []db.ListApplicationEventsInRangeRow{{
		ID:           7,
		Kind:         "employer_reply",
		Signal:       "interview_invitation",
		Source:       "mail_gmail",
		OccurredAt:   pgtype.Timestamptz{Time: at, Valid: true},
		CompanySlug:  "derq",
		RoleTitle:    pgtype.Text{String: "Senior Go Engineer", Valid: true},
		EmailID:      pgtype.Int8{Int64: 42, Valid: true},
		EmailSubject: pgtype.Text{String: "Invitation to interview", Valid: true},
	}}})

	status, body := getTimeline(t, app, "/me/timeline?from=2026-08-01T00:00:00Z&to=2026-08-31T00:00:00Z", token)
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", status, body)
	}

	var got struct {
		Data []struct {
			ID           int64     `json:"id"`
			Kind         string    `json:"kind"`
			Signal       string    `json:"signal"`
			Source       string    `json:"source"`
			Observed     bool      `json:"observed"`
			OccurredAt   time.Time `json:"occurred_at"`
			CompanySlug  string    `json:"company_slug"`
			RoleTitle    string    `json:"role_title"`
			EmailID      int64     `json:"email_id"`
			EmailSubject string    `json:"email_subject"`
		} `json:"data"`
		Meta struct {
			Count int `json:"count"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	if len(got.Data) != 1 || got.Meta.Count != 1 {
		t.Fatalf("served %d events with meta count %d, want 1 and 1", len(got.Data), got.Meta.Count)
	}
	e := got.Data[0]
	if !e.Observed {
		t.Error("a gmail-sourced reply rendered as unobserved")
	}
	if !e.OccurredAt.Equal(at) {
		t.Errorf("occurred_at = %v, want the instant %v — the reader groups days, not the server", e.OccurredAt, at)
	}
	if e.Kind != "employer_reply" || e.Signal != "interview_invitation" || e.CompanySlug != "derq" {
		t.Errorf("event rendered as %+v, want the seeded reply", e)
	}
	if e.EmailID != 42 || e.EmailSubject != "Invitation to interview" {
		t.Errorf("message rendered as %d/%q, want 42 and its subject", e.EmailID, e.EmailSubject)
	}
}

// A quiet month is an answer, not a fault: the series comes back empty, reporting the
// range it answered for, so a reader can tell "nothing happened" from "I asked wrongly".
// data must serialize as [] and not null — a reader iterating it should not have to guard.
func TestTimeline_AQuietRangeAnswersEmptyAndNamesItsBounds(t *testing.T) {
	app, token := meTimelineApp(t, timelineStore{})

	status, body := getTimeline(t, app, "/me/timeline?from=2026-08-01T00:00:00Z&to=2026-08-31T00:00:00Z", token)
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", status, body)
	}
	var got struct {
		Data []timelineEvent `json:"data"`
		Meta struct {
			From  time.Time `json:"from"`
			To    time.Time `json:"to"`
			Count int       `json:"count"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	if got.Data == nil {
		t.Errorf("data came back as null in %s, want an empty array", body)
	}
	if got.Meta.Count != 0 {
		t.Errorf("meta count = %d, want 0", got.Meta.Count)
	}
	wantFrom := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	wantTo := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	if !got.Meta.From.Equal(wantFrom) || !got.Meta.To.Equal(wantTo) {
		t.Errorf("meta reports %v..%v, want the requested %v..%v", got.Meta.From, got.Meta.To, wantFrom, wantTo)
	}
}

// A hand-recorded stage change carries no message, and the wire must say so by omission
// rather than by inventing an id nobody can open.
func TestTimeline_AHandRecordedEventCarriesNoMessage(t *testing.T) {
	app, token := meTimelineApp(t, timelineStore{rows: []db.ListApplicationEventsInRangeRow{{
		ID:          8,
		Kind:        "stage_set",
		Signal:      "screening",
		Source:      "user",
		OccurredAt:  pgtype.Timestamptz{Time: time.Date(2026, 8, 13, 21, 15, 0, 0, time.UTC), Valid: true},
		CompanySlug: "linear",
	}}})

	status, body := getTimeline(t, app, "/me/timeline?from=2026-08-01T00:00:00Z&to=2026-08-31T00:00:00Z", token)
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", status, body)
	}
	var got struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	if observed := got.Data[0]["observed"]; observed != false {
		t.Errorf("a hand-recorded stage change rendered observed=%v, want false", observed)
	}
	for _, absent := range []string{"email_id", "email_subject", "role_title"} {
		if _, present := got.Data[0][absent]; present {
			t.Errorf("%q was rendered for an event that has none: %v", absent, got.Data[0])
		}
	}
}

func (s timelineStore) ListApplicationInterviewsInRange(context.Context, db.ListApplicationInterviewsInRangeParams) ([]db.ListApplicationInterviewsInRangeRow, error) {
	return s.interviews, nil
}

// The second layer: what is arranged, served beside what happened. A cancelled meeting
// comes back marked rather than withheld — an interview that simply vanished from a
// Thursday cannot be told apart from a calendar that failed to load.
func TestInterviews_ServesArrangedMeetingsIncludingCancelledOnes(t *testing.T) {
	at := time.Date(2026, 8, 13, 9, 0, 0, 0, time.UTC)
	iss := auth.NewIssuer("test-secret", time.Hour)
	token, err := iss.Issue(1, testTokenVersion)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	store := timelineStore{interviews: []db.ListApplicationInterviewsInRangeRow{
		{
			ID: 3, ApplicationID: 31,
			StartsAt: pgtype.Timestamptz{Time: at, Valid: true},
			EndsAt:   pgtype.Timestamptz{Time: at.Add(time.Hour), Valid: true},
			Title:    "Technical screen", JoinUrl: "https://meet.google.com/abc",
			Status: "confirmed", CompanySlug: "derq", RoleTitle: "Senior Go Engineer",
		},
		{
			ID: 4, ApplicationID: 32,
			StartsAt: pgtype.Timestamptz{Time: at.AddDate(0, 0, 2), Valid: true},
			Status:   "cancelled", CompanySlug: "vercel",
		},
	}}
	h := &timelineHandlers{timeline: apptimeline.New(store)}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/me/interviews", auth.RequireAuth(iss, testVersions), h.Interviews)

	status, body := getTimeline(t, app, "/me/interviews?from=2026-08-01T00:00:00Z&to=2026-08-31T00:00:00Z", token)
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", status, body)
	}
	var got struct {
		Data []scheduledInterview `json:"data"`
		Meta struct {
			Count int `json:"count"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	if len(got.Data) != 2 || got.Meta.Count != 2 {
		t.Fatalf("served %d meetings (meta %d), want 2", len(got.Data), got.Meta.Count)
	}
	if !got.Data[0].StartsAt.Equal(at) || got.Data[0].JoinURL == "" || got.Data[0].Status != "confirmed" {
		t.Errorf("first meeting = %+v", got.Data[0])
	}
	if got.Data[1].Status != "cancelled" {
		t.Errorf("second meeting status = %q, want it served as cancelled", got.Data[1].Status)
	}
	// A meeting with no end is absent, not the year 1. time.Time has no empty value for
	// encoding/json, so `omitempty` on one is inert and an all-day entry would serialise
	// as 0001-01-01 — a reader trusting it would draw a two-millennia meeting.
	if got.Data[1].EndsAt != nil {
		t.Errorf("a meeting with no end serialised %v, want the field omitted", *got.Data[1].EndsAt)
	}
}

func TestInterviews_RefusesABadRangeAndRequiresAuth(t *testing.T) {
	iss := auth.NewIssuer("test-secret", time.Hour)
	token, err := iss.Issue(1, testTokenVersion)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	h := &timelineHandlers{timeline: apptimeline.New(nil)}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/me/interviews", auth.RequireAuth(iss, testVersions), h.Interviews)

	if got, _ := getTimeline(t, app, "/me/interviews?from=2026-08-01T00:00:00Z&to=2026-08-31T00:00:00Z", ""); got != fiber.StatusUnauthorized {
		t.Errorf("unauthenticated status = %d, want 401", got)
	}
	// Refused before the store is reached — the nil store proves it, as it does for the
	// timeline read, and both share one bound rule so neither can accept what the other
	// refuses.
	if got, _ := getTimeline(t, app, "/me/interviews?from=2020-01-01T00:00:00Z&to=2026-08-01T00:00:00Z", token); got != fiber.StatusBadRequest {
		t.Errorf("over-long range status = %d, want 400", got)
	}
}
