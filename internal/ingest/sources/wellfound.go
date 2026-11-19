package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"golang.org/x/net/html"
)

// wellfound adapts Wellfound (formerly AngelList Talent), a large startup-marketplace job board.
// The board is a role-taxonomy slug (e.g. "software-engineer", crawled as
// https://wellfound.com/role/r/<slug>) rather than a per-tenant ATS board: the same
// "board = a facet the site itself defines" shape hh (professional_role) and schoolspring
// (keyword) use. The unscoped marketplace is mostly non-technical, and Wellfound's role taxonomy
// already picks a technical slice.
//
// Wellfound's listing pages answer 403 to a plain request, direct or proxied; the Scrapling
// tier's impersonating fetcher is served, so in production this adapter is wired through
// scraplingProviders (scraplingtier.go) like bayt/gulftalent. Each page is server-rendered
// by Next.js and embeds a `<script id="__NEXT_DATA__">` tag whose JSON body is
// props.pageProps.apolloState.data — a normalized Apollo GraphQL cache. A JobListingSearchResult
// entry carries every scalar field this adapter needs directly EXCEPT the hiring company, which
// is a reverse-only reference: the entry itself names no startup at all, and the link exists only
// as StartupResult.highlightedJobListings[].__ref, an array the STARTUP holds pointing at its
// jobs. parseWellfoundPage therefore builds that reverse index before mapping entries.
//
// The listing already carries each posting's full HTML description, so this is NOT a
// HydratingSource. Pagination trusts the payload's own stated pageCount rather than treating an
// empty page as the only proof pagination has ended (as Dayforce/UKG Ready do with a vendor-stated
// count, as opposed to SEEK's untrustworthy totalCount).
type wellfound struct {
	http HTMLGetter
}

// NewWellfound builds the Wellfound adapter over the given HTTP client (the Scrapling tier's
// client in production).
func NewWellfound(c HTMLGetter) Source { return wellfound{http: c} }

func (wellfound) Provider() string { return "wellfound" }

// Each posting names its own hiring startup (resolved via the reverse index below), matching
// bayt/gulftalent's shape: a board selects a slice of a shared, multi-employer catalogue.
func (wellfound) Aggregator() {}

const (
	// wellfoundRoleSearchURL is a role-taxonomy search page, paginated by ?page=N — confirmed
	// live to accept the same query parameter on both page 1 and later pages.
	wellfoundRoleSearchURL = "https://wellfound.com/role/r/%s?page=%d"
	// wellfoundJobURL is a posting's own public page. The slug is cosmetic (the page resolves
	// off the numeric id), but it is included because that is the real, canonical URL form.
	wellfoundJobURL = "https://wellfound.com/jobs/%s-%s"
	// wellfoundMaxPages bounds pagination so a payload that never reports a sane pageCount (or
	// reports one that keeps growing) cannot loop unboundedly. Comfortably above any role
	// slice's real depth.
	wellfoundMaxPages = 500
)

// wellfoundPageURL builds a role slice's page-N URL.
func wellfoundPageURL(role string, page int) string {
	return fmt.Sprintf(wellfoundRoleSearchURL, role, page)
}

// wellfoundJobEntry is one JobListingSearchResult entry from the normalized Apollo cache. It
// carries no field naming its hiring company at all — see the package doc comment.
type wellfoundJobEntry struct {
	ID            string   `json:"id"`
	Slug          string   `json:"slug"`
	Title         string   `json:"title"`
	Description   string   `json:"description"`
	Compensation  string   `json:"compensation"`
	Remote        bool     `json:"remote"`
	LocationNames []string `json:"locationNames"`
}

// wellfoundStartupEntry is one StartupResult entry. HighlightedJobListings is the ONLY place a
// job's hiring company is stated in the normalized cache: the reference runs from the startup to
// its jobs, never the other way.
type wellfoundStartupEntry struct {
	Name                   string         `json:"name"`
	HighlightedJobListings []wellfoundRef `json:"highlightedJobListings"`
}

// wellfoundRef is a normalized Apollo cache reference, e.g. {"__ref": "JobListingSearchResult:100"}.
type wellfoundRef struct {
	Ref string `json:"__ref"`
}

// wellfoundSearchTotals is the "seoLandingPageJobSearchResults(...)" field's value, nested one
// level under ROOT_QUERY.talent. Only PageCount drives pagination; TotalJobCount is decoded
// alongside it (same object, no extra cost) but not otherwise consumed yet.
type wellfoundSearchTotals struct {
	PageCount     int `json:"pageCount"`
	TotalJobCount int `json:"totalJobCount"`
}

// wellfoundResolvedJob is one job entry with its hiring company already resolved (possibly to ""
// when no StartupResult references it).
type wellfoundResolvedJob struct {
	Entry   wellfoundJobEntry
	Company string
}

