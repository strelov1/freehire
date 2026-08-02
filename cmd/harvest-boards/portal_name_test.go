package main

import (
	"context"
	"testing"
)

// jazzhr and trakstar publish no employer name in the payload the prober reads for liveness,
// but both put one in the portal itself. Reading it is what lets the corroboration gate fire
// for them at all — without it their boards are accepted on liveness alone, which is how 44
// boards were filed under the wrong employer (jones → "Test Company", intel → "IntelContact",
// cme → a guitar shop) before an out-of-band audit caught them.
func TestJazzHRProbeReadsTheEmployerFromTheCareerPage(t *testing.T) {
	getter := fakeGetter{
		"https://83bar.applytojob.com/apply": `<html><head><title>83BAR - Career Page</title></head>
			<body><a href="/apply/aBcDeF123456/nurse">Nurse</a></body></html>`,
		// A portal whose title carries no employer name still counts as live; it just
		// reports no name, and the seed's label stands.
		"https://nameless.applytojob.com/apply": `<html><head><title>Careers</title></head>
			<body><a href="/apply/zZyYxX987654/engineer">Engineer</a></body></html>`,
	}
	p := jazzhrProber{}

	if name, n, err := p.probe(context.Background(), getter, "83bar"); err != nil || name != "83BAR" || n != 1 {
		t.Errorf("got (%q,%d,%v), want (\"83BAR\",1,nil)", name, n, err)
	}
	if name, n, err := p.probe(context.Background(), getter, "nameless"); err != nil || name != "" || n != 1 {
		t.Errorf("titleless portal: got (%q,%d,%v), want (\"\",1,nil)", name, n, err)
	}
}

func TestTrakstarProbeReadsTheEmployerFromTheFeed(t *testing.T) {
	getter := fakeGetter{
		"https://aflac.hire.trakstar.com/jobfeeds/aflac": `<rss><channel>
			<title>Jobs at AFLAC</title><item><title>Agent</title></item></channel></rss>`,
		"https://plain.hire.trakstar.com/jobfeeds/plain": `<rss><channel>
			<title>Jobs</title><item><title>Agent</title></item></channel></rss>`,
	}
	p := trakstarProber{}

	if name, n, err := p.probe(context.Background(), getter, "aflac"); err != nil || name != "AFLAC" || n != 1 {
		t.Errorf("got (%q,%d,%v), want (\"AFLAC\",1,nil)", name, n, err)
	}
	// "Jobs" alone names nobody — reporting it would gate every board against a word.
	if name, n, err := p.probe(context.Background(), getter, "plain"); err != nil || name != "" || n != 1 {
		t.Errorf("nameless feed: got (%q,%d,%v), want (\"\",1,nil)", name, n, err)
	}
}

// lever and ashby publish the employer on their storefront page but not in the posting API
// the prober reads for liveness. The storefront is fetched only once a board is known to
// have jobs, so the extra request costs one per live board, not one per candidate.
func TestLeverProbeReadsTheEmployerFromTheStorefront(t *testing.T) {
	getter := fakeGetter{
		"https://api.lever.co/v0/postings/findhelp?mode=json": `[{"id":"a"},{"id":"b"}]`,
		"https://jobs.lever.co/findhelp":                      `<html><head><title>Findhelp, A Public Benefit Corporation</title></head></html>`,
		// A live board whose storefront cannot be read still counts as live.
		"https://api.lever.co/v0/postings/quiet?mode=json": `[{"id":"a"}]`,
	}
	p := leverProber{}

	name, n, err := p.probe(context.Background(), getter, "findhelp")
	if err != nil || name != "Findhelp, A Public Benefit Corporation" || n != 2 {
		t.Errorf("got (%q,%d,%v), want the storefront name and 2", name, n, err)
	}
	if name, n, err := p.probe(context.Background(), getter, "quiet"); err != nil || name != "" || n != 1 {
		t.Errorf("unreadable storefront: got (%q,%d,%v), want (\"\",1,nil)", name, n, err)
	}
}

func TestAshbyProbeReadsTheEmployerFromTheStorefront(t *testing.T) {
	getter := fakeGetter{
		"https://api.ashbyhq.com/posting-api/job-board/ddn": `{"jobs":[{"id":"a"}]}`,
		"https://jobs.ashbyhq.com/ddn":                      `<html><head><title>DDN Jobs</title></head></html>`,
		// An unknown ashby slug still serves a 200 titled just "Jobs" — stripping the suffix
		// leaves nothing, which must read as "no name" rather than as an employer.
		"https://api.ashbyhq.com/posting-api/job-board/bare": `{"jobs":[{"id":"a"}]}`,
		"https://jobs.ashbyhq.com/bare":                      `<html><head><title>Jobs</title></head></html>`,
	}
	p := ashbyProber{}

	if name, n, err := p.probe(context.Background(), getter, "ddn"); err != nil || name != "DDN" || n != 1 {
		t.Errorf("got (%q,%d,%v), want (\"DDN\",1,nil)", name, n, err)
	}
	if name, n, err := p.probe(context.Background(), getter, "bare"); err != nil || name != "" || n != 1 {
		t.Errorf("bare title: got (%q,%d,%v), want (\"\",1,nil)", name, n, err)
	}
}
