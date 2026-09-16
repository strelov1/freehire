package handler

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/job/ghost"
	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/search/search"
)

type fakeOJCPStore struct {
	job     db.Job
	jobErr  error
	form    db.GetApplyFormByJobIDRow
	formErr error
	company db.Company
	compErr error
}

func (f fakeOJCPStore) GetJobBySlug(context.Context, string) (db.Job, error) {
	return f.job, f.jobErr
}

func (f fakeOJCPStore) GetApplyFormByJobID(context.Context, int64) (db.GetApplyFormByJobIDRow, error) {
	return f.form, f.formErr
}

func (f fakeOJCPStore) GetCompany(context.Context, string) (db.Company, error) {
	return f.company, f.compErr
}

// ojcpJobView projects a stored row the way every read path does. Building a jobview.Job by
// hand skips outboundurl.Tag and the facet normalisation, so a literal carries values no
// production read can produce — internal/api/ojcp/AGENTS.md forbids it for exactly that
// reason, and two defects got through that way before a review caught them.
func ojcpJobView(t *testing.T, row db.Job) jobview.Job {
	t.Helper()

	view, err := jobview.FromRow(row)
	if err != nil {
		t.Fatalf("jobview.FromRow: %v", err)
	}
	return view
}

func ojcpApp(s searcher, store ojcpStore) *fiber.App {
	h := newOJCPHandlers(s, store, "https://freehire.me", map[string]bool{"greenhouse": true})
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Post("/ojcp/v1/search", h.OJCPSearchJobs)
	app.Get("/ojcp/v1/jobs/:slug", h.OJCPJobDetail)
	app.Get("/ojcp/v1/employers/:slug", h.OJCPEmployerContext)
	return app
}

