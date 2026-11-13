package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/search/search"
)

func onboardingMatchesApp(s searcher) *fiber.App {
	h := &searchHandlers{search: s}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Post("/api/v1/onboarding/matches", newContract(contractHandlers{searchHandlers: h}))
	return app
}

type onboardingMatchesAnswer struct {
	Data []struct {
		Job          map[string]any `json:"job"`
		MatchPercent int            `json:"match_percent"`
	} `json:"data"`
	Meta struct {
		Total         int64            `json:"total"`
		Limit         int64            `json:"limit"`
		IgnoredParams []map[string]any `json:"ignored_params"`
	} `json:"meta"`
}

func postOnboardingMatches(t *testing.T, app *fiber.App, body string) (int, onboardingMatchesAnswer, string) {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost, "/api/v1/onboarding/matches", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out onboardingMatchesAnswer
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out, string(raw)
}

func TestOnboardingMatches_RanksByTheGivenSkills(t *testing.T) {
	fake := &fakeSearcher{res: search.SearchResult{
		Hits: []search.JobDocument{
			{ID: 1, Job: jobview.Job{PublicSlug: "go-dev", Title: "Go Dev", Skills: []string{"go", "kubernetes"}}},
			{ID: 2, Job: jobview.Job{PublicSlug: "java-dev", Title: "Java Dev", Skills: []string{"java", "spring", "go", "kafka"}}},
		},
		Total: 42,
	}}
	app := onboardingMatchesApp(fake)

	status, out, raw := postOnboardingMatches(t, app,
		`{"skills":["go","kubernetes"],"query":"work_mode=remote&posted_within_days=7"}`)
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200: %s", status, raw)
	}
	if fake.got.Vector == nil {
		t.Error("search ran without a skill vector — the answer would not be ranked by the CV")
	}
	if fake.got.Limit != onboardingMatchesLimit || fake.got.Offset != 0 {
		t.Errorf("limit/offset = %d/%d, want %d/0", fake.got.Limit, fake.got.Offset, onboardingMatchesLimit)
	}
	filter := fmt.Sprint(fake.got.Filter)
	if !strings.Contains(filter, "remote") || !strings.Contains(filter, "posted_ts") {
		t.Errorf("filter = %s, want the query's work_mode and posted_within_days applied", filter)
	}
	if out.Meta.Total != 42 || len(out.Data) != 2 {
		t.Fatalf("total/len = %d/%d, want 42/2: %s", out.Meta.Total, len(out.Data), raw)
	}
	if out.Data[0].Job["public_slug"] != "go-dev" || out.Data[0].MatchPercent != 100 {
		t.Errorf("first = %v / %d%%, want go-dev at 100%%", out.Data[0].Job["public_slug"], out.Data[0].MatchPercent)
	}
	if p := out.Data[1].MatchPercent; p <= 0 || p >= 100 {
		t.Errorf("second match = %d%%, want a partial match", p)
	}
	if len(out.Meta.IgnoredParams) != 0 {
		t.Errorf("ignored_params = %v, want none", out.Meta.IgnoredParams)
	}
}

func TestOnboardingMatches_ReportsAnUnknownParam(t *testing.T) {
	app := onboardingMatchesApp(&fakeSearcher{})
	status, out, raw := postOnboardingMatches(t, app, `{"skills":["go"],"query":"work_mode=remote&countrry=br"}`)
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200: %s", status, raw)
	}
	if len(out.Meta.IgnoredParams) != 1 || out.Meta.IgnoredParams[0]["param"] != "countrry" {
		t.Errorf("ignored_params = %v, want countrry named", out.Meta.IgnoredParams)
	}
}

func TestOnboardingMatches_CapsTheSkillList(t *testing.T) {
	skills := make([]string, 51) // the contract's @maxItems(50), plus one
	for i := range skills {
		skills[i] = fmt.Sprintf("skill-%d", i)
	}
	body, _ := json.Marshal(map[string]any{"skills": skills, "query": ""})
	status, _, raw := postOnboardingMatches(t, onboardingMatchesApp(&fakeSearcher{}), string(body))
	if status != fiber.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for %d skills: %s", status, len(skills), raw)
	}
}

func TestOnboardingMatches_NoSearchIs503(t *testing.T) {
	status, _, _ := postOnboardingMatches(t, onboardingMatchesApp(nil), `{"skills":["go"],"query":""}`)
	if status != fiber.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", status)
	}
}
