package main

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/strelov1/freehire/internal/sources"
	"golang.org/x/net/html"
)

// errMissing is the sentinel a test getter returns for an unmapped URL. In production the
// real client returns its own transport error for a missing board, treated identically.
var errMissing = errors.New("not found")

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

// greenhouseProber probes the Greenhouse public boards API. The jobs endpoint lists only
// live postings, so a non-empty list means a live board. The company name comes from the
// board-metadata endpoint, fetched only once a board is known to have jobs.
type greenhouseProber struct{}

func (greenhouseProber) probe(ctx context.Context, c httpClient, slug string) (string, int, error) {
	var jr struct {
		Jobs []struct {
			ID int64 `json:"id"`
		} `json:"jobs"`
	}
	// A missing/moved board returns 4xx and the client surfaces it as an error. For harvest
	// that simply means "not a live greenhouse board" — skip silently, do not propagate.
	if err := c.GetJSON(ctx, fmt.Sprintf("%s/%s/jobs", greenhouseBoardsAPI, slug), &jr); err != nil {
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

// leverProber probes the Lever postings API. The JSON-mode endpoint returns a bare array
// of live postings, so a non-empty array is a live board. Lever exposes no company name, so
// the name falls back to the slug.
type leverProber struct{}

func (leverProber) probe(ctx context.Context, c httpClient, slug string) (string, int, error) {
	var postings []struct {
		ID string `json:"id"`
	}
	if err := c.GetJSON(ctx, fmt.Sprintf("https://api.lever.co/v0/postings/%s?mode=json", slug), &postings); err != nil {
		return "", 0, nil
	}
	if len(postings) == 0 {
		return "", 0, nil
	}
	return slug, len(postings), nil
}

// ashbyProber probes the Ashby public job-board API. The list endpoint returns the live
// postings, so a non-empty list is a live board; the name falls back to the (case-sensitive)
// slug, which Ashby itself uses as the board identity.
type ashbyProber struct{}

func (ashbyProber) probe(ctx context.Context, c httpClient, slug string) (string, int, error) {
	var resp struct {
		Jobs []struct {
			ID string `json:"id"`
		} `json:"jobs"`
	}
	if err := c.GetJSON(ctx, fmt.Sprintf("https://api.ashbyhq.com/posting-api/job-board/%s", slug), &resp); err != nil {
		return "", 0, nil
	}
	if len(resp.Jobs) == 0 {
		return "", 0, nil
	}
	return slug, len(resp.Jobs), nil
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
	return slug, len(list.Result), nil
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
	return tenant, n, nil
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
	return slug, n, nil
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
// provider whose board ids are case-insensitive (Workday) implements it to fold case; the
// rest dedup case-sensitively (Ashby slugs differ by case), so they do not implement it.
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

// orSlug applies the slug-fallback doctrine the API probers share: the platform-reported
// company name when present, else the slug (board id) itself.
func orSlug(name, slug string) string {
	if name != "" {
		return name
	}
	return slug
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
	return orSlug(resp.Name, slug), len(resp.Jobs), nil
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
	return orSlug(meta.Name, slug), n, nil
}

// recruiteeProber probes the Recruitee per-subdomain offers API. A non-empty offers array
// is a live board; the list carries no company name, so it falls back to the slug.
type recruiteeProber struct{}

func (recruiteeProber) probe(ctx context.Context, c httpClient, slug string) (string, int, error) {
	var resp struct {
		Offers []struct {
			ID int64 `json:"id"`
		} `json:"offers"`
	}
	if err := c.GetJSON(ctx, fmt.Sprintf("https://%s.recruitee.com/api/offers/", slug), &resp); err != nil {
		return "", 0, nil
	}
	if len(resp.Offers) == 0 {
		return "", 0, nil
	}
	return slug, len(resp.Offers), nil
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
	return slug, len(resp.Data), nil
}

// breezyProber probes the Breezy per-subdomain JSON list (a bare array of postings). A
// non-empty array is a live board; the list carries no company name, so it falls back to
// the slug.
type breezyProber struct{}

func (breezyProber) probe(ctx context.Context, c httpClient, slug string) (string, int, error) {
	var postings []struct {
		ID string `json:"id"`
	}
	if err := c.GetJSON(ctx, fmt.Sprintf("https://%s.breezy.hr/json", slug), &postings); err != nil {
		return "", 0, nil
	}
	if len(postings) == 0 {
		return "", 0, nil
	}
	return slug, len(postings), nil
}

// teamtailorProber probes a Teamtailor career site. The board is the site host, and its
// /jobs listing serves a JSON Feed (title + items) to a JSON Accept header, so a non-empty
// items array is a live board and the feed title is the company name.
type teamtailorProber struct{}

func (teamtailorProber) probe(ctx context.Context, c httpClient, host string) (string, int, error) {
	var feed struct {
		Title string `json:"title"`
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if err := c.GetJSON(ctx, fmt.Sprintf("https://%s/jobs?page=1", host), &feed); err != nil {
		return "", 0, nil
	}
	if len(feed.Items) == 0 {
		return "", 0, nil
	}
	return orSlug(feed.Title, host), len(feed.Items), nil
}

// trakstarProber probes the Trakstar Hire per-subdomain RSS feed. A non-empty channel of
// items is a live board; the feed carries no company name, so it falls back to the slug.
type trakstarProber struct{}

func (trakstarProber) probe(ctx context.Context, c httpClient, slug string) (string, int, error) {
	var feed struct {
		Items []struct{} `xml:"channel>item"`
	}
	if err := c.GetXML(ctx, fmt.Sprintf("https://%s.hire.trakstar.com/jobfeeds/%s", slug, slug), &feed); err != nil {
		return "", 0, nil
	}
	if len(feed.Items) == 0 {
		return "", 0, nil
	}
	return slug, len(feed.Items), nil
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
	return slug, len(resp.Positions), nil
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
	return slug, n, nil
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
	return slug, n, nil
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
	return slug, n, nil
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
	return orSlug(meta.Name, id), jobs.Pagination.RowCount, nil
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

func (apploiProber) probe(ctx context.Context, c httpClient, slug string) (string, int, error) {
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