func doPost(t *testing.T, app *fiber.App, path, body string) (int, map[string]any) {
	t.Helper()

	req := httptest.NewRequestWithContext(t.Context(), fiber.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	var decoded map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	return resp.StatusCode, decoded
}

func TestOJCPSearchAnswersTheStandardsEnvelope(t *testing.T) {
	fake := &fakeSearcher{res: search.SearchResult{
		Hits:  []search.JobDocument{{ID: 7, Job: ojcpJobView(t, db.Job{PublicSlug: "go-dev-x", Title: "Go Dev", Company: "Acme"})}},
		Total: 3,
	}}

	status, body := doPost(t, ojcpApp(fake, fakeOJCPStore{}), "/ojcp/v1/search", `{"query":"go"}`)

	if status != fiber.StatusOK {
		t.Fatalf("status = %d, body = %v", status, body)
	}
	if body["ojcp_version"] != "0.1" {
		t.Errorf("ojcp_version = %v", body["ojcp_version"])
	}
	if body["total_results"] != float64(3) {
		t.Errorf("total_results = %v, want 3", body["total_results"])
	}
	if jobs, _ := body["jobs"].([]any); len(jobs) != 1 {
		t.Errorf("jobs = %v, want one posting", body["jobs"])
	}
}

func TestOJCPSearchRunsTheSameQueryThePublicSearchRuns(t *testing.T) {
	// The whole point of translating onto url.Values: an agent's question reaches the same
	// search core, so it cannot be answered differently from the same question asked
	// through /jobs/search.
	fake := &fakeSearcher{}

	doPost(t, ojcpApp(fake, fakeOJCPStore{}), "/ojcp/v1/search",
		`{"query":"go engineer","pagination":{"limit":20,"offset":40}}`)

	if fake.got.Query != "go engineer" {
		t.Errorf("query = %q", fake.got.Query)
	}
	if fake.got.Limit != 20 || fake.got.Offset != 40 {
		t.Errorf("page = %d/%d, want 20/40", fake.got.Limit, fake.got.Offset)
	}
}

func TestOJCPSearchSaysWhichFilterItCouldNotHonour(t *testing.T) {
	// Answering the whole catalogue where an agent asked for a 20-mile radius, and saying
	// nothing about it, is the failure this endpoint's house rule exists to prevent.
	status, body := doPost(t, ojcpApp(&fakeSearcher{}, fakeOJCPStore{}), "/ojcp/v1/search",
		`{"query":"go","location":{"radius_miles":20}}`)

	if status != fiber.StatusOK {
		t.Fatalf("status = %d", status)
	}
	ignored, _ := body["ignored_params"].([]any)
	if len(ignored) != 1 || ignored[0] != "location.radius_miles" {
		t.Errorf("ignored_params = %v, want the dropped filter named", body["ignored_params"])
	}
}

func TestOJCPSearchIgnoresAFieldFromALaterSpecVersion(t *testing.T) {
	// The standard's extensibility rule: implementations MUST ignore what they do not
	// recognise rather than rejecting it, or an agent written against v0.2 gets nothing.
	status, _ := doPost(t, ojcpApp(&fakeSearcher{}, fakeOJCPStore{}), "/ojcp/v1/search",
		`{"query":"go","some_field_from_v0_2":{"nested":true}}`)

	if status != fiber.StatusOK {
		t.Errorf("status = %d, want the call to succeed despite the unknown field", status)
	}
}

func TestOJCPSearchRefusesABodyThatIsNotJSON(t *testing.T) {
	status, body := doPost(t, ojcpApp(&fakeSearcher{}, fakeOJCPStore{}), "/ojcp/v1/search", `not json at all`)

	if status != fiber.StatusBadRequest {
		t.Errorf("status = %d, want 400", status)
	}
	if _, present := body["error_code"]; !present {
		t.Errorf("body = %v, want the OJCP error envelope", body)
	}
}

func TestOJCPJobDetailAnswersWithThePosting(t *testing.T) {
	store := fakeOJCPStore{
		job:     db.Job{ID: 7, PublicSlug: "go-dev-x", Title: "Go Dev", Company: "Acme", Source: "greenhouse"},
		formErr: pgx.ErrNoRows,
	}

	status, body := doGet(t, ojcpApp(&fakeSearcher{}, store), "/ojcp/v1/jobs/go-dev-x")

	if status != fiber.StatusOK {
		t.Fatalf("status = %d, body = %v", status, body)
	}
	job, _ := body["job"].(map[string]any)
	if job["ojcp_id"] != "go-dev-x" {
		t.Errorf("job.ojcp_id = %v", job["ojcp_id"])
	}
}

func TestOJCPJobDetailRefusesAPostingTheCatalogueDoesNotPublish(t *testing.T) {
	// The requirement: "no OJCP tool returns it, on either transport". GetJobBySlug carries
	// no predicate at all — it is the read a private job's own creator uses — so the filter
	// has to be here. Answering not-found rather than a 403 is deliberate: whether a private
	// posting exists is itself not this caller's business.
	for name, row := range map[string]db.Job{
		"private": {ID: 7, PublicSlug: "secret-role", Title: "Secret", IsPrivate: true},
		"closed": {ID: 7, PublicSlug: "gone-role", Title: "Gone",
			ClosedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true}},
		"suppressed duplicate": {ID: 7, PublicSlug: "dupe-role", Title: "Dupe",
			DuplicateOf: pgtype.Int8{Int64: 3, Valid: true}},
	} {
		t.Run(name, func(t *testing.T) {
			status, body := doGet(t, ojcpApp(&fakeSearcher{}, fakeOJCPStore{job: row}), "/ojcp/v1/jobs/"+row.PublicSlug)

			if status != fiber.StatusNotFound {
				t.Errorf("status = %d, want 404 — this posting is not published", status)
			}
			if _, present := body["job"]; present {
				t.Errorf("body carries the posting: %v", body)
			}
		})
	}
}

func TestOJCPJobDetailStatesThePostingRealityVerdict(t *testing.T) {
	// agent_notes is the whole reason this catalogue has something to say that other OJCP
	// providers do not. But Ghost is NOT intrinsic to a jobview — every other surface
	// attaches it explicitly — so a projection that simply reads j.Ghost publishes nothing,
	// for every posting, forever. The package's own test set it by hand and stayed green.
	// What was broken was the SEAM, not the classification: nothing attached a verdict, so
	// the projection read nil for every posting. This drives the seam — the handler must
	// call the attacher, and the projection must publish what it left behind. What COUNTS as
	// a verdict is internal/job/ghost's own business and has its own tests.
	store := fakeOJCPStore{job: db.Job{ID: 7, PublicSlug: "go-dev-x", Title: "Go Dev", Company: "Acme"}}

	h := newOJCPHandlers(&fakeSearcher{}, store, "https://freehire.me", nil)
	attached := false
	h.attachReality = func(_ context.Context, _ db.Job, view *jobview.Job) {
		attached = true
		view.Ghost = &jobview.Ghost{
			Level:         ghost.LevelLikely,
			Criteria:      []string{ghost.CriterionATSAbsent},
			CriteriaTotal: ghost.CriteriaTotal,
		}
	}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/ojcp/v1/jobs/:slug", h.OJCPJobDetail)

	_, body := doGet(t, app, "/ojcp/v1/jobs/go-dev-x")

	if !attached {
		t.Fatal("the handler never asked for a reality verdict")
	}
	job, _ := body["job"].(map[string]any)
	notes, _ := job["agent_notes"].(string)
	if notes == "" {
		t.Fatal("agent_notes is empty though the view carries a verdict")
	}
	if !strings.Contains(notes, "employer's own ATS") {
		t.Errorf("agent_notes does not state the criterion that fired: %q", notes)
	}
}

