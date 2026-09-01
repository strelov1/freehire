package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/search/search"
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
			log.Printf("search-drain: duplicate-closure geography for wave: %v", err)
		} else {
			for _, r := range rows {
				geo[r.OwnerID] = r
			}
		}
	}

	docs := make([]search.JobDocument, 0, len(jobs))
	for _, job := range jobs {
		// A job whose category neither the title dictionary nor the LLM ever resolved
		// (search.CategoryUnresolved), or one with no posting body at all
		// (search.DescriptionMissing), never enters the index — see cmd/reindex's splitJobs
		// for the same rules applied to the full-rebuild path. This skips rather than
		// deletes: if the row is a rare pre-existing index entry from before these rules
		// existed, it goes stale here the same way a closed job's does between drain
		// waves (internal/search/searchdrain/AGENTS.md) — the next full reindex swap is the
		// reconciler for both.
		if search.CategoryUnresolved(job) || search.DescriptionMissing(job) {
			continue
		}
		doc, err := search.FromJob(job)
		if err != nil {
			return fmt.Errorf("build document (job %d): %w", job.ID, err)
		}
		repost, mass := int64(1), int64(1)
		if job.RoleFingerprint.Valid && job.RoleFingerprint.String != "" {
			if c, ok := counts[clusterKey{job.CompanySlug, job.RoleFingerprint.String}]; ok {
				repost, mass = c.RepostCount, c.MassCount
			}
		}
		reality := jobview.ClassifyReality(job, time.Now(), int(repost), int(mass))
		doc.Reality = &reality
		if g, ok := geo[job.ID]; ok {
			doc.MergeClosureGeography(g.Countries, g.Regions, g.Cities)
		}
		docs = append(docs, doc)
	}
	return ix.client.IndexJobs(ctx, docs)
}

// facetDeleter adapts the search client to searchdrain.Deleter.
//
// Unlike searchIndexer it needs no queries: a removal is identified by primary key alone,
// and by the time an entry drains its job row is often gone (cmd/prune hard-deletes).
type facetDeleter struct {
	client *search.Client
}

func (d facetDeleter) DeleteBatch(ctx context.Context, jobIDs []int64) error {
	return d.client.DeleteJobs(ctx, jobIDs)
}
