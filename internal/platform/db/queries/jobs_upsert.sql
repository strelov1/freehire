-- name: UpsertJob :one
-- Single atomic write: upsert the company (only when the slug is non-empty,
-- via the WHERE on the SELECT) and the job together, keeping the "one write =
-- one job" property of the pipeline's write path.
-- The enrichment columns are deliberately NOT written here: ingest carries no
-- enrichment, so a new row takes the table defaults ('{}' / NULL / 0) and a
-- re-ingest leaves any existing enrichment untouched. SetJobEnrichment (the
-- enrichment worker) is the sole writer of those columns.
-- countries/regions ARE written here: they are source facts parsed from the
-- location, not enrichment. COALESCE maps a nil arg to '{}', so a location that
-- yields no geography stores empty arrays (the columns are NOT NULL).
-- content_hash is the incremental-index change signal (internal/job/jobhash): the
-- `existing` CTE captures the row's pre-update hash (snapshot from before this
-- statement), so RETURNING can report whether the write inserted a new row
-- (`inserted`) or changed its searchable content (`changed`, true on insert and
-- for a legacy NULL hash). A re-ingest that only bumps last_seen_at reports both
-- false and needs no re-push to the search index. A reopen deliberately reads a NULL old
-- hash, so identical content still returns to search.
WITH existing AS (
    SELECT id, closed_at, closed_reason, application_deadline, is_private,
           CASE WHEN closed_at IS NULL THEN content_hash END AS old_hash, true AS existed
    FROM jobs
    WHERE source = sqlc.arg(source) AND external_id = sqlc.arg(external_id)
),
-- An employer extending the deadline reopens the posting. Remove an older expiry's
-- pending deletion so a delayed retry cannot delete the newly re-indexed document.
reopened_dequeued AS (
    DELETE FROM search_delete_outbox
    WHERE job_id IN (
        SELECT id FROM existing
        WHERE closed_at IS NOT NULL AND closed_reason = 'expired'
          AND sqlc.narg(application_deadline)::date >= (now() AT TIME ZONE 'UTC')::date
    )
),
expired_queued AS (
    INSERT INTO search_delete_outbox (job_id)
    SELECT id FROM existing
    WHERE closed_at IS NULL AND NOT is_private
      AND COALESCE(sqlc.narg(application_deadline)::date, application_deadline) < (now() AT TIME ZONE 'UTC')::date
    ON CONFLICT (job_id) DO NOTHING
),
company_upsert AS (
    INSERT INTO companies (slug, name)
    SELECT sqlc.arg(company_slug), sqlc.arg(company)
    WHERE sqlc.arg(company_slug) <> ''
    -- The WHERE is what keeps a crawl from rewriting one company row once per posting. Without
    -- it a board of 5,000 vacancies updated its company 5,000 times per pass: measured on prod
    -- 2026-08-02, 286M updates against 305k rows, leaving that table 32% dead tuples at 26 KB
    -- per live row. The guard sits on the UPDATE branch only, so a new company is still created.
    -- companies.updated_at is also served as a hiring company's sitemap <lastmod>, so this stops
    -- it claiming every company changed on every crawl.
    --
    -- Only this copy is guarded. The same CTE in UpsertManualJob and UpdateManualJob fires once
    -- per moderator action, where the write costs nothing worth guarding.
    ON CONFLICT (slug) DO UPDATE SET
        name       = EXCLUDED.name,
        updated_at = now()
    WHERE companies.name IS DISTINCT FROM EXCLUDED.name
)
INSERT INTO jobs (
    source, external_id, url, title, company, company_slug, company_slug_folded, location, remote, description, posted_at,
    public_slug, countries, regions, cities, metros, work_mode, skills, seniority, category, is_tech, requires_clearance, via_recruiter, equity, clearance_levels, perks, engagement,
    posting_language, employment_type, education_level, english_level, experience_years_min,
    salary_min_source, salary_max_source, salary_currency_source, salary_period_source,
    content_hash, role_fingerprint, requirements_derived, hydrated_at, application_deadline, closed_at, closed_reason
) VALUES (
    sqlc.arg(source), sqlc.arg(external_id), sqlc.arg(url), sqlc.arg(title),
    sqlc.arg(company), sqlc.arg(company_slug), replace(sqlc.arg(company_slug), '-', ''), sqlc.arg(location), sqlc.arg(remote),
    sqlc.arg(description), sqlc.arg(posted_at),
    sqlc.arg(public_slug),
    COALESCE(sqlc.arg(countries)::text[], '{}'), COALESCE(sqlc.arg(regions)::text[], '{}'), COALESCE(sqlc.arg(cities)::text[], '{}'), COALESCE(sqlc.arg(metros)::text[], '{}'),
    sqlc.arg(work_mode), COALESCE(sqlc.arg(skills)::text[], '{}'), sqlc.arg(seniority), sqlc.arg(category), sqlc.arg(is_tech), sqlc.arg(requires_clearance), sqlc.arg(via_recruiter), sqlc.arg(equity), COALESCE(sqlc.arg(clearance_levels)::text[], '{}'), COALESCE(sqlc.arg(perks)::text[], '{}'), COALESCE(sqlc.arg(engagement)::text[], '{}'),
    sqlc.arg(posting_language), sqlc.arg(employment_type), sqlc.arg(education_level), sqlc.arg(english_level), sqlc.arg(experience_years_min),
    sqlc.arg(salary_min_source), sqlc.arg(salary_max_source), sqlc.arg(salary_currency_source), sqlc.arg(salary_period_source),
    sqlc.arg(content_hash), sqlc.arg(role_fingerprint),
    -- The requirements the posting states as a list under a recognized heading
    -- (internal/job/reqextract), derived from the same description the dictionary
    -- facets above are. COALESCE keeps the column NOT NULL when a posting yields
    -- none: an empty array and "never processed" are the same fact here.
    COALESCE(sqlc.arg(requirements_derived)::jsonb, '[]'::jsonb),
    -- A first ingest reads the posting's body, so the row starts out freshly hydrated. NULL
    -- is reserved for the rows that predate the column (see 0144), which read as stale.
    now(), sqlc.narg(application_deadline)::date,
    CASE WHEN sqlc.narg(application_deadline)::date < (now() AT TIME ZONE 'UTC')::date THEN now() END,
    CASE WHEN sqlc.narg(application_deadline)::date < (now() AT TIME ZONE 'UTC')::date THEN 'expired' ELSE '' END
)
-- public_slug is deliberately NOT in the DO UPDATE SET: the slug is minted once
-- at insert and is the row's stable public identity. Re-ingest of the same
-- (source, external_id) must not rewrite it, so external links stay valid even
-- if the slug builder changes later (that would be a deliberate migration).
ON CONFLICT (source, external_id) DO UPDATE SET
    url          = EXCLUDED.url,
    title        = EXCLUDED.title,
    company      = EXCLUDED.company,
    company_slug = EXCLUDED.company_slug,
    company_slug_folded = EXCLUDED.company_slug_folded,
    location     = EXCLUDED.location,
    remote       = EXCLUDED.remote,
    -- description comes from a separate, best-effort detail fetch (some adapters,
    -- e.g. habr_career, load it from a per-vacancy page that an anti-bot layer can
    -- intermittently fail). A failed fetch yields an empty description but still
    -- upserts the job, so writing EXCLUDED unconditionally would let a transient
    -- failure wipe a good description. Keep the stored value when the incoming one is
    -- empty; a non-empty description still overwrites, so real edits propagate.
    -- content_hash below stays the incoming fingerprint; the search drain rebuilds
    -- its document from the stored row, including the preserved description.
    description  = COALESCE(NULLIF(EXCLUDED.description, ''), jobs.description),
    posted_at    = EXCLUDED.posted_at,
    application_deadline = COALESCE(EXCLUDED.application_deadline, jobs.application_deadline),
    countries    = EXCLUDED.countries,
    regions      = EXCLUDED.regions,
    cities       = EXCLUDED.cities,
    metros       = EXCLUDED.metros,
    work_mode    = EXCLUDED.work_mode,
    skills       = EXCLUDED.skills,
    seniority    = EXCLUDED.seniority,
    category     = EXCLUDED.category,
    is_tech      = EXCLUDED.is_tech,
    -- Keep both readings in step with the preserved description after a failed fetch.
    requires_clearance = CASE
        WHEN EXCLUDED.description <> '' THEN EXCLUDED.requires_clearance
        ELSE jobs.requires_clearance
    END,
    clearance_levels = CASE
        WHEN EXCLUDED.description <> '' THEN EXCLUDED.clearance_levels
        ELSE jobs.clearance_levels
    END,
    -- Description-derived and never re-read elsewhere: a failed detail fetch (an empty
    -- incoming description, which leaves jobs.description as it was) must not drop the
    -- mark the stored description still earns — the requirements_derived guard below.
    via_recruiter = CASE
        WHEN EXCLUDED.description <> '' THEN EXCLUDED.via_recruiter
        ELSE jobs.via_recruiter
    END,
    -- The same guard: an empty incoming description is a failed detail fetch.
    equity = CASE
        WHEN EXCLUDED.description <> '' THEN EXCLUDED.equity
        ELSE jobs.equity
    END,
    -- perks is description-derived the same way: an empty incoming description derives
    -- none, and must not wipe the perks the kept description still states.
    perks = CASE
        WHEN EXCLUDED.description <> '' THEN EXCLUDED.perks
        ELSE jobs.perks
    END,
    -- engagement reads the title as well, but most postings state it in the body, so an
    -- empty incoming description keeps what the stored one was read as.
    engagement = CASE
        WHEN EXCLUDED.description <> '' THEN EXCLUDED.engagement
        ELSE jobs.engagement
    END,
    posting_language     = EXCLUDED.posting_language,
    employment_type      = EXCLUDED.employment_type,
    education_level      = EXCLUDED.education_level,
    english_level        = EXCLUDED.english_level,
    experience_years_min = EXCLUDED.experience_years_min,
    salary_min_source      = EXCLUDED.salary_min_source,
    salary_max_source      = EXCLUDED.salary_max_source,
    salary_currency_source = EXCLUDED.salary_currency_source,
    salary_period_source   = EXCLUDED.salary_period_source,
    content_hash = EXCLUDED.content_hash,
    -- role_fingerprint is the repost-identity (internal/job/jobhash.RoleFingerprint):
    -- refreshed on re-ingest so a title/description edit re-clusters the role.
    role_fingerprint = EXCLUDED.role_fingerprint,
    -- Re-derived on every write, like the dictionary facets above, so an edited
    -- description refreshes the list — including to EMPTY, when the edit removed the
    -- posting's requirements section.
    --
    -- The guard keys on the incoming DESCRIPTION, not on the incoming derivation, and
    -- the difference is the whole point. What must not wipe a stored list is a failed
    -- detail fetch, which is exactly the empty-description case `description` guards
    -- against one branch up. Keying on the empty derivation instead would ALSO keep the
    -- list when a real edit deleted the section, and the page would then quote a
    -- posting that no longer says it — a false claim the reader cannot detect, arriving
    -- through the guard meant to prevent one.
    requirements_derived = CASE
        WHEN EXCLUDED.description <> '' THEN EXCLUDED.requirements_derived
        ELSE jobs.requirements_derived
    END,
    -- The crawl saw the posting: refresh liveness and reopen if it was closed. A
    -- reopen (the row was closed) resets the strike count so a single later expired
    -- probe can't immediately re-close it — the two-strike grace survives a reopen.
    last_seen_at = now(),
    closed_at = CASE
        WHEN COALESCE(EXCLUDED.application_deadline, jobs.application_deadline) < (now() AT TIME ZONE 'UTC')::date
        THEN COALESCE(jobs.closed_at, now())
        ELSE NULL END,
    closed_reason = CASE
        WHEN COALESCE(EXCLUDED.application_deadline, jobs.application_deadline) < (now() AT TIME ZONE 'UTC')::date
        THEN 'expired' ELSE '' END,
    liveness_strikes = CASE WHEN jobs.closed_at IS NOT NULL THEN 0 ELSE jobs.liveness_strikes END,
    -- The write carried a body, so the row's stored text is as fresh as this crawl. See the
    -- column's migration (0144) for why this is not updated_at, and RefreshUnchangedJob for
    -- the other half: a body that came back byte-identical takes that statement, and stamps
    -- it there. body_from_catalogue is true when the pipeline filled an empty fetch with the stored
    -- body (job.Draft.BodyFromCatalogue): nothing was read, so the stamp stays.
    hydrated_at  = CASE WHEN sqlc.arg(body_from_catalogue)::boolean THEN jobs.hydrated_at ELSE now() END,
    updated_at   = now()
