package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/strelov1/freehire/internal/dict/skillvec"
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
	// skillWeights are the match sort's rarity weights, resolved once when the worker
	// starts. They describe the catalogue, not any single posting, so re-reading them
	// per wave would be a query every drain pass for a snapshot that changes only when
	// cmd/rollup-facets runs. The zero value is legitimate: documents then carry no
	// vector, exactly as they did before the match sort existed.
	skillWeights skillvec.Weights
}

// clusterKey identifies one role cluster. RoleClusterCountsFor/RoleClusterGeoFor match
// their two input sets as a cross product (see those queries' doc comments), so the
// caller keys results by the exact pair rather than trusting that every returned row
// belongs to a job in this wave.
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

	// Only clusters with more than one open row need a geography union — a singleton's
	// own geography is already what search.FromJob wrote, so RoleClusterGeo(For) on it is
	// a documented no-op. Narrowing the second query to just those pairs mirrors the
	// per-job askGeo gate this replaces.
	geoSlugs := make([]string, 0, len(slugs))
	geoPrints := make([]string, 0, len(prints))
	for key, c := range counts {
		if c.MassCount > 1 {
			geoSlugs = append(geoSlugs, key.companySlug)
			geoPrints = append(geoPrints, key.roleFingerprint)
		}
	}
	geo := map[clusterKey]db.RoleClusterGeoForRow{}
	if len(geoSlugs) > 0 {
		rows, err := ix.q.RoleClusterGeoFor(ctx, db.RoleClusterGeoForParams{
			CompanySlugs:     geoSlugs,
			RoleFingerprints: geoPrints,
		})
		if err != nil {
			log.Printf("search-drain: role-cluster geography for wave: %v", err)
		} else {
			for _, r := range rows {
				geo[clusterKey{r.CompanySlug, r.RoleFingerprint.String}] = r
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
		doc, err := search.FromJob(job, ix.skillWeights)
		if err != nil {
			return fmt.Errorf("build document (job %d): %w", job.ID, err)
		}
		repost, mass := int64(1), int64(1)
		// askGeo defaults to true because a lookup miss (the counts query failed, or the
		// job's cluster simply has no row) must not also suppress the geography merge
		// below: skipping it is destructive (the push replaces the stored union), not
		// conservative. Only a known singleton can safely skip.
		askGeo := true
		if job.RoleFingerprint.Valid && job.RoleFingerprint.String != "" {
			if c, ok := counts[clusterKey{job.CompanySlug, job.RoleFingerprint.String}]; ok {
				repost, mass = c.RepostCount, c.MassCount
				askGeo = mass > 1
			}
		}
		reality := jobview.ClassifyReality(job, time.Now(), int(repost), int(mass))
		doc.Reality = &reality
		if askGeo && job.RoleFingerprint.Valid && job.RoleFingerprint.String != "" {
			if g, ok := geo[clusterKey{job.CompanySlug, job.RoleFingerprint.String}]; ok {
				doc.MergeClusterGeography(g.Countries, g.Regions, g.Cities)
			}
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
