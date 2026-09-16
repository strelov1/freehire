package handler

import (
	"encoding/json"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/ojcp"
	"github.com/strelov1/freehire/internal/api/ojcpmcp"
	"github.com/strelov1/freehire/internal/job/jobview"
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
		Hits:  []search.JobDocument{{ID: 7, Job: jobview.Job{PublicSlug: "go-dev-x", Title: "Go Dev", Company: "Acme"}}},
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

// sameJSON compares a decoded REST body against a value the MCP side returns, by the bytes
// each serialises to — which is what an agent actually receives over either transport.
func sameJSON(t *testing.T, restBody map[string]any, direct any) bool {
	t.Helper()

	directRaw, err := json.Marshal(direct)
	if err != nil {
		t.Fatalf("marshalling the direct value: %v", err)
	}
	var directDecoded map[string]any
	if err := json.Unmarshal(directRaw, &directDecoded); err != nil {
		t.Fatalf("re-reading the direct value: %v", err)
	}

	restRaw, err := json.Marshal(restBody)
	if err != nil {
		t.Fatalf("re-marshalling the REST body: %v", err)
	}
	reencoded, err := json.Marshal(directDecoded)
	if err != nil {
		t.Fatalf("re-marshalling the direct value: %v", err)
	}
	return string(restRaw) == string(reencoded)
}
