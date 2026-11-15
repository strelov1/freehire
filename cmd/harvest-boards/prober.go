package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/strelov1/freehire/internal/ingest/sources"
	"golang.org/x/net/html"
)

// greenhouseBoardsAPI mirrors the unexported sources.greenhouseBaseURL.
const greenhouseBoardsAPI = "https://boards-api.greenhouse.io/v1/boards"

// httpClient is the transport a prober needs: GetJSON, PostJSON (Workday CXS), GetXML (iCIMS/Deel
// sitemaps) and GetHTML (Freshteam). *sources.Client implements all of them.
type httpClient interface {
	sources.JSONGetter
	sources.JSONPoster
	sources.XMLGetter
	sources.HTMLGetter
	sources.TextGetter
	sources.HeaderJSONGetter
	sources.HeaderJSONPoster
}

// prober checks one candidate board, returning the platform's company name and open-job count.
// An absent, closed or unreachable board yields ("", 0, nil), a skip that never aborts the
// harvest; a non-nil error is for failures worth surfacing (the caller logs and skips those too).
type prober interface {
	probe(ctx context.Context, c httpClient, slug string) (company string, openJobs int, err error)
}

// seededProber is the optional half for a prober that needs the seed's employer to probe at all
// (today only adapterProber). probeAll calls it instead of probe, passing "" for a company-less seed.
type seededProber interface {
	probeSeeded(ctx context.Context, c httpClient, slug, company string) (string, int, error)
}

// idProber is the optional half that reports the ids of a board's live postings, so a seed that
// knows one posting's id can identify the board by evidence instead of by a name resemblance.
//
// Implement it only where one request yields the COMPLETE live list, because a missing id reads as
// "wrong board". A prober that pages, or asks per posting, must not: its silence leaves the id inert.
//
// It re-requests the listing rather than widening prober's return, and only for seeds naming a posting.
type idProber interface {
	postingIDs(ctx context.Context, c httpClient, slug string) ([]string, error)
}

// greenhouseProber probes the Greenhouse public boards API. The jobs endpoint lists only
// live postings, so a non-empty list means a live board. The company name comes from the
// board-metadata endpoint, fetched only once a board is known to have jobs.
type greenhouseProber struct{}

func greenhouseJobsURL(slug string) string {
	return fmt.Sprintf("%s/%s/jobs", greenhouseBoardsAPI, slug)
}

// postingIDs lists the live posting ids; one response carries the complete list.
func (greenhouseProber) postingIDs(ctx context.Context, c httpClient, slug string) ([]string, error) {
	var jr struct {
		Jobs []struct {
			ID int64 `json:"id"`
		} `json:"jobs"`
	}
	if err := c.GetJSON(ctx, greenhouseJobsURL(slug), &jr); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(jr.Jobs))
	for _, j := range jr.Jobs {
		ids = append(ids, strconv.FormatInt(j.ID, 10))
	}
	return ids, nil
}

func (greenhouseProber) probe(ctx context.Context, c httpClient, slug string) (string, int, error) {
	var jr struct {
		Jobs []struct {
			ID int64 `json:"id"`
		} `json:"jobs"`
	}
	// A missing or moved board is a 4xx error: skip it, don't propagate.
	if err := c.GetJSON(ctx, greenhouseJobsURL(slug), &jr); err != nil {
		return "", 0, nil
	}
	if len(jr.Jobs) == 0 {
		return "", 0, nil
	}
	var meta struct {
		Name string `json:"name"`
	}
	_ = c.GetJSON(ctx, fmt.Sprintf("%s/%s", greenhouseBoardsAPI, slug), &meta)
	name := meta.Name
	if name == "" {
		name = slug
	}
	return name, len(jr.Jobs), nil
}

// greenhouseBoardsHost is the web host Common Crawl actually visits. Validation still goes through
// greenhouseBoardsAPI.
const greenhouseBoardsHost = "boards.greenhouse.io"

