package main

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/strelov1/freehire/internal/sources"
	"golang.org/x/net/html"
)

// greenhouseBoardsAPI is the public boards API root (mirrors sources.greenhouseBaseURL,
// which is unexported; this tool lives outside the sources package).
const greenhouseBoardsAPI = "https://boards-api.greenhouse.io/v1/boards"

// httpClient is the transport a prober needs: most platforms list over GetJSON, Workday's
// CXS listing is POST-only (PostJSON), iCIMS/Deel read an XML sitemap (GetXML), and
// Freshteam has no API so its prober reads the listing HTML (GetHTML). The real
// *sources.Client implements all four.
type httpClient interface {
	sources.JSONGetter
	sources.JSONPoster
	sources.XMLGetter
	sources.HTMLGetter
	sources.TextGetter
	sources.HeaderJSONGetter
	sources.HeaderJSONPoster
}

// prober checks one candidate board on its ATS platform, returning the company name the
// platform reports and the number of open jobs. A board that is absent, closed, or
// unreachable yields ("", 0, nil) — a skip, never a fatal error — so one dead candidate
// cannot abort the harvest. A non-nil error is reserved for failures a prober genuinely
// wants surfaced (the caller logs and skips those too).
type prober interface {
	probe(ctx context.Context, c httpClient, slug string) (company string, openJobs int, err error)
}

// idProber is the optional half of the prober contract: it reports the ids of a board's live
// postings, in the platform's own id space, so a seed that knows one posting's id can identify
// the board by evidence instead of by a name resemblance.
//
// It is optional because the check reads a missing id as "wrong board", which is only sound
// where one request yields the COMPLETE live list. A prober that pages, or that asks for a
// single posting to settle liveness, must not implement it — its silence leaves the expected
// id inert, which is the safe reading of partial evidence.
//
// This re-requests the listing probe already fetched, rather than widening prober's return to
// carry ids through the ~35 implementations that have no use for them. The extra request is
// only spent on the seeds that name a posting — the offline-derived candidates, which are a
// minority and which produce no board at all without it.
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

// postingIDs lists the board's live posting ids. The jobs endpoint returns every live posting
// in one response, so an id it does not carry is genuinely not on the board.
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
	// A missing/moved board returns 4xx and the client surfaces it as an error. For harvest
	// that simply means "not a live greenhouse board" — skip silently, do not propagate.
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

// greenhouseBoardsHost is the web host Greenhouse boards are served under, the surface
// Common Crawl's crawler actually visits (unlike greenhouseBoardsAPI, which the crawler
// never touches — it's an API, not a page). Validation still goes through
// greenhouseBoardsAPI in probe/postingIDs; a discovered candidate's redirect from this host
// to Greenhouse's newer "job-boards.greenhouse.io" frontend (visible in raw Common Crawl
// records) doesn't affect that.
const greenhouseBoardsHost = "boards.greenhouse.io"

// discover finds Greenhouse board candidates from the Common Crawl CDX index: every URL
// Common Crawl has seen under greenhouseBoardsHost, sliced to its company slug.
func (greenhouseProber) discover(ctx context.Context, c httpClient) ([]string, error) {
	return commonCrawlCandidates(ctx, c, greenhouseBoardsHost)
}

// dedupKey folds a Greenhouse board id to lower case, like Workday's and Ashby's. The boards
// API is case-insensitive (boards-api.greenhouse.io/v1/boards/adyen and .../Adyen both
// resolve to the same company, confirmed live) — without folding, Common Crawl discovery
// surfacing a different case than an already-known board dedups as a distinct one.
func (greenhouseProber) dedupKey(boardID string) string { return strings.ToLower(boardID) }

// leverProber probes the Lever postings API. The JSON-mode endpoint returns a bare array
// of live postings, so a non-empty array is a live board. The posting API exposes no company
// name, but the board's storefront titles itself after the employer, so the name is read
// there once the board is known to be live.
type leverProber struct{}

func leverPostingsURL(slug string) string {
	return fmt.Sprintf("https://api.lever.co/v0/postings/%s?mode=json", slug)
}

// postingIDs lists the board's live posting ids. The JSON-mode endpoint returns every live
// posting in one array, so an id it does not carry is genuinely not on the board.
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

