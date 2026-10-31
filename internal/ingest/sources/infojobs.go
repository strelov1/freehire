package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"
)

// infojobs adapts infojobs.net, Spain's largest job board. Boardless and multi-company: the
// listing (/ofertas-trabajo) is paged one category at a time and each posting carries its own
// employer, so it stays in the source facet and takes the company from the offer. The walk reads
// the whole catalogue or fails, so it is swept by source (fullCatalog).
// The pages are a server-rendered React app that embeds its data as a `window.__INITIAL_PROPS__`
// blob; the listing blob carries every field EXCEPT the full description (the list holds a
// truncated teaser), so — like justjoin — the full body comes from a per-offer detail fetch run
// only for offers the catalogue does not already have (FetchNew).
//
// Every page sits behind a GeeTest bot challenge for a plain client: the crawl host's IP is
// refused outright (405 since 2026-10-08) and the proxy is challenged from the sixth listing
// page. Through the Scrapling tier's browser over the proxy, listing and offer pages alike are
// served (measured 2026-10-20: 20 of 20 listing pages, 5 of 5 offer pages with full bodies), so
// production crawls it there (scraplingtier.go).
type infojobs struct {
	http TextGetter
	// maxNewPerRun bounds how many new offers one run hydrates; non-positive takes
	// infojobsMaxNewPerRun. Anything past it is deferred — not yielded, so not stored — and
	// offered again by a later run.
	maxNewPerRun int
}

// infojobsMaxNewPerRun is the default hydration budget. An offer page is a browser navigation
// of ~7 s through the Scrapling tier, three in flight, so 1 000 is ~40 minutes on top of the
// ~2 h listing walk; the ~64k-offer backlog the first runs face drains over days rather than
// pushing one run past its timeout.
const infojobsMaxNewPerRun = 1000

const (
	// infojobsRootURL is the whole-site listing's first page, read only for the total it states
	// and the categories it aggregates. (The IT category's semantic path,
	// /ofertas-trabajo/informatica-telecomunicaciones, answered a GeeTest challenge by 2026-10-20
	// and, like every /ofertas-trabajo/<slug> path, serves the UNFILTERED listing anyway.)
	infojobsRootURL = "https://www.infojobs.net/ofertas-trabajo?page=1"
	// infojobsCategoryURL is one category's listing; the filter is the categoryIds query
	// parameter, which the page echoes back in its search props.
	infojobsCategoryURL = "https://www.infojobs.net/ofertas-trabajo?categoryIds=%s&page=%d"
	// infojobsMaxPages is the deepest page the listing serves. Measured 2026-10-30: the
	// whole-site listing stated 63,729 offers over 3,187 pages, served pages 1-2999 and answered
	// page 3000 onwards with an error page carrying no offers — so the unfiltered walk this
	// replaced never saw the oldest ~3.7k live offers, and stopped (at a page count computed
	// from page 1's 22 cards, two of them promoted, against 20 on every later page) after ~2.9k
	// pages. A category stating more pages than this cannot be read whole and fails the crawl;
	// the largest, "Comercial y ventas", stated 367.
	infojobsMaxPages = 2999
	// infojobsDriftPages is how many of a category's stated last pages may come back empty: offers
	// removed while the category is walked pull the tail forward, and its last page or two can
	// empty before they are read. An empty page with offers after it, or more empty pages than
	// this, is a listing that stopped serving, not one that shrank.
	infojobsDriftPages = 2
)

// NewInfoJobs builds the infojobs.net adapter over the given HTTP client.
func NewInfoJobs(c TextGetter) Source { return infojobs{http: c, maxNewPerRun: infojobsMaxNewPerRun} }

func (infojobs) Provider() string { return "infojobs" }

func (infojobs) boardless() {}

func (infojobs) aggregator() {}

func (infojobs) fullCatalog() {}

// infojobsListProps is the subset of a listing page's __INITIAL_PROPS__ we read: the page of
// offers; on the root page the catalogue's total and its categories with their counts; on a
// category page the filter it applied and how many pages it states.
type infojobsListProps struct {
	Offers   []infojobsOffer `json:"offers"`
	Overview struct {
		TotalElements int `json:"totalElements"`
	} `json:"overview"`
	Aggregation struct {
		Category []struct {
			Value string `json:"value"`
			Count int    `json:"count"`
		} `json:"category"`
	} `json:"aggregation"`
	Navigation struct {
		TotalPages int `json:"totalPages"`
	} `json:"navigation"`
	Search struct {
		CategoryIDs []string `json:"categoryIds"`
	} `json:"search"`
}

// infojobsDetailProps is the subset of an offer page's __INITIAL_PROPS__ we read: the full
// description, which the listing truncates.
type infojobsDetailProps struct {
	Offer struct {
		Description string `json:"description"`
	} `json:"offer"`
}