// discover finds Greenhouse board candidates from the Common Crawl CDX index: every URL
// Common Crawl has seen under greenhouseBoardsHost, sliced to its company slug.
func (greenhouseProber) discover(ctx context.Context, c httpClient) ([]string, error) {
	return commonCrawlCandidates(ctx, c, greenhouseBoardsHost+"/*", commonCrawlSlug)
}

// dedupKey folds case: the boards API is case-insensitive, so a differently cased Common Crawl hit
// is the same board.
func (greenhouseProber) dedupKey(boardID string) string { return strings.ToLower(boardID) }

// leverProber probes the Lever postings API. The JSON-mode endpoint returns a bare array
// of live postings, so a non-empty array is a live board. The posting API exposes no company
// name, but the board's storefront titles itself after the employer, so the name is read
// there once the board is known to be live.
type leverProber struct{}

func leverPostingsURL(slug string) string {
	return fmt.Sprintf("https://api.lever.co/v0/postings/%s?mode=json", slug)
}

// postingIDs lists the live posting ids; one response carries the complete list.
func (leverProber) postingIDs(ctx context.Context, c httpClient, slug string) ([]string, error) {
	var postings []struct {
		ID string `json:"id"`
	}
	if err := c.GetJSON(ctx, leverPostingsURL(slug), &postings); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(postings))
	for _, p := range postings {
		ids = append(ids, p.ID)
	}
	return ids, nil
}

func (leverProber) probe(ctx context.Context, c httpClient, slug string) (string, int, error) {
	var postings []struct {
		ID string `json:"id"`
	}
	if err := c.GetJSON(ctx, leverPostingsURL(slug), &postings); err != nil {
		return "", 0, nil
	}
	if len(postings) == 0 {
		return "", 0, nil
	}
	return storefrontEmployer(ctx, c, fmt.Sprintf("https://jobs.lever.co/%s", slug), ""), len(postings), nil
}

// ashbyProber probes the Ashby job-board API. A non-empty list is a live board; the name is read
// from the storefront title "<Company> Jobs". The API is case-insensitive, see dedupKey.
type ashbyProber struct{}

// dedupKey folds case: the API serves identical content for "abridge" and "Abridge", so they are
// one board.
func (ashbyProber) dedupKey(boardID string) string { return strings.ToLower(boardID) }

func ashbyBoardURL(slug string) string {
	return fmt.Sprintf("https://api.ashbyhq.com/posting-api/job-board/%s", slug)
}

// postingIDs lists the live posting ids; one response carries the complete list.
func (ashbyProber) postingIDs(ctx context.Context, c httpClient, slug string) ([]string, error) {
	var resp struct {
		Jobs []struct {
			ID string `json:"id"`
		} `json:"jobs"`
	}
	if err := c.GetJSON(ctx, ashbyBoardURL(slug), &resp); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(resp.Jobs))
	for _, j := range resp.Jobs {
		ids = append(ids, j.ID)
	}
	return ids, nil
}

func (ashbyProber) probe(ctx context.Context, c httpClient, slug string) (string, int, error) {
	var resp struct {
		Jobs []struct {
			ID string `json:"id"`
		} `json:"jobs"`
	}
	if err := c.GetJSON(ctx, ashbyBoardURL(slug), &resp); err != nil {
		return "", 0, nil
	}
	if len(resp.Jobs) == 0 {
		return "", 0, nil
	}
	return storefrontEmployer(ctx, c, fmt.Sprintf("https://jobs.ashbyhq.com/%s", slug), " Jobs"), len(resp.Jobs), nil
}

// ashbyBoardsHost is the host Common Crawl visits, unlike the api host probe validates against.
const ashbyBoardsHost = "jobs.ashbyhq.com"

// discover finds Ashby board candidates from the Common Crawl CDX index: every URL Common
// Crawl has seen under ashbyBoardsHost, sliced to its company slug.
func (ashbyProber) discover(ctx context.Context, c httpClient) ([]string, error) {
	return commonCrawlCandidates(ctx, c, ashbyBoardsHost+"/*", commonCrawlSlug)
}

// bamboohrProber probes the BambooHR per-subdomain careers list. A non-empty result is a
// live board; the name falls back to the slug (the subdomain), as the list carries none.
type bamboohrProber struct{}