// wellfoundParsedPage is one fetched and parsed role-search page.
type wellfoundParsedPage struct {
	Jobs          []wellfoundResolvedJob
	PageCount     int
	TotalJobCount int
}

// wellfoundNextData is the shape of the page's __NEXT_DATA__ script this adapter reads: the
// normalized Apollo cache, keyed "<Type>:<id>".
type wellfoundNextData struct {
	Props struct {
		PageProps struct {
			ApolloState struct {
				Data map[string]json.RawMessage `json:"data"`
			} `json:"apolloState"`
		} `json:"pageProps"`
	} `json:"props"`
}

// parseWellfoundPage extracts and decodes a fetched page's __NEXT_DATA__ payload into its job
// listings (with company already resolved) and its stated page totals. It returns an error —
// rather than a page with zero postings — when the script is absent, its JSON does not decode at
// all, or it carries no usable ROOT_QUERY totals, per the spec's "unparseable page is a loud
// failure" requirement. An individual malformed job or startup entry is silently skipped instead:
// one re-templated posting must not abort an otherwise healthy page.
func parseWellfoundPage(root *html.Node) (wellfoundParsedPage, error) {
	raw := scriptTextByID(root, "__NEXT_DATA__")
	if raw == "" {
		return wellfoundParsedPage{}, fmt.Errorf("wellfound: __NEXT_DATA__ script not found")
	}
	var nd wellfoundNextData
	if err := json.Unmarshal([]byte(raw), &nd); err != nil {
		return wellfoundParsedPage{}, fmt.Errorf("wellfound: decode __NEXT_DATA__: %w", err)
	}
	data := nd.Props.PageProps.ApolloState.Data
	if len(data) == 0 {
		return wellfoundParsedPage{}, fmt.Errorf("wellfound: apolloState carries no data")
	}

	totals, err := wellfoundTotals(data)
	if err != nil {
		return wellfoundParsedPage{}, err
	}

	companyByJobKey := wellfoundCompanyIndex(data)

	var jobs []wellfoundResolvedJob
	for key, entryRaw := range data {
		if !strings.HasPrefix(key, "JobListingSearchResult:") {
			continue
		}
		var entry wellfoundJobEntry
		if json.Unmarshal(entryRaw, &entry) != nil {
			continue // malformed entry — dropped, per spec, without failing the page
		}
		jobs = append(jobs, wellfoundResolvedJob{Entry: entry, Company: companyByJobKey[key]})
	}

	return wellfoundParsedPage{Jobs: jobs, PageCount: totals.PageCount, TotalJobCount: totals.TotalJobCount}, nil
}

// wellfoundCompanyIndex builds the job-id-key -> hiring-company-name reverse index by walking
// every StartupResult entry's highlightedJobListings. A malformed StartupResult entry simply
// contributes no mappings rather than failing the page.
//
// StartupResult keys are visited in SORTED order and the first mapping for a job key wins:
// ranging the map directly is randomized per run, so a job referenced by more than one startup
// would flip its resolved company between runs of the same input, churning content_hash and
// misattributing the employer. Real data has one owner per job, so this is a defensive tie-break.
func wellfoundCompanyIndex(data map[string]json.RawMessage) map[string]string {
	startupKeys := make([]string, 0, len(data))
	for key := range data {
		if strings.HasPrefix(key, "StartupResult:") {
			startupKeys = append(startupKeys, key)
		}
	}
	sort.Strings(startupKeys)

	byJobKey := map[string]string{}
	for _, key := range startupKeys {
		var startup wellfoundStartupEntry
		if json.Unmarshal(data[key], &startup) != nil {
			continue
		}
		for _, ref := range startup.HighlightedJobListings {
			if ref.Ref == "" {
				continue
			}
			if _, exists := byJobKey[ref.Ref]; !exists {
				byJobKey[ref.Ref] = startup.Name
			}
		}
	}
	return byJobKey
}

