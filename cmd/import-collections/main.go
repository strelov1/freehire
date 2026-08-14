// Command import-collections populates the company-tag membership defined in
// internal/collections. For each entry it resolves the member companies — a static
// hand list (e.g. bigtech), a remote dataset (e.g. yc, unicorn), or a public
// register (the UK and NL visa-sponsor credentials) — matches them onto our
// companies, applies the entry's gate where it has one, writes
// companies.collections (reconciling only the tags it manages so any other tags
// survive), then denormalizes the result onto jobs.collections. It is a
// run-once-and-exit worker; a search reindex is required afterwards to surface the
// changes in the facet index.
//
// Idempotent: re-running with the same inputs writes the same membership and
// changes nothing on the second pass.
//
// Three things abort the run before it writes, all guarding the same hazard — that
// a broken source reconciles a tag off every company that holds it: a failed fetch
// or resolve, a payload that parses to no records, and a tag that would lose most
// of its current holders. The last covers the case the other two cannot see, where
// an upstream relabelling leaves the row count intact but matches nobody; override
// it with -force when the loss is genuinely correct.
//
// Run with -dry-run first when a gate or threshold has changed: it resolves,
// matches, and gates exactly as a real run does, reports the outcome per
// collection, and writes nothing.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/collections"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/safehttp"
	"github.com/strelov1/freehire/internal/worker"
)

// fetchTimeout bounds each dataset download so a stalled endpoint can't hang the run.
const fetchTimeout = 60 * time.Second

// maxDatasetBody bounds how many bytes fetchDataset reads from any one dataset
// response. Every dataset here is a company directory or register (yc-oss,
// unicorn list, the UK/NL/US sponsor registers) — a few MiB at most — but the
// URL can be operator-overridden per collection via <SLUG>_DATASET_URL, and
// several collections resolve theirs dynamically (Dataset.ResolveURL) from a
// third-party site (e.g. the monthly-republished GOV.UK register). A
// misconfigured override, a redirect to an unexpectedly large resource, or a
// misbehaving upstream host must not be able to buffer an unbounded response
// into memory on a run that otherwise executes unattended on a schedule.
const maxDatasetBody = 32 << 20 // 32 MiB

// dryRun resolves, matches, and gates exactly as a real run does, reports what it
// would write, and writes nothing.
var (
	dryRun = flag.Bool("dry-run", false, "resolve and match, report the outcome, write nothing")
	// force overrides the collapse guard, for the rare run where a tag genuinely
	// should lose most of its holders (retiring a collection, a real register purge).
	force = flag.Bool("force", false, "write even when a tag would lose most of its holders")
)

func main() {
	flag.Parse()
	worker.Main(run)
}

func run() int {
	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	// Resolve every collection's candidate company names/slugs up front. A failure
	// here aborts before any write: proceeding with a collection missing would
	// reconcile its tag off every company (a transient fetch error must not wipe
	// membership).
	resolved, err := resolveAll(ctx)
	if err != nil {
		log.Printf("import-collections: %v", err)
		return 1
	}

	q := db.New(pool)
	rows, err := q.ListCompanyCollections(ctx)
	if err != nil {
		log.Printf("import-collections: list companies: %v", err)
		return 1
	}

	p := plan(rows, resolved)

	// A dry run stops here, before the first write. Every threshold in the credential
	// gates is a guess until it is measured against the real catalogue — in
	// particular the single-token rule leans on hq_country, and if that column is
	// sparse the rule discards exactly the single-word brands worth surfacing. This
	// is how that gets answered before it can do damage rather than after.
	if *dryRun {
		report(p, len(rows))
		for _, c := range p.collapsed {
			log.Printf("import-collections: WOULD COLLAPSE %s — %d holders, %d would survive", c.slug, c.had, c.keeps)
		}
		log.Printf("import-collections: dry run — %d companies would be updated, nothing written", len(p.writes))
		return 0
	}

	// A tag losing most of its holders in one run is a broken source, not a change in
	// the world. The fetch and empty-parse aborts cannot see this case: a register
	// whose values were relabelled upstream still parses to its full row count, then
	// matches nobody. Stop before writing and make a human confirm.
	if len(p.collapsed) > 0 && !*force {
		for _, c := range p.collapsed {
			log.Printf("import-collections: %s would drop from %d holders to %d", c.slug, c.had, c.keeps)
		}
		log.Printf("import-collections: aborting before write — re-run with -dry-run to inspect, or -force if this is genuinely correct")
		return 1
	}

	for _, w := range p.writes {
		if err := q.SetCompanyCollections(ctx, w); err != nil {
			log.Printf("import-collections: set %q: %v", w.Slug, err)
			return 1
		}
	}

	propagated, err := q.PropagateCollectionsToJobs(ctx)
	if err != nil {
		log.Printf("import-collections: propagate to jobs: %v", err)
		return 1
	}

	report(p, len(rows))
	log.Printf("import-collections done: companies updated=%d, jobs updated=%d", len(p.writes), propagated)
	log.Printf("import-collections: run `make reindex` to surface jobs.collections in the search index")
	return 0
}