func (bamboohrProber) probe(ctx context.Context, c httpClient, slug string) (string, int, error) {
	var list struct {
		Result []struct {
			ID string `json:"id"`
		} `json:"result"`
	}
	if err := c.GetJSON(ctx, fmt.Sprintf("https://%s.bamboohr.com/careers/list", slug), &list); err != nil {
		return "", 0, nil
	}
	if len(list.Result) == 0 {
		return "", 0, nil
	}
	return "", len(list.Result), nil
}

// workdayProber probes Workday's POST-only CXS listing. The board id is "<host>/<site>"; the tenant
// is the host's first label and doubles as the name fallback. The site path is case-insensitive.
type workdayProber struct{}

func (workdayProber) probe(ctx context.Context, c httpClient, boardID string) (string, int, error) {
	host, site, ok := strings.Cut(boardID, "/")
	if !ok || host == "" || site == "" {
		return "", 0, nil
	}
	tenant, _, ok := strings.Cut(host, ".")
	if !ok || tenant == "" {
		return "", 0, nil
	}
	url := fmt.Sprintf("https://%s/wday/cxs/%s/%s/jobs", host, tenant, site)
	body := map[string]any{"appliedFacets": map[string]any{}, "limit": 1, "offset": 0, "searchText": ""}
	var resp struct {
		Total       int `json:"total"`
		JobPostings []struct {
			Title string `json:"title"`
		} `json:"jobPostings"`
	}
	if err := c.PostJSON(ctx, url, body, &resp); err != nil {
		return "", 0, nil
	}
	n := resp.Total
	if n == 0 {
		n = len(resp.JobPostings)
	}
	if n == 0 {
		return "", 0, nil
	}
	return "", n, nil
}

// icimsProber judges liveness from the XML sitemap (no JSON API): a live board lists ≥1 /jobs/<id>/
// URL. A missing site 404s; an empty one lists only /jobs/search and /jobs/intro. The name is the slug.
type icimsProber struct{}

// icimsJobLocPattern matches the /jobs/<id>/ segment the adapter keys off; duplicated to avoid
// widening the sources API.
var icimsJobLocPattern = regexp.MustCompile(`/jobs/\d+/`)

// icimsHost mirrors internal/ingest/sources/icims.go: a board containing a dot IS the careers host,
// anything else is a bare slug. Rebuilding a vanity host would resolve nowhere and report it dead.
func icimsHost(board string) string {
	if strings.Contains(board, ".") {
		return board
	}
	return "careers-" + board + ".icims.com"
}

// dedupKey folds "careers-<slug>.icims.com" to "<slug>", since the catalogue holds both spellings.
// Only that form folds: "pppl-princeton.icims.com" and bare "pppl-princeton" are different hosts.
func (icimsProber) dedupKey(boardID string) string {
	id := strings.ToLower(boardID)
	if host, isHost := strings.CutSuffix(id, ".icims.com"); isHost {
		if slug, prefixed := strings.CutPrefix(host, "careers-"); prefixed && slug != "" {
			return slug
		}
	}
	return id
}

func (icimsProber) probe(ctx context.Context, c httpClient, slug string) (string, int, error) {
	var sitemap struct {
		URLs []struct {
			Loc string `xml:"loc"`
		} `xml:"url"`
	}
	if err := c.GetXML(ctx, "https://"+icimsHost(slug)+"/sitemap.xml", &sitemap); err != nil {
		return "", 0, nil
	}
	n := 0
	for _, u := range sitemap.URLs {
		if icimsJobLocPattern.MatchString(u.Loc) {
			n++
		}
	}
	if n == 0 {
		return "", 0, nil
	}
	return "", n, nil
}

// discoverer is the opt-in capability of a prober that enumerates its own candidate board ids;
// used when the tool runs without a seed file.
type discoverer interface {
	discover(ctx context.Context, c httpClient) ([]string, error)
}