func TestOJCPProductionWiringAttachesTheRealityVerdict(t *testing.T) {
	// The seam above is only worth anything if production fills it. newOJCPHandlers wires the
	// attacher when the store is the concrete queries — the one path a fake store cannot
	// exercise, so it is asserted directly.
	if h := newOJCPHandlers(&fakeSearcher{}, &db.Queries{}, "https://freehire.me", nil); h.attachReality == nil {
		t.Error("attachReality is nil for a production store; agent_notes would be silent forever")
	}
	if h := newOJCPHandlers(&fakeSearcher{}, fakeOJCPStore{}, "https://freehire.me", nil); h.attachReality != nil {
		t.Error("attachReality is set for a store that cannot answer the lookups")
	}
}

func TestOJCPJobDetailAnswersTheErrorEnvelopeForAnUnknownID(t *testing.T) {
	// Not an empty posting: an agent must be able to tell "no such job" from "a job with
	// no fields".
	status, body := doGet(t, ojcpApp(&fakeSearcher{}, fakeOJCPStore{jobErr: pgx.ErrNoRows}), "/ojcp/v1/jobs/nope")

	if status != fiber.StatusNotFound {
		t.Errorf("status = %d, want 404", status)
	}
	if _, present := body["job"]; present {
		t.Errorf("body = %v, want no job object at all", body)
	}
	if _, present := body["error_code"]; !present {
		t.Errorf("body = %v, want the OJCP error envelope", body)
	}
}

func TestOJCPJobDetailSurvivesAnUnreadableApplyForm(t *testing.T) {
	// Most of the catalogue has no captured form, and a store that cannot answer must not
	// turn a job read into a failure — the posting falls back to the redirect path.
	store := fakeOJCPStore{
		job:  db.Job{ID: 7, PublicSlug: "go-dev-x", Title: "Go Dev", Company: "Acme", URL: "https://boards.greenhouse.io/acme/1"},
		form: db.GetApplyFormByJobIDRow{Provider: "greenhouse", Payload: []byte(`{{{ not json`)},
	}

	status, body := doGet(t, ojcpApp(&fakeSearcher{}, store), "/ojcp/v1/jobs/go-dev-x")

	if status != fiber.StatusOK {
		t.Fatalf("status = %d, body = %v", status, body)
	}
	job, _ := body["job"].(map[string]any)
	paths, _ := job["apply_paths"].([]any)
	if len(paths) != 1 {
		t.Fatalf("apply_paths = %v, want the redirect fallback", job["apply_paths"])
	}
	if first, _ := paths[0].(map[string]any); first["type"] != "external_redirect" {
		t.Errorf("apply path = %v, want external_redirect", paths[0])
	}
}

func TestOJCPEmployerContextAnswersWithTheCompany(t *testing.T) {
	store := fakeOJCPStore{company: db.Company{Slug: "acme", Name: "Acme Corp", JobCount: 12}}

	status, body := doGet(t, ojcpApp(&fakeSearcher{}, store), "/ojcp/v1/employers/acme")

	if status != fiber.StatusOK {
		t.Fatalf("status = %d, body = %v", status, body)
	}
	if body["employer_id"] != "acme" {
		t.Errorf("employer_id = %v", body["employer_id"])
	}
	if body["open_roles_count"] != float64(12) {
		t.Errorf("open_roles_count = %v, want 12", body["open_roles_count"])
	}
}

func TestOJCPEmployerContextAnswersTheErrorEnvelopeForAnUnknownID(t *testing.T) {
	status, body := doGet(t, ojcpApp(&fakeSearcher{}, fakeOJCPStore{compErr: pgx.ErrNoRows}), "/ojcp/v1/employers/nope")

	if status != fiber.StatusNotFound {
		t.Errorf("status = %d, want 404", status)
	}
	if _, present := body["error_code"]; !present {
		t.Errorf("body = %v, want the OJCP error envelope", body)
	}
}

func TestOJCPSearchReportsSearchBeingUnavailable(t *testing.T) {
	// Without MEILI_MASTER_KEY there is no search backend at all. A 200 with an empty page
	// would tell an agent the catalogue holds nothing.
	status, body := doPost(t, ojcpApp(nil, fakeOJCPStore{}), "/ojcp/v1/search", `{"query":"go"}`)

	if status != fiber.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", status)
	}
	if _, present := body["error_code"]; !present {
		t.Errorf("body = %v, want the OJCP error envelope", body)
	}
}
