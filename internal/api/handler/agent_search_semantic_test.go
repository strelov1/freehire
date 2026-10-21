package handler

import (
	"errors"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func agentSemanticApp(s searcher, e queryEmbedder) *fiber.App {
	h := &searchHandlers{search: s, descriptions: &fakeDescriptions{}}
	h.semantic = semanticSearch{embedder: e, threshold: 0.7}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/agent/jobs/search", h.AgentSearchJobs)
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

// Without the flag the agent endpoint asks exactly what it asked before, costs no embed,
// and says nothing about semantic search.
func TestAgentSearchJobs_SemanticIsNotRunUnlessAsked(t *testing.T) {
	s := &hybridRecordingSearcher{}
	e := &fakeQueryEmbedder{vec: []float32{1}}

	status, body := doGet(t, agentSemanticApp(s, e), "/agent/jobs/search?q=payments")
	if status != fiber.StatusOK {
		t.Fatalf("status = %d", status)
	}
	if e.calls != 0 {
		t.Errorf("embedder called %d times, want 0", e.calls)
	}
	if len(s.calls) != 1 || s.calls[0].SemanticVector != nil || s.calls[0].ScoreThreshold != 0 {
		t.Errorf("search calls = %+v, want one plain keyword request", s.calls)
	}
	if _, ok := agentMeta(t, body)["semantic"]; ok {
		t.Errorf("meta.semantic present, want omitted when not asked")
	}
}

// semantic=true embeds the query and sends one hybrid request carrying the vector, the
// threshold and every filter the keyword request would have carried.
func TestAgentSearchJobs_SemanticAppliedSendsHybrid(t *testing.T) {
	s := &hybridRecordingSearcher{}
	e := &fakeQueryEmbedder{vec: []float32{0.1, 0.2}}

	_, body := doGet(t, agentSemanticApp(s, e), "/agent/jobs/search?q=help+kids+read&semantic=true&seniority=senior")

	if e.calls != 1 || e.got != "help kids read" {
		t.Errorf("embedder calls = %d with %q, want one call with the query", e.calls, e.got)
	}
	if len(s.calls) != 1 {
		t.Fatalf("search calls = %d, want 1", len(s.calls))
	}
	got := s.calls[0]
	if len(got.SemanticVector) != 2 || got.ScoreThreshold != 0.7 {
		t.Errorf("request vector=%v threshold=%v, want the embedding and 0.7", got.SemanticVector, got.ScoreThreshold)
	}
	if got.Filter == nil {
		t.Errorf("request filter is nil, want the seniority filter kept on the hybrid")
	}
	meta := agentMeta(t, body)
	if meta["semantic"] != "applied" {
		t.Errorf("meta.semantic = %v, want applied", meta["semantic"])
	}
	if names := ignoredNames(meta); len(names) != 0 {
		t.Errorf("ignored_params = %v, want none — semantic is this endpoint's own param", names)
	}
}

// A failing embed and an engine refusing the hybrid both answer keyword-only, and say so.
func TestAgentSearchJobs_SemanticUnavailableFallsBackToKeyword(t *testing.T) {
	cases := map[string]struct {
		s *hybridRecordingSearcher
		e queryEmbedder
	}{
		"no backend":     {&hybridRecordingSearcher{}, nil},
		"embed fails":    {&hybridRecordingSearcher{}, &fakeQueryEmbedder{err: errors.New("deadline exceeded")}},
		"engine refuses": {&hybridRecordingSearcher{refuseHybrid: true}, &fakeQueryEmbedder{vec: []float32{1}}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			status, body := doGet(t, agentSemanticApp(tc.s, tc.e), "/agent/jobs/search?q=payments&semantic=true")
			if status != fiber.StatusOK {
				t.Fatalf("status = %d, want 200", status)
			}
			last := tc.s.calls[len(tc.s.calls)-1]
			if last.SemanticVector != nil || last.ScoreThreshold != 0 {
				t.Errorf("answering request = %+v, want plain keyword", last)
			}
			if meta := agentMeta(t, body); meta["semantic"] != "unavailable" {
				t.Errorf("meta.semantic = %v, want unavailable", meta["semantic"])
			}
		})
	}
}

// With no text to embed the flag cannot be honoured, so it is reported like any dropped
// param rather than silently ignored.
func TestAgentSearchJobs_SemanticWithoutQueryIsReported(t *testing.T) {
	s := &hybridRecordingSearcher{}
	e := &fakeQueryEmbedder{vec: []float32{1}}

	_, body := doGet(t, agentSemanticApp(s, e), "/agent/jobs/search?semantic=true&seniority=senior")

	if e.calls != 0 {
		t.Errorf("embedder called %d times, want 0", e.calls)
	}
	meta := agentMeta(t, body)
	if _, ok := meta["semantic"]; ok {
		t.Errorf("meta.semantic = %v, want omitted when the flag was dropped", meta["semantic"])
	}
	if names := ignoredNames(meta); len(names) != 1 || names[0] != "semantic" {
		t.Errorf("ignored_params = %v, want [semantic]", names)
	}
}

func TestAgentSearchJobs_SemanticTotalIsMarkedALowerBound(t *testing.T) {
	e := &fakeQueryEmbedder{vec: []float32{1}}

	_, body := doGet(t, agentSemanticApp(&hybridRecordingSearcher{}, e), "/agent/jobs/search?q=payments&semantic=true")
	meta := agentMeta(t, body)
	if meta["total"] != float64(5) || meta["total_is_lower_bound"] != true {
		t.Errorf("meta = %v, want total 5 marked total_is_lower_bound", meta)
	}

	_, body = doGet(t, agentSemanticApp(&hybridRecordingSearcher{}, e), "/agent/jobs/search?q=payments")
	if _, ok := agentMeta(t, body)["total_is_lower_bound"]; ok {
		t.Errorf("keyword answer carries total_is_lower_bound")
	}
}