// infojobsOffer is one listing offer. Description here is a truncated teaser (the full body lives
// on the detail page); Link is a protocol-relative URL with a tracking query. Teleworking is the
// structured work-mode label.
type infojobsOffer struct {
	Code        string `json:"code"`
	Title       string `json:"title"`
	CompanyName string `json:"companyName"`
	Link        string `json:"link"`
	City        string `json:"city"`
	PublishedAt string `json:"publishedAt"`
	Teleworking string `json:"teleworking"`
	Description string `json:"description"`
}

// Fetch is the list-only crawl (teaser descriptions) for callers that drive no hydration.
// FetchNew is the path ingest uses.
func (s infojobs) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	offers, err := s.crawl(ctx)
	if err != nil {
		return nil, err
	}
	var jobs []Job
	for _, o := range offers {
		if job, ok := o.toJob(); ok {
			jobs = append(jobs, job)
		}
	}
	return jobs, nil
}

// FetchNew pages the listing and hydrates only what the catalogue lacks. A seen offer is a
// liveness refresh only, with no request of its own; a new one is yielded with its full body,
// up to maxNewPerRun of them. A new offer past the budget, or whose page fails, is deferred
// rather than stored with its teaser: a stored row is seen, and would never get its body.
func (s infojobs) FetchNew(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	offers, err := s.crawl(ctx)
	if err != nil {
		return nil, err
	}
	budget := s.maxNewPerRun
	if budget <= 0 {
		budget = infojobsMaxNewPerRun
	}
	var jobs []Job
	var fresh []Job
	deferred := 0
	for _, o := range offers {
		job, ok := o.toJob()
		if !ok {
			continue
		}
		if seen(o.Code) {
			job.SeenRefresh = true
			jobs = append(jobs, job)
			continue
		}
		if len(fresh) >= budget {
			deferred++
			continue
		}
		fresh = append(fresh, job)
	}
	if deferred > 0 {
		log.Printf("infojobs: hydration budget %d reached; %d new offers deferred to a later run", budget, deferred)
	}
	hydrated := fetchDetails(fresh, infojobsListWorkers, func(j Job) (Job, bool) {
		body, ok := s.detail(ctx, j.URL)
		if !ok {
			return Job{}, false
		}
		j.Description = sanitizeHTML(body)
		return j, true
	})
	if failed := len(fresh) - len(hydrated); failed > 0 {
		log.Printf("infojobs: %d of %d new offers' pages failed; deferred to a later run", failed, len(fresh))
	}
	return append(jobs, hydrated...), nil
}

// detail fetches an offer page and returns its full description, ok=false on a failed request
// or a page with no parseable body (the bot challenge among them).
func (s infojobs) detail(ctx context.Context, jobURL string) (string, bool) {
	text, err := s.http.GetText(ctx, jobURL)
	if err != nil {
		return "", false
	}
	var props infojobsDetailProps
	if err := infojobsDecodeProps(text, &props); err != nil {
		return "", false
	}
	if strings.TrimSpace(props.Offer.Description) == "" {
		return "", false
	}
	return props.Offer.Description, true
}

// infojobsListWorkers bounds how many listing pages are in flight at once. Through the
// Scrapling tier each page is a browser navigation of ~7.5 s, so a ~3.2k-page walk is ~6 h on
// one session; this matches the service's default browser cap (SCRAPLING_MAX_BROWSERS).
const infojobsListWorkers = 3

