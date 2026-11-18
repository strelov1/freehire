package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/search/search"
)

// rawGet is doGet that keeps the bytes, for a test that fixes the exact JSON.
func rawGet(t *testing.T, app *fiber.App, target string) (int, string) {
	t.Helper()
	resp, err := app.Test(httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, target, nil))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp.StatusCode, string(b)
}

func TestSearchJobs_ExactAnswer(t *testing.T) {
	job := jobview.Job{PublicSlug: "go-dev-x", Title: "Go Dev"}
	fake := &fakeSearcher{res: search.SearchResult{
		Hits:              []search.JobDocument{{ID: 1, Job: job}},
		Total:             5,
		TotalIsLowerBound: true,
	}}
	status, body := rawGet(t, searchApp(fake), "/api/v1/jobs/search?country=it&work_mode=foo&work_mode=bar&seniority=zzz&limit=10")
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, body %s", status, body)
	}
	raw, err := json.Marshal(job)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"data":[` + string(raw) + `],"meta":{"total":5,"limit":10,"offset":0,` +
		`"ignored_params":[{"param":"country","did_you_mean":"countries"}],` +
		`"ignored_values":{"seniority":["zzz"],"work_mode":["bar","foo"]},"total_is_lower_bound":true}}`
	if body != want {
		t.Errorf("body =\n%s\nwant\n%s", body, want)
	}
}

func TestAgentSearchJobs_ExactAnswer(t *testing.T) {
	fake := &fakeSearcher{res: search.SearchResult{Total: 0}}
	status, body := rawGet(t, agentSearchApp(fake, &fakeDescriptions{}), "/api/v1/agent/jobs/search?semantic=true&country=it")
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, body %s", status, body)
	}
	want := `{"data":[],"meta":{"total":0,"limit":20,"offset":0,` +
		`"ignored_params":[{"param":"country","did_you_mean":"countries"},{"param":"semantic"}]}}`
	if body != want {
		t.Errorf("body =\n%s\nwant\n%s", body, want)
	}
}

func TestJobFacets_ExactAnswer(t *testing.T) {
	app := facetsApp(&fakeFacetCounter{})
	status, body := rawGet(t, app, "/api/v1/jobs/facets?facets=seniority&country=it&work_mode=foo")
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, body %s", status, body)
	}
	want := `{"data":{"total":0,"facets":{},"stats":{}},"meta":{"ignored_params":[{"param":"country","did_you_mean":"countries"}],"ignored_values":{"work_mode":["foo"]}}}`
	if body != want {
		t.Errorf("body =\n%s\nwant\n%s", body, want)
	}

	status, body = rawGet(t, app, "/api/v1/jobs/facets?facets=seniority")
	if status != fiber.StatusOK || body != `{"data":{"total":0,"facets":{},"stats":{}}}` {
		t.Errorf("clean request = %d %s, want no meta", status, body)
	}
}

func TestSearchJobs_UnavailableIs503(t *testing.T) {
	status, body := rawGet(t, searchApp(nil), "/api/v1/jobs/search")
	if status != fiber.StatusServiceUnavailable || body != `{"error":"search is not available"}` {
		t.Errorf("unconfigured = %d %s", status, body)
	}
}