// probeClienter is the opt-in capability of a prober whose platform refuses the default
// client the way it refuses the adapter's (an edge that filters on the User-Agent): its
// non-nil client replaces the provider's default egress for the whole run, pacing included.
type probeClienter interface {
	probeClient() *sources.Client
}

// seedMapper converts a provider's raw seed token into its canonical board id. Providers whose
// seed token already IS the board id do not implement it.
type seedMapper interface {
	boardID(seedToken string) string
}

// seedResolver is seedMapper for a platform whose seed token may name a POSTING rather than a
// board, so finding the board costs a request: AppOne's apply.appone.com/job/<id> carries no
// portal, which only the posting's own record names. It returns "" for a token that names no
// board; resolveCandidates drops those. A prober implements one of the two, never both.
type seedResolver interface {
	resolveSeed(ctx context.Context, c httpClient, seedToken string) (string, error)
}

// dedupKeyer folds a board id into the key used for dedup against existing boards. A
// provider whose board ids are case-insensitive (Workday, Ashby) implements it to fold case;
// the rest dedup case-sensitively, so they do not implement it.
type dedupKeyer interface {
	dedupKey(boardID string) string
}

// dedupKey folds case: Workday's CXS API is case-insensitive.
func (workdayProber) dedupKey(boardID string) string { return strings.ToLower(boardID) }

// boardID turns a "tenant|dc|site" seed token into "<tenant>.<dc>.myworkdayjobs.com/<site>".
// A token that is not exactly three non-empty parts is returned unchanged (probe drops it).
func (workdayProber) boardID(seedToken string) string {
	parts := strings.Split(seedToken, "|")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return seedToken
	}
	return fmt.Sprintf("%s.%s.myworkdayjobs.com/%s", parts[0], parts[1], parts[2])
}

// workableProber probes the Workable public widget API. A board is the account subdomain;
// the widget endpoint returns the account name and its open jobs, so a non-empty jobs list
// is a live board.
type workableProber struct{}

func (workableProber) probe(ctx context.Context, c httpClient, slug string) (string, int, error) {
	var resp struct {
		Name string `json:"name"`
		Jobs []struct {
			Shortcode string `json:"shortcode"`
		} `json:"jobs"`
	}
	if err := c.GetJSON(ctx, fmt.Sprintf("https://apply.workable.com/api/v1/widget/accounts/%s?details=true", slug), &resp); err != nil {
		return "", 0, nil
	}
	if len(resp.Jobs) == 0 {
		return "", 0, nil
	}
	return resp.Name, len(resp.Jobs), nil
}

// storefrontEmployer reads the employer name from a board's storefront page title, trimming the
// boilerplate suffix. Call it only for boards known to have jobs: one request per LIVE board.
// Anything unreadable yields "" (unreachable page, a title that is only the boilerplate, a 404), and
// the board is then kept on liveness, so the name gate cannot fire for it.
func storefrontEmployer(ctx context.Context, c httpClient, url, suffix string) string {
	root, err := c.GetHTML(ctx, url)
	if err != nil {
		return ""
	}
	title := strings.TrimSpace(pageTitle(root))
	if suffix != "" {
		trimmed, ok := strings.CutSuffix(title, suffix)
		if !ok {
			return ""
		}
		title = trimmed
	}
	return strings.TrimSpace(title)
}

// smartRecruitersProber probes the SmartRecruiters public postings API. The listing carries
// totalFound, so one limit=1 page settles liveness; the company name comes from the
// company-metadata endpoint, fetched only once a board is known to have jobs.
type smartRecruitersProber struct{}

func (smartRecruitersProber) probe(ctx context.Context, c httpClient, slug string) (string, int, error) {
	var page struct {
		TotalFound int `json:"totalFound"`
		Content    []struct {
			ID string `json:"id"`
		} `json:"content"`
	}
	if err := c.GetJSON(ctx, fmt.Sprintf("https://api.smartrecruiters.com/v1/companies/%s/postings?limit=1", slug), &page); err != nil {
		return "", 0, nil
	}
	n := page.TotalFound
	if n == 0 {
		n = len(page.Content)
	}
	if n == 0 {
		return "", 0, nil
	}
	var meta struct {
		Name string `json:"name"`
	}
	_ = c.GetJSON(ctx, fmt.Sprintf("https://api.smartrecruiters.com/v1/companies/%s", slug), &meta)
	return meta.Name, n, nil
}