// ashbyProber probes the Ashby public job-board API. The list endpoint returns the live
// postings, so a non-empty list is a live board. The posting API exposes no company name, but
// the storefront titles itself "<Company> Jobs", so the name is read there once the board is
// known to be live. The job-board API is case-insensitive (posting-api/job-board/abridge and
// .../Abridge both resolve to the same company, confirmed live) — see dedupKey.
type ashbyProber struct{}

// dedupKey folds an Ashby board id to lower case, like Workday's. Common Crawl discovery
// surfaces the same company under whatever case a crawled inbound link happened to use, and
// without folding, "abridge" and "Abridge" dedup as two different boards even though the API
// serves identical content for both — verified live, and by a stale duplicate this folding
// removes from sources/ashby.yml (see the CDX-discovery change that added it).
func (ashbyProber) dedupKey(boardID string) string { return strings.ToLower(boardID) }

func ashbyBoardURL(slug string) string {
	return fmt.Sprintf("https://api.ashbyhq.com/posting-api/job-board/%s", slug)
}

// postingIDs lists the board's live posting ids. The list endpoint returns every live posting
// in one response, so an id it does not carry is genuinely not on the board.
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

// ashbyBoardsHost is the web host Ashby boards are served under, the surface Common Crawl's
// crawler actually visits (unlike the api.ashbyhq.com host probe/postingIDs validate
// against).
const ashbyBoardsHost = "jobs.ashbyhq.com"

