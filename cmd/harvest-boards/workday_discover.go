package main

import (
	"context"
	"net/url"
	"regexp"
	"strings"
)

// workdayCDXPattern is the Common Crawl query that enumerates Workday career sites. Each tenant has
// its own subdomain, so the glob is on the HOST with deliberately no path glob after it (see
// commonCrawlCandidates for why).
//
// The crawl is the only enumeration: Workday publishes no tenant directory, the data-centre label
// ("wd1".."wd503") and site path ("External", "Careers") do not follow from the employer's name, and
// a tenant runs SEVERAL sites at once, each its own pool of postings.
const workdayCDXPattern = "*.myworkdayjobs.com"

// workdayHost matches a Workday career-site host "<tenant>.wd<N>.myworkdayjobs.com". The data-centre
// label separates a real board host from the vendor's marketing and documentation hosts.
var workdayHost = regexp.MustCompile(`^[a-z0-9-]+\.wd\d+\.myworkdayjobs\.com$`)

// workdayLocale matches the optional locale segment ("/en-US/Careers/job/..."). It is a route, not a
// board: treating it as the site would mint "acme/en-US".
var workdayLocale = regexp.MustCompile(`^[a-z]{2}-[A-Za-z]{2}$`)

// workdayNonSite lists path segments that sit where a site path would but name something else:
// service files, bare language shorthand, and the CXS API root the prober posts to. Without it the
// sweep proposes "acme.wd1.myworkdayjobs.com/robots.txt" as a board.
var workdayNonSite = regexp.MustCompile(`^(?i:robots\.txt|sitemap.*|favicon.*|[a-z]{2}|wday|static|assets|api|images?)$`)

// discover enumerates candidate Workday boards from Common Crawl's index of
// *.myworkdayjobs.com, slicing each crawled URL to "<host>/<site>".
func (workdayProber) discover(ctx context.Context, c httpClient) ([]string, error) {
	return commonCrawlCandidates(ctx, c, workdayCDXPattern, workdayCandidate)
}

// workdayCandidate slices a crawled Workday URL to its board id "<host>/<site>", reporting ok=false
// for anything else.
//
// The host is lower-cased and the site path is not, matching commonCrawlSlug: the CXS site path is
// case-insensitive and folding it is workdayProber.dedupKey's job; folding here would lose the
// casing the board file displays.
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
