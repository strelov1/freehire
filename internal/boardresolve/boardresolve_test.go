package boardresolve

import (
	"context"
	"errors"
	"testing"
)

type fakeFetcher struct {
	html string
	err  error
}

func (f fakeFetcher) GetText(_ context.Context, _ string) (string, error) {
	return f.html, f.err
}

func resolver(html string, err error) *Resolver {
	return &Resolver{http: fakeFetcher{html: html, err: err}}
}

func TestResolveDetectsGreenhouseEmbed(t *testing.T) {
	// A vanity careers page embedding Greenhouse — the board is in the embed script's for=.
	html := `<html><body>
	  <script src="https://boards.greenhouse.io/embed/job_board/js?for=talkspace"></script>
	</body></html>`
	src, board, canon, ok := resolver(html, nil).Resolve(context.Background(),
		"https://www.talkspace.com/careers/job?gh_jid=6118228004")
	if !ok {
		t.Fatal("ok=false, want the embedded greenhouse board detected")
	}
	if src != "greenhouse" || board != "talkspace" {
		t.Errorf("(source, board) = (%q, %q), want (greenhouse, talkspace)", src, board)
	}
	if canon != "https://www.talkspace.com/careers/job" {
		t.Errorf("canonical = %q, want the URL without query/fragment", canon)
	}
}

func TestResolveDetectsPeopleForceApplyUrl(t *testing.T) {
	// A company careers page on its own domain that links to its PeopleForce board — caught by
	// scanning the page's URLs through the full recognizer (atsdetect doesn't know peopleforce).
	html := `{"applyUrl":"https://akvelon.peopleforce.io/careers/v/215183-senior-dev/a/new"}`
	src, board, _, ok := resolver(html, nil).Resolve(context.Background(), "https://akvelon.dev/jobs/215183")
	if !ok || src != "peopleforce" || board != "akvelon" {
		t.Errorf("(%q,%q,%v), want (peopleforce, akvelon, true)", src, board, ok)
	}
}

func TestResolveDetectsDirectAshbyLinkOnPage(t *testing.T) {
	html := `<a href="https://jobs.ashbyhq.com/acme/uuid">Apply</a>`
	src, board, _, ok := resolver(html, nil).Resolve(context.Background(), "https://acme.io/careers")
	if !ok || src != "ashby" || board != "acme" {
		t.Errorf("(%q,%q,%v), want (ashby, acme, true)", src, board, ok)
	}
}

func TestResolveDetectsWorkdayLinkOnPage(t *testing.T) {
	html := `<a href="https://acme.wd1.myworkdayjobs.com/Careers/job/x/Eng_JR-1">Apply</a>`
	src, board, _, ok := resolver(html, nil).Resolve(context.Background(), "https://acme.com/jobs")
	if !ok || src != "workday" || board != "acme.wd1.myworkdayjobs.com/Careers" {
		t.Errorf("(%q,%q,%v), want (workday, acme.wd1.myworkdayjobs.com/Careers, true)", src, board, ok)
	}
}

func TestResolveRejectsUnsupportedAts(t *testing.T) {
	// Taleo isn't in the recognizer — a page exposing only a Taleo link resolves nothing.
	html := `<a href="https://acme.taleo.net/careersection/x/jobdetail.ftl?job=123">Apply</a>`
	if _, _, _, ok := resolver(html, nil).Resolve(context.Background(), "https://acme.com/jobs"); ok {
		t.Error("ok=true for an unsupported ATS, want false")
	}
}

func TestResolveNoAtsAndFetchError(t *testing.T) {
	if _, _, _, ok := resolver("<html>no ats here</html>", nil).Resolve(context.Background(), "https://x.com"); ok {
		t.Error("ok=true for a page with no ATS, want false")
	}
	if _, _, _, ok := resolver("", errors.New("boom")).Resolve(context.Background(), "https://x.com"); ok {
		t.Error("ok=true on fetch error, want false")
	}
}