-- A substituted body was read before this statement took the row lock. If another write stored
-- a newer body in between, writing the facets of the older one over it would regress the row,
-- so the update is refused (no row returned) and the next crawl retries the posting.
WHERE NOT sqlc.arg(body_from_catalogue)::boolean OR jobs.description = EXCLUDED.description
-- Narrow on purpose, and never sqlc.embed(jobs): that expands to every jobs column, the
-- liveness stamps included, which the activity split moves to job_activity. A caller reads the
-- id, the identity the sweep scope and the index-push gate need, and whether the write left the
-- row closed; one that needs the whole row (linkimport builds a search document) re-reads it
-- through GetJob, which reads the jobs_read view.
RETURNING jobs.id, jobs.source, jobs.company_slug, jobs.duplicate_of, jobs.is_private, jobs.closed_at,
    NOT COALESCE((SELECT existed FROM existing), false) AS inserted,
    -- The deadline rides the search document without changing the global content-hash
    -- format. Compare the resulting stored date: a missing incoming date preserves it.
    COALESCE(
        (SELECT old_hash FROM existing) IS DISTINCT FROM sqlc.arg(content_hash)
        OR (SELECT application_deadline FROM existing) IS DISTINCT FROM jobs.application_deadline,
        false
    )::boolean AS changed;

