package handler

import (
	"testing"

	"github.com/gofiber/fiber/v2"
)

func agentSemanticApp(s searcher) *fiber.App {
	h := &searchHandlers{search: s, descriptions: &fakeDescriptions{}}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/agent/jobs/search", newContract(contractHandlers{searchHandlers: h}))
	return app
}

func agentMeta(t *testing.T, body map[string]any) map[string]any {
	t.Helper()
	meta, ok := body["meta"].(map[string]any)
	if !ok {
		t.Fatalf("meta = %v, want an object", body["meta"])
	}
	return meta
}

func ignoredNames(meta map[string]any) []string {
	var names []string
	list, _ := meta["ignored_params"].([]any)
	for _, e := range list {
		if m, ok := e.(map[string]any); ok {
			names = append(names, m["param"].(string))
		}
	}
	return names
}

// Without the flag the agent endpoint says nothing about semantic search.
func TestAgentSearchJobs_SemanticIsNotReportedUnlessAsked(t *testing.T) {
	status, body := doGet(t, agentSemanticApp(&fakeSearcher{}), "/api/v1/agent/jobs/search?q=payments")
	if status != fiber.StatusOK {
		t.Fatalf("status = %d", status)
	}
	if _, ok := agentMeta(t, body)["semantic"]; ok {
		t.Errorf("meta.semantic present, want omitted when not asked")
	}
}

// semantic=true is accepted but cannot be honoured: the keyword answer keeps every filter
// and says it is unavailable, without listing the endpoint's own param as ignored.
func TestAgentSearchJobs_SemanticIsUnavailable(t *testing.T) {
	s := &fakeSearcher{}
	status, body := doGet(t, agentSemanticApp(s), "/api/v1/agent/jobs/search?q=payments&semantic=true&seniority=senior")
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if s.got.Filter == nil {
		t.Errorf("request filter is nil, want the seniority filter kept")
	}
	meta := agentMeta(t, body)
	if meta["semantic"] != "unavailable" {
		t.Errorf("meta.semantic = %v, want unavailable", meta["semantic"])
	}
	if names := ignoredNames(meta); len(names) != 0 {
		t.Errorf("ignored_params = %v, want none — semantic is this endpoint's own param", names)
	}
}

// With no text the flag has nothing to apply to, so it is reported like any dropped
// param rather than silently ignored.
func TestAgentSearchJobs_SemanticWithoutQueryIsReported(t *testing.T) {
	_, body := doGet(t, agentSemanticApp(&fakeSearcher{}), "/api/v1/agent/jobs/search?semantic=true&seniority=senior")

	meta := agentMeta(t, body)
	if _, ok := meta["semantic"]; ok {
		t.Errorf("meta.semantic = %v, want omitted when the flag was dropped", meta["semantic"])
	}
	if names := ignoredNames(meta); len(names) != 1 || names[0] != "semantic" {
		t.Errorf("ignored_params = %v, want [semantic]", names)
	}
}
