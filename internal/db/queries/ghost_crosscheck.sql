-- name: ListAggregatorJobsForCrosscheck :many
-- Keyset page of open aggregator postings to cross-check, ordered by id so the scan
-- resumes exactly where it stopped. The worker buffers a page and groups it by company
-- itself, so a company's board is read once rather than once per posting; ordering by
-- company instead would make the keyset cursor non-unique and risk skipping rows.
--
-- The caller passes the aggregator provider names (sources.AggregatorProviders): the
-- provider taxonomy lives in Go adapter markers, not in the database, and copying it
-- into SQL would leave two lists to keep in step.
SELECT j.id, j.company_slug, j.title, j.ats_absent_at
FROM jobs j
WHERE j.closed_at IS NULL
  AND j.source = ANY(sqlc.arg(aggregator_sources)::text[])
  AND j.id > sqlc.arg(after_id)
ORDER BY j.id
LIMIT sqlc.arg(page_size);

-- name: ListCompanyBoardTitles :many
-- The open titles a company carries on its OWN board — a source of kind `ats` or
-- `company`, never an aggregator. The worker turns these into role keys and asks
-- whether an aggregator posting's key is among them.
--
-- This is the COVERAGE GATE's data as well as its answer. An empty result means we do
-- not crawl this company's board at all, and the worker must then stamp nothing:
-- absence is evidence only where we looked, and without the gate the signal would
-- report our own blind spots as the employer's fault.
SELECT j.title
FROM jobs j
WHERE j.closed_at IS NULL
  AND j.company_slug = sqlc.arg(company_slug)
  AND j.source = ANY(sqlc.arg(board_sources)::text[]);

-- name: StampJobATSAbsent :exec
-- Record that this posting's role was not found on its company's own board, as of now.
-- Re-stamped on every run, so the reader can ignore a stamp that has aged out and a
-- worker that has stopped falls silent instead of accusing from a frozen snapshot.
UPDATE jobs SET ats_absent_at = now() WHERE id = ANY(sqlc.arg(job_ids)::bigint[]);

-- name: ClearJobATSAbsent :exec
-- Withdraw the absence stamp: the role turned up on the company's board after all.
-- Scoped to rows that carry a stamp so a run over a healthy company writes nothing.
UPDATE jobs SET ats_absent_at = NULL
WHERE id = ANY(sqlc.arg(job_ids)::bigint[]) AND ats_absent_at IS NOT NULL;

-- name: ListJobGhostStamps :many
-- The absence stamp AND the closed state of a page of jobs, for the read paths that do
-- not already hold the rows — search results come back from Meilisearch, which does not
-- carry ats_absent_at (and cannot: reindex is content_hash-incremental, so a column no
-- adapter writes would never reach the index on its own).
--
-- closed_at rides along because a closed posting must carry no ghost signal, and the
-- index is NOT a reliable source for that either: a sweep-closed job stays in Meili
-- until a reindex, whose timer is disabled. Reading the truth from Postgres is what
-- stops a warning appearing on a posting that has already been taken down.
SELECT id, ats_absent_at, closed_at FROM jobs WHERE id = ANY(sqlc.arg(job_ids)::bigint[]);
