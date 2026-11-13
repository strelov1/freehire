package pipeline

import (
	"context"
	"log"
	"strings"
	"sync"

	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/job/logodomain"
)

// boardTally is one writer's share of a board's outcome; writeBoard merges them.
type boardTally struct {
	st       Stats
	rej      rejections
	firstErr error
	// refreshIDs holds listed identities needing an open-only liveness refresh.
	refreshIDs []string
	// coveredIDs holds listed identities the coverage gate kept out of the write path.
	coveredIDs []string
}

// writeBoard writes raw through saveConcurrency workers, each tallying into its own
// boardTally, and merges them. Everything a write touches is safe to share: the Store already
// serves boards concurrently, and canon and covered are read-only lookups built up front.
func (r Runner) writeBoard(ctx context.Context, e sources.CompanyEntry, raw []sources.Job, seen map[string]bool, covered aggregatorCoverage, canon companyCanonical) boardTally {
	tallies := make([]boardTally, min(saveConcurrency, max(len(raw), 1)))
	jobs := make(chan sources.Job)
	var wg sync.WaitGroup
	for i := range tallies {
		wg.Add(1)
		go func(t *boardTally) {
			defer wg.Done()
			for j := range jobs {
				r.ingestOne(ctx, e, j, seen, covered, canon, t)
			}
		}(&tallies[i])
	}
	for _, j := range raw {
		jobs <- j
	}
	close(jobs)
	wg.Wait()

	var merged boardTally
	for _, t := range tallies {
		merged.st.Ingested += t.st.Ingested
		merged.st.Skipped += t.st.Skipped
		merged.st.ATSCovered += t.st.ATSCovered
		merged.st.Unreadable += t.st.Unreadable
		merged.rej.merge(t.rej)
		if merged.firstErr == nil {
			merged.firstErr = t.firstErr
		}
		merged.refreshIDs = append(merged.refreshIDs, t.refreshIDs...)
		merged.coveredIDs = append(merged.coveredIDs, t.coveredIDs...)
	}
	return merged
}

// ingestOne applies one fetched posting: counts an unreadable one, refreshes a re-listed one,
// and saves the rest, tallying the outcome into t.
func (r Runner) ingestOne(ctx context.Context, e sources.CompanyEntry, j sources.Job, seen map[string]bool, covered aggregatorCoverage, canon companyCanonical, t *boardTally) {
	// A posting the listing named but whose detail could not be read. There is nothing to
	// write, but a stored row must retain the listing's liveness evidence. Count it
	// as unreadable even after refreshing: the sweep still has incomplete evidence. Handled ahead of the filter so it never enters the rejection denominator,
	// exactly like the SeenRefresh branch below.
	if j.Unreadable {
		t.st.Unreadable++
		_, externalID := jobIdentity(e, j)
		t.refreshIDs = append(t.refreshIDs, externalID)
		return
	}
	// A HydratingSource marks an already-ingested posting it re-listed but did not
	// re-fetch: refresh its liveness by identity instead of re-upserting content-less
	// (which would wipe the description/facets hydrated when it was new).
	if j.SeenRefresh {
		// A refresh faces the same catalogue filter as a write, so a posting the
		// dictionary turns away ages out of the catalogue instead of being kept alive
		// by its own re-listing. Judged on the STORED tech evidence, not on the
		// content-less listing: the dictionary matches its terms anywhere in a title,
		// and a title alone would condemn engineering roles whose evidence lives in
		// the description they were hydrated from.
		t.rej.candidate()
		_, externalID := jobIdentity(e, j)
		if outOfCatalogueTitle(j.Title, seen[externalID]) {
			t.rej.reject(j.Title)
			return
		}
		// A covered employer's re-listed copy is closed like a written one would be. A listing
		// naming no company (a sitemap refresh) cannot be judged here and keeps its touch.
		if covered != nil && j.Company != "" && covered(companySlugOf(canon, j.Company)) {
			t.st.ATSCovered++
			t.coveredIDs = append(t.coveredIDs, externalID)
			return
		}
		slug, err := r.touch(ctx, e, j)
		if err != nil {
			t.st.Skipped++
			if t.firstErr == nil {
				t.firstErr = err
			}
			return
		}
		t.st.Ingested++
		// A re-listed posting still states its employer's website, and the employers whose
		// postings were all stored before the field existed are only ever reached here. The
		// stored row's slug, since a slug rule may have moved since it was written.
		r.recordCompanyWebsite(ctx, e, slug, j.CompanyWebsite)
		return
	}
	r.saveOne(ctx, e, j, covered, canon, t)
}