// report logs how each collection's candidates fell out. Credentials additionally
// report what their gate and ambiguity guard rejected — the numbers a dry run is
// read for — and how densely hq_country is populated, since the single-token rule
// depends on it.
func report(p planResult, companies int) {
	for _, c := range collections.All {
		s := p.stats[c.Slug]
		if c.Kind == collections.KindCredential {
			log.Printf("import-collections: %s matched=%d gated=%d ambiguous=%d unmatched=%d",
				c.Slug, s.Matched, s.Gated, s.Ambiguous, s.Unmatched)
			continue
		}
		log.Printf("import-collections: %s matched=%d unmatched=%d", c.Slug, s.Matched, s.Unmatched)
		// For a hand list the unmatched entries are actionable (a typo'd slug, or a
		// marquee company we don't ingest yet), so list them. Datasets have thousands
		// of unmatched names — only their count is logged, above.
		if len(s.UnmatchedNames) > 0 {
			log.Printf("import-collections: %s unmatched entries: %s", c.Slug, strings.Join(s.UnmatchedNames, ", "))
		}
	}
	log.Printf("import-collections: hq_country known for %d/%d companies (the single-token credential rule depends on it)",
		p.withHQCountry, companies)
}

// resolveAll returns, per collection slug, its candidate company names (or slugs
// for a hand list). A static-list collection resolves locally; a dataset
// collection is fetched and parsed. Any resolution failure is returned (the caller
// aborts rather than write a partial membership).
func resolveAll(ctx context.Context) (map[string][]collections.Record, error) {
	direct := &http.Client{Timeout: fetchTimeout}
	proxied, err := proxiedClient(fetchTimeout)
	if err != nil {
		return nil, err
	}
	resolved := make(map[string][]collections.Record, len(collections.All))
	for _, c := range collections.All {
		records, err := resolveOne(ctx, clientFor(c.Slug, direct, proxied), c)
		if err != nil {
			return nil, fmt.Errorf("resolve %q: %w", c.Slug, err)
		}
		resolved[c.Slug] = records
	}
	return resolved, nil
}

// proxiedCollectionSlugs are collections whose register blocks the prod datacenter
// IP outright, confirmed for us-h1b-sponsor on 2026-08-05: uscis.gov 403s a direct
// request even with a browser User-Agent, so it is IP reputation rather than a bot-
// detection header — the same class of problem internal/sources/proxy.go already
// solves for several board providers via the same SOURCES_PROXY_URL. Membership is
// opt-in; every other collection stays on the direct client.
var proxiedCollectionSlugs = map[string]struct{}{
	"us-h1b-sponsor": {},
}

// clientFor picks direct or proxied for a collection. A nil proxied — meaning
// SOURCES_PROXY_URL is unset — always falls back to direct, so every collection is
// reachable without a proxy configured.
func clientFor(slug string, direct, proxied *http.Client) *http.Client {
	if proxied == nil {
		return direct
	}
	if _, ok := proxiedCollectionSlugs[slug]; ok {
		return proxied
	}
	return direct
}