// dedupKey folds case: SmartRecruiters' API treats "archirodongroup" and "ArchirodonGroup" alike, so
// a name-derived candidate differing only in case must not read as new.
func (smartRecruitersProber) dedupKey(boardID string) string { return strings.ToLower(boardID) }

// recruiteeProber probes the Recruitee per-subdomain offers API. A non-empty offers array is
// a live board, and each offer carries the employer's own name, so the board can be gated
// against the name the seed expected rather than accepted on liveness alone.
type recruiteeProber struct{}

func recruiteeOffersURL(slug string) string {
	return fmt.Sprintf("https://%s.recruitee.com/api/offers/", slug)
}

// postingIDs lists the live posting ids; one response carries the complete list.
func (recruiteeProber) postingIDs(ctx context.Context, c httpClient, slug string) ([]string, error) {
	var resp struct {
		Offers []struct {
			ID int64 `json:"id"`
		} `json:"offers"`
	}
	if err := c.GetJSON(ctx, recruiteeOffersURL(slug), &resp); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(resp.Offers))
	for _, o := range resp.Offers {
		ids = append(ids, strconv.FormatInt(o.ID, 10))
	}
	return ids, nil
}

func (recruiteeProber) probe(ctx context.Context, c httpClient, slug string) (string, int, error) {
	var resp struct {
		Offers []struct {
			ID          int64  `json:"id"`
			CompanyName string `json:"company_name"`
		} `json:"offers"`
	}
	if err := c.GetJSON(ctx, recruiteeOffersURL(slug), &resp); err != nil {
		return "", 0, nil
	}
	if len(resp.Offers) == 0 {
		return "", 0, nil
	}
	return resp.Offers[0].CompanyName, len(resp.Offers), nil
}

// pinpointProber probes the Pinpoint public postings API. A non-empty data array is a live
// board; the list carries no company name, so it falls back to the slug.
type pinpointProber struct{}

func (pinpointProber) probe(ctx context.Context, c httpClient, slug string) (string, int, error) {
	var resp struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := c.GetJSON(ctx, fmt.Sprintf("https://%s.pinpointhq.com/postings.json", slug), &resp); err != nil {
		return "", 0, nil
	}
	if len(resp.Data) == 0 {
		return "", 0, nil
	}
	return "", len(resp.Data), nil
}

// breezyProber probes the Breezy per-subdomain JSON list (a bare array of postings). A
// non-empty array is a live board, and each posting embeds the employer's own name, so the
// board can be gated against the name the seed expected rather than accepted on liveness
// alone.
type breezyProber struct{}

func (breezyProber) probe(ctx context.Context, c httpClient, slug string) (string, int, error) {
	var postings []struct {
		ID      string `json:"id"`
		Company struct {
			Name string `json:"name"`
		} `json:"company"`
	}
	if err := c.GetJSON(ctx, fmt.Sprintf("https://%s.breezy.hr/json", slug), &postings); err != nil {
		return "", 0, nil
	}
	if len(postings) == 0 {
		return "", 0, nil
	}
	return postings[0].Company.Name, len(postings), nil
}

// teamtailorProber probes a Teamtailor career site, whose board is the host itself (vendor
// sub-domain or the employer's own). There is no JSON feed, so liveness is judged like the adapter:
// a live board links ≥1 posting permalink. The employer is og:site_name, which every site sets to
// the company, so a name-derived seed is gated against it.
type teamtailorProber struct{}

// boardID turns a seed token into a host. A bare company name can only predict the vendor
// sub-domain; a token containing a dot already names a host and is kept.
func (teamtailorProber) boardID(token string) string {
	if strings.Contains(token, ".") {
		return token
	}
	return token + ".teamtailor.com"
}

