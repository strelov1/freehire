package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/strelov1/freehire/internal/ingest/applyform"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/search/search"
	"github.com/strelov1/freehire/internal/search/searchdrain"
)

// searchIndexer adapts the search client to searchdrain.Indexer: build each job's
// document from its persisted row (search.FromJob) and push the whole wave into the
// live facet index as one Meili task. It also attaches the job-reality signal and
// widens the canon's geography with its role cluster's — the same enrichment the old
// inline ingest push and cmd/embed's semantic indexer both attach — so a job first
// indexed here carries them immediately rather than only after the next full reindex.
type searchIndexer struct {
	client *search.Client
	q      *db.Queries
}

// clusterKey identifies one role cluster, which is still how the job-reality counts are
// grouped (geography moved to the duplicate closure, keyed by job id). RoleClusterCountsFor
// matches its two input sets as a cross product (see that query's doc comment), so the caller
// keys results by the exact pair rather than trusting that every returned row belongs to a job
// in this wave.
type clusterKey struct {
	companySlug     string
	roleFingerprint string
}

// sourceTypeOf is each source's `source_type` document attribute, resolved once from the
// adapter taxonomy (which needs no credential, so every host answers alike).
var sourceTypeOf = sources.SourceTypes(sources.Taxonomy())

// unseenWindows is how long each crawled source's postings may go unseen before search
// withholds them (search.Withheld), resolved once from the same adapter taxonomy.
var unseenWindows = search.UnseenWindows(sources.UnseenWindows(sources.Taxonomy(), search.UnseenWindow))

