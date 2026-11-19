//go:build integration

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/contactfinder"
	"github.com/strelov1/freehire/internal/platform/db"
)

// fakeFinder stands in for the contact-finder service.
type fakeFinder struct {
	status     int    // of /v1/find; 0 = 200
	contacts   string // the JSON list /v1/find returns
	suppressed string // the JSON list /v1/check returns
	finds      atomic.Int32
	checks     atomic.Int32
	checkRef   atomic.Value // user_ref of the last /v1/check
}

func (f *fakeFinder) client(t *testing.T) *contactfinder.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/find":
			f.finds.Add(1)
			if f.status != 0 {
				w.WriteHeader(f.status)
				return
			}
			_, _ = w.Write([]byte(`{"contacts":` + f.contacts + `}`))
		case "/v1/check":
			f.checks.Add(1)
			var body struct {
				UserRef string `json:"user_ref"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			f.checkRef.Store(body.UserRef)
			_, _ = w.Write([]byte(`{"suppressed":` + f.suppressed + `}`))
		}
	}))
	t.Cleanup(srv.Close)
	return contactfinder.New(srv.URL, "tok")
}

const twoRecruiters = `[{"name":"Anna","title":"Recruiter","email":"anna@acme.io","confidence":"high"},` +
	`{"name":"Bob","title":"Sourcer","email":"bob@acme.io","confidence":"medium"}]`

type contactsFixture struct {
	pool   *pgxpool.Pool
	plans  *plan.Store
	app    *fiber.App
	iss    *auth.Issuer
	alice  int64
	bob    int64
	jobIDs map[string]int64
}

// newContactsFixture seeds two pro users, a company with a website and jobs at it ("job-a",
// "job-b") plus one at a company with none ("no-site"). Pro's ceiling is proCeiling a day.
func newContactsFixture(t *testing.T, finder *contactfinder.Client, proCeiling int) contactsFixture {
	t.Helper()
	ctx := context.Background()
	pool := startPostgres(t)
	fx := contactsFixture{pool: pool, jobIDs: map[string]int64{}}
	for _, u := range []*int64{&fx.alice, &fx.bob} {
		if err := pool.QueryRow(ctx, `INSERT INTO users (email, email_verified) VALUES ('u'||gen_random_uuid()||'@e.test', true) RETURNING id`).Scan(u); err != nil {
			t.Fatal(err)
		}
		makePro(t, pool, *u)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO companies (slug, name, company_info) VALUES
		('acme', 'Acme', '{"website":"https://www.acme.io/careers"}'), ('nosite', 'No Site', '{}')`); err != nil {
		t.Fatal(err)
	}
	for slug, company := range map[string]string{"job-a": "acme", "job-b": "acme", "no-site": "nosite"} {
		id := seedJobSlug(t, pool, slug)
		if _, err := pool.Exec(ctx, `UPDATE jobs SET company_slug = $2 WHERE id = $1`, id, company); err != nil {
			t.Fatal(err)
		}
		fx.jobIDs[slug] = id
	}

	queries := db.New(pool)
	fx.plans = plan.NewStore(queries, pool, plan.DefaultConfig().WithProCeiling(plan.FeatureRecruiterContact, proCeiling))
	h := &recruiterContactHandlers{queries: queries, plans: fx.plans, finder: finder}
	fx.iss = auth.NewIssuer("test-secret-that-is-long-enough-0001", time.Hour)
	fx.app = fiber.New(fiber.Config{ErrorHandler: RenderError})
	fx.app.Post("/api/v1/jobs/:slug/recruiter-contacts",
		auth.RequireAuthOrKey(fx.iss, testVersions, apiKeys{queries}), markPaidAI(plan.FeatureRecruiterContact),
		newContract(contractHandlers{recruiterContactHandlers: h}))
	return fx
}

