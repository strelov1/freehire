package handler

import (
	"context"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/search/suggest"
)

type fakeSuggester struct{ gotLimit int }

func (f *fakeSuggester) Suggest(_ context.Context, q string, limit int) ([]suggest.Suggestion, error) {
	f.gotLimit = limit
	if q == "" {
		return nil, nil
	}
	return []suggest.Suggestion{{Text: "go developer", Jobs: 12, Parts: []suggest.Part{
		{Kind: suggest.KindSkill, Slug: "go", Text: "go"},
		{Kind: suggest.KindTitle, Text: "developer"},
	}}}, nil
}

func suggestApp(s suggester) *fiber.App {
	h := &suggestHandlers{suggest: s}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	h.register(app.Group("/api/v1"), newContract(contractHandlers{suggestHandlers: h}))
	return app
}

func getSuggest(t *testing.T, app *fiber.App, target string) (int, string) {
	t.Helper()
	resp, err := app.Test(httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/api/v1"+target, nil))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

func TestSuggest_AnswersTheShapeTheSearchBoxReads(t *testing.T) {
	status, body := getSuggest(t, suggestApp(&fakeSuggester{}), "/suggest?q=go")
	want := `{"data":[{"text":"go developer","parts":[{"kind":"skill","slug":"go","text":"go"},{"kind":"title","text":"developer"}],"jobs":12}],"meta":{"count":1}}`
	if status != fiber.StatusOK || body != want {
		t.Errorf("got %d %s\nwant %s", status, body, want)
	}
}

func TestSuggest_NoMatchesIsAnEmptyListNotNull(t *testing.T) {
	_, body := getSuggest(t, suggestApp(&fakeSuggester{}), "/suggest")
	if !strings.Contains(body, `"data":[]`) {
		t.Errorf("body = %s, want an empty data array", body)
	}
}

func TestSuggest_LimitFallsBackAndClamps(t *testing.T) {
	for target, want := range map[string]int{"/suggest?q=a&limit=abc": 5, "/suggest?q=a&limit=0": 5, "/suggest?q=a&limit=500": 20, "/suggest?q=a&limit=8": 8} {
		f := &fakeSuggester{}
		if status, body := getSuggest(t, suggestApp(f), target); status != fiber.StatusOK || f.gotLimit != want {
			t.Errorf("%s = %d %s limit %d, want %d", target, status, body, f.gotLimit, want)
		}
	}
}

func TestSuggest_WithoutSearchIsA503(t *testing.T) {
	status, body := getSuggest(t, suggestApp(nil), "/suggest?q=go")
	if status != fiber.StatusServiceUnavailable || !strings.Contains(body, "search is not configured") {
		t.Errorf("got %d %s, want the 503", status, body)
	}
}
