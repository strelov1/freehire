package handler

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/engage/sponsored"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
)

type fakeCharger struct {
	calls []sponsored.Click
	ids   []int64
	err   error
}

func (f *fakeCharger) Charge(_ context.Context, id int64, c sponsored.Click, _ time.Time) (sponsored.Decision, error) {
	f.ids = append(f.ids, id)
	f.calls = append(f.calls, c)
	return sponsored.Decision{}, f.err
}

type fakeClickQueries struct {
	job         db.Job
	jobErr      error
	campaignJob int64
}

func (f fakeClickQueries) GetJob(context.Context, int64) (db.Job, error) { return f.job, f.jobErr }
func (f fakeClickQueries) GetSponsoredCampaignJobID(context.Context, int64) (int64, error) {
	return f.campaignJob, nil
}

func clickApp(ch clickCharger, q clickQueries) *fiber.App {
	app := fiber.New()
	newSponsoredClicks(testClickKey, ch, q, "https://site.test/").register(app.Group("/api/v1"))
	return app
}

func clickJob() db.Job {
	return db.Job{ID: 7, Source: "greenhouse", ExternalID: "x", URL: "https://ats.example/jobs/7", PublicSlug: "dev-7"}
}

func getClick(t *testing.T, app *fiber.App, token, ua string) (status int, location string) {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), "GET", "/api/v1/sponsored/go/"+token, nil)
	if ua != "" {
		req.Header.Set("User-Agent", ua)
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if got := resp.Header.Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q", got)
	}
	_ = resp.Body.Close()
	return resp.StatusCode, resp.Header.Get("Location")
}

func clickToken() string {
	return sponsored.SignToken(testClickKey, sponsored.Claims{CampaignID: 3, JobID: 7, Placement: sponsored.PlacementSearch}, time.Now())
}

const clickUA = "Mozilla/5.0 (X11; Linux x86_64) Firefox/130.0"

func TestSponsoredClickChargesAndRedirectsToTheJob(t *testing.T) {
	ch := &fakeCharger{}
	app := clickApp(ch, fakeClickQueries{job: clickJob(), campaignJob: 7})
	status, loc := getClick(t, app, clickToken(), clickUA)
	if status != 302 || loc != "https://ats.example/jobs/7?utm_source=freehire.me" {
		t.Fatalf("got %d %q", status, loc)
	}
	if len(ch.calls) != 1 || ch.ids[0] != 3 {
		t.Fatalf("charge calls = %v %v", ch.ids, ch.calls)
	}
	c := ch.calls[0]
	if c.Placement != "search" || c.Bot || c.RateLimited || len(c.VisitorHash) != 64 {
		t.Errorf("click = %+v", c)
	}
}

func TestSponsoredClickBadTokenGoesToJobsPage(t *testing.T) {
	ch := &fakeCharger{}
	app := clickApp(ch, fakeClickQueries{job: clickJob(), campaignJob: 7})
	for _, tok := range []string{"garbage", clickToken() + "x"} {
		if status, loc := getClick(t, app, tok, clickUA); status != 302 || loc != "https://site.test/jobs" {
			t.Errorf("%q: got %d %q", tok, status, loc)
		}
	}
	if len(ch.calls) != 0 {
		t.Error("charged on a bad token")
	}
}

func TestSponsoredClickBotAndEmptyUserAgent(t *testing.T) {
	ch := &fakeCharger{}
	app := clickApp(ch, fakeClickQueries{job: clickJob(), campaignJob: 7})
	getClick(t, app, clickToken(), "Googlebot/2.1")
	getClick(t, app, clickToken(), "")
	if len(ch.calls) != 2 || !ch.calls[0].Bot || !ch.calls[1].Bot {
		t.Errorf("calls = %+v", ch.calls)
	}
}

func TestSponsoredClickRateLimitFromThe21st(t *testing.T) {
	ch := &fakeCharger{}
	app := clickApp(ch, fakeClickQueries{job: clickJob(), campaignJob: 7})
	for range 21 {
		getClick(t, app, clickToken(), clickUA)
	}
	if ch.calls[19].RateLimited || !ch.calls[20].RateLimited {
		t.Errorf("20th limited=%v, 21st limited=%v", ch.calls[19].RateLimited, ch.calls[20].RateLimited)
	}
}

func TestSponsoredClickCampaignOfAnotherJobIsNotCharged(t *testing.T) {
	ch := &fakeCharger{}
	app := clickApp(ch, fakeClickQueries{job: clickJob(), campaignJob: 99})
	if status, loc := getClick(t, app, clickToken(), clickUA); status != 302 || loc != "https://ats.example/jobs/7?utm_source=freehire.me" {
		t.Errorf("got %d %q", status, loc)
	}
	if len(ch.calls) != 0 {
		t.Error("charged a mismatched campaign")
	}
}

func TestSponsoredClickGoneJobGoesToJobsPage(t *testing.T) {
	ch := &fakeCharger{}
	app := clickApp(ch, fakeClickQueries{jobErr: errors.New("no rows")})
	if status, loc := getClick(t, app, clickToken(), clickUA); status != 302 || loc != "https://site.test/jobs" {
		t.Errorf("got %d %q", status, loc)
	}
	if len(ch.calls) != 0 {
		t.Error("charged for a gone job")
	}
}

func TestSponsoredClickChargeErrorStillRedirects(t *testing.T) {
	ch := &fakeCharger{err: errors.New("db down")}
	app := clickApp(ch, fakeClickQueries{job: clickJob(), campaignJob: 7})
	if status, loc := getClick(t, app, clickToken(), clickUA); status != 302 || loc != "https://ats.example/jobs/7?utm_source=freehire.me" {
		t.Errorf("got %d %q", status, loc)
	}
}

func TestSponsoredClickDirectApplyJobGoesToItsFreehirePage(t *testing.T) {
	j := clickJob()
	j.Source, j.ExternalID = "employer", "freehire:1"
	app := clickApp(&fakeCharger{}, fakeClickQueries{job: j, campaignJob: 7})
	if status, loc := getClick(t, app, clickToken(), clickUA); status != 302 || loc != "https://site.test/jobs/dev-7" {
		t.Errorf("got %d %q", status, loc)
	}
}

// A job closed since its card was served still takes the seeker to it, but costs nothing: the
// hourly sweep that ends its campaign may not have run yet.
func TestSponsoredClickClosedJobIsNotCharged(t *testing.T) {
	ch := &fakeCharger{}
	job := clickJob()
	job.ClosedAt = pgconv.Instant(time.Now().Add(-time.Hour))
	app := clickApp(ch, fakeClickQueries{job: job, campaignJob: 7})
	status, loc := getClick(t, app, clickToken(), clickUA)
	if status != 302 || loc == "" {
		t.Fatalf("got %d %q", status, loc)
	}
	if len(ch.calls) != 0 {
		t.Fatalf("a closed job must not be charged, calls = %v", ch.calls)
	}
}
