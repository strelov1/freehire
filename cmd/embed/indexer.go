package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/embed"
	"github.com/strelov1/freehire/internal/jobview"
	"github.com/strelov1/freehire/internal/search"
)

// searchIndexer adapts the search client to embed.Indexer: embed+upsert a batch of open
// jobs' vectors in place (no swap), or remove a batch of closed jobs' documents. It
// builds each document from the persisted row (search.FromJob) so a re-embedded job
// keeps its enrichment facets — the same path the incremental ingest indexer uses. It
// also attaches the job-reality signal so a job first indexed here carries reality.class
// immediately, rather than only after the next full reindex.
type searchIndexer struct {
	client *search.Client
	q      *db.Queries
	// pgOnly makes the indexer embed to Postgres ONLY: it computes vectors (persisted by
	// the store's CompleteOpen) but never writes Meilisearch, so a bulk backfill is not
	// gated by Meili's serial task queue. The semantic index is rebuilt from Postgres
	// afterwards with `reindex --semantic --from-pg`.
	pgOnly bool
}

func (ix searchIndexer) IndexOpen(ctx context.Context, jobs []db.Job) (map[int64][]float32, map[int64][]embed.ChunkEmbedding, error) {
	docs := make([]search.JobDocument, 0, len(jobs))
	for _, job := range jobs {
		doc, err := search.FromJob(job)
		if err != nil {
			return nil, nil, fmt.Errorf("build document (job %d): %w", job.ID, err)
		}
		// The job-reality signal is a Meili-document facet, so pg-only mode (which never
		// builds a Meili doc) skips it — and its per-job cluster-count query. In the
		// normal path attach it so an incrementally-embedded job carries reality.class
		// immediately; a cluster-count lookup failure degrades to a unique role (1,1).
		if !ix.pgOnly {
			repost, mass := int64(1), int64(1)
			// askGeo defaults to true because a FAILED count must not also suppress the
			// geography merge below: skipping it is destructive (the push replaces the
			// stored union), not conservative. Only a known singleton can safely skip.
			askGeo := true
			if c, err := ix.q.RoleClusterCount(ctx, db.RoleClusterCountParams{
				CompanySlug:     job.CompanySlug,
				RoleFingerprint: job.RoleFingerprint,
			}); err != nil {
				log.Printf("embed: role-cluster count for job %d: %v", job.ID, err)
			} else {
				repost, mass = c.RepostCount, c.MassCount
				askGeo = mass > 1
			}
			reality := jobview.ClassifyReality(job, time.Now(), int(repost), int(mass))
			doc.Reality = &reality
			// Widen the canon with its cluster's geography — the push is a field-level
			// document update, so omitting this replaces the reindex's union with the
			// canon's own narrow set. mass counts the cluster's open rows: at a known 1
			// there is nothing to widen with. This sits inside the !pgOnly branch alongside
			// the count it reuses — pg-only builds no Meili document at all.
			if askGeo {
				if g, err := ix.q.RoleClusterGeo(ctx, db.RoleClusterGeoParams{
					CompanySlug:     job.CompanySlug,
					RoleFingerprint: job.RoleFingerprint,
				}); err != nil {
					log.Printf("embed: role-cluster geography for job %d: %v", job.ID, err)
				} else {
					doc.MergeClusterGeography(g.Countries, g.Regions, g.Cities)
				}
			}
		}
		docs = append(docs, doc)
	}
	// pg-only: compute vectors only (the store persists them to Postgres); Meili is
	// rebuilt from Postgres afterwards via `reindex --semantic --from-pg`. Otherwise embed
	// the whole batch AND upsert it into the live semantic index as ONE Meili task. This
	// single-vector path is UNCHANGED by the chunk pipeline below — same inputs, same
	// Meili/Postgres side effects, same error behavior.
	var (
		vectors map[int64][]float32
		err     error
	)
	if ix.pgOnly {
		vectors, err = ix.client.EmbedJobs(ctx, docs)
	} else {
		vectors, err = ix.client.IndexSemanticJobs(ctx, docs)
	}
	if err != nil {
		return nil, nil, err
	}
	// Additive: each job's chunked embeddings for the pgvector-backed
	// job_semantic_chunks table (design.md Decisions 1/1a/2.2) — computed from the raw
	// jobs, not the Meili documents built above, and never touching Meilisearch
	// regardless of pgOnly. Bundled into this same IndexOpen call (rather than a
	// separate Indexer method) so a chunk-embed failure fails the whole batch the same
	// way a vector-embed failure does, reusing internal/embed's existing
	// batch-then-per-item-fallback machinery instead of a second failure path.
	chunkVecs, err := ix.client.EmbedJobChunks(ctx, jobs)
	if err != nil {
		return nil, nil, fmt.Errorf("embed chunks: %w", err)
	}
	chunks := make(map[int64][]embed.ChunkEmbedding, len(chunkVecs))
	for jobID, cs := range chunkVecs {
		out := make([]embed.ChunkEmbedding, len(cs))
		for i, c := range cs {
			out[i] = embed.ChunkEmbedding{ChunkIndex: c.ChunkIndex, Vector: c.Vector}
		}
		chunks[jobID] = out
	}
	return vectors, chunks, nil
}

func (ix searchIndexer) RemoveClosed(ctx context.Context, ids []int64) error {
	// pg-only mode leaves Meili untouched: the closed job's stamp+vector are cleared in
	// Postgres by CompleteClosed, and the --from-pg rebuild simply omits closed jobs.
	if ix.pgOnly {
		return nil
	}
	return ix.client.DeleteSemanticJobs(ctx, ids)
}
