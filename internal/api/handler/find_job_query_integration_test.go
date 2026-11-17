//go:build integration

package handler

import (
	"context"
	"net/url"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/ingest/linksource"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/db"
)

// freehire#4821: on CGI's Njoyn the query, not the path, names the posting, and /jobs/find
// answered every CGI page with the one Njoyn posting the catalogue held.
func TestFindJobAnswersAQueryIdentifiedPostingOnlyOnItsOwnPage(t *testing.T) {
	pool := startPostgres(t)
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO jobs (source, external_id, url, title, public_slug) VALUES
		 ('wttj', 'cgi:expert-full-stack',
		  'https://cgi.njoyn.com/corp/xweb/xweb.asp?clid=21001&page=jobdetails&jobid=J1026-1962&BRID=1342736&SBDID=22288&utm_source=freehire.me',
		  'Expert(e) technique Full Stack F/H', 'expert-full-stack-cgi')`); err != nil {
		t.Fatalf("seed job: %v", err)
	}
	h := &jobsHandlers{queries: db.New(pool), postings: sources.NewPostingURLResolver(stubPostingAPI{})}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	mountJobsContract(app, h, middleware{})

	own := "https://cgi.njoyn.com/CORP/xweb/xweb.asp?Jobid=J1026-1962&SBDID=22288&BRID=1342736&Page=JobDetails&CLID=21001&gclid=x"
	if got := findSlug(t, app, url.QueryEscape(own)); got != "expert-full-stack-cgi" {
		t.Errorf("own page resolved to %q, want the posting", got)
	}
	// The two URLs from the report, and the bare script.
	for _, other := range []string{
		"https://cgi.njoyn.com/CORP/xweb/xweb.asp?CLID=21001&page=joblisting",
		"https://cgi.njoyn.com/corp/xweb/xweb.asp?NTKN=c&clid=21001&Page=JobDetails&Jobid=J1126-1603&BRID=1350470&lang=1",
		"https://cgi.njoyn.com/corp/xweb/xweb.asp",
	} {
		if got := findSlug(t, app, url.QueryEscape(other)); got != "" {
			t.Errorf("find %q = %q, want data: null", other, got)
		}
	}
}

// normalize_job_url repeats linksource's tracking vocabulary (an IMMUTABLE index function
// cannot read it), so the two are held together here: SQL must drop every parameter Go
// calls tracking, and keep the ones that name a posting. A key added to linksource fails
// this until a migration replaces the function too.
func TestNormalizeJobURLDropsExactlyLinksourceTrackingParams(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	normalize := func(raw string) string {
		t.Helper()
		var got string
		if err := pool.QueryRow(ctx, "SELECT normalize_job_url($1)", raw).Scan(&got); err != nil {
			t.Fatalf("normalize %q: %v", raw, err)
		}
		return got
	}
	const page = "https://acme.example/careers?jobid=42"
	want := normalize(page)
	for _, k := range append(linksource.TrackingParamNames(), "utm_source", "UTM_Medium") {
		if got := normalize(page + "&" + k + "=x"); got != want {
			t.Errorf("SQL kept tracking param %q: %q, want %q", k, got, want)
		}
	}
	// Parameters that name a posting on some platform must survive.
	for _, k := range []string{"id", "jobid", "gh_jid", "jk", "oid", "token", "page"} {
		if got := normalize(page + "&" + k + "=x"); got == want {
			t.Errorf("SQL dropped identifying param %q", k)
		}
	}
}
