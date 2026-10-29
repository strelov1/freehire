package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strings"

	"golang.org/x/sync/errgroup"
)

// infojobs adapts infojobs.net, Spain's largest job board. Boardless and multi-company: the
// whole-site listing (/ofertas-trabajo) is paged and each posting carries its own employer, so it
// stays in the source facet and takes the company from the offer.
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
	// infojobsListURL is the whole-site listing. It used to be the IT category's
	// (/ofertas-trabajo/informatica-telecomunicaciones), back when the catalogue was IT-only;
	// by 2026-10-20 that page answered a GeeTest bot challenge instead of the listing.
	infojobsListURL = "https://www.infojobs.net/ofertas-trabajo?page=%d"
	// infojobsMaxPages refuses a listing whose stated total implies more pages than this, rather
	// than crawling an absurd one. Measured 2026-10-20: 63.6k offers at 22 a page is ~2.9k pages.
	infojobsMaxPages = 4000
)

// NewInfoJobs builds the infojobs.net adapter over the given HTTP client.
func NewInfoJobs(c TextGetter) Source { return infojobs{http: c, maxNewPerRun: infojobsMaxNewPerRun} }

func (infojobs) Provider() string { return "infojobs" }

func (infojobs) boardless() {}

func (infojobs) aggregator() {}

// infojobsListProps is the subset of a listing page's __INITIAL_PROPS__ we read: the page of
// offers and the catalogue size used to stop pagination.
type infojobsListProps struct {
	Offers   []infojobsOffer `json:"offers"`
	Overview struct {
		TotalElements int `json:"totalElements"`
	} `json:"overview"`
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
// Scrapling tier each page is a browser navigation of ~7.5 s, so a ~2.9k-page listing is ~6 h on
// one session; this matches the service's default browser cap (SCRAPLING_MAX_BROWSERS).
const infojobsListWorkers = 3

// crawl pages the whole-site listing and returns every raw offer — the shared list walk behind
// Fetch and FetchNew. The first page states the listing's total, which fixes how many pages
// there are; the rest are fetched concurrently, in page order, and an offer that shifted to the
// next page while the crawl ran is kept once.
//
// Any page that still fails after its attempts — a request error, or the bot-challenge page that
// carries no props blob — fails the whole crawl, as does a listing longer than the ceiling:
// infojobs is swept, so a partial list would close every offer past the cut. Measured 2026-10-20
// through the proxy on a plain client, the sixth consecutive page was a challenge, and an earlier
// "keep what was gathered" rule closed 300.
func (s infojobs) crawl(ctx context.Context) ([]infojobsOffer, error) {
	first, err := s.listPage(ctx, 1)
	if err != nil {
		return nil, err
	}
	if len(first.Offers) == 0 {
		return nil, nil
	}
	total, perPage := first.Overview.TotalElements, len(first.Offers)
	if total <= 0 {
		return nil, fmt.Errorf("infojobs: listing page 1 states no total")
	}
	pages := (total + perPage - 1) / perPage
	if pages > infojobsMaxPages {
		return nil, fmt.Errorf("infojobs: listing states %d offers (%d pages), past the %d-page ceiling", total, pages, infojobsMaxPages)
	}

	byPage := make([][]infojobsOffer, pages)
	byPage[0] = first.Offers
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(infojobsListWorkers)
	for page := 2; page <= pages; page++ {
		g.Go(func() error {
			props, err := s.listPage(gctx, page)
			if err != nil {
				return err
			}
			byPage[page-1] = props.Offers
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}

	seen := make(map[string]bool, total)
	offers := make([]infojobsOffer, 0, total)
	for _, page := range byPage {
		for _, o := range page {
			if seen[o.Code] {
				continue
			}
			seen[o.Code] = true
			offers = append(offers, o)
		}
	}
	return offers, nil
}

// infojobsListAttempts is how many times one listing page is sent before the crawl gives up.
// Through the Scrapling tier a failed navigation retires its browser session, so a retry opens
// on the next sticky exit IP. Measured 2026-10-26..28 on the crawl host, every failed run died on
// the first page of a freshly opened session — pages 1, 501 and 1001, against the service's
// 500-page session life — while the pages around it were served: some exits never finish
// loading the page (a 90 s timeout), and one such page used to fail the whole ~2.9k-page walk.
const infojobsListAttempts = 3

// listPage fetches and decodes one listing page, sending it again on failure.
func (s infojobs) listPage(ctx context.Context, page int) (infojobsListProps, error) {
	var err error
	for attempt := 1; attempt <= infojobsListAttempts; attempt++ {
		props, perr := s.listPageOnce(ctx, page)
		if perr == nil {
			return props, nil
		}
		err = perr
		if ctx.Err() != nil {
			break
		}
	}
	return infojobsListProps{}, fmt.Errorf("infojobs: listing page %d: %w", page, err)
}

func (s infojobs) listPageOnce(ctx context.Context, page int) (infojobsListProps, error) {
	var props infojobsListProps
	text, err := s.http.GetText(ctx, fmt.Sprintf(infojobsListURL, page))
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
