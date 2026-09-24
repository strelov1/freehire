package main

import (
	"context"
	"net/url"
	"regexp"
	"strings"
)

// workdayCDXPattern is the Common Crawl query that enumerates Workday career sites. Workday
// gives each tenant its own subdomain, so the glob is on the HOST and there is deliberately no
// path glob after it — see commonCrawlCandidates for why the two spellings are not
// interchangeable and what the index answers to the other one.
//
// This is the only enumeration there is: Workday publishes no directory of tenants, a board id
// carries a data-centre label ("wd1".."wd503") and a site path ("External", "Careers",
// "Search") that neither follow from the employer's name, and a tenant runs SEVERAL sites at
// once (Activision lists blizzard_external_careers, centraltech and ss_external side by side),
// each its own pool of postings. Guessing cannot reach any of that; the crawl has seen all of
// it.
const workdayCDXPattern = "*.myworkdayjobs.com"

// workdayHost matches a Workday career-site host: "<tenant>.wd<N>.myworkdayjobs.com". The
// data-centre label is what separates a real board host from the vendor's own marketing and
// documentation hosts on the same domain, which carry no tenant and no site.
var workdayHost = regexp.MustCompile(`^[a-z0-9-]+\.wd\d+\.myworkdayjobs\.com$`)

// workdayLocale matches the optional locale segment Workday puts between the host and the site
// path ("/en-US/Careers/job/..."). It is a route, not a board: the same site under a second
// locale is the same board, and treating the locale as the site would mint "acme/en-US".
var workdayLocale = regexp.MustCompile(`^[a-z]{2}-[A-Za-z]{2}$`)

// workdayNonSite lists the path segments that sit where a site path would but name something
// else: the crawler's own fetches of service files, the bare language shorthand some sites
// redirect through, and the CXS API root the prober itself posts to. Without this the sweep
// proposes "acme.wd1.myworkdayjobs.com/robots.txt" as a board and spends a probe proving it is
// not one.
var workdayNonSite = regexp.MustCompile(`^(?i:robots\.txt|sitemap.*|favicon.*|[a-z]{2}|wday|static|assets|api|images?)$`)

// discover enumerates candidate Workday boards from Common Crawl's index of
// *.myworkdayjobs.com, slicing each crawled URL to "<host>/<site>".
func (workdayProber) discover(ctx context.Context, c httpClient) ([]string, error) {
	return commonCrawlCandidates(ctx, c, workdayCDXPattern, workdayCandidate)
}

// workdayCandidate slices a crawled Workday URL to its board id — "<host>/<site>", the shape
// the prober and the board catalog both use — reporting ok=false for anything that is not one.
//
// The host is folded to lower case and the site path is not, matching commonCrawlSlug's
// division of labour: the CXS site path is case-insensitive, and folding it is the dedup
// layer's job (workdayProber.dedupKey), not this function's. Folding it here would also lose
// the casing the board file then displays.
func workdayCandidate(rawURL string) (string, bool) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", false
	}
	host := strings.ToLower(u.Hostname())
	if !workdayHost.MatchString(host) {
		return "", false
	}
	for _, part := range strings.Split(u.Path, "/") {
		if part == "" || workdayLocale.MatchString(part) {
			continue
		}
		if workdayNonSite.MatchString(part) {
			return "", false
		}
		return host + "/" + part, true
	}
	return "", false
}
