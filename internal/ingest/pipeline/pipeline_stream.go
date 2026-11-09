package pipeline

import (
	"context"
	"log"
	"sync"

	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/job/job"
	"github.com/strelov1/freehire/internal/job/jobderive"
	"github.com/strelov1/freehire/internal/platform/externalid"
)

// ingestStream drives a streaming board: it persists each posting the adapter emits as it is
// crawled, so progress is durable mid-run. The emit sink normalizes and saves under a mutex
// (the adapter may emit concurrently) and tallies the same ingested/skipped counts as the
// buffered path; a board-level FetchStream error counts the board failed but keeps whatever was
// already saved (the 48h unseen-sweep guards against a short crawl closing the un-reached tail).
func (r Runner) ingestStream(ctx context.Context, e sources.CompanyEntry, ss sources.StreamingSource) (Stats, []string) {
	var (
		mu     sync.Mutex
		t      boardTally
		total  int
		listed []string // every posting the stream named, for the listing fingerprint
	)
	// The buffered path resolves a whole board's companies in one batched call because it
	// already has every posting in hand; a streaming source (jobtech is the one aggregator on
	// this path today) never does, so this resolves — and memoizes — one company at a time as
	// postings arrive. cache is read/written only from within emit, which already serializes
	// access via mu, so it needs no locking of its own.
	canon := r.streamCompanyCanonical(ctx, e)
	covered := r.streamAggregatorCoverage(ctx, e)
	emit := func(j sources.Job) {
		mu.Lock()
		defer mu.Unlock()
		total++
		if !j.Removed {
			listed = append(listed, j.ExternalID)
		}
		// A self-closing source emits removed postings: close by identity instead of
		// upserting. The Store must implement Closer (the ingest dbStore does); a Store
		// without it simply drops the removal (e.g. a test fake that never sees them).
		if j.Removed {
			c, ok := r.Store.(Closer)
			if !ok {
				return
			}
			source, externalID := jobIdentity(e, j)
			if err := c.Close(ctx, source, externalID); err != nil {
				t.st.Skipped++
				if t.firstErr == nil {
					t.firstErr = err
				}
				return
			}
			// A close is not counted as ingested — Stats.Ingested is saved jobs only.
			return
		}
		r.saveOne(ctx, e, j, covered, canon, &t)
	}

	err := ss.FetchStream(ctx, e, emit)
	st := t.st
	st.Refreshed, st.RefreshFailed, _ = r.refreshCovered(ctx, e, t.refreshIDs)
	t.rej.log(e)
	if st.Skipped > 0 {
		log.Printf("ingest: %s board %q (%s): skipped %d/%d jobs on save error (e.g. %v)",
			e.Provider, e.Board, e.Company, st.Skipped, total, t.firstErr)
	}
	if st.ATSCovered > 0 {
		log.Printf("ingest: %s board %q (%s): skipped %d/%d postings — company already covered by a non-aggregator source",
			e.Provider, e.Board, e.Company, st.ATSCovered, total)
	}
	st.Rejected = t.rej.rejected
	markInterrupted(ctx, &st)
	if err != nil {
		log.Printf("ingest: %s board %q (%s) failed after %d saved: %v",
			e.Provider, e.Board, e.Company, st.Ingested, err)
		st.Failed = 1
	}
	return st, listed
}

// completeListingFingerprint is the listingFingerprint of a board's crawl when that crawl read
// the whole listing, and "" — which leaves the quiet-board backoff's state untouched — when it
// cannot vouch for that: the board failed, the run's deadline interrupted it, the context ended,
// a posting it listed could not be read (Unreadable) or saved (Skipped) — both are retried by
// the next crawl, which a quiet pause would postpone — or the source is self-closing and emits
// changes rather than its listing.
func (r Runner) completeListingFingerprint(ctx context.Context, src sources.Source, listed []string, st Stats) string {
	if ctx.Err() != nil || st.Failed > 0 || st.Interrupted > 0 || st.Unreadable > 0 || st.Skipped > 0 ||
		src == nil || sources.SelfCloses(src) {
		return ""
	}
	return listingFingerprint(listed)
}

// jobIdentity is the dedup key a posting persists under: the provider is the source, and the
// external id is namespaced by board so two companies on one platform cannot collide. Both
// the upsert (via normalizeJob) and the stream-driven close derive identity here, so a
// removal closes exactly the row a live emit would have upserted.
func jobIdentity(e sources.CompanyEntry, j sources.Job) (source, externalID string) {
	return e.Provider, externalid.Namespace(e.Board, j.ExternalID)
}

// normalizeJob turns a raw posting into the Job aggregate through the factory: the
// platform becomes the source and the external id is namespaced by board (so two
// companies on one platform cannot collide), and job.New derives the slugs and the
// dictionary facets internally — so ingest, the moderator write path, and Telegram
// extraction all produce identical facets from the one door. It returns
// job.ErrInvalidDraft for a posting with no title/identity, which the caller skips.
func normalizeJob(e sources.CompanyEntry, j sources.Job, canon companyCanonical, bodyFromCatalogue bool) (job.Job, error) {
	source, externalID := jobIdentity(e, j)
	return job.New(job.Draft{
		// Empty unless the registry re-keys this employer, and empty is what Draft reads as
		// "keep what jobderive derived". Resolving here, at construction, is what makes the
		// slug saveOne gates on the same value it stores.
		CanonicalCompanySlug: canonicalCompanySlug(canon, j.Company),
		BodyFromCatalogue:    bodyFromCatalogue,
		Input: jobderive.Input{
			Source:             source,
			ExternalID:         externalID,
			Title:              j.Title,
			Company:            j.Company,
			Location:           j.Location,
			Description:        j.Description,
			WorkMode:           j.WorkMode,
			Countries:          j.Countries,
			FallbackCountries:  j.FallbackCountries,
			Seniority:          j.Seniority,
			Category:           j.Category,
			EmploymentType:     j.EmploymentType,
			Engagement:         j.Engagement,
			Skills:             j.Skills,
			ExperienceYearsMin: j.ExperienceYearsMin,
			EducationLevel:     j.EducationLevel,
			EnglishLevel:       j.EnglishLevel,
			SalaryMin:          j.SalaryMin,
			SalaryMax:          j.SalaryMax,
			SalaryCurrency:     j.SalaryCurrency,
			SalaryPeriod:       j.SalaryPeriod,
			IsTechHint:         j.IsTechHint,
		},
		URL:                 j.URL,
		Remote:              j.Remote,
		PostedAt:            j.PostedAt,
		ApplicationDeadline: j.ApplicationDeadline,
	})
}
