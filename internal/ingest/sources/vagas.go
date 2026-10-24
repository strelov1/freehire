package sources

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// vagas adapts vagas.com.br, a large Brazilian national job board. It has no per-tenant board:
// the whole catalogue is one paginated listing (see vagasListingURL), and each posting's company
// comes from the vacancy itself, so it is a boardless aggregator. The listing pages are
// server-rendered HTML enumerating /vagas/v<id>/<slug> links; each job page carries a schema.org
// JobPosting ld+json block, so the description and fields come from a per-job detail fetch
// (bounded concurrency), like the other JSON-LD detail adapters.
type vagas struct {
	http HTMLGetter
	// maxNewPerRun bounds how many new postings one run hydrates; non-positive takes
	// vagasMaxNewPerRun. The rest are deferred — not yielded, so not stored — to a later run.
	maxNewPerRun int
}

// NewVagas builds the vagas.com.br adapter over the given HTTP client.
func NewVagas(c HTMLGetter) Source { return vagas{http: c} }

func (vagas) Provider() string { return "vagas" }

// vagas is a national board crawled by fixed area listings, not per company.
func (vagas) boardless() {}

// vagas aggregates postings from many companies, so it stays in the source facet.
func (vagas) aggregator() {}

// vagasListingURL is one page of the whole catalogue. The site's search path is
// /vagas-de-<query>, and an EMPTY query is the unfiltered catalogue — the site links it itself
// from its home page. Measured 2026-10-23 its heading reported 3 981 postings, the same total its
// own facet counts sum to, over 100 pages of 40 (page 101 empty). The /vagas-de-<area> listings
// it replaced are keyword searches, not partitions — "todas as areas" is one more, at 2 431 — and
// the three tech ones together reached ~800.
func vagasListingURL(page int) string {
	return fmt.Sprintf("https://www.vagas.com.br/vagas-de-?ordenar_por=mais_recentes&pagina=%d", page)
}

// vagasPageSize is how many cards a full listing page carries.
const vagasPageSize = 40

// vagasMaxPages bounds the walk whatever total the heading states. Twice the measured 100 pages:
// a cap the catalogue outgrew would read fewer postings every run, and the 48-hour sweep would
// close what it stopped reaching.
const vagasMaxPages = 200

// vagasTotalPattern reads the listing heading's total ("3981 vagas de emprego").
var vagasTotalPattern = regexp.MustCompile(`^\s*(\d+)\s+vagas`)

// vagasMaxNewPerRun is the hydration budget per run. Measured 2026-10-23 through the production
// fetch path, the whole listing (100 pages) took under four minutes and 30 detail pages took 29
// seconds, the one-per-second pacer being the floor; 1 000 is therefore ~17 minutes plus the
// listing, well inside the 50-minute unit timeout less the sweep's reserve. That matters because
// the board writes only once the fetch returns: a run that overruns stores nothing and the next
// one faces the same backlog. Widening to the whole catalogue left ~2 500 postings to bootstrap,
// so three runs.
const vagasMaxNewPerRun = 1000

// vagasBase is the scheme+host that relative job hrefs resolve against; parsed once.
var vagasBase = &url.URL{Scheme: "https", Host: "www.vagas.com.br"}

func (v vagas) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	return v.FetchNew(ctx, CompanyEntry{}, func(string) bool { return false })
}

func (v vagas) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	return v.FetchNewGated(ctx, e, seen, DetailGate{})
}