// discover finds Ashby board candidates from the Common Crawl CDX index: every URL Common
// Crawl has seen under ashbyBoardsHost, sliced to its company slug.
func (ashbyProber) discover(ctx context.Context, c httpClient) ([]string, error) {
	return commonCrawlCandidates(ctx, c, ashbyBoardsHost)
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

// workdayProber probes Workday's public CXS listing (POST-only). The board id is
// "<host>/<site>" (e.g. "aig.wd1.myworkdayjobs.com/early_careers"); the tenant is the
// host's first dot-label. The listing carries no company name, so it falls back to the
// tenant (slug-fallback doctrine). The CXS site path is case-insensitive, so the seed's
// lowercased sites work unchanged.
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

// icimsProber probes an iCIMS career site by its slug. iCIMS exposes no JSON list API, so
// liveness is judged from the site's XML sitemap: a live board lists ≥1 job-posting URL
// (a /jobs/<id>/ entry). This rejects both a missing site (404 → getter error) and a
// present-but-empty one (200 with only the non-posting /jobs/search and /jobs/intro
// entries). The sitemap carries no company name, so the name falls back to the slug.
type icimsProber struct{}

// icimsJobLocPattern matches an iCIMS job-posting URL's /jobs/<id>/ segment, the same shape
// the adapter keys off. It is duplicated here (a small literal) rather than exported from
// internal/sources, to avoid widening that package's API for a dev tool.
var icimsJobLocPattern = regexp.MustCompile(`/jobs/\d+/`)

func (icimsProber) probe(ctx context.Context, c httpClient, slug string) (string, int, error) {
	var sitemap struct {
		URLs []struct {
			Loc string `xml:"loc"`
		} `xml:"url"`
	}
	if err := c.GetXML(ctx, fmt.Sprintf("https://careers-%s.icims.com/sitemap.xml", slug), &sitemap); err != nil {
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

// discoverer is the opt-in capability of a prober whose boards are not available as a seed
// list: it enumerates its own candidate board ids from the platform API. When a provider's
// prober implements it and the tool is run with no seed file, discovery supplies the
// candidates that a seed would otherwise provide. Mirrors the optional-marker idiom of
// seedMapper/dedupKeyer.
type discoverer interface {
	discover(ctx context.Context, c httpClient) ([]string, error)
}

// seedMapper converts a provider's raw seed token into its canonical board id. Providers
// whose seed token already IS the board id (greenhouse/lever/ashby/bamboohr/icims) do not
// implement it. Mirrors the optional-marker idiom of sources.boardless.
type seedMapper interface {
	boardID(seedToken string) string
}

// dedupKeyer folds a board id into the key used for dedup against existing boards. A
// provider whose board ids are case-insensitive (Workday, Ashby) implements it to fold case;
// the rest dedup case-sensitively, so they do not implement it.
type dedupKeyer interface {
	dedupKey(boardID string) string
}

// dedupKey folds a Workday board id to lower case: Workday's CXS API is case-insensitive,
// so "acme.wd1.myworkdayjobs.com/Careers" and ".../careers" are the same board.
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

// storefrontEmployer reads the employer name from a board's public storefront page title,
// trimming the platform's boilerplate suffix. It is called only once a board is known to have
// jobs, so it costs one request per LIVE board rather than one per candidate — the same
// discipline greenhouse's board-metadata fetch follows.
//
// Anything unreadable yields "": an unreachable page, a title that is only the boilerplate
// (an unknown ashby slug still serves a 200 titled just "Jobs"), or a platform 404. A board
// whose name cannot be read is kept on liveness and labelled from the seed, exactly as before
// — the gate simply cannot fire for it.
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

// dedupKey folds a SmartRecruiters board id to lower case, like Workday's, Greenhouse's and
// Ashby's. Without this, a name-derived candidate that only differs in case from an existing
// entry (e.g. "archirodongroup" vs the stored "ArchirodonGroup") reads as new — confirmed live
// twice (see #1884, #1888) — even though SmartRecruiters' API treats the two identically.
func (smartRecruitersProber) dedupKey(boardID string) string { return strings.ToLower(boardID) }

// recruiteeProber probes the Recruitee per-subdomain offers API. A non-empty offers array is
// a live board, and each offer carries the employer's own name, so the board can be gated
// against the name the seed expected rather than accepted on liveness alone.
type recruiteeProber struct{}

func recruiteeOffersURL(slug string) string {
	return fmt.Sprintf("https://%s.recruitee.com/api/offers/", slug)
}

// postingIDs lists the board's live posting ids. The offers endpoint returns every live offer
// in one response, so an id it does not carry is genuinely not on the board.
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

// teamtailorProber probes a Teamtailor career site. The board is the site host, and its
// /jobs listing serves a JSON Feed (title + items) to a JSON Accept header, so a non-empty
// items array is a live board and the feed title is the company name.
// teamtailorProber probes a Teamtailor career site, whose board is the host itself (the vendor
// sub-domain `acme.teamtailor.com` or the employer's own `careers.acme.com`). The platform
// serves its listing as HTML on both — there is no JSON feed — so liveness is judged the way
// the ingest adapter reads the same page: a live board links at least one posting permalink.
// The listing carries no company name worth trusting, so it falls back to the slug.
type teamtailorProber struct{}

// teamtailorJobPattern is the posting permalink on a Teamtailor listing: /jobs/<numeric id>.
// It must not match the bare /jobs nav anchor, which every listing carries whether or not it
// has postings.
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
	return "", n, nil
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

// trakstarEmployer pulls the employer out of a Trakstar feed title, which renders as
// "Jobs at <Company>". A title without that prefix names nobody, so it yields "".
func trakstarEmployer(title string) string {
	name, ok := strings.CutPrefix(strings.TrimSpace(title), "Jobs at ")
	if !ok {
		return ""
	}
	return strings.TrimSpace(name)
}

// personioProber probes the Personio public XML feed on the .com host (the host the adapter
// crawls). A non-empty positions list is a live board; a tenant served only on a regional
// TLD (.de/.es) 404s here and is skipped — correctly, since the adapter could not crawl it
// either. The feed carries no company name, so it falls back to the slug.
type personioProber struct{}

func (personioProber) probe(ctx context.Context, c httpClient, slug string) (string, int, error) {
	var resp struct {
		Positions []struct{} `xml:"position"`
	}
	if err := c.GetXML(ctx, fmt.Sprintf("https://%s.jobs.personio.com/xml", slug), &resp); err != nil {
		return "", 0, nil
	}
	if len(resp.Positions) == 0 {
		return "", 0, nil
	}
	return "", len(resp.Positions), nil
}

// gemProbeQuery is the minimal list query (extId only) the Gem prober uses for liveness — a
// trimmed sibling of the adapter's gemListQuery.
const gemProbeQuery = `query JobBoardList($boardId: String!) { oatsExternalJobPostings(boardId: $boardId) { jobPostings { extId } } }`

// gemProber probes the Gem public job-board GraphQL API. A non-empty jobPostings list is a
// live board; the query exposes no company name, so it falls back to the slug.
type gemProber struct{}

func (gemProber) probe(ctx context.Context, c httpClient, slug string) (string, int, error) {
	body := map[string]any{
		"operationName": "JobBoardList",
		"query":         gemProbeQuery,
		"variables":     map[string]any{"boardId": slug},
	}
	var resp struct {
		Data struct {
			Postings struct {
				JobPostings []struct {
					ExtID string `json:"extId"`
				} `json:"jobPostings"`
			} `json:"oatsExternalJobPostings"`
		} `json:"data"`
	}
	if err := c.PostJSON(ctx, "https://jobs.gem.com/api/public/graphql", body, &resp); err != nil {
		return "", 0, nil
	}
	n := len(resp.Data.Postings.JobPostings)
	if n == 0 {
		return "", 0, nil
	}
	return "", n, nil
}

// deelJobLocPattern matches a Deel sitemap entry that is a job-detail page (the same shape
// the adapter keys off), distinguishing a live board from the board's empty shell sitemap.
var deelJobLocPattern = regexp.MustCompile(`/job-details/`)

// deelProber probes a Deel ATS tenant by its per-board sitemap (the adapter's own tenant
// validation): a sitemap listing ≥1 /job-details/ URL is a live board. The sitemap carries
// no company name, so it falls back to the slug.
type deelProber struct{}

func (deelProber) probe(ctx context.Context, c httpClient, slug string) (string, int, error) {
	var sitemap struct {
		URLs []struct {
			Loc string `xml:"loc"`
		} `xml:"url"`
	}
	if err := c.GetXML(ctx, fmt.Sprintf("https://jobs.deel.com/%s/sitemap.xml", slug), &sitemap); err != nil {
		return "", 0, nil
	}
	n := 0
	for _, u := range sitemap.URLs {
		if deelJobLocPattern.MatchString(u.Loc) {
			n++
		}
	}
	if n == 0 {
		return "", 0, nil
	}
	return "", n, nil
}

// freshteamJobPattern matches a Freshteam job permalink's /jobs/<12-char-id> segment, the
// same id shape the adapter keys off. It anchors to a path boundary so non-job paths
// (/jobs, /jobs/search) do not match.
var freshteamJobPattern = regexp.MustCompile(`/jobs/[A-Za-z0-9_-]{12}(?:[/?#]|$)`)

// freshteamProber probes a Freshteam career site. Freshteam exposes no public JSON list
// (its API is auth-gated), so liveness is judged from the listing HTML: a live board links
// ≥1 job permalink. The page carries no reliable company name, so it falls back to the slug.
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

// countMatchingLinks counts the <a href> values in the tree whose value matches pat. It is
// the prober's own minimal anchor walk (the sources package's link helpers are unexported),
// used only for liveness, so a duplicate link inflating the count is harmless.
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

// joinProber probes a Join.com company by its numeric company id (the board id the adapter
// stores — the slug path returns empty). A non-zero rowCount is a live board; the company
// name comes from the company endpoint, fetched only once the board is known to have jobs.
// The seed is numeric ids: a slug is resolved to its id during seed building, not here.
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

// probers maps a provider key to its prober. Adding an ATS is one entry here plus the
// prober type — the same shape as sources.All.
var probers = map[string]prober{
	"greenhouse":      greenhouseProber{},
	"lever":           leverProber{},
	"ashby":           ashbyProber{},
	"bamboohr":        bamboohrProber{},
	"workday":         workdayProber{},
	"icims":           icimsProber{},
	"gupy":            gupyProber{},
	"workable":        workableProber{},
	"smartrecruiters": smartRecruitersProber{},
	"recruitee":       recruiteeProber{},
	"pinpoint":        pinpointProber{},
	"breezy":          breezyProber{},
	"teamtailor":      teamtailorProber{},
	"trakstar":        trakstarProber{},
	"personio":        personioProber{},
	"gem":             gemProber{},
	"deel":            deelProber{},
	"freshteam":       freshteamProber{},
	"jobvite":         jobviteProber{},
	"opencats":        opencatsProber{},
	"join":            joinProber{},
	"oracle":          oracleProber{},
	"jazzhr":          jazzhrProber{},
	"careerplug":      careerplugProber{},
	"paycom":          paycomProber{},
	"traffit":         traffitProber{},
	"isolvedhire":     isolvedProber{host: "isolvedhire.com"},
	"applicantpro":    isolvedProber{host: "applicantpro.com"},
	"apploi":          apploiProber{},
	"paylocity":       paylocityProber{},
	"hireology":       hireologyProber{},
	"pageup":          pageupProber{},
	"cornerstone":     adapterProber{provider: "cornerstone", newSource: func() sources.Source { return sources.NewCornerstone(sources.NewClient()) }},
	"taleo":           adapterProber{provider: "taleo", newSource: func() sources.Source { return sources.NewTaleo(sources.NewCookieClient()) }},
	"neogov":          adapterProber{provider: "neogov", newSource: func() sources.Source { return sources.NewNeogov(sources.NewClient()) }},
}

// hireologyProber probes a careers.hireology.com tenant (slug = board) via the public
// api.hireology.com/v1/careers/<slug> JSON:API; a non-zero count of Open postings means a
// live board. The employer name comes from the seed.
type hireologyProber struct{}

func (hireologyProber) probe(ctx context.Context, c httpClient, slug string) (string, int, error) {
	var resp struct {
		Data []struct {
			Attributes struct {
				Status string `json:"status"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if err := c.GetJSON(ctx, fmt.Sprintf("https://api.hireology.com/v1/careers/%s", slug), &resp); err != nil {
		return "", 0, nil
	}
	open := 0
	for _, d := range resp.Data {
		if strings.EqualFold(d.Attributes.Status, "Open") {
			open++
		}
	}
	return "", open, nil
}

// isolvedSitemapJobID captures the numeric posting id from a /jobs/<id> URL in an iSolved
// Hire / ApplicantPro sitemap.
var isolvedSitemapJobID = regexp.MustCompile(`/jobs/(\d+)`)

// isolvedProber probes an iSolved Hire / ApplicantPro tenant (slug = subdomain) by counting
// the distinct /jobs/<id> URLs in its sitemap.xml; a non-zero count is a live board. Host
// distinguishes the two sibling platforms. The employer name comes from the seed.
type isolvedProber struct{ host string }

func (p isolvedProber) probe(ctx context.Context, c httpClient, slug string) (string, int, error) {
	body, err := c.GetText(ctx, fmt.Sprintf("https://%s.%s/sitemap.xml", slug, p.host))
	if err != nil {
		return "", 0, nil
	}
	ids := map[string]struct{}{}
	for _, m := range isolvedSitemapJobID.FindAllStringSubmatch(body, -1) {
		ids[m[1]] = struct{}{}
	}
	return "", len(ids), nil
}

// apploiProber probes an apploi employer board (slug = numeric employer id) via the public
// api.apploi.com/v1/jobs?employer=<id> list; a non-zero count of live (published, non-archived)
// postings means a live board. The employer name comes from the seed.
type apploiProber struct{}

// apploiNumericSlugRE matches apploi's own employer-id shape: digits only. A name-derived
// candidate (e.g. "101-edu") is never a valid employer id, but api.apploi.com doesn't error on
// one — it silently ignores an `employer` value it can't parse and answers with a non-empty,
// "live" jobs list regardless. Fed a 5,216-candidate name-derived seed, that read as a 100%
// hit rate (confirmed live — see #1884). Reject non-numeric slugs up front rather than trust
// the API to say no.
var apploiNumericSlugRE = regexp.MustCompile(`^\d+$`)

func (apploiProber) probe(ctx context.Context, c httpClient, slug string) (string, int, error) {
	if !apploiNumericSlugRE.MatchString(slug) {
		return "", 0, nil
	}
	var resp struct {
		Data []struct {
			Published bool `json:"published"`
			Archived  bool `json:"archived"`
		} `json:"data"`
	}
	if err := c.GetJSON(ctx, fmt.Sprintf("https://api.apploi.com/v1/jobs?employer=%s&limit=100", slug), &resp); err != nil {
		return "", 0, nil
	}
	live := 0
	for _, j := range resp.Data {
		if j.Published && !j.Archived {
			live++
		}
	}
	return "", live, nil
}

// paylocityProber probes a recruiting.paylocity.com company board (slug = company GUID). The
// listing page embeds the openings in window.pageData's Jobs[] array; counting the JobId keys
// is enough to tell a live board (>=1 job) from an empty/dead one without a full parse. The
// employer name comes from the seed (the listing exposes none the prober bothers to read).
type paylocityProber struct{}

func (paylocityProber) probe(ctx context.Context, c httpClient, slug string) (string, int, error) {
	body, err := c.GetText(ctx, fmt.Sprintf("https://recruiting.paylocity.com/Recruiting/Jobs/All/%s", slug))
	if err != nil {
		return "", 0, nil
	}
	return "", strings.Count(body, `"JobId":`), nil
}