-- name: RefreshUnchangedJob :one
-- The cheap half of the ingest write path, tried before UpsertJob: a crawl that re-sees a
-- posting identical to the stored row refreshes its liveness and writes NOTHING else. Matching
-- nothing (pgx.ErrNoRows) is the signal to run the full upsert — which is also what a brand-new
-- posting gets, since it too matches no row, and both want the same statement.
--
-- Why this exists: UpsertJob's DO UPDATE carries no WHERE, so a re-ingest rewrites the whole
-- tuple — re-TOASTing a ~2.5KB description and touching every index on the table — to move a
-- timestamp.
--
-- last_seen_at is the ONLY column written, and it is deliberately in no index, so the update is
-- heap-only and maintains none of them.
--
-- updated_at is deliberately NOT stamped, so the column comes to mean "content last changed"
-- rather than "last crawled". Two live readers see that: the jobs sitemap serves it as <lastmod>
-- (ListJobSitemapFreshest -> internal/api/handler/sitemap.go), where a timestamp that stopped
-- claiming every posting changed on every crawl is the honest signal rather than the one search
-- engines learn to discount; and jobview puts it on the public wire. It also makes
-- ListJobsUpdatedAfter viable for the first time — that query is currently dormant (no caller,
-- and cmd/reindex has no --since flag despite what its comment says), because a column stamped
-- on every crawl selects the whole catalogue and answers nothing.
--
-- The match key is (content_hash, cities, salary_*_source, english_level, is_tech), not the
-- hash alone. Those are what the upsert writes that jobhash.Of does not read — a caller's
-- structured city list overrides the location-derived one, a structured salary
-- (Lever/Ashby/Recruitee) is a base fact rather than something Of hashes, english_level gained
-- a structured tier of its own (profession.hu states it as a picklist) so it too can move while
-- every hashed field stands still, and is_tech can now move the same way: a source's structured
-- IsTechHint (Profession's dedicated itdev/itops boards, issue #2601) can flip it from unknown
-- to true while title/category/every hashed field stay put. Folding them into the hash instead
-- would change every stored content_hash at once and make the first crawl after deploy rewrite
-- and re-index the whole catalogue — 6.6M rows through a queue that drains at ~9 documents a
-- second. engagement is in the key for the same reason as english_level: a Brazilian board's
-- CLT/PJ picklist (sources.Job.Engagement) can move it while every hashed field stands still,
-- and a dictionary extension moves it for the postings whose text gains a term — both reach
-- the full upsert, and its search enqueue, on the next crawl. Note the asymmetry with education_level, which IS hashed: it was hashed before either
-- had a structured tier, and moving it out now would cost exactly the storm this key exists to
-- avoid. IS NOT DISTINCT FROM (not =) on the nullable salary bounds and on is_tech so a NULL
-- (unset/unknown) still matches its own kind — a plain = would push every sourceless/unknown job
-- off the cheap path forever. Whether the key still covers every written column is enforced by
-- TestUpsertParams_CheapWriteMatchKeyCoversEveryColumnItWrites (internal/job); add a derived
-- column outside the hash and it fails there. metros is covered the way countries and regions
-- are: Derive reads it off the location, which the hash covers, or off a caller's structured
-- cities (in the key) and countries.
--
-- A NULL stored content_hash (a legacy row predating the column) compares unequal and so takes
-- the full path, which is right: nothing is known about what it holds.
--
-- closed_at IS NULL is correctness, not economy. A closed posting that reappears with identical
-- content must reach UpsertJob, which is what clears closed_at and resets the strike count.
-- Refreshing its liveness here would leave it closed while the unseen sweep kept seeing it.
--
-- RETURNING is four narrow columns, NOT sqlc.embed(jobs), and that is the point rather than an
-- economy: embedding the row would detoast the ~2.5KB description and ship semantic_embedding
-- back for every re-seen posting, adding read amplification to the path built to remove write
-- amplification. These four are everything the caller reads on this branch — the id for the
-- enrichment enqueue and the apply-form capture queue, source and company_slug for the
-- crawled-set that scopes the post-run sweep, and duplicate_of for the index-push gate. The
-- fuller row is only ever needed to BUILD a search document, which by construction this branch
-- never does. TouchJob, the hydrating-source sibling, returns company_slug alone for the same
-- reason.
-- hydrated_at IS stamped, unlike updated_at, and the two say different things on purpose:
-- updated_at means "content last changed", hydrated_at means "last written from a body we had
-- just fetched". Reaching this statement at all means the crawl carried a body — an adapter
-- that cannot fetch a posting's detail drops the posting rather than writing a body-less row
-- (echojobs.FetchNew, workday.detail) — and that the body was identical to the stored one,
-- which is precisely the case a staleness check over updated_at would misread as never-read.
-- Like last_seen_at it is in no index, so the update stays heap-only.
--
-- One thing IS queued: a canonical posting re-seen after search had withheld it for going
-- unseen (search.Withheld). Its content is unchanged, but its document was removed, so the
-- drain must bring it back; prev carries the last_seen_at the update replaces. The cutoff is
-- search.UnseenWindow, the shortest window — a source with a longer one gets a harmless
-- extra push.
-- Lock order: `prev` locks the jobs row and then the activity row, FOR NO KEY UPDATE, and the UPDATE
-- joins it, so both are held before the statement queues anything. Release A took the activity row
-- last, through the legacy bridge trigger at the end of the statement, AFTER the search_outbox write
-- below; a writer that takes the activity row first and then queues (release B) interleaved with it
-- and the two deadlocked. A row without an activity row does not match this cheap path.
WITH prev AS MATERIALIZED (
    SELECT j.id, a.last_seen_at FROM jobs j
    JOIN job_activity a ON a.job_id = j.id
    WHERE j.source = sqlc.arg(source) AND j.external_id = sqlc.arg(external_id)
    FOR NO KEY UPDATE OF j, a
), refreshed AS (
UPDATE jobs
SET last_seen_at = now(),
    -- Not when the "identical" body is the stored one the pipeline substituted for a failed
    -- fetch (job.Draft.BodyFromCatalogue): that body was not read.
    hydrated_at  = CASE WHEN sqlc.arg(body_from_catalogue)::boolean THEN jobs.hydrated_at ELSE now() END
FROM prev p
WHERE jobs.id = p.id
  AND jobs.source = sqlc.arg(source)
  AND jobs.external_id = sqlc.arg(external_id)
  AND jobs.content_hash = sqlc.arg(content_hash)
  AND jobs.cities = COALESCE(sqlc.arg(cities)::text[], '{}')
  AND jobs.salary_min_source IS NOT DISTINCT FROM sqlc.arg(salary_min_source)
  AND jobs.salary_max_source IS NOT DISTINCT FROM sqlc.arg(salary_max_source)
  AND jobs.salary_currency_source = sqlc.arg(salary_currency_source)
  AND jobs.salary_period_source = sqlc.arg(salary_period_source)
  AND jobs.english_level = sqlc.arg(english_level)
  AND jobs.is_tech IS NOT DISTINCT FROM sqlc.arg(is_tech)
  AND jobs.engagement = COALESCE(sqlc.arg(engagement)::text[], '{}')
  AND jobs.application_deadline IS NOT DISTINCT FROM COALESCE(sqlc.narg(application_deadline)::date, jobs.application_deadline)
  AND (jobs.application_deadline IS NULL OR jobs.application_deadline >= (now() AT TIME ZONE 'UTC')::date)
  AND jobs.closed_at IS NULL
RETURNING jobs.id, jobs.source, jobs.company_slug, jobs.duplicate_of, jobs.is_private,
          p.last_seen_at AS prev_seen_at, COALESCE(jobs.posted_at, jobs.created_at) AS eff_posted_at
), requeued AS (
    INSERT INTO search_outbox (job_id, job_posted_at)
    SELECT id, eff_posted_at FROM refreshed
    WHERE duplicate_of IS NULL AND NOT is_private
      AND prev_seen_at < sqlc.arg(withheld_if_seen_before)::timestamptz
    -- Re-stamped: a drain wave holding this entry may be removing the document right now.
    ON CONFLICT (job_id) DO UPDATE
        SET id = DEFAULT, created_at = now(), attempts = 0, last_error = '', failed_at = NULL
)
SELECT id, source, company_slug, duplicate_of FROM refreshed;
