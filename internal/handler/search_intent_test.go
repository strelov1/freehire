package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/llm"
	"github.com/strelov1/freehire/internal/searchintent"
)

// intentApp mounts the endpoint behind a middleware that signs the caller in when
// userID is non-zero, standing in for RequireAuth.
func intentApp(t *testing.T, h *intentHandlers, userID int64) *fiber.App {
	t.Helper()
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Use(func(c *fiber.Ctx) error {
		if userID != 0 {
			c.Locals(auth.LocalsUserID, userID)
		}
		return c.Next()
	})
	app.Post("/api/v1/search/interpret", h.InterpretSearch)
	return app
}

// modelSayingJSON is a gateway that answers every completion with one canned body.
func modelSayingJSON(t *testing.T, content string) *llm.Client {
	t.Helper()
	quoted, err := json.Marshal(content)
	if err != nil {
		t.Fatalf("quote: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"id":"1","object":"chat.completion","choices":`+
			`[{"index":0,"message":{"role":"assistant","content":%s},"finish_reason":"stop"}]}`, quoted)
	}))
	t.Cleanup(srv.Close)
	client, err := llm.New(srv.URL, "sk-test", "test-model")
	if err != nil {
		t.Fatalf("llm.New: %v", err)
	}
	return client
}

func postIntent(t *testing.T, app *fiber.App, body string) (int, string) {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(),
		fiber.MethodPost, "/api/v1/search/interpret", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	out, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp.StatusCode, string(out)
}

func TestInterpretSearchRefusesAnUnauthenticatedCaller(t *testing.T) {
	h := newIntentHandlers(llmBinding{client: modelSayingJSON(t, `{}`)})
	status, _ := postIntent(t, intentApp(t, h, 0), `{"text":"senior go"}`)
	if status != fiber.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", status)
	}
}

// The cap must be enforced before the model is called, not after: a request past it is
// someone spending tokens, and paying for the call to find that out defeats the cap.
func TestInterpretSearchRefusesOverlongTextWithoutCallingTheModel(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"id":"1","object":"chat.completion","choices":`+
			`[{"index":0,"message":{"role":"assistant","content":"{}"},"finish_reason":"stop"}]}`)
	}))
	t.Cleanup(srv.Close)
	client, err := llm.New(srv.URL, "sk-test", "test-model")
	if err != nil {
		t.Fatalf("llm.New: %v", err)
	}

	h := newIntentHandlers(llmBinding{client: client})
	body, err := json.Marshal(map[string]string{"text": strings.Repeat("x", searchintent.MaxTextRunes+1)})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	status, _ := postIntent(t, intentApp(t, h, 7), string(body))
	if status != fiber.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
	if calls != 0 {
		t.Fatalf("model calls = %d, want 0 — the cap must be checked before spending", calls)
	}
}

func TestInterpretSearchReportsAnUnconfiguredModel(t *testing.T) {
	h := newIntentHandlers(llmBinding{})
	status, _ := postIntent(t, intentApp(t, h, 7), `{"text":"senior go"}`)
	if status != fiber.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 — the feature is off, not the request wrong", status)
	}
}

func TestInterpretSearchReturnsTheResolvedFilter(t *testing.T) {
	h := newIntentHandlers(llmBinding{client: modelSayingJSON(t,
		`{"summary":"Senior Go roles in Portugal.","seniority":["senior"],"skills":["Golang"],`+
			`"countries":["Portugal"],"cities":["Atlantis City"]}`)})

	status, body := postIntent(t, intentApp(t, h, 7), `{"text":"senior go in portugal"}`)
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200: %s", status, body)
	}
	var out struct {
		Data struct {
			Summary    string              `json:"summary"`
			Facets     map[string][]string `json:"facets"`
			Unresolved []string            `json:"unresolved"`
			Empty      bool                `json:"empty"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	if out.Data.Summary != "Senior Go roles in Portugal." {
		t.Fatalf("summary = %q", out.Data.Summary)
	}
	if got := out.Data.Facets["skills"]; len(got) != 1 || got[0] != "go" {
		t.Fatalf("skills = %v, want [go]", got)
	}
	if len(out.Data.Unresolved) == 0 {
		t.Fatal("unresolved is empty — the invented city must be reported, not silently dropped")
	}
	if out.Data.Empty {
		t.Fatal("empty = true, want false")
	}
}

// A model that returns a proposal nothing resolves must reach the caller as "I could
// not turn that into filters" — never as an empty filter, which shows the whole
// catalogue and reads as "everything matches you".
func TestInterpretSearchFlagsAnEmptyResult(t *testing.T) {
	h := newIntentHandlers(llmBinding{client: modelSayingJSON(t,
		`{"summary":"anything","skills":["blockchain-adjacent"]}`)})

	status, body := postIntent(t, intentApp(t, h, 7), `{"text":"something odd"}`)
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200: %s", status, body)
	}
	var out struct {
		Data struct {
			Empty bool `json:"empty"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	if !out.Data.Empty {
		t.Fatal("empty = false, want true — nothing resolved")
	}
}

func TestInterpretSearchRefusesARequestWithNothingInIt(t *testing.T) {
	h := newIntentHandlers(llmBinding{client: modelSayingJSON(t, `{}`)})
	status, _ := postIntent(t, intentApp(t, h, 7), `{}`)
	if status != fiber.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
}

// Every per-user model call goes out on that user's own gateway credential under a
// feature tag. This one is no exception, and the tag has to be a constant beside the
// others rather than a string written here.
func TestSearchIntentHasItsOwnFeatureTag(t *testing.T) {
	if tagSearchIntent == "" || !strings.HasPrefix(tagSearchIntent, "feature:") {
		t.Fatalf("tagSearchIntent = %q, want a feature: tag", tagSearchIntent)
	}
	if tagSearchIntent == tagAssistant {
		t.Fatal("search intent must not be billed as assistant work")
	}
}