// FetchNewGated fetches detail only for a posting the catalogue does not already have — seen
// reports whether a native id is already ingested. A seen posting yields a liveness-refresh job
// (no detail request), so the pipeline refreshes its open state WITHOUT rewriting the content
// hydrated when it was new.
//
// This is what makes the crawl finish at all. Every request here — listing and detail alike —
// goes through a one-per-second pacer, so re-reading all 1 939 stored postings costs half an
// hour before the proxy's own latency, and the run overran its 50-minute budget every time:
// measured 2026-09-07, the last successful crawl was 2026-08-18 and every attempt since either
// timed out mid-detail or never got a slot. Hydrating only what is new turns that into a
// handful of requests in the steady state.
//
// The gate is what keeps it so. A posting the pipeline discards — its employer already covered
// by a first-party source (a quarter of the tech listing, measured 2026-10-23) — is never stored,
// so never seen, and without the gate would take a slot of the hydration budget on EVERY run,
// newest first, until those slots crowded out the genuinely new postings behind them. The listing
// card carries the company and title the gate needs, so such a posting is yielded body-less.
func (v vagas) FetchNewGated(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool, gate DetailGate) ([]Job, error) {
	cards, err := v.list(ctx)
	if err != nil {
		return nil, err
	}

	var jobs []Job
	var fresh []vagasCard
	for _, c := range cards {
		if seen(c.id) {
			// Already ingested: refresh liveness only, no detail request. Just the identity
			// the pipeline's touch needs; the stored content is left untouched.
			jobs = append(jobs, Job{ExternalID: c.id, URL: c.url, SeenRefresh: true})
			continue
		}
		fresh = append(fresh, c)
	}

	var covered map[string]bool
	if gate.Covered != nil {
		companies := make([]string, 0, len(fresh))
		for _, c := range fresh {
			companies = append(companies, c.company)
		}
		covered = gate.Covered(companies)
	}
	budget := v.maxNewPerRun
	if budget <= 0 {
		budget = vagasMaxNewPerRun
	}
	var toHydrate []string
	deferred := 0
	for _, c := range fresh {
		switch j := c.job(); {
		case covered[c.company] || (gate.Rejected != nil && gate.Rejected(j)):
			jobs = append(jobs, j) // the pipeline discards it; its body would buy nothing
		case len(toHydrate) < budget:
			toHydrate = append(toHydrate, c.url)
		default:
			deferred++
		}
	}
	if deferred > 0 {
		log.Printf("vagas: hydration budget %d reached; %d new postings deferred to a later run", budget, deferred)
	}
	return append(jobs, fetchDetails(toHydrate, defaultDetailWorkers, func(u string) (Job, bool) {
		return v.detail(ctx, u)
	})...), nil
}

// vagasCard is one posting as the listing shows it: enough to refresh a stored posting or to
// let the pipeline's gate judge a new one, not enough to store it.
type vagasCard struct {
	id, url, title, company string
}

// job is the card as a body-less Job, for the gate to judge.
func (c vagasCard) job() Job {
	return Job{ExternalID: c.id, URL: c.url, Title: c.title, Company: c.company}
}

// list walks the whole catalogue and returns each posting's card once.
//
// It does NOT stop at the first page that fails or adds nothing, the way crawlPagedLinks does,
// because on this site page N is not a fixed slice of the catalogue. Measured 2026-10-23, the
// same page fetched twice from one IP came back in a different order (even sorted newest
// first, which only orders by day), so two full walks read 3 260 and 3 240 distinct postings of
// 3 981, overlapping in only 2 734 — every page repeats some of what earlier pages showed and
// hides some of the rest. Through the production fetch path 6-7 of 100 pages also failed. Stopping
// at either kind of page read ~100 postings a run; reading every page reads ~80% of the
// catalogue, a near-independent ~80% each run, so a posting the 48-hour sweep would close has to
// be missed by every run in two days.
//
// So the walk is driven by the heading's total instead: page 1 failing, or stating no total, is
// the board failing (a layout change must not read as an empty catalogue), and any later page
// failing costs that page alone.
func (v vagas) list(ctx context.Context) ([]vagasCard, error) {
	root, err := v.http.GetHTML(ctx, vagasListingURL(1))
	if err != nil {
		return nil, fmt.Errorf("vagas: listing: %w", err)
	}
	total := vagasListingTotal(root)
	if total == 0 {
		return nil, errors.New("vagas: listing page 1 states no posting total; the layout may have changed")
	}
	pages := min((total+vagasPageSize-1)/vagasPageSize, vagasMaxPages)

	byID := map[string]bool{}
	var out []vagasCard
	add := func(root *html.Node) {
		for _, c := range parseVagasListing(root) {
			if !byID[c.id] {
				byID[c.id] = true
				out = append(out, c)
			}
		}
	}
	add(root)
	failed := 0
	for page := 2; page <= pages; page++ {
		root, err := v.http.GetHTML(ctx, vagasListingURL(page))
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			failed++
			continue
		}
		add(root)
	}
	log.Printf("vagas: listing states %d postings over %d pages; read %d distinct from %d pages (%d failed)",
		total, pages, len(out), pages-failed, failed)
	return out, nil
}

