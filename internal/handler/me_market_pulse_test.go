package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/userprofile"
)

func historyDate(s string) pgtype.Date {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return pgtype.Date{Time: t, Valid: true}
}

func TestBuildSkillPulses(t *testing.T) {
	rows := []db.InsightsSkillHistory{
		// go: two snapshots, newest first (matches ListInsightsSkillHistory's ORDER BY).
		{Skill: "go", WeekStart: historyDate("2026-08-10"), OpenCount: 120},
		{Skill: "go", WeekStart: historyDate("2026-08-03"), OpenCount: 100},
		// rust: exactly one snapshot.
		{Skill: "rust", WeekStart: historyDate("2026-08-10"), OpenCount: 40},
		// zero-open-earliest: guards the percent-change divide-by-zero.
		{Skill: "zig", WeekStart: historyDate("2026-08-10"), OpenCount: 5},
		{Skill: "zig", WeekStart: historyDate("2026-08-03"), OpenCount: 0},
	}
	// "php" is in the profile but has no rows at all (never seen in an open job) — must
	// be omitted, not reported with a fabricated count.
	got := buildSkillPulses([]string{"go", "rust", "zig", "php"}, rows)

	if len(got) != 3 {
		t.Fatalf("len(got) = %d, want 3 (php omitted)", len(got))
	}

	goP := got[0]
	if goP.Skill != "go" || goP.OpenCount != 120 {
		t.Errorf("go = %+v, want skill=go open_count=120", goP)
	}
	if len(goP.Series) != 2 || goP.Series[0].OpenCount != 100 || goP.Series[1].OpenCount != 120 {
		t.Errorf("go series = %+v, want ascending [100, 120]", goP.Series)
	}
	if goP.ChangePct == nil || *goP.ChangePct != 20 {
		t.Errorf("go change_pct = %v, want 20 (100 -> 120)", goP.ChangePct)
	}

	rustP := got[1]
	if rustP.ChangePct != nil {
		t.Errorf("rust change_pct = %v, want nil (only one snapshot)", *rustP.ChangePct)
	}
	if len(rustP.Series) != 1 {
		t.Errorf("rust series len = %d, want 1", len(rustP.Series))
	}

	zigP := got[2]
	if zigP.ChangePct != nil {
		t.Errorf("zig change_pct = %v, want nil (earliest open_count is 0)", *zigP.ChangePct)
	}
}

// fakeSkillHistoryReader is a skillHistoryReader that returns canned rows and records
// the requested skills, so the handler's profile-to-history wiring is testable without
// a database.
type fakeSkillHistoryReader struct {
	rows      []db.InsightsSkillHistory
	gotSkills []string
}

func (f *fakeSkillHistoryReader) ListInsightsSkillHistory(_ context.Context, skills []string) ([]db.InsightsSkillHistory, error) {
	f.gotSkills = skills
	return f.rows, nil
}

func marketPulseApp(t *testing.T, repo *fakeProfileRepo, history *fakeSkillHistoryReader) (*fiber.App, string) {
	t.Helper()
	iss := auth.NewIssuer("test-secret", time.Hour)
	token, err := iss.Issue(1, testTokenVersion)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	h := &marketPulseHandlers{userProfile: userprofile.New(repo), history: history}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	g := auth.RequireAuth(iss, testVersions)
	app.Get("/me/market-pulse", g, h.GetMarketPulse)
	return app, token
}

func doMarketPulse(t *testing.T, app *fiber.App, token string) *http.Response {
	t.Helper()
	r := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/me/market-pulse", nil)
	if token != "" {
		r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	}
	resp, err := app.Test(r)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	return resp
}

// marketPulseBody is the wire shape this test file decodes responses into, to confirm
// the actual JSON on the wire (not just status code and mock call args).
type marketPulseBody struct {
	Data []skillPulse `json:"data"`
	Meta struct {
		WeekStart string `json:"week_start"`
	} `json:"meta"`
}

func decodeMarketPulse(t *testing.T, resp *http.Response) marketPulseBody {
	t.Helper()
	var body marketPulseBody
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	return body
}

func TestMarketPulseWithProfileSkills(t *testing.T) {
	repo := &fakeProfileRepo{getRet: userprofile.Profile{Skills: []string{"go", "rust"}}}
	history := &fakeSkillHistoryReader{rows: []db.InsightsSkillHistory{
		{Skill: "go", WeekStart: historyDate("2026-08-10"), OpenCount: 120},
		{Skill: "go", WeekStart: historyDate("2026-08-03"), OpenCount: 100},
	}}
	app, token := marketPulseApp(t, repo, history)

	resp := doMarketPulse(t, app, token)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if got := history.gotSkills; len(got) != 2 || got[0] != "go" || got[1] != "rust" {
		t.Errorf("history queried with %v, want [go rust] (the profile's own skills)", got)
	}
	body := decodeMarketPulse(t, resp)
	if len(body.Data) != 1 || body.Data[0].Skill != "go" {
		t.Fatalf("data = %+v, want one entry for go (rust has no history, omitted)", body.Data)
	}
	if body.Meta.WeekStart == "" {
		t.Errorf("meta.week_start is empty, want the resolved current week")
	}
}

func TestMarketPulseEmptyProfileReturnsEmptyData(t *testing.T) {
	repo := &fakeProfileRepo{getErr: userprofile.ErrNotFound}
	history := &fakeSkillHistoryReader{}
	app, token := marketPulseApp(t, repo, history)

	resp := doMarketPulse(t, app, token)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if history.gotSkills != nil {
		t.Errorf("history reader called with an empty profile; should not be reached")
	}
	body := decodeMarketPulse(t, resp)
	if body.Data == nil || len(body.Data) != 0 {
		t.Errorf("data = %v, want a non-null empty array", body.Data)
	}
	if body.Meta.WeekStart == "" {
		t.Errorf("meta.week_start is empty, want the resolved current week even for an empty profile")
	}
}

func TestMarketPulseUnauthenticatedRejected(t *testing.T) {
	repo := &fakeProfileRepo{}
	history := &fakeSkillHistoryReader{}
	app, _ := marketPulseApp(t, repo, history)

	resp := doMarketPulse(t, app, "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}