// teamtailorJobPattern is the posting permalink /jobs/<numeric id>. It must not match the bare
// /jobs nav anchor, which every listing carries.
var teamtailorJobPattern = regexp.MustCompile(`/jobs/\d+`)

func (teamtailorProber) probe(ctx context.Context, c httpClient, host string) (string, int, error) {
	root, err := c.GetHTML(ctx, fmt.Sprintf("https://%s/jobs?page=1", host))
	if err != nil {
		return "", 0, nil
	}
	n := countMatchingLinks(root, teamtailorJobPattern)
	if n == 0 {
		return "", 0, nil
	}
	// A vendor sub-domain often redirects to the employer's own domain (tibber.teamtailor.com serves
	// jobs.tibber.com); kept, that is the SAME board under a second id and ingest would store every
	// posting twice. og:url names the host that served the page; a board is only this host if it says
	// so. The redirect target is logged in a fixed shape so a second run can probe it under its own id.
	if u, err := url.Parse(metaProperty(root, "og:url")); err == nil && u.Host != "" && !strings.EqualFold(u.Host, host) {
		log.Printf("harvest-boards: teamtailor %s serves %s", host, strings.ToLower(u.Host))
		return "", 0, nil
	}
	return metaProperty(root, "og:site_name"), n, nil
}

// trakstarProber probes the Trakstar Hire per-subdomain RSS feed. A non-empty channel of
// items is a live board, and the channel title names the employer, so the board can be gated
// against the name the seed expected instead of accepted on liveness alone.
type trakstarProber struct{}

func (trakstarProber) probe(ctx context.Context, c httpClient, slug string) (string, int, error) {
	var feed struct {
		Title string     `xml:"channel>title"`
		Items []struct{} `xml:"channel>item"`
	}
	if err := c.GetXML(ctx, fmt.Sprintf("https://%s.hire.trakstar.com/jobfeeds/%s", slug, slug), &feed); err != nil {
		return "", 0, nil
	}
	if len(feed.Items) == 0 {
		return "", 0, nil
	}
	return trakstarEmployer(feed.Title), len(feed.Items), nil
}

// trakstarEmployer pulls the employer from a feed title "Jobs at <Company>"; anything else yields "".
func trakstarEmployer(title string) string {
	name, ok := strings.CutPrefix(strings.TrimSpace(title), "Jobs at ")
	if !ok {
		return ""
	}
	return strings.TrimSpace(name)
}

// gethiredProber probes a GetHired tenant's RSS feed (<board>.easyapply.co/rss), the adapter's own
// request. A non-empty channel is a live board and its title is the employer. An unserved subdomain
// redirects to a sign-in page whose HTML fails the decode and is skipped.
type gethiredProber struct{}

func (gethiredProber) probe(ctx context.Context, c httpClient, slug string) (string, int, error) {
	var feed struct {
		Title string     `xml:"channel>title"`
		Items []struct{} `xml:"channel>item"`
	}
	if err := c.GetXML(ctx, fmt.Sprintf("https://%s.easyapply.co/rss", slug), &feed); err != nil {
		return "", 0, nil
	}
	if len(feed.Items) == 0 {
		return "", 0, nil
	}
	return strings.TrimSpace(feed.Title), len(feed.Items), nil
}

// taleobeProber probes a Taleo Business Edition career site ("<ORG>:<cws>") through the adapter's
// RSS request. A non-empty channel is a live board and its title names the employer. An unknown or
// deactivated org, or a missing career-site number, redirects to HTML and fails the decode.
type taleobeProber struct{}

func (taleobeProber) probe(ctx context.Context, c httpClient, board string) (string, int, error) {
	feedURL, err := sources.TaleoBEFeedURL(board)
	if err != nil {
		return "", 0, nil
	}
	var feed struct {
		Title string     `xml:"channel>title"`
		Items []struct{} `xml:"channel>item"`
	}
	if err := c.GetXML(ctx, feedURL, &feed); err != nil {
		return "", 0, nil
	}
	if len(feed.Items) == 0 {
		return "", 0, nil
	}
	return strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(feed.Title), " Job Feed")), len(feed.Items), nil
}