// crawl reads the whole catalogue and returns every raw offer, newest first — the shared list
// walk behind Fetch and FetchNew. The whole-site listing cannot be walked to its end (it stops
// serving at infojobsMaxPages, ~3.7k offers short), so its first page is read only for the total
// and the categories, which partition the catalogue (measured 2026-10-30: 21 categories whose
// counts sum exactly to the 63,729 stated), and each category is walked to its own end. An offer
// that shifted between pages while the crawl ran is kept once.
//
// infojobs is swept by source, so a partial list would close every offer the walk missed, and
// anything that leaves the read unproven fails the whole crawl: a page that still fails after its
// attempts (a request error, or the bot-challenge page that carries no props blob), categories
// that do not account for the stated total, a category page that did not apply its filter, a
// category longer than the listing serves, or a hole in a category's pages. Measured 2026-10-20
// through the proxy on a plain client, the sixth consecutive page was a challenge, and an earlier
// "keep what was gathered" rule closed 300.
func (s infojobs) crawl(ctx context.Context) ([]infojobsOffer, error) {
	root, err := s.listPage(ctx, infojobsRootURL)
	if err != nil {
		return nil, err
	}
	total := root.Overview.TotalElements
	if total == 0 && len(root.Offers) == 0 {
		return nil, nil
	}
	if total <= 0 {
		return nil, fmt.Errorf("infojobs: listing page 1 states no total")
	}
	cats := root.Aggregation.Category
	covered := 0
	for _, c := range cats {
		covered += c.Count
	}
	if covered != total {
		return nil, fmt.Errorf("infojobs: the %d categories hold %d offers, not the %d the listing states", len(cats), covered, total)
	}

	// Each category's first page states how many pages it has; the rest are fetched after.
	byCat := make([][][]infojobsOffer, len(cats))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(infojobsListWorkers)
	for i, c := range cats {
		g.Go(func() error {
			first, err := s.listPage(gctx, infojobsCategoryPageURL(c.Value, 1))
			if err != nil {
				return err
			}
			if !slices.Equal(first.Search.CategoryIDs, []string{c.Value}) {
				return fmt.Errorf("infojobs: category %s: the listing did not apply the filter (it applied %v)", c.Value, first.Search.CategoryIDs)
			}
			pages := max(first.Navigation.TotalPages, 1)
			if pages > infojobsMaxPages {
				return fmt.Errorf("infojobs: category %s states %d pages, past the %d the listing serves", c.Value, pages, infojobsMaxPages)
			}
			byCat[i] = make([][]infojobsOffer, pages)
			byCat[i][0] = first.Offers
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}

	g, gctx = errgroup.WithContext(ctx)
	g.SetLimit(infojobsListWorkers)
	for i, c := range cats {
		for page := 2; page <= len(byCat[i]); page++ {
			g.Go(func() error {
				props, err := s.listPage(gctx, infojobsCategoryPageURL(c.Value, page))
				if err != nil {
					return err
				}
				byCat[i][page-1] = props.Offers
				return nil
			})
		}
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}

	g, gctx = errgroup.WithContext(ctx)
	g.SetLimit(infojobsListWorkers)
	for i, c := range cats {
		g.Go(func() error {
			tail, err := s.categoryTail(gctx, c.Value, byCat[i])
			if err != nil {
				return err
			}
			byCat[i] = append(byCat[i], tail...)
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}

	seen := make(map[string]bool, total)
	offers := make([]infojobsOffer, 0, total)
	for _, pages := range byCat {
		for _, page := range pages {
			for _, o := range page {
				if seen[o.Code] {
					continue
				}
				seen[o.Code] = true
				offers = append(offers, o)
			}
		}
	}
	sortInfojobsNewestFirst(offers)
	return offers, nil
}

// categoryTail checks a category's stated pages for proof they were read whole, and reads past
// them to the listing's real end.
//
// Offers removed during the walk pull the tail forward, so the last pages may come back empty —
// at most infojobsDriftPages of them, and only as a run at the end. Offers published during the
// walk push the category's oldest offers past the page that was its last when the walk began;
// those are the same offers every run, so missing them would be systematic rather than the
// random drift the 48-hour sweep window absorbs. So when the stated last page is not empty, the
// pages after it are read one by one until one comes back empty (the listing answers a page past
// its end with no offers).
func (s infojobs) categoryTail(ctx context.Context, cat string, pages [][]infojobsOffer) ([][]infojobsOffer, error) {
	last := len(pages) - 1
	for last >= 0 && len(pages[last]) == 0 {
		last--
	}
	for page := range last {
		if len(pages[page]) == 0 {
			return nil, fmt.Errorf("infojobs: category %s: page %d is empty while page %d holds offers", cat, page+1, last+1)
		}
	}
	if empty := len(pages) - 1 - last; empty > 0 {
		if empty > infojobsDriftPages {
			return nil, fmt.Errorf("infojobs: category %s: the last %d of its %d pages are empty", cat, empty, len(pages))
		}
		return nil, nil
	}
	var tail [][]infojobsOffer
	for page := len(pages) + 1; ; page++ {
		if page > infojobsMaxPages {
			return nil, fmt.Errorf("infojobs: category %s runs past the %d pages the listing serves", cat, infojobsMaxPages)
		}
		props, err := s.listPage(ctx, infojobsCategoryPageURL(cat, page))
		if err != nil {
			return nil, err
		}
		if len(props.Offers) == 0 {
			return tail, nil
		}
		tail = append(tail, props.Offers)
	}
}

func infojobsCategoryPageURL(cat string, page int) string {
	return fmt.Sprintf(infojobsCategoryURL, url.QueryEscape(cat), page)
}

// sortInfojobsNewestFirst orders offers by publication time, newest first, so a run's hydration
// budget goes to the freshest new offers whatever category they sit in; an offer with no
// readable time goes last.
func sortInfojobsNewestFirst(offers []infojobsOffer) {
	at := make(map[string]time.Time, len(offers))
	for _, o := range offers {
		if t := parseRFC3339(o.PublishedAt); t != nil {
			at[o.Code] = *t
		}
	}
	slices.SortStableFunc(offers, func(a, b infojobsOffer) int {
		return at[b.Code].Compare(at[a.Code])
	})
}

// infojobsListAttempts is how many times one listing page is sent before the crawl gives up.
// Through the Scrapling tier a failed navigation retires its browser session, so a retry opens
// on the next sticky exit IP. Measured 2026-10-26..28 on the crawl host, every failed run died on
// the first page of a freshly opened session — pages 1, 501 and 1001, against the service's
// 500-page session life — while the pages around it were served: some exits never finish
// loading the page (a 90 s timeout), and one such page used to fail the whole ~2.9k-page walk.
const infojobsListAttempts = 3

// listPage fetches and decodes one listing page, sending it again on failure.
func (s infojobs) listPage(ctx context.Context, pageURL string) (infojobsListProps, error) {
	var err error
	for attempt := 1; attempt <= infojobsListAttempts; attempt++ {
		props, perr := s.listPageOnce(ctx, pageURL)
		if perr == nil {
			return props, nil
		}
		err = perr
		if ctx.Err() != nil {
			break
		}
	}
	return infojobsListProps{}, fmt.Errorf("infojobs: listing %s: %w", pageURL, err)
}

func (s infojobs) listPageOnce(ctx context.Context, pageURL string) (infojobsListProps, error) {
	var props infojobsListProps
	text, err := s.http.GetText(ctx, pageURL)
	if err != nil {
		return props, err
	}
	if err := infojobsDecodeProps(text, &props); err != nil {
		return props, err
	}
	return props, nil
}

// infojobsPropsMarker matches the `window.__INITIAL_PROPS__ = JSON.parse(` assignment; the JSON
// string literal that follows is the (double-encoded) props blob.
var infojobsPropsMarker = regexp.MustCompile(`window\.__INITIAL_PROPS__\s*=\s*JSON\.parse\(`)

// infojobsDecodeProps extracts the __INITIAL_PROPS__ blob from a page and unmarshals it into v.
// The blob is double-encoded — a JSON string literal (produced for JSON.parse) whose content is
// itself the JSON document — so it is decoded in two steps: a json.Decoder reads the leading
// string token (correctly honouring the `\"` escapes and stopping at the true closing quote,
// which a lazy regex could not), then its unescaped content is unmarshalled into v.
func infojobsDecodeProps(pageText string, v any) error {
	loc := infojobsPropsMarker.FindStringIndex(pageText)
	if loc == nil {
		return errors.New("infojobs: __INITIAL_PROPS__ not found")
	}
	dec := json.NewDecoder(strings.NewReader(pageText[loc[1]:]))
	var inner string
	if err := dec.Decode(&inner); err != nil {
		return fmt.Errorf("infojobs: decode props string: %w", err)
	}
	if err := json.Unmarshal([]byte(inner), v); err != nil {
		return fmt.Errorf("infojobs: unmarshal props: %w", err)
	}
	return nil
}

// infojobsWorkMode maps the structured teleworking label to freehire's work-mode vocabulary.
func infojobsWorkMode(teleworking string) string {
	switch strings.TrimSpace(teleworking) {
	case "Solo teletrabajo":
		return "remote"
	case "Híbrido":
		return "hybrid"
	case "Presencial":
		return "onsite"
	default:
		return ""
	}
}

// toJob maps a listing offer to a Job, returning ok=false for an unusable offer (no code to key
// on, no link to build the URL, or no company). The URL is the offer's protocol-relative link
// with its tracking query stripped and the scheme restored.
//
// infojobs.net is the Spanish board, and its location is the bare city ("Murcia", "Vigo",
// "Getafe"), which GeoNames also gives to Latin-American and Philippine towns: measured
// 2026-10-27, 28% of its open postings had no country at all. Spain is therefore the
// FallbackCountries signal — read only when the city states no country of its own.
func (o infojobsOffer) toJob() (Job, bool) {
	if o.Code == "" || o.Link == "" || o.CompanyName == "" {
		return Job{}, false
	}
	mode := infojobsWorkMode(o.Teleworking)
	return Job{
		ExternalID:  o.Code,
		URL:         "https:" + trimURLSuffix(o.Link),
		Title:       o.Title,
		Company:     o.CompanyName,
		Location:    o.City,
		Description: sanitizeHTML(o.Description),
		Remote:      mode == "remote",
		WorkMode:    mode,
		PostedAt:    parseRFC3339(o.PublishedAt),

		FallbackCountries: []string{"es"},
	}, true
}
