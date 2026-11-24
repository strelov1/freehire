package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/job/job"
	"github.com/strelov1/freehire/internal/platform/db"
)

type fakeGoStore struct {
	jobs      map[string]db.Job
	lookupErr error
	recordErr error
	clicks    []db.RecordJobOutboundClickParams
}

func (f *fakeGoStore) GetJobBySlug(_ context.Context, slug string) (db.Job, error) {
	if f.lookupErr != nil {
		return db.Job{}, f.lookupErr
	}
	if j, ok := f.jobs[slug]; ok {
		return j, nil
	}
	return db.Job{}, pgx.ErrNoRows
}

func (f *fakeGoStore) ListJobsBySlugCode(context.Context, string) ([]db.Job, error) { return nil, nil }

func (f *fakeGoStore) RecordJobOutboundClick(_ context.Context, p db.RecordJobOutboundClickParams) error {
	f.clicks = append(f.clicks, p)
	return f.recordErr
}

func goApp(store *fakeGoStore) *fiber.App {
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/go/:slug", (&goHandlers{store: store, salt: "pepper"}).Redirect)
	return app
}

func openJob() db.Job {
	return db.Job{ID: 42, Source: "greenhouse", ExternalID: "acme:1", PublicSlug: "go-engineer-acme-1a2b3c4d", URL: "https://boards.greenhouse.io/acme/jobs/1"}
}

// goReply is what a test reads off a response; the body is closed before it is returned.
type goReply struct {
	StatusCode int
	Header     http.Header
}

func goGet(t *testing.T, app *fiber.App, target string, hdr map[string]string) goReply {
	t.Helper()
	req := humanGet(target)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	_ = resp.Body.Close()
	return goReply{StatusCode: resp.StatusCode, Header: resp.Header}
}

func TestGoRedirectsToTheStoredDestinationAndCountsTheClick(t *testing.T) {
	store := &fakeGoStore{jobs: map[string]db.Job{openJob().PublicSlug: openJob()}}
	resp := goGet(t, goApp(store), "/go/go-engineer-acme-1a2b3c4d?src=mcp", map[string]string{
		"CF-IPCountry": "de", "Referer": "https://mail.example.com/inbox/thread/secret?x=1",
	})

	if resp.StatusCode != fiber.StatusFound {
		t.Fatalf("status = %d, want 302", resp.StatusCode)
	}
	if got, want := resp.Header.Get("Location"), "https://boards.greenhouse.io/acme/jobs/1?utm_source=freehire.me"; got != want {
		t.Errorf("Location = %q, want %q", got, want)
	}
	if len(store.clicks) != 1 {
		t.Fatalf("recorded %d clicks, want 1", len(store.clicks))
	}
	c := store.clicks[0]
	if c.JobID != 42 || c.Src != "mcp" || c.IsLikelyBot || c.Country != "DE" || c.ReferrerHost != "mail.example.com" {
		t.Errorf("click = %+v", c)
	}
	if c.DeviceType != "desktop" || c.OsFamily != "macos" || c.UaFamily != "chrome" || len(c.VisitorHash) != 64 {
		t.Errorf("client = %+v", c)
	}
}

func TestGoUsesTheVerifiedApplicationDestination(t *testing.T) {
	j := openJob()
	j.ApplicationUrl = "https://apply.example.com/form?id=9"
	store := &fakeGoStore{jobs: map[string]db.Job{j.PublicSlug: j}}
	resp := goGet(t, goApp(store), "/go/"+j.PublicSlug, nil)
	if got, want := resp.Header.Get("Location"), "https://apply.example.com/form?id=9&utm_source=freehire.me"; got != want {
		t.Errorf("Location = %q, want %q", got, want)
	}
}

// The destination is the job's. No query parameter or header may name another one.
func TestGoNeverTakesTheDestinationFromTheRequest(t *testing.T) {
	store := &fakeGoStore{jobs: map[string]db.Job{openJob().PublicSlug: openJob()}}
	resp := goGet(t, goApp(store), "/go/go-engineer-acme-1a2b3c4d?src=web&url=https://evil.example&to=https://evil.example&redirect=//evil.example",
		map[string]string{"X-Forwarded-Host": "evil.example", "Host": "evil.example"})
	if loc := resp.Header.Get("Location"); !strings.HasPrefix(loc, "https://boards.greenhouse.io/") {
		t.Errorf("Location = %q", loc)
	}
}