func (fx contactsFixture) post(t *testing.T, userID int64, slug string) (int, []contactfinder.Contact) {
	t.Helper()
	cookie, _ := fx.iss.Issue(userID, testTokenVersion)
	r := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost, "/api/v1/jobs/"+slug+"/recruiter-contacts", nil)
	r.Header.Set("Cookie", auth.CookieName+"="+cookie)
	resp, err := fx.app.Test(r, -1)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var body struct {
		Data []contactfinder.Contact `json:"data"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	return resp.StatusCode, body.Data
}

func (fx contactsFixture) used(t *testing.T, userID int64) int {
	t.Helper()
	st, err := fx.plans.Standing(context.Background(), userID, plan.FeatureRecruiterContact)
	if err != nil {
		t.Fatal(err)
	}
	return st.Used
}

func TestRecruiterContacts_FirstViewChargesRepeatIsFreeSecondUserPaysFromCache(t *testing.T) {
	finder := &fakeFinder{contacts: twoRecruiters, suppressed: `[]`}
	fx := newContactsFixture(t, finder.client(t), 5)

	status, got := fx.post(t, fx.alice, "job-a")
	if status != fiber.StatusOK || len(got) != 2 || got[0].Email != "anna@acme.io" || got[0].Confidence != "high" {
		t.Fatalf("first view: %d %+v", status, got)
	}
	if status, _ := fx.post(t, fx.alice, "job-a"); status != fiber.StatusOK || fx.used(t, fx.alice) != 1 {
		t.Fatalf("repeat view: status %d, used %d, want 200 and still 1", status, fx.used(t, fx.alice))
	}
	if status, got := fx.post(t, fx.bob, "job-a"); status != fiber.StatusOK || len(got) != 2 || fx.used(t, fx.bob) != 1 {
		t.Fatalf("second user: %d %+v used %d", status, got, fx.used(t, fx.bob))
	}
	if n := finder.finds.Load(); n != 1 {
		t.Errorf("finds = %d, want 1 (the rest from the cache)", n)
	}
	if n := finder.checks.Load(); n != 2 {
		t.Errorf("checks = %d, want one per cache hit", n)
	}
	// The cache serve is reported to the service as shown to its viewer.
	if ref, want := finder.checkRef.Load(), strconv.FormatInt(fx.bob, 10); ref != want {
		t.Errorf("last check user_ref = %v, want %s", ref, want)
	}
}

func TestRecruiterContacts_EmptyAndFailureAreNotCharged(t *testing.T) {
	finder := &fakeFinder{contacts: `[]`, suppressed: `[]`}
	fx := newContactsFixture(t, finder.client(t), 5)
	if status, got := fx.post(t, fx.alice, "job-a"); status != fiber.StatusOK || got == nil || len(got) != 0 {
		t.Fatalf("empty: %d %#v, want 200 and []", status, got)
	}
	if n := fx.used(t, fx.alice); n != 0 {
		t.Errorf("empty answer charged: used %d", n)
	}

	finder.status = http.StatusServiceUnavailable
	if status, _ := fx.post(t, fx.alice, "job-b"); status != fiber.StatusServiceUnavailable {
		t.Fatalf("failing lookup: status %d, want 503", status)
	}
	if n := fx.used(t, fx.alice); n != 0 {
		t.Errorf("failed lookup charged: used %d", n)
	}
	var cached int
	_ = fx.pool.QueryRow(context.Background(), `SELECT count(*) FROM job_recruiter_contacts WHERE job_id = $1`, fx.jobIDs["job-b"]).Scan(&cached)
	if cached != 0 {
		t.Error("a failed lookup was cached")
	}
}

func TestRecruiterContacts_OverTheCeilingRefusesBeforeLookingUp(t *testing.T) {
	finder := &fakeFinder{contacts: twoRecruiters, suppressed: `[]`}
	fx := newContactsFixture(t, finder.client(t), 1)
	if status, _ := fx.post(t, fx.alice, "job-a"); status != fiber.StatusOK {
		t.Fatalf("first: %d", status)
	}
	if status, _ := fx.post(t, fx.alice, "job-b"); status != fiber.StatusPaymentRequired {
		t.Fatalf("over the ceiling: %d, want 402", status)
	}
	if status, _ := fx.post(t, fx.alice, "job-a"); status != fiber.StatusOK {
		t.Errorf("a job already opened must stay free over the ceiling: %d", status)
	}
	if n := finder.finds.Load(); n != 1 {
		t.Errorf("finds = %d, want the refusal before any lookup", n)
	}
}

func TestRecruiterContacts_SuppressedAddressIsDroppedFromTheCache(t *testing.T) {
	finder := &fakeFinder{contacts: twoRecruiters, suppressed: `[]`}
	fx := newContactsFixture(t, finder.client(t), 5)
	if status, _ := fx.post(t, fx.alice, "job-a"); status != fiber.StatusOK {
		t.Fatalf("seed view: %d", status)
	}

	finder.suppressed = `["anna@acme.io"]`
	status, got := fx.post(t, fx.bob, "job-a")
	if status != fiber.StatusOK || len(got) != 1 || got[0].Email != "bob@acme.io" {
		t.Fatalf("after removal: %d %+v", status, got)
	}
	var raw string
	_ = fx.pool.QueryRow(context.Background(), `SELECT contacts::text FROM job_recruiter_contacts WHERE job_id = $1`, fx.jobIDs["job-a"]).Scan(&raw)
	var cached []contactfinder.Contact
	_ = json.Unmarshal([]byte(raw), &cached)
	if len(cached) != 1 || cached[0].Email != "bob@acme.io" {
		t.Errorf("cache still holds the removed address: %s", raw)
	}
}

func TestRecruiterContacts_StaleCacheIsLookedUpAgain(t *testing.T) {
	finder := &fakeFinder{contacts: `[]`, suppressed: `[]`}
	fx := newContactsFixture(t, finder.client(t), 5)
	_, _ = fx.post(t, fx.alice, "job-a")
	if _, err := fx.pool.Exec(context.Background(),
		`UPDATE job_recruiter_contacts SET found_at = now() - interval '8 days'`); err != nil {
		t.Fatal(err)
	}
	finder.contacts = twoRecruiters
	if status, got := fx.post(t, fx.alice, "job-a"); status != fiber.StatusOK || len(got) != 2 {
		t.Fatalf("after an empty answer went stale: %d %+v", status, got)
	}
	if n := finder.finds.Load(); n != 2 {
		t.Errorf("finds = %d, want a second lookup", n)
	}
}

func TestRecruiterContacts_NoWebsiteAndFeatureOff(t *testing.T) {
	fx := newContactsFixture(t, (&fakeFinder{contacts: `[]`, suppressed: `[]`}).client(t), 5)
	if status, _ := fx.post(t, fx.alice, "no-site"); status != fiber.StatusUnprocessableEntity {
		t.Errorf("no website: %d, want 422", status)
	}
	if status, _ := fx.post(t, fx.alice, "no-such-job"); status != fiber.StatusNotFound {
		t.Errorf("unknown job: %d, want 404", status)
	}
	if n := fx.used(t, fx.alice); n != 0 {
		t.Errorf("used = %d, want nothing charged", n)
	}

	off := newContactsFixture(t, nil, 5)
	if status, _ := off.post(t, off.alice, "job-a"); status != fiber.StatusNotFound {
		t.Errorf("feature off: %d, want 404", status)
	}
}

func TestDeleteExpiredJobRecruiterContacts(t *testing.T) {
	fx := newContactsFixture(t, nil, 5)
	ctx := context.Background()
	if _, err := fx.pool.Exec(ctx, `INSERT INTO job_recruiter_contacts (job_id, contacts, found_at) VALUES
		($1, '[{"email":"a@acme.io"}]', now() - interval '29 days'),
		($2, '[]', now() - interval '8 days'),
		($3, '[{"email":"a@acme.io"}]', now() - interval '31 days')`,
		fx.jobIDs["job-a"], fx.jobIDs["job-b"], fx.jobIDs["no-site"]); err != nil {
		t.Fatal(err)
	}
	n, err := db.New(fx.pool).DeleteExpiredJobRecruiterContacts(ctx)
	if err != nil || n != 2 {
		t.Fatalf("deleted %d, %v; want the stale empty and the 31-day rows", n, err)
	}
	var left int64
	_ = fx.pool.QueryRow(ctx, `SELECT job_id FROM job_recruiter_contacts`).Scan(&left)
	if left != fx.jobIDs["job-a"] {
		t.Errorf("kept job %d, want job-a", left)
	}
}