// personioProber lists a board like the adapter: the XML feed on the .com host, and search.json when
// the tenant's feed answers 404. An empty feed that answers is not live, since the adapter does not
// fall back from it either. A tenant served only on a regional TLD (.de/.es) is skipped, as the
// adapter could not crawl it. No company name, so it falls back to the slug.
type personioProber struct{}

func (personioProber) probe(ctx context.Context, c httpClient, slug string) (string, int, error) {
	host := fmt.Sprintf("https://%s.jobs.personio.com", slug)
	var resp struct {
		Positions []struct{} `xml:"position"`
	}
	err := c.GetXML(ctx, host+"/xml", &resp)
	if err == nil {
		return "", len(resp.Positions), nil
	}
	var se *sources.StatusError
	if !errors.As(err, &se) || se.Code != http.StatusNotFound {
		return "", 0, nil
	}
	var jobs []struct{}
	if err := c.GetJSON(ctx, host+"/search.json", &jobs); err != nil {
		return "", 0, nil
	}
	return "", len(jobs), nil
}

// freshteamJobPattern matches a job permalink's /jobs/<12-char-id> segment, anchored to a path
// boundary so /jobs and /jobs/search do not match.
var freshteamJobPattern = regexp.MustCompile(`/jobs/[A-Za-z0-9_-]{12}(?:[/?#]|$)`)

// freshteamProber probes a Freshteam career site. Its API is auth-gated, so liveness is judged from
// the listing HTML: a live board links ≥1 job permalink. No reliable company name; falls back to the slug.
type freshteamProber struct{}

func (freshteamProber) probe(ctx context.Context, c httpClient, slug string) (string, int, error) {
	root, err := c.GetHTML(ctx, fmt.Sprintf("https://%s.freshteam.com/jobs", slug))
	if err != nil {
		return "", 0, nil
	}
	n := countMatchingLinks(root, freshteamJobPattern)
	if n == 0 {
		return "", 0, nil
	}
	return "", n, nil
}

// countMatchingLinks counts <a href> values matching pat. A duplicate link inflating the count is
// harmless, since it is only used for liveness.
func countMatchingLinks(root *html.Node, pat *regexp.Regexp) int {
	n := 0
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode && node.Data == "a" {
			for _, a := range node.Attr {
				if a.Key == "href" && pat.MatchString(a.Val) {
					n++
					break
				}
			}
		}
		for ch := node.FirstChild; ch != nil; ch = ch.NextSibling {
			walk(ch)
		}
	}
	walk(root)
	return n
}

// joinProber probes a Join.com company by its numeric id (the slug path returns empty); a slug is
// resolved to its id during seed building. A non-zero rowCount is a live board.
type joinProber struct{}

func (joinProber) probe(ctx context.Context, c httpClient, id string) (string, int, error) {
	var jobs struct {
		Pagination struct {
			RowCount int `json:"rowCount"`
		} `json:"pagination"`
	}
	if err := c.GetJSON(ctx, fmt.Sprintf("https://join.com/api/public/companies/%s/jobs?page=1&pageSize=1", id), &jobs); err != nil {
		return "", 0, nil
	}
	if jobs.Pagination.RowCount == 0 {
		return "", 0, nil
	}
	var meta struct {
		Name string `json:"name"`
	}
	_ = c.GetJSON(ctx, fmt.Sprintf("https://join.com/api/public/companies/%s", id), &meta)
	return meta.Name, jobs.Pagination.RowCount, nil
}

