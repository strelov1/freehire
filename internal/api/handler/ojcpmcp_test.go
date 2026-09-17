package handler

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/ojcp"
	"github.com/strelov1/freehire/internal/api/ojcpmcp"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/search/search"
)

// The handlers must satisfy the MCP transport's Reader, or the two transports are two
// implementations rather than one answer rendered twice. A compile-time assertion states it
// where a reader of either side will see it.
var _ ojcpmcp.Reader = (*ojcpHandlers)(nil)

func TestBothTransportsAnswerTheSameQuestionIdentically(t *testing.T) {
	// The MCP tools call these methods directly, so comparing a REST response body against
	// the method's own value is what proves the two cannot drift. A second projection on
	// either side would show up here as a difference in the bytes.
	fake := &fakeSearcher{res: search.SearchResult{
		Hits:  []search.JobDocument{{ID: 7, Job: ojcpJobView(t, db.Job{PublicSlug: "go-dev-x", Title: "Go Dev", Company: "Acme"})}},
		Total: 3,
	}}
	h := newOJCPHandlers(fake, fakeOJCPStore{}, "https://freehire.me", map[string]bool{"greenhouse": true})
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Post("/ojcp/v1/search", h.OJCPSearchJobs)

	_, restBody := doPost(t, app, "/ojcp/v1/search", `{"query":"go"}`)

	direct, err := h.SearchJobs(t.Context(), ojcp.SearchInput{Query: "go"})
	if err != nil {
		t.Fatalf("SearchJobs: %v", err)
	}
	if !sameJSON(t, restBody, direct) {
		t.Errorf("the transports disagree:\nREST: %v\nMCP:  %v", restBody, direct)
	}
}

func TestBothTransportsAgreeOnAPostingsDetail(t *testing.T) {
	store := fakeOJCPStore{job: db.Job{ID: 7, PublicSlug: "go-dev-x", Title: "Go Dev", Company: "Acme"}}
	h := newOJCPHandlers(&fakeSearcher{}, store, "https://freehire.me", nil)
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/ojcp/v1/jobs/:slug", h.OJCPJobDetail)

	_, restBody := doGet(t, app, "/ojcp/v1/jobs/go-dev-x")

	direct, err := h.JobDetail(t.Context(), "go-dev-x")
	if err != nil {
		t.Fatalf("JobDetail: %v", err)
	}
	if !sameJSON(t, restBody, direct) {
		t.Errorf("the transports disagree:\nREST: %v\nMCP:  %v", restBody, direct)
	}
}

func TestManifestCannotAdvertiseAToolNothingServes(t *testing.T) {
	// `tools` is a binding claim: an agent reads it and calls what it names. The manifest
	// derives the list from the MCP server's own registrations, and this asserts that every
	// name in it has a method behind it — a tool added to the manifest by hand, or a method
	// deleted, fails here rather than at an agent.
	h := newOJCPHandlers(&fakeSearcher{}, fakeOJCPStore{}, "https://freehire.me", nil)

	served := map[string]func(){
		"search_jobs":          func() { _, _ = h.SearchJobs(t.Context(), ojcp.SearchInput{}) },
		"get_job_detail":       func() { _, _ = h.JobDetail(t.Context(), "x") },
		"get_employer_context": func() { _, _ = h.EmployerContext(t.Context(), "x") },
	}

	tools := h.manifest().Tools
	for _, name := range tools {
		call, ok := served[name]
		if !ok {
			t.Errorf("manifest advertises %q, which nothing here answers", name)
			continue
		}
		call()
	}
	if len(tools) != len(served) {
		t.Errorf("tools = %v, want every served tool advertised", tools)
	}
}

func TestManifestIsServedUnderTheAPIPath(t *testing.T) {
	// It is served to an AGENT at /.well-known/ojcp.json, which the SPA proxies here.
	// nginx routes /api/ to this service and everything else to the Node process, so a Go
	// route at the well-known path itself never receives a request — found in production,
	// with the API path answering 200 and the manifest answering 404.
	h := newOJCPHandlers(&fakeSearcher{}, fakeOJCPStore{}, "https://freehire.me", nil)
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/ojcp/manifest", h.OJCPManifest)

	status, body := doGet(t, app, "/ojcp/manifest")

	if status != fiber.StatusOK {
		t.Fatalf("status = %d, body = %v", status, body)
	}
	if body["ojcp_version"] != "0.1" {
		t.Errorf("ojcp_version = %v", body["ojcp_version"])
	}
	if tools, _ := body["tools"].([]any); len(tools) != 3 {
		t.Errorf("tools = %v, want the three read tools", body["tools"])
	}
}

func TestManifestDeclaresTheLimitTheRoutesEnforce(t *testing.T) {
	// The spec makes a declared rate limit binding. Declaring one figure and enforcing
	// another is a conformance failure nothing else in this repo would notice.
	h := newOJCPHandlers(&fakeSearcher{}, fakeOJCPStore{}, "https://freehire.me", nil)

	limits := h.manifest().RateLimits
	if limits == nil {
		t.Fatal("rate_limits is absent; the routes do enforce one")
	}
	if want := agentSearchPerMinute / 60; limits.AnonymousRPS != want {
		t.Errorf("anonymous_rps = %d, want %d — the limiter's own budget", limits.AnonymousRPS, want)
	}
}

func TestEveryErrorCodeHasAnHTTPStatus(t *testing.T) {
	// A code absent from the table serves status 0, which Fiber rejects — so this walks the
	// standard's codes rather than the table's own keys, where a missing entry would be
	// invisible. Introducing a code without deciding what it means over HTTP fails here.
	for _, code := range []string{
		ojcp.ErrorInvalidRequest, ojcp.ErrorJobNotFound, ojcp.ErrorEmployerNotFound,
		ojcp.ErrorProviderError, ojcp.ErrorRateLimited,
	} {
		if ojcpErrorStatus[code] == 0 {
			t.Errorf("error code %q has no HTTP status; it would be served as 0", code)
		}
	}
}

// sameJSON compares a decoded REST body against the value the MCP side returns.
//
// The MCP value is serialised and read back first so both sides are the same kind of thing:
// a generic JSON tree, which is what an agent actually receives over either transport. A
// struct compared against a decoded map would differ on types alone and prove nothing.
func sameJSON(t *testing.T, restBody map[string]any, direct any) bool {
	t.Helper()

	raw, err := json.Marshal(direct)
	if err != nil {
		t.Fatalf("marshalling the direct value: %v", err)
	}
	var asTree map[string]any
	if err := json.Unmarshal(raw, &asTree); err != nil {
		t.Fatalf("re-reading the direct value: %v", err)
	}
	return reflect.DeepEqual(restBody, asTree)
}