// proxiedClient builds an http.Client egressing through SOURCES_PROXY_URL, or
// returns nil when the variable is unset. An unparseable value is a fail-fast
// error rather than a silent fallback to the direct (blocked) IP — the same
// discipline as internal/sources/proxy.go's ApplyProxyEgress.
func proxiedClient(timeout time.Duration) (*http.Client, error) {
	raw := strings.TrimSpace(os.Getenv("SOURCES_PROXY_URL"))
	if raw == "" {
		return nil, nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("import-collections: invalid SOURCES_PROXY_URL")
	}
	return safehttp.NewClientWithProxy(timeout, u), nil
}

// resolveOne resolves a single collection's members. A dataset whose payload parses
// to nothing is an error, not an empty membership: a successful fetch of a source
// that has changed shape parses to zero rows just as convincingly as a genuinely
// empty register would, and letting it through would reconcile that collection's
// tag off every company. That is the same reasoning as the fetch-failure abort,
// applied to the failure mode a fetch check cannot see.
func resolveOne(ctx context.Context, client *http.Client, c collections.Collection) ([]collections.Record, error) {
	if c.Slugs != nil {
		return slugRecords(c.Slugs), nil
	}
	if c.Dataset == nil {
		return nil, errors.New("collection has no membership source")
	}
	if !c.Dataset.Valid() {
		return nil, errors.New("collection declares no single dataset source")
	}

	// Embedded, in-repo dataset (e.g. eastern-roots): parse the bundled bytes
	// directly, no network fetch.
	if len(c.Dataset.Data) > 0 {
		return nonEmpty(c.Dataset.Parse(c.Dataset.Data))
	}

	// Self-fetching source (e.g. the a16z directory, paginated across responses): it
	// owns its own reads, so there is no single body for the fetch-and-parse path
	// below. The zero-record guard still applies — it is the same failure mode.
	if c.Dataset.Records != nil {
		return nonEmpty(c.Dataset.Records(ctx, client))
	}

	url, err := datasetURL(ctx, client, c)
	if err != nil {
		return nil, err
	}
	body, err := fetchDataset(ctx, client, url)
	if err != nil {
		return nil, err
	}
	return nonEmpty(c.Dataset.Parse(body))
}

// nonEmpty turns a zero-record parse into an error. See resolveOne.
func nonEmpty(records []collections.Record, err error) ([]collections.Record, error) {
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, errors.New("dataset parsed to no records")
	}
	return records, nil
}

// slugRecords lifts a hand list of canonical company slugs into the record shape a
// dataset yields, so both membership sources reach the matcher as one type.
func slugRecords(slugs []string) []collections.Record {
	out := make([]collections.Record, len(slugs))
	for i, s := range slugs {
		out[i] = collections.Record{Name: s}
	}
	return out
}

// collapseRatio is the share of a tag's holders that may be lost in one run before
// the run is treated as a broken source rather than a genuine change. Membership
// moves by a few percent between runs; half of it vanishing at once has never been
// real data.
const collapseRatio = 0.5

// collapse is a managed tag about to lose most of the companies that hold it.
type collapse struct {
	slug       string
	had, keeps int
}

// planResult is the computed membership change plus the per-collection match stats.
type planResult struct {
	writes []db.SetCompanyCollectionsParams
	stats  map[string]collections.MatchStat
	// collapsed lists tags losing at least collapseRatio of their holders in this
	// run — the failure the fetch and empty-parse aborts cannot see. A source that
	// parses fine but whose values have been relabelled upstream yields a full row
	// count, matches nobody, and Reconcile then strips the tag from every company
	// that held it, with no error and a zero exit.
	collapsed []collapse
	// withHQCountry counts companies with a known headquarters country, reported
	// because the single-token credential rule is only as good as that column.
	withHQCountry int
}