// wellfoundTotals locates ROOT_QUERY.talent's "seoLandingPageJobSearchResults(...)" field —
// named with its call arguments baked into the key, Apollo's normalized-cache convention — and
// decodes its stated totals. Returning an error when it cannot be found is deliberate: without
// it, pagination has no trustworthy stopping point at all.
func wellfoundTotals(data map[string]json.RawMessage) (wellfoundSearchTotals, error) {
	rootRaw, ok := data["ROOT_QUERY"]
	if !ok {
		return wellfoundSearchTotals{}, fmt.Errorf("wellfound: apolloState carries no ROOT_QUERY")
	}
	var root struct {
		Talent map[string]json.RawMessage `json:"talent"`
	}
	if err := json.Unmarshal(rootRaw, &root); err != nil {
		return wellfoundSearchTotals{}, fmt.Errorf("wellfound: decode ROOT_QUERY: %w", err)
	}
	for key, fieldRaw := range root.Talent {
		if !strings.HasPrefix(key, "seoLandingPageJobSearchResults(") {
			continue
		}
		var totals wellfoundSearchTotals
		if err := json.Unmarshal(fieldRaw, &totals); err != nil {
			return wellfoundSearchTotals{}, fmt.Errorf("wellfound: decode search results totals: %w", err)
		}
		return totals, nil
	}
	return wellfoundSearchTotals{}, fmt.Errorf("wellfound: ROOT_QUERY.talent carries no seoLandingPageJobSearchResults field")
}

// Fetch crawls one role slice to the payload's own stated page count. A page failing to fetch or
// parse errors the whole call, even a later one: unlike the usual "a later page ends the walk with
// what was gathered" rule, an unparseable page here signals a vendor-markup change, since the
// stated pageCount already says where the board ends.
func (s wellfound) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	first, err := s.fetchPage(ctx, e.Board, 1)
	if err != nil {
		return nil, fmt.Errorf("wellfound: role %q page 1: %w", e.Board, err)
	}
	pages := []wellfoundParsedPage{first}
	// A silent truncation, deliberately: unlike bayt/gulftalent's fullBoardListing promise (where
	// reaching a page cap before proving the true end must fail the crawl), wellfound never claims
	// completeness, so a role slice deeper than the cap yields a partial crawl, not an error.
	pageCount := min(first.PageCount, wellfoundMaxPages)
	for page := 2; page <= pageCount; page++ {
		p, err := s.fetchPage(ctx, e.Board, page)
		if err != nil {
			return nil, fmt.Errorf("wellfound: role %q page %d: %w", e.Board, page, err)
		}
		pages = append(pages, p)
	}

	var jobs []Job
	for _, p := range pages {
		for _, rj := range p.Jobs {
			if job, ok := rj.toJob(); ok {
				jobs = append(jobs, job)
			}
		}
	}
	return jobs, nil
}

// fetchPage fetches and parses one role-slice page.
func (s wellfound) fetchPage(ctx context.Context, role string, page int) (wellfoundParsedPage, error) {
	root, err := s.http.GetHTML(ctx, wellfoundPageURL(role, page))
	if err != nil {
		return wellfoundParsedPage{}, err
	}
	return parseWellfoundPage(root)
}

// toJob maps a resolved job entry to a Job, returning ok=false for an entry the catalogue cannot
// key or attribute: no extractable id, or no resolvable hiring company (the entry names none and
// no StartupResult references it).
func (rj wellfoundResolvedJob) toJob() (Job, bool) {
	e := rj.Entry
	company := strings.TrimSpace(rj.Company)
	if e.ID == "" || company == "" {
		return Job{}, false
	}
	j := Job{
		ExternalID:  e.ID,
		URL:         fmt.Sprintf(wellfoundJobURL, e.ID, e.Slug),
		Title:       strings.TrimSpace(e.Title),
		Company:     company,
		Location:    strings.Join(e.LocationNames, "; "),
		Description: wellfoundDescription(e),
		Remote:      e.Remote,
	}
	j.SalaryMin, j.SalaryMax, j.SalaryCurrency, j.SalaryPeriod = wellfoundSalary(e.Compensation)
	return j, true
}

// wellfoundSalary reads the rupee range of a compensation label ("₹25L – ₹30L • No
// equity") into the structured salary fields. The equity half after "•" is not pay. Only
// a lakh or crore range, or a rupee figure with its period, is read (inrSalary); every
// other label — a dollar range, "₹10,000 – ₹15,000" with no period, "Competitive" — stays
// display text only.
func wellfoundSalary(comp string) (lo, hi *int, currency, period string) {
	pay, _, _ := strings.Cut(comp, "•")
	return inrSalary(pay)
}

// wellfoundDescription folds the listing's free-text compensation range into the rendered HTML
// body. Wellfound's description field is Markdown, not HTML, so it goes through the same
// markdownToHTML (goldmark) conversion as join.go's Join.com adapter before sanitizing. The
// compensation range is a display string, carried as text through sanitizeHTML; only the rupee
// ranges whose unit and period are unambiguous also reach the structured salary fields
// (wellfoundSalary).
func wellfoundDescription(e wellfoundJobEntry) string {
	body := sanitizeHTML(markdownToHTML(e.Description))
	comp := strings.TrimSpace(e.Compensation)
	if comp == "" {
		return body
	}
	return sanitizeHTML("<p><strong>Compensation:</strong> "+comp+"</p>") + body
}
