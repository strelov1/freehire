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

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/jobtracking"
	"github.com/strelov1/freehire/internal/jobview"
	"github.com/strelov1/freehire/internal/userjob"
)

// silenceRepo serves a fixed page of tracked rows, so the test pins what the
// handler makes of them rather than how they were stored.
type silenceRepo struct {
	stubTrackingRepo
	items []jobtracking.TrackedJob
}

func (r silenceRepo) ListInteractions(context.Context, int64, jobtracking.Filter, int32, int32) ([]jobtracking.TrackedJob, error) {
	return r.items, nil
}

func (r silenceRepo) CountInteractions(context.Context, int64) (jobtracking.Counts, error) {
	return jobtracking.Counts{All: int64(len(r.items))}, nil
}

// trackingRow is one decoded listing item, narrowed to the silence fields.
type trackingRow struct {
	Stage          *string    `json:"stage"`
	AppliedAt      *time.Time `json:"applied_at"`
	LastActivityAt *time.Time `json:"last_activity_at"`
	DaysSilent     *int       `json:"days_silent"`
	SilenceState   *string    `json:"silence_state"`
	FollowedUpAt   *time.Time `json:"followed_up_at"`
}

func listSilence(t *testing.T, items []jobtracking.TrackedJob) []trackingRow {
	t.Helper()
	iss := auth.NewIssuer("test-secret-that-is-long-enough-0001", time.Hour)
	token, _ := iss.Issue(1, testTokenVersion)
	h := &trackingHandlers{tracking: jobtracking.New(silenceRepo{items: items})}

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/me/tracking", auth.RequireAuth(iss, testVersions), h.ListTrackedJobs)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/me/tracking", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("list = %d, want 200 (%s)", resp.StatusCode, raw)
	}
	var body struct {
		Data []trackingRow `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return body.Data
}

func row(stage string, appliedAgo, activityAgo time.Duration, pending bool) jobtracking.TrackedJob {
	j := jobtracking.TrackedJob{
		Job:                  &jobview.Card{PublicSlug: "a-job"},
		HasPendingSuggestion: pending,
	}
	if appliedAgo > 0 {
		at := time.Now().Add(-appliedAgo)
		j.AppliedAt = &at
	}
	if stage != "" {
		j.Stage = &stage
	}
	if activityAgo > 0 {
		at := time.Now().Add(-activityAgo)
		j.LastActivityAt = &at
	}
	return j
}

const oneDay = 24 * time.Hour

// TestTrackingSilenceFieldsOnApplications asserts an application carries all
// three silence fields, and that the state reflects its stage rather than the
// raw elapsed time.
func TestTrackingSilenceFieldsOnApplications(t *testing.T) {
	rows := listSilence(t, []jobtracking.TrackedJob{
		row("applied", 40*oneDay, 30*oneDay, false),   // 30 days silent, threshold 21
		row("applied", 40*oneDay, 5*oneDay, false),    // 5 days silent
		row("interview", 40*oneDay, 13*oneDay, false), // 13 days silent, threshold 12
	})
	if len(rows) != 3 {
		t.Fatalf("listing returned %d rows, want 3", len(rows))
	}
	want := []struct {
		days  int
		state string
	}{
		{30, userjob.SilenceSilent},
		{5, userjob.SilenceActive},
		{13, userjob.SilenceSilent},
	}
	for i, w := range want {
		got := rows[i]
		if got.LastActivityAt == nil || got.DaysSilent == nil || got.SilenceState == nil {
			t.Fatalf("row %d is missing silence fields: %+v", i, got)
		}
		if *got.DaysSilent != w.days {
			t.Errorf("row %d days_silent = %d, want %d", i, *got.DaysSilent, w.days)
		}
		if *got.SilenceState != w.state {
			t.Errorf("row %d silence_state = %q, want %q", i, *got.SilenceState, w.state)
		}
	}
}

// TestTrackingSilenceNullOffApplications asserts every row that is not an
// application awaiting a reply carries all three fields as null — a saved job,
// and a settled application. Null is how the board tells "nothing owed" from
// "owed and answered promptly"; a zero-day active state would blur them.
func TestTrackingSilenceNullOffApplications(t *testing.T) {
	rows := listSilence(t, []jobtracking.TrackedJob{
		row("", 0, 0, false),                         // saved only, never applied
		row("rejected", 40*oneDay, 40*oneDay, false), // settled
		row("accepted", 40*oneDay, 40*oneDay, false), // settled
	})
	for i, got := range rows {
		if got.LastActivityAt != nil || got.DaysSilent != nil || got.SilenceState != nil {
			t.Errorf("row %d carries silence fields, want all null: %+v", i, got)
		}
	}
}

// TestTrackingSilenceUnconfirmed asserts mail awaiting confirmation turns the
// verdict into a question rather than an assertion.
func TestTrackingSilenceUnconfirmed(t *testing.T) {
	rows := listSilence(t, []jobtracking.TrackedJob{
		row("applied", 40*oneDay, 30*oneDay, true),
	})
	if len(rows) != 1 || rows[0].SilenceState == nil {
		t.Fatalf("unexpected listing: %+v", rows)
	}
	if *rows[0].SilenceState != userjob.SilenceUnconfirmed {
		t.Errorf("silence_state = %q, want %q", *rows[0].SilenceState, userjob.SilenceUnconfirmed)
	}
}

// TestTrackingCarriesTheChaseBesideTheSilence asserts a chased application reports both readings
// at once: it is still silent for the full elapsed time, and it additionally says when it was
// chased. The board renders a third state from that pair, so dropping either half — omitting
// followed_up_at from the wire, or letting the chase move the clock — collapses it back to two.
func TestTrackingCarriesTheChaseBesideTheSilence(t *testing.T) {
	chased := row("applied", 40*oneDay, 30*oneDay, false)
	at := time.Now().Add(-2 * oneDay)
	chased.FollowedUpAt = &at

	rows := listSilence(t, []jobtracking.TrackedJob{chased, row("applied", 40*oneDay, 30*oneDay, false)})
	if len(rows) != 2 {
		t.Fatalf("listing returned %d rows, want 2", len(rows))
	}
	if rows[0].FollowedUpAt == nil {
		t.Fatal("a chased application reports no followed_up_at; the board cannot say it was chased")
	}
	if !rows[0].FollowedUpAt.Equal(at) {
		t.Errorf("followed_up_at = %v, want %v", rows[0].FollowedUpAt, at)
	}
	if rows[0].SilenceState == nil || *rows[0].SilenceState != userjob.SilenceSilent {
		t.Errorf("a chased application is no longer silent: %+v", rows[0])
	}
	if rows[0].DaysSilent == nil || *rows[0].DaysSilent != 30 {
		t.Errorf("chasing moved the clock: days_silent = %v, want 30", rows[0].DaysSilent)
	}
	if rows[1].FollowedUpAt != nil {
		t.Errorf("an unchased application reports followed_up_at = %v, want null", rows[1].FollowedUpAt)
	}
}