// plan computes the membership change for every company: it matches each
// collection's candidates against the existing companies, then reconciles each
// company's managed tags (preserving any unmanaged ones), emitting a write only for
// the companies whose set actually changes. It is pure — all I/O lives in run.
// `resolved` maps a collection slug to its candidate company names/slugs.
func plan(rows []db.ListCompanyCollectionsRow, resolved map[string][]collections.Record) planResult {
	companies := make(map[string]collections.Company, len(rows))
	withHQCountry := 0
	for _, r := range rows {
		companies[r.Slug] = collections.Company{
			Slug:      r.Slug,
			Countries: r.Countries,
			HQCountry: r.HqCountry.String,
		}
		if r.HqCountry.String != "" {
			withHQCountry++
		}
	}

	want := make(map[string][]string)
	stats := make(map[string]collections.MatchStat, len(resolved))
	for _, c := range collections.All {
		matched, s := c.Members(resolved[c.Slug], companies)
		if c.Slugs != nil { // hand list: keep the unmatched entries for diagnostics
			s.UnmatchedNames = s.UnmatchedNames[:min(len(s.UnmatchedNames), collections.MaxLoggedUnmatched)]
		} else {
			s.UnmatchedNames = nil
		}
		stats[c.Slug] = s
		for _, slug := range matched {
			want[slug] = append(want[slug], c.Slug)
		}
	}

	collapsed := detectCollapse(rows, want)

	// Managed = live collection slugs plus retired ones, so Reconcile strips a
	// renamed/removed collection's stale tags (no wanted members) as well as
	// reconciling the current set.
	managed := append(collections.Slugs(), collections.RetiredSlugs...)
	var writes []db.SetCompanyCollectionsParams
	for _, r := range rows {
		next := collections.Reconcile(r.Collections, managed, want[r.Slug])
		if !slices.Equal(next, normalizedCurrent(r.Collections)) {
			writes = append(writes, db.SetCompanyCollectionsParams{Slug: r.Slug, Collections: next})
		}
	}

	return planResult{writes: writes, stats: stats, withHQCountry: withHQCountry, collapsed: collapsed}
}

// detectCollapse finds managed tags about to lose most of their holders. A tag
// nobody currently holds cannot collapse — that is a new collection's first run,
// not a broken source — and RetiredSlugs are skipped because losing every holder is
// exactly what retiring one is for.
func detectCollapse(rows []db.ListCompanyCollectionsRow, want map[string][]string) []collapse {
	had := map[string]int{}
	for _, r := range rows {
		for _, tag := range r.Collections {
			had[tag]++
		}
	}
	keeps := map[string]int{}
	for _, tags := range want {
		for _, tag := range tags {
			keeps[tag]++
		}
	}

	var out []collapse
	for _, c := range collections.All {
		before := had[c.Slug]
		if before == 0 {
			continue
		}
		if after := keeps[c.Slug]; float64(before-after) >= collapseRatio*float64(before) {
			out = append(out, collapse{slug: c.Slug, had: before, keeps: after})
		}
	}
	return out
}

// normalizedCurrent sorts a copy of the stored collections so the change check
// compares against Reconcile's sorted output (membership is a set; column order is
// not meaningful and must not trigger a spurious write).
func normalizedCurrent(current []string) []string {
	cp := slices.Clone(current)
	slices.Sort(cp)
	return cp
}

// datasetURL returns the address to fetch a collection's dataset from. A
// <SLUG>_DATASET_URL environment override wins (e.g. YC_DATASET_URL), then the
// dataset's own resolver where it has one — the GOV.UK register republishes at a
// new dated URL each month, so its address is discovered per run — then its fixed
// URL. The override is honoured ahead of the resolver so a run can be pinned to a
// downloaded snapshot without touching the network.
func datasetURL(ctx context.Context, client *http.Client, c collections.Collection) (string, error) {
	env := strings.ToUpper(strings.ReplaceAll(c.Slug, "-", "_")) + "_DATASET_URL"
	if u := os.Getenv(env); u != "" {
		return u, nil
	}
	if c.Dataset.ResolveURL != nil {
		return c.Dataset.ResolveURL(ctx, client)
	}
	return c.Dataset.URL, nil
}

// fetchDataset downloads a dataset URL. The URLs are ours or resolved from a source
// we control, not user input, so a plain client is appropriate.
func fetchDataset(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("dataset %s: status %d", url, resp.StatusCode)
	}
	// Capped on the read, not on Content-Length: that header is the server's
	// claim, not a promise about what the stream delivers.
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxDatasetBody+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxDatasetBody {
		return nil, fmt.Errorf("dataset %s: body exceeds %d bytes", url, maxDatasetBody)
	}
	return body, nil
}