// saveOne normalizes one posting, puts it through the catalogue filter, and saves what
// survives, tallying the outcome into t and keeping the first error for the board's
// skip-summary log line. Both the buffered (ingestBoard) and the streaming (ingestStream)
// save loops share it so the normalize→filter→save→count step behaves identically; the
// SeenRefresh and Removed branches stay on the caller, which is why neither reaches the
// filter's candidate denominator.
func (r Runner) saveOne(ctx context.Context, e sources.CompanyEntry, j sources.Job, covered aggregatorCoverage, canon companyCanonical, t *boardTally) {
	fromCatalogue := false
	if j.Description == "" {
		if l, ok := r.Store.(StoredBodyLookup); ok {
			source, externalID := jobIdentity(e, j)
			body, err := l.StoredDescription(ctx, source, externalID)
			if err != nil {
				// Saving anyway would derive from no body over a stored one; the next crawl
				// retries. The posting was listed, so its liveness is refreshed through the
				// covered-refresh path — otherwise the post-run sweep could close a live row.
				t.st.Skipped++
				if t.firstErr == nil {
					t.firstErr = err
				}
				t.refreshIDs = append(t.refreshIDs, externalID)
				return
			}
			j.Description = body
			fromCatalogue = body != ""
		}
	}
	dj, err := normalizeJob(e, j, canon, fromCatalogue)
	if err != nil {
		t.st.Skipped++
		if t.firstErr == nil {
			t.firstErr = err
		}
		return
	}
	t.rej.candidate()
	if outOfCatalogue(dj) {
		t.rej.reject(dj.Fields().Title)
		return
	}
	if covered != nil && covered(dj.Fields().CompanySlug) {
		t.st.ATSCovered++
		t.coveredIDs = append(t.coveredIDs, dj.Fields().ExternalID)
		return
	}
	if err := r.save(ctx, dj, j.ApplyForm); err != nil {
		t.st.Skipped++
		if t.firstErr == nil {
			t.firstErr = err
		}
		return
	}
	t.st.Ingested++
	r.recordCompanyWebsite(ctx, e, dj.Fields().CompanySlug, j.CompanyWebsite)
}

// recordCompanyWebsite writes a posting's stated employer website onto the company it was
// filed under, which after alias resolution is the canonical slug. It is best-effort: the
// posting is already saved, and a missed website costs a merge signal, not a posting.
func (r Runner) recordCompanyWebsite(ctx context.Context, e sources.CompanyEntry, companySlug, raw string) {
	website := companyWebsite(raw)
	if website == "" || companySlug == "" {
		return
	}
	rec, ok := r.Store.(CompanyWebsiteRecorder)
	if !ok {
		return
	}
	if err := rec.RecordCompanyWebsite(ctx, companySlug, website); err != nil {
		log.Printf("ingest: %s board %q: record website of %q: %v", e.Provider, e.Board, companySlug, err)
	}
}

// companyWebsite keeps a stated website only when logodomain.Domain reads a domain from it,
// the same reader the logo map and the duplicate report apply to the stored value. A feed's
// free-text field holds "n/a" or a relative path often enough that storing it as given
// would put junk where those read a domain. An e-mail address is refused as well: its
// domain is often a mail provider's.
func companyWebsite(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.Contains(raw, "@") || logodomain.Domain(raw) == "" {
		return ""
	}
	return raw
}

// refreshCovered refreshes the liveness of whichever listed, unwritten postings the
// catalogue already holds (see CoveredRefresher). It only logs a failure: a row the refresh
// misses is left exactly as stale as it was, and is never closed because of it. It returns how
// many rows were refreshed.
func (r Runner) refreshCovered(ctx context.Context, e sources.CompanyEntry, externalIDs []string) (refreshed, failed int, companies []string) {
	cr, ok := r.Store.(CoveredRefresher)
	if !ok || len(externalIDs) == 0 {
		return 0, 0, nil
	}
	companies, err := cr.RefreshCovered(ctx, e.Provider, externalIDs)
	if err != nil {
		log.Printf("ingest: %s board %q (%s): refresh %d listed postings: %v",
			e.Provider, e.Board, e.Company, len(externalIDs), err)
		return 0, len(externalIDs), nil
	}
	return len(companies), 0, companies
}

// closeCovered closes the stored open rows behind the postings the coverage gate kept out of the
// write path (see CoveredCloser), falling back to refreshing them for a Store without it. A
// closed row counts in Stats.Refreshed: the listing named it and the run accounted for it, which
// is what lets a provider whose every posting was covered still sweep. A failure counts in
// Stats.RefreshFailed, which withholds the close scopes as a failed refresh does.
func (r Runner) closeCovered(ctx context.Context, e sources.CompanyEntry, externalIDs []string) (closed, failed int, companies []string) {
	cc, ok := r.Store.(CoveredCloser)
	if !ok {
		return r.refreshCovered(ctx, e, externalIDs)
	}
	if len(externalIDs) == 0 {
		return 0, 0, nil
	}
	companies, err := cc.CloseCovered(ctx, e.Provider, externalIDs)
	if err != nil {
		log.Printf("ingest: %s board %q (%s): close %d covered postings: %v",
			e.Provider, e.Board, e.Company, len(externalIDs), err)
		return 0, len(externalIDs), nil
	}
	return len(companies), 0, companies
}

// aggregatorGate reports whether the ingest-time coverage gate applies to e's board: a
// Coverage port must be wired AND the provider must be an aggregator (an ATS or
// single-company board is never gated, regardless of what Coverage would report — see
// TestRunIgnoresCoverageForATSProvider). When it applies, it also returns the aggregator
// provider list and the board's market countries (sources.MarketCountries; nil when the
// aggregator is not national) both callers need to pass to CoverageLookup. The
// classification MUST come from sources.Taxonomy(), not r.Registry: a keyed adapter's
// credential lives only on the crawl host, so a keyless host's classification would wrongly
// read it as non-aggregator (see the aggregator-ats-coverage-skip design doc).
func (r Runner) aggregatorGate(e sources.CompanyEntry) (aggregators, countries []string, applies bool) {
	if r.Coverage == nil {
		return nil, nil, false
	}
	taxonomy := sources.Taxonomy()
	if !sources.IsAggregator(taxonomy, e.Provider) {
		return nil, nil, false
	}
	return sources.AggregatorProviders(taxonomy), sources.MarketCountries(taxonomy, e), true
}