// probers maps a provider key to its prober. Adding an ATS is one entry here plus the prober type.
var probers = map[string]prober{
	"adp":               adpProber{},
	"adpmyjobs":         adpMyJobsProber{},
	"greenhouse":        greenhouseProber{},
	"lever":             leverProber{},
	"ashby":             ashbyProber{},
	"bamboohr":          bamboohrProber{},
	"workday":           workdayProber{},
	"icims":             icimsProber{},
	"gupy":              gupyProber{},
	"pandape":           pandapeBrazilProber,
	"pandapelatam":      pandapeLatamProber,
	"workable":          workableProber{},
	"smartrecruiters":   smartRecruitersProber{},
	"recruitee":         recruiteeProber{},
	"pinpoint":          pinpointProber{},
	"breezy":            breezyProber{},
	"teamtailor":        teamtailorProber{},
	"trakstar":          trakstarProber{},
	"gethired":          gethiredProber{},
	"taleobe":           taleobeProber{},
	"personio":          personioProber{},
	"gem":               gemProber{},
	"deel":              deelProber{},
	"freshteam":         freshteamProber{},
	"jobvite":           jobviteProber{},
	"opencats":          opencatsProber{},
	"join":              joinProber{},
	"oracle":            oracleProber{},
	"jazzhr":            jazzhrProber{},
	"careerplug":        careerplugProber{},
	"applitrack":        applitrackProber{},
	"applicantstack":    applicantstackProber{},
	"hiringthing":       hiringthingProber{},
	"hrmdirect":         hrmdirectProber{},
	"idealtraits":       idealtraitsProber{},
	"hireclick":         hireclickProber{client: sources.NewLongTimeoutClient()},
	"paycom":            paycomProber{},
	"paycor":            paycorProber{},
	"tenstreet":         tenstreetProber{},
	"traffit":           traffitProber{},
	"erecruiter":        erecruiterProber{},
	"recruitify":        recruitifyProber{},
	"isolvedhire":       isolvedProber{host: "isolvedhire.com"},
	"applicantpro":      isolvedProber{host: "applicantpro.com"},
	"apploi":            apploiProber{},
	"paylocity":         paylocityProber{},
	"hireology":         hireologyProber{},
	"higherme":          highermeProber{},
	"pageup":            pageupProber{},
	"manatal":           manatalProber{},
	"ukgready":          ukgreadyProber{},
	"edjoin":            edjoinProber{},
	"workstream":        workstreamProber{},
	"trinethire":        trinethireProber{},
	"gohire":            gohireProber{},
	"harri":             harriProber{},
	"hcshiring":         hcshiringProber{},
	"appone":            apponeProber{},
	"onlyfy":            onlyfyProber{},
	"paradox":           paradoxProber{},
	"phenom":            phenomProber{},
	"polymer":           polymerProber{},
	"flatchr":           flatchrProber{},
	"dvinci":            dvinciProber{},
	"rexx":              rexxProber{},
	"coveto":            covetoProber{},
	"concludis":         concludisProber{},
	"kenjo":             kenjoProber{},
	"homerun":           homerunProber{},
	"easycruit":         easycruitProber{},
	"hroffice":          hrofficeProber{},
	"cvwarehouse":       cvwarehouseProber{},
	"bizneo":            bizneoProber{},
	"velorahr":          velorahrProber{},
	"talentsoft":        talentsoftProber{},
	"digitalrecruiters": digitalrecruitersProber{},
	"keka":              kekaProber{},
	"hirehive":          hirehiveProber{},
	"beehire":           beehireProber{},
	"inploi":            inploiProber{},
	"seamlesshiring":    seamlesshiringProber{},
	"darwinbox":         darwinboxProber,
	"zohorecruit":       zohoRecruitProber,
	"pyjamahr":          pyjamahrProber,
	"greythr":           greythrProber{},
	"meincheckin":       meincheckinProber{},
	"helixjobs":         helixjobsProber{},
	"talentstorm":       talentstormProber{},
	// Not an ATS: its boards are the platform's own job categories, enumerated from its
	// sitemap index rather than seeded. See profession_prober.go.
	"profession":  newProfessionProber(),
	"cornerstone": adapterProber{provider: "cornerstone", newSource: func() sources.Source { return sources.NewCornerstone(sources.NewClient()) }},
	"taleo":       adapterProber{provider: "taleo", newSource: func() sources.Source { return sources.NewTaleo(sources.NewCookieClient()) }},
	"neogov":      adapterProber{provider: "neogov", newSource: func() sources.Source { return sources.NewNeogov(sources.NewClient()) }},
}