func TestGoUnknownSrcIsStoredAsOtherNotRejected(t *testing.T) {
	store := &fakeGoStore{jobs: map[string]db.Job{openJob().PublicSlug: openJob()}}
	for _, target := range []string{"/go/go-engineer-acme-1a2b3c4d?src=fax", "/go/go-engineer-acme-1a2b3c4d"} {
		if resp := goGet(t, goApp(store), target, nil); resp.StatusCode != fiber.StatusFound {
			t.Errorf("%s: status = %d, want 302", target, resp.StatusCode)
		}
	}
	for _, c := range store.clicks {
		if c.Src != "other" {
			t.Errorf("src = %q, want other", c.Src)
		}
	}
}

func TestGoFlagsBotsButStillRedirects(t *testing.T) {
	store := &fakeGoStore{jobs: map[string]db.Job{openJob().PublicSlug: openJob()}}
	resp := goGet(t, goApp(store), "/go/go-engineer-acme-1a2b3c4d", map[string]string{"User-Agent": "Mozilla/5.0 (compatible; Googlebot/2.1)"})
	if resp.StatusCode != fiber.StatusFound || len(store.clicks) != 1 || !store.clicks[0].IsLikelyBot {
		t.Errorf("status=%d clicks=%+v, want a redirect and one flagged click", resp.StatusCode, store.clicks)
	}
}

func TestGoRedirectsEvenWhenTheClickWriteFails(t *testing.T) {
	store := &fakeGoStore{jobs: map[string]db.Job{openJob().PublicSlug: openJob()}, recordErr: errors.New("pool exhausted")}
	if resp := goGet(t, goApp(store), "/go/go-engineer-acme-1a2b3c4d", nil); resp.StatusCode != fiber.StatusFound {
		t.Errorf("status = %d, want 302", resp.StatusCode)
	}
}

func TestGoClosedJobStillRedirects(t *testing.T) {
	j := openJob()
	j.ClosedAt.Valid = true
	store := &fakeGoStore{jobs: map[string]db.Job{j.PublicSlug: j}}
	if resp := goGet(t, goApp(store), "/go/"+j.PublicSlug, nil); resp.StatusCode != fiber.StatusFound {
		t.Errorf("status = %d, want 302", resp.StatusCode)
	}
}

func TestGoShowsAPageWhereThereIsNowhereToSend(t *testing.T) {
	private := openJob()
	private.PublicSlug, private.IsPrivate = "private-1", true
	direct := db.Job{ID: 7, Source: job.EmployerSource, ExternalID: job.DirectApplyPrefix + "1", PublicSlug: "direct-1", URL: "https://freehire.me/jobs/direct-1"}
	badScheme := openJob()
	badScheme.PublicSlug, badScheme.URL = "bad-1", "javascript:alert(1)"
	store := &fakeGoStore{jobs: map[string]db.Job{"private-1": private, "direct-1": direct, "bad-1": badScheme}}

	for _, slug := range []string{"never-existed", "private-1", "direct-1", "bad-1"} {
		resp := goGet(t, goApp(store), "/go/"+slug, nil)
		if resp.StatusCode != fiber.StatusGone || !strings.Contains(resp.Header.Get("Content-Type"), "text/html") {
			t.Errorf("%s: status=%d type=%q, want a 410 page", slug, resp.StatusCode, resp.Header.Get("Content-Type"))
		}
	}
	if len(store.clicks) != 0 {
		t.Errorf("recorded %d clicks for links that lead nowhere", len(store.clicks))
	}
}

// A storage failure is not "gone": a 410 would tell the reader the posting was removed.
func TestGoALookupFailureIsNotGone(t *testing.T) {
	store := &fakeGoStore{lookupErr: errors.New("pool timeout")}
	resp, err := goApp(store).Test(httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/go/x", nil))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != fiber.StatusInternalServerError {
		t.Errorf("status = %d, want 500", resp.StatusCode)
	}
}

func TestCountryKeepsOnlyACloudflareCode(t *testing.T) {
	for in, want := range map[string]string{"de": "DE", "us": "US", "XX": "XX", "T1": "T1", "": "", "DEU": "", "<s": "", "d-": ""} {
		if got := country(in); got != want {
			t.Errorf("country(%q) = %q, want %q", in, got, want)
		}
	}
}
