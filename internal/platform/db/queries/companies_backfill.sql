-- name: ListSlugLikeCompaniesForBackfill :many
-- Companies whose ingested name is still a squished slug (lowercase, no
-- whitespace or uppercase) and that have at least one open job, with a
-- representative open job's source and URL so the backfill worker can locate the
-- ATS board. Only boards with live jobs matter, so dead ones never appear. The Go
-- side re-validates slug-likeness authoritatively before touching anything.
--
-- AND NOT is_private excludes the jd-tailor-intake private-job path: the representative
-- row here supplies the URL the worker reads a real display name from, and the name it
-- reads is written to the PUBLIC companies row (RenameSlugCompany below). A JD one user
-- pasted in is not a board, and it must not be the source of an employer's public name.
SELECT DISTINCT ON (company_slug)
       company_slug AS slug,
       company      AS name,
       source,
       url
FROM jobs
WHERE closed_at IS NULL
  AND duplicate_of IS NULL
  AND NOT is_private
  AND company_slug <> ''
  AND company ~ '^[a-z0-9._-]+$'
ORDER BY company_slug, created_at DESC;

-- name: RenameSlugCompany :execrows
-- Apply a resolved display name to every job under a slug-like company and
-- re-key its company_slug (computed by the caller via normalize.Slug); the caller then
-- reconciles the touched slugs through SyncCompaniesForSlugs + DeleteOrphanCompaniesForSlugs.
-- The name guard keeps a re-run from overwriting a name that is no longer a slug.
UPDATE jobs
SET company = @name,
    company_slug = @new_slug,
    -- Kept in step with company_slug by hand, like every other write path that sets
    -- it — see migrations/0109 for why the folded value is a stored column at all,
    -- and internal/platform/db/folded_slug_rule_test.go for the test that catches a new write
    -- path forgetting this line.
    company_slug_folded = replace(@new_slug, '-', ''),
    updated_at = now()
WHERE company_slug = @old_slug
  AND company ~ '^[a-z0-9._-]+$';

-- name: RenamePlaceholderBoardJobs :many
-- Apply a resolved display name to every stored job of ONE board whose company is still that
-- board's placeholder (see ListPlaceholderCompanyBoards). board_pattern is
-- externalid.BoardPattern(board), the board's external_id namespace, so only this board's rows
-- move. Needed beside the board write because a crawl only REWRITES a posting that changed — an
-- unchanged one is refreshed for liveness and keeps the placeholder, and a closed one is never
-- seen again. Guarded on the old name, so a re-run writes nothing. The moved rows' indexed
-- company and slug change, so an open, canonical, public one is queued for search in the same
-- statement, as every other writer of indexed job content does.
--
-- label_name is the neutral label the postings may show instead (sources.PlaceholderLabel), '' for none.
--
-- Returns the company_slug each moved job carried BEFORE the write, one row per job: the rows
-- are matched by board and name, not by slug, and an alias the merge worker recorded may have
-- re-keyed them away from the slug the placeholder name folds to — the caller sweeps exactly
-- these for orphaned companies rows.
WITH moved AS (
    SELECT j.id, j.company_slug
    FROM jobs j
    WHERE j.source = sqlc.arg(source)
      AND j.external_id LIKE sqlc.arg(board_pattern)
      AND j.company IN (sqlc.arg(old_name), NULLIF(sqlc.arg(label_name)::text, ''))
    FOR UPDATE
), updated AS (
    UPDATE jobs
    SET company = sqlc.arg(name),
        company_slug = sqlc.arg(new_slug),
        -- Kept in step with company_slug by hand; see RenameSlugCompany.
        company_slug_folded = replace(sqlc.arg(new_slug), '-', ''),
        updated_at = now()
    FROM moved
    WHERE jobs.id = moved.id
    RETURNING jobs.id, moved.company_slug AS old_slug, COALESCE(jobs.posted_at, jobs.created_at) AS eff_posted_at,
        jobs.closed_at IS NULL AND jobs.duplicate_of IS NULL AND NOT jobs.is_private AS indexed
), queued AS (
    INSERT INTO search_outbox (job_id, job_posted_at)
    SELECT id, eff_posted_at FROM updated WHERE indexed
    -- Re-stamp a live entry as RequeueSearchOutbox does: a drain completing a claim it took
    -- before the rename deletes by the old id and must not take the rename's entry with it.
    ON CONFLICT (job_id) DO UPDATE
        SET id = DEFAULT, created_at = now(), attempts = 0, last_error = '', failed_at = NULL
)
SELECT old_slug FROM updated;

-- name: ListPlaceholderBoardDescriptions :many
-- A few of one board's stored posting bodies, open ones first, for a placeholder whose ATS
-- states the employer nowhere but in the posting text (adp; see cmd/backfill-company-names).
-- board_pattern is externalid.BoardPattern(board). A board is small — the index behind
-- (source, external_id LIKE) reaches its rows directly — so the sort is over one board only.
SELECT description
FROM jobs
WHERE source = sqlc.arg(source)
  AND external_id LIKE sqlc.arg(board_pattern)
  AND description <> ''
ORDER BY (closed_at IS NULL) DESC, id DESC
LIMIT sqlc.arg(max_rows);