// vagasListingTotal is the posting count the listing heading states, or 0 when it states none.
func vagasListingTotal(root *html.Node) int {
	h1 := firstByTag(root, "h1")
	if h1 == nil {
		return 0
	}
	n, _ := strconv.Atoi(firstSubmatch(vagasTotalPattern, textContent(h1)))
	return n
}

// parseVagasListing reads a listing page's cards: each li.vaga holds the posting link
// (a.link-detalhes-vaga, whose title attribute is the job title) and the employer
// (span.emprVaga). A card whose link carries no native id is skipped.
func parseVagasListing(root *html.Node) []vagasCard {
	var out []vagasCard
	walk(root, func(n *html.Node) bool {
		if n.Type != html.ElementNode || n.Data != "li" || !HasClass(n, "vaga") {
			return true
		}
		link := firstByClass(n, "link-detalhes-vaga")
		if link == nil {
			return false
		}
		ref, err := url.Parse(Attr(link, "href"))
		if err != nil {
			return false
		}
		u := vagasBase.ResolveReference(ref).String()
		id := vagasJobID(u)
		if id == "" {
			return false
		}
		title := strings.TrimSpace(Attr(link, "title"))
		if title == "" {
			title = textContent(link)
		}
		company := ""
		if span := firstByClass(n, "emprVaga"); span != nil {
			company = textContent(span)
		}
		out = append(out, vagasCard{id: id, url: u, title: title, company: company})
		return false
	})
	return out
}

// detail fetches one job page and maps its JobPosting ld+json to a Job, returning ok=false when
// the page fetch fails, carries no JobPosting, or has no id, so the caller skips just that posting.
func (v vagas) detail(ctx context.Context, jobURL string) (Job, bool) {
	id := vagasJobID(jobURL)
	if id == "" {
		return Job{}, false // no native id → would collide on the dedup key; skip it
	}
	root, err := v.http.GetHTML(ctx, jobURL)
	if err != nil {
		return Job{}, false
	}
	var p vagasPosting
	if !ldJobPosting(root, &p) {
		return Job{}, false
	}

	location := p.JobLocation.Address.Location()
	return Job{
		ExternalID:  id,
		URL:         jobURL,
		Title:       p.Title,
		Company:     p.HiringOrganization.Name,
		Location:    location,
		Description: sanitizeHTML(vagasDescription(root, p.Description)),
		Remote:      isRemote(location),
		PostedAt:    parseDate(p.DatePosted),
	}, true
}

// vagasDescription returns the job's description HTML. It prefers the page's
// job-description__text block, whose markup (paragraphs, lists, emphasis) is preserved by
// sanitizeHTML; the JSON-LD description is a flattened, separator-stripped version that glues
// headings and list items into the prose, so it is only a fallback when the block is absent.
func vagasDescription(root *html.Node, ldDescription string) string {
	if block := firstByClass(root, "job-description__text"); block != nil {
		return innerHTML(block)
	}
	return html.UnescapeString(ldDescription)
}

// vagasJobIDPattern captures the native posting id from a /vagas/v<id>/<slug> URL.
var vagasJobIDPattern = regexp.MustCompile(`/vagas/v(\d+)`)

// vagasJobID extracts the native posting id from a job URL, or "" when the URL is not a job link.
func vagasJobID(u string) string {
	return firstSubmatch(vagasJobIDPattern, u)
}

// vagasPosting is the schema.org JobPosting decoded from a vagas.com job page. identifier.value
// is the native id but is redundant with the URL's v<id>, so only the display fields are read.
type vagasPosting struct {
	Title              string `json:"title"`
	Description        string `json:"description"`
	DatePosted         string `json:"datePosted"`
	HiringOrganization struct {
		Name string `json:"name"`
	} `json:"hiringOrganization"`
	JobLocation schemaPlace `json:"jobLocation"`
}
