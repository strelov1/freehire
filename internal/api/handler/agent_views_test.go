package handler

import (
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/api/mcpapp"
	"github.com/strelov1/freehire/internal/api/ojcpmcp"
	"github.com/strelov1/freehire/internal/application/agentview"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/search/search"
)

type recordingSink struct {
	mu  sync.Mutex
	got []agentview.Event
}

func (s *recordingSink) Write(e agentview.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.got = append(s.got, e)
}

func TestAnMCPSearchThroughTheMountedRouteIsRecorded(t *testing.T) {
	// The scope crosses Fiber -> fasthttp user values -> adaptor -> MCP SDK -> tool. No
	// unit test on SearchJobs can prove that chain; this one goes through all of it.
	sink := &recordingSink{}
	fake := &fakeSearcher{res: search.SearchResult{Hits: []search.JobDocument{{ID: 7}, {ID: 9}}}}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.All("/mcp", agentViews(sink, agentview.SurfaceMCP),
		adaptor.HTTPHandler(mcpapp.Handler(mcpappUnder(fake, nil, fakeOJCPStore{}), "")))

	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"search_jobs","arguments":{"query":"go"}}}`
	req := httptest.NewRequestWithContext(t.Context(), fiber.MethodPost, "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("User-Agent", "Claude-User")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if len(sink.got) != 1 {
		t.Fatalf("events = %d, want 1", len(sink.got))
	}
	e := sink.got[0]
	if e.Kind != agentview.KindSearch || e.Surface != agentview.SurfaceMCP || len(e.JobIDs) != 2 || e.JobIDs[0] != 7 {
		t.Errorf("event = %+v", e)
	}
	if !strings.HasSuffix(e.Viewer, "|ua:Claude-User") {
		t.Errorf("viewer = %q, want the User-Agent in the key", e.Viewer)
	}
}

// mcpCall posts one tools/call to a mounted /mcp route and drains the answer.
func mcpCall(t *testing.T, app *fiber.App, ua, body string) {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), fiber.MethodPost, "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("User-Agent", ua)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
}

func mountedMCP(sink agentview.Sink, store fakeOJCPStore) *fiber.App {
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.All("/mcp", agentViews(sink, agentview.SurfaceMCP),
		adaptor.HTTPHandler(mcpapp.Handler(mcpappUnder(&fakeSearcher{}, nil, store), "")))
	return app
}

const getJobCall = `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"get_job","arguments":{"slug":"go-dev-x"}}}`

func TestAnMCPGetJobThroughTheMountedRouteIsRecorded(t *testing.T) {
	sink := &recordingSink{}
	store := fakeOJCPStore{
		job:     db.Job{ID: 7, PublicSlug: "go-dev-x", Title: "Go Dev", Company: "Acme", Source: "greenhouse"},
		formErr: pgx.ErrNoRows,
	}
	mcpCall(t, mountedMCP(sink, store), "Claude-User", getJobCall)

	if len(sink.got) != 1 {
		t.Fatalf("events = %d, want 1", len(sink.got))
	}
	e := sink.got[0]
	if e.Kind != agentview.KindDetail || e.Surface != agentview.SurfaceMCP || len(e.JobIDs) != 1 || e.JobIDs[0] != 7 {
		t.Errorf("event = %+v", e)
	}
}

func TestAPostingRefusedToAgentsIsNotRecorded(t *testing.T) {
	sink := &recordingSink{}
	private := db.Job{ID: 7, PublicSlug: "go-dev-x", IsPrivate: true}

	mcpCall(t, mountedMCP(sink, fakeOJCPStore{job: private}), "Claude-User", getJobCall)
	mcpCall(t, mountedMCP(sink, fakeOJCPStore{jobErr: pgx.ErrNoRows}), "Claude-User", getJobCall)

	h := newOJCPHandlers(&fakeSearcher{}, fakeOJCPStore{job: private}, "https://freehire.me", nil)
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/ojcp/v1/jobs/:slug", agentViews(sink, agentview.SurfaceOJCP), h.OJCPJobDetail)
	doGet(t, app, "/ojcp/v1/jobs/go-dev-x")

	if len(sink.got) != 0 {
		t.Errorf("events = %+v, want none for a posting the agent was not shown", sink.got)
	}
}

func TestASpoofedAccountHeaderOnAPublicSurfaceIsIgnored(t *testing.T) {
	sink := &recordingSink{}
	app := fiber.New()
	app.Get("/x", agentViews(sink, agentview.SurfaceMCP), func(c *fiber.Ctx) error {
		agentview.Record(c.Context(), agentview.KindDetail, []int64{1})
		return c.SendStatus(fiber.StatusNoContent)
	})
	req := httptest.NewRequestWithContext(t.Context(), fiber.MethodGet, "/x", nil)
	req.Header.Set(mcpapp.AccountUserIDHeader, "42")
	req.Header.Set("User-Agent", "bot")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if len(sink.got) != 1 || !strings.HasPrefix(sink.got[0].Viewer, "ip:") || !strings.HasSuffix(sink.got[0].Viewer, "|ua:bot") {
		t.Errorf("events = %+v, want an ip|ua viewer, not user:42", sink.got)
	}
}

func TestAnOJCPDetailIsRecorded(t *testing.T) {
	sink := &recordingSink{}
	store := fakeOJCPStore{
		job:     db.Job{ID: 7, PublicSlug: "go-dev-x", Title: "Go Dev", Company: "Acme", Source: "greenhouse"},
		formErr: pgx.ErrNoRows,
	}
	h := newOJCPHandlers(&fakeSearcher{}, store, "https://freehire.me", nil)
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/ojcp/v1/jobs/:slug", agentViews(sink, agentview.SurfaceOJCP), h.OJCPJobDetail)

	if status, body := doGet(t, app, "/ojcp/v1/jobs/go-dev-x"); status != fiber.StatusOK {
		t.Fatalf("status = %d, body = %v", status, body)
	}

	if len(sink.got) != 1 {
		t.Fatalf("events = %d, want 1", len(sink.got))
	}
	e := sink.got[0]
	if e.Kind != agentview.KindDetail || e.Surface != agentview.SurfaceOJCP || len(e.JobIDs) != 1 || e.JobIDs[0] != 7 {
		t.Errorf("event = %+v", e)
	}
}

func TestAnAgentSearchIsRecorded(t *testing.T) {
	sink := &recordingSink{}
	fake := &fakeSearcher{res: search.SearchResult{Hits: []search.JobDocument{{ID: 3}}}}
	h := &searchHandlers{search: fake, descriptions: &fakeDescriptions{}}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/agent/jobs/search", agentViews(sink, agentview.SurfaceAgentAPI), h.AgentSearchJobs)

	if status, body := doGet(t, app, "/agent/jobs/search?q=go"); status != fiber.StatusOK {
		t.Fatalf("status = %d, body = %v", status, body)
	}

	if len(sink.got) != 1 || sink.got[0].Kind != agentview.KindSearch ||
		sink.got[0].Surface != agentview.SurfaceAgentAPI || sink.got[0].JobIDs[0] != 3 {
		t.Errorf("events = %+v", sink.got)
	}
}

func TestAnAccountRequestIsKeyedByTheVerifiedAccount(t *testing.T) {
	sink := &recordingSink{}
	app := fiber.New()
	app.Get("/x", agentViews(sink, agentview.SurfaceMCPAccount), func(c *fiber.Ctx) error {
		agentview.Record(c.Context(), agentview.KindDetail, []int64{1})
		return c.SendStatus(fiber.StatusNoContent)
	})
	req := httptest.NewRequestWithContext(t.Context(), fiber.MethodGet, "/x", nil)
	req.Header.Set(mcpapp.AccountUserIDHeader, "42")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if len(sink.got) != 1 || sink.got[0].Viewer != "user:42" {
		t.Errorf("events = %+v, want one viewed by user:42", sink.got)
	}
}

func TestANilSinkRecordsNothingAndPassesThrough(t *testing.T) {
	app := fiber.New()
	app.Get("/x", agentViews(nil, agentview.SurfaceMCP), func(c *fiber.Ctx) error {
		agentview.Record(c.Context(), agentview.KindDetail, []int64{1})
		return c.SendStatus(fiber.StatusNoContent)
	})
	resp, err := app.Test(httptest.NewRequestWithContext(t.Context(), fiber.MethodGet, "/x", nil))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != fiber.StatusNoContent {
		t.Errorf("status = %d", resp.StatusCode)
	}
}

// ojcpMCPCall posts one tools/call to a mounted /ojcp/mcp route and drains the answer.
func ojcpMCPCall(t *testing.T, app *fiber.App, body string) {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), fiber.MethodPost, "/ojcp/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("User-Agent", "Claude-User")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
}

func TestAnOJCPMCPCallThroughTheMountedRouteIsRecorded(t *testing.T) {
	sink := &recordingSink{}
	store := fakeOJCPStore{
		job:     db.Job{ID: 7, PublicSlug: "go-dev-x", Title: "Go Dev", Company: "Acme", Source: "greenhouse"},
		formErr: pgx.ErrNoRows,
	}
	fake := &fakeSearcher{res: search.SearchResult{Hits: []search.JobDocument{{ID: 7}, {ID: 9}}}}
	h := newOJCPHandlers(fake, store, "https://freehire.me", nil)
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.All("/ojcp/mcp", agentViews(sink, agentview.SurfaceOJCP), adaptor.HTTPHandler(ojcpmcp.Handler(h, h.origin)))

	ojcpMCPCall(t, app, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"search_jobs","arguments":{"query":"go"}}}`)
	if len(sink.got) != 1 {
		t.Fatalf("events after search = %d, want 1", len(sink.got))
	}
	e := sink.got[0]
	if e.Kind != agentview.KindSearch || e.Surface != agentview.SurfaceOJCP || len(e.JobIDs) != 2 || e.JobIDs[0] != 7 {
		t.Errorf("search event = %+v", e)
	}

	ojcpMCPCall(t, app, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"get_job_detail","arguments":{"job_id":"go-dev-x"}}}`)
	if len(sink.got) != 2 {
		t.Fatalf("events after detail = %d, want 2", len(sink.got))
	}
	e = sink.got[1]
	if e.Kind != agentview.KindDetail || e.Surface != agentview.SurfaceOJCP || len(e.JobIDs) != 1 || e.JobIDs[0] != 7 {
		t.Errorf("detail event = %+v", e)
	}
}

func TestAnAgentViewBehindAVerifiedKeyIsCountedAsItsAccount(t *testing.T) {
	// The public gate verifies a presented key for its rate budget; the view scope must
	// reuse that identity, or every account behind one address and client counts as one.
	sink := &recordingSink{}
	g := newTestGate(&checkRecorder{}, paidKeys(), false, "")
	app := fiber.New(fiber.Config{ProxyHeader: fiber.HeaderXForwardedFor})
	app.Get("/agent", g.limiter(agentSearchBudget), agentViews(sink, agentview.SurfaceAgentAPI),
		func(c *fiber.Ctx) error {
			agentview.Record(c.Context(), agentview.KindSearch, []int64{1})
			return c.SendStatus(fiber.StatusOK)
		})

	gateGet(t, app, "/agent", bearer(unpaidToken))
	gateGet(t, app, "/agent", bearer(unknownToken))

	if len(sink.got) != 2 {
		t.Fatalf("events = %d, want 2", len(sink.got))
	}
	if got := sink.got[0].Viewer; got != "user:9" {
		t.Errorf("verified key viewer = %q, want user:9", got)
	}
	if got := sink.got[1].Viewer; !strings.HasPrefix(got, "ip:"+gateIP) {
		t.Errorf("unknown key viewer = %q, want the address", got)
	}
}