func (ix searchIndexer) IndexBatch(ctx context.Context, jobs []db.Job) error {
	// Resolve every job's role-cluster counts (and, for clusters with more than one open
	// row, geography) in two queries scoped to the whole wave instead of one
	// RoleClusterCount/RoleClusterGeo round trip per job — at the documented default wave
	// size (500) that was up to ~1000 sequential DB calls per drain pass.
	slugs := make([]string, 0, len(jobs))
	prints := make([]string, 0, len(jobs))
	seen := map[clusterKey]struct{}{}
	for _, job := range jobs {
		if !job.RoleFingerprint.Valid || job.RoleFingerprint.String == "" {
			continue
		}
		key := clusterKey{job.CompanySlug, job.RoleFingerprint.String}
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		slugs = append(slugs, job.CompanySlug)
		prints = append(prints, job.RoleFingerprint.String)
	}
	counts := map[clusterKey]db.RoleClusterCountsForRow{}
	if len(slugs) > 0 {
		rows, err := ix.q.RoleClusterCountsFor(ctx, db.RoleClusterCountsForParams{
			CompanySlugs:     slugs,
			RoleFingerprints: prints,
		})
		if err != nil {
			log.Printf("search-drain: role-cluster counts for wave: %v", err)
		} else {
			for _, r := range rows {
				counts[clusterKey{r.CompanySlug, r.RoleFingerprint.String}] = r
			}
		}
	}

	// Geography is asked for EVERY job in the wave, keyed by id. There is no cheap
	// pre-filter for "this row represents nobody" the way mass_count was for a singleton
	// cluster — and wanting one was the hazard: the gate it fed had to default to asking,
	// because skipping the merge is destructive (the push replaces the stored union) rather
	// than conservative. Asking unconditionally deletes that reasoning. A row that
	// represents nobody answers with its own geography, so merging it is a no-op.
	geoIDs := make([]int64, 0, len(jobs))
	for _, job := range jobs {
		geoIDs = append(geoIDs, job.ID)
	}
	geo := map[int64]db.DuplicateClosureGeoForRow{}
	if len(geoIDs) > 0 {
		rows, err := ix.q.DuplicateClosureGeoFor(ctx, geoIDs)
		if err != nil {
			// Not a degradation this wave may absorb, unlike the counts above. The push
			// is a field-level update and countries/regions/cities are always in the
			// payload, so a document built without the closure union does not merely
			// fail to widen a collapsed multi-city role — it OVERWRITES the widened
			// values already indexed with the canon's own (see
			// search.JobDocument.MergeClosureGeography). Skip the wave; the lease
			// expires and a later run reads the geography again.
			return fmt.Errorf("%w: duplicate-closure geography for wave: %v",
				searchdrain.ErrSkipWave, err)
		}
		for _, r := range rows {
			geo[r.OwnerID] = r
		}
	}

	// The description vectors carry the same hazard as the geography: a document built
	// without them sends `desc: null`, which CLEARS the vectors already indexed. So a failed
	// read skips the wave rather than pushing documents that would erase them.
	desc, err := search.ChunkVectorsForJobs(ctx, ix.q, geoIDs)
	if err != nil {
		return fmt.Errorf("%w: %v", searchdrain.ErrSkipWave, err)
	}

	// The application forms' question counts. A document without one would not erase a
	// stored count (the push merges fields), but a document indexed for the first time
	// would go without until something queued it again — so a failed read skips the wave
	// like the two reads above, and the lease brings it back.
	questions, err := applyform.QuestionCounts(ctx, ix.q, geoIDs)
	if err != nil {
		return fmt.Errorf("%w: apply-form question counts for wave: %v", searchdrain.ErrSkipWave, err)
	}

	docs := make([]search.JobDocument, 0, len(jobs))
	now := time.Now()
	for _, job := range jobs {
		// A job search withholds — no resolved category, no posting body, or a source that
		// stopped showing it (search.Withheld) — never enters the index; cmd/reindex's
		// splitJobs applies the same predicate to the full-rebuild path. This skips rather
		// than deletes: an entry already in the index goes stale here the same way a closed
		// job's does between drain waves (internal/search/searchdrain/AGENTS.md) — the next
		// full reindex swap is the reconciler for both.
		if search.Withheld(job, now, unseenWindows) {
			continue
		}
		doc, err := search.FromJob(job, desc[job.ID], sourceTypeOf(job.Source))
		if err != nil {
			return fmt.Errorf("build document (job %d): %w", job.ID, err)
		}
		doc.AttachApplyQuestions(questions)
		repost, mass := int64(1), int64(1)
		if job.RoleFingerprint.Valid && job.RoleFingerprint.String != "" {
			if c, ok := counts[clusterKey{job.CompanySlug, job.RoleFingerprint.String}]; ok {
				repost, mass = c.RepostCount, c.MassCount
			}
		}
		reality := jobview.ClassifyReality(job, now, int(repost), int(mass))
		doc.Reality = &reality
		if g, ok := geo[job.ID]; ok {
			doc.MergeClosureGeography(jobview.ClosureGeo{Countries: g.Countries, Regions: g.Regions, Cities: g.Cities, Metros: g.Metros})
		}
		docs = append(docs, doc)
	}
	return ix.client.IndexJobs(ctx, docs)
}

// facetDeleter rechecks current state before removing documents. A missing row
// still needs removal, while an open row must survive its old queued closure.
type facetDeleter struct {
	client *search.Client
	q      *db.Queries
}

func (d facetDeleter) DeleteBatch(ctx context.Context, jobIDs []int64) error {
	rows, err := d.q.PrepareSearchDeletions(ctx, jobIDs)
	if err != nil {
		return fmt.Errorf("%w: prepare removals: %v", searchdrain.ErrSkipWave, err)
	}
	if len(rows) == 0 {
		return nil
	}
	var closed, removed []int64
	for _, row := range rows {
		if row.HardDelete {
			removed = append(removed, row.JobID)
		} else {
			closed = append(closed, row.JobID)
		}
	}
	if err := d.client.CloseJobs(ctx, closed); err != nil {
		return fmt.Errorf("%w: update closure markers: %v", searchdrain.ErrSkipWave, err)
	}
	return d.client.DeleteJobs(ctx, removed)
}
