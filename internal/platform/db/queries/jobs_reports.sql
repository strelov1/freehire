-- name: SkillGapReportBounds :one
-- The id span cmd/report-skill-gaps walks. Same MIN/MAX-over-the-primary-key shape as
-- RequirementsDerivedBackfillBounds — two index probes, deliberately unfiltered.
SELECT COALESCE(MIN(id), 0)::bigint AS min_id,
       COALESCE(MAX(id), 0)::bigint AS max_id
FROM jobs;

-- name: ListJobSkillsForGapReport :many
-- One chunk of the skill-gap report: every (job id, raw skill phrase) pair LLM
-- enrichment recorded, for jobs in an id range.
--
-- The CASE inside the LATERAL call keeps the query safe regardless of what the
-- query planner decides to do: jsonb_array_elements_text errors on a non-array JSON
-- value, and this call does not rely on a later WHERE predicate being pushed down
-- ahead of it to avoid that — substituting '[]'::jsonb for anything that is not a
-- JSON array (enrichment.skills absent, enrichment itself an empty object, or
-- defensively some other JSON shape) makes the call total on its own: it always
-- sees an array, and a job with no skills array simply contributes zero rows.
--
-- The LIMIT bounds how many (id, skill) pairs one statement returns, not how many
-- jobs it reads. Unlike ListJobsForRequirementsBackfill's one-row-per-job cap, this
-- is a one-row-per-(job, skill) LATERAL expansion, so the LIMIT is not guaranteed to
-- land on a job-id boundary — the caller (cmd/report-skill-gaps) accounts for that:
-- when a chunk comes back full, it holds back the last id's rows and resumes AT
-- that id rather than past it, so a job's skill list is never read half-counted.
--
-- enriched_at IS NOT NULL, not `enrichment IS NOT NULL`: jobs.enrichment defaults to
-- '{}'::jsonb NOT NULL (migration 0001), so a job that has never been enriched still
-- has a non-NULL enrichment column — enriched_at is the real "this job has actually
-- been enriched" signal. The CASE above already makes an unenriched row contribute
-- zero rows on its own (an empty object has no 'skills' array), so this filter is a
-- cheaper way to skip most of the table rather than a correctness requirement.
SELECT j.id, s.skill::text AS skill
FROM jobs j
CROSS JOIN LATERAL jsonb_array_elements_text(
    CASE WHEN jsonb_typeof(j.enrichment -> 'skills') = 'array'
         THEN j.enrichment -> 'skills'
         ELSE '[]'::jsonb
    END
) AS s(skill)
WHERE j.id >= sqlc.arg(from_id) AND j.id < sqlc.arg(to_id)
  AND j.enriched_at IS NOT NULL
ORDER BY j.id
LIMIT sqlc.arg(row_limit);

-- name: ClassifyDriftReportBounds :one
-- The id span cmd/report-classify-drift walks. Same shape as SkillGapReportBounds.
SELECT COALESCE(MIN(id), 0)::bigint AS min_id,
       COALESCE(MAX(id), 0)::bigint AS max_id
FROM jobs;

-- name: ListTitlesForClassifyDrift :many
-- One chunk of the classify-drift report: every distinct title among enriched jobs in
-- an id range, with how many jobs in THIS CHUNK carried it and one representative
-- enrichment seniority/category pair (MIN picks an arbitrary but deterministic one —
-- the report only needs a disagreement signal per title, not a distribution across
-- postings that share a title but disagree with each other).
--
-- Deliberately NO row LIMIT, unlike ListJobSkillsForGapReport: GROUP BY already caps
-- this statement's output at the number of DISTINCT titles in the id range, which is
-- always far below the range's row count, so the id range width alone (the caller's
-- chunk-size knob) is what bounds one statement's cost. A LIMIT on top of an
-- unordered GROUP BY would silently drop titles from the chunk rather than bounding
-- memory, with no id to resume from — aggregated rows carry no single id to resume a
-- partial chunk from, unlike the per-row chunks elsewhere in this file.
--
-- Grouping happens per chunk, not across the whole table: a title spanning more than
-- one id range comes back as more than one row, one per chunk it appears in. The
-- caller (cmd/report-classify-drift) merges those by title across chunks.
--
-- enriched_at IS NOT NULL, not `enrichment IS NOT NULL` or is_tech/closed_at:
-- jobs.enrichment defaults to '{}'::jsonb NOT NULL (migration 0001), so a job that
-- has never been enriched still has a non-NULL enrichment column — enriched_at is
-- the real "this job has actually been enriched" signal, the same one
-- EnqueuePendingJobs uses to decide what still needs enriching. A title's dictionary
-- answer is a fact about the title text alone, independent of whether the posting is
-- open or confirmed technical, and filtering on either would hide real drift on
-- titles that skew toward one state. COALESCE to '' rather than leaving the
-- aggregate nullable: a title where every enriched job left a facet unstated is
-- exactly the "no opinion" case dictgap.ClassifyDriftCandidates already treats as
-- empty.
SELECT title,
       count(*)::bigint AS job_count,
       COALESCE(MIN(enrichment ->> 'seniority'), '')::text AS enrichment_seniority,
       COALESCE(MIN(enrichment ->> 'category'), '')::text AS enrichment_category
FROM jobs
WHERE id >= sqlc.arg(from_id) AND id < sqlc.arg(to_id)
  AND enriched_at IS NOT NULL
GROUP BY title;

-- name: UnclassifiedTitleReportBounds :one
-- The id span cmd/report-unclassified-titles walks. Same shape as
-- ClassifyDriftReportBounds.
SELECT COALESCE(MIN(id), 0)::bigint AS min_id,
       COALESCE(MAX(id), 0)::bigint AS max_id
FROM jobs;

-- name: ListTitlesForUnclassifiedReport :many
-- One chunk of the unclassified-title report: every distinct title among PUBLISHABLE
-- postings in an id range, with how many postings in THIS CHUNK carried it.
--
-- The scope is the deliberate difference from ListTitlesForClassifyDrift beside it.
-- That query takes enriched postings whatever their state, because a title's
-- dictionary answer is a fact about the text. This one asks a different question —
-- what is the catalogue failing to PUBLISH — so a title carried only by closed,
-- duplicate or private postings is not a gap: recognising it would publish nothing.
--
-- No is_tech predicate, and that is not an omission. The stored column is the OLD
-- dictionary's answer until cmd/backfill-derive reaches the row (~171 rows/s over
-- 12.7M rows), so filtering on it would rank gaps already closed and hide gaps the
-- newest terms opened. dictgap.UnclassifiedTitles recomputes instead; this statement
-- hands it every publishable title and lets the dictionary decide, the same
-- over-fetch-and-let-the-dictionary-decide shape cmd/backfill-clearance uses.
--
-- Reads no description column, so it never de-TOASTs.
--
-- Deliberately NO row LIMIT, for the reason ListTitlesForClassifyDrift states: GROUP
-- BY already caps the output at the number of DISTINCT titles in the id range, while a
-- LIMIT on an unordered aggregate would silently drop titles with no id to resume
-- from. Grouping happens per chunk, so a title spanning more than one range comes back
-- once per chunk and the caller sums.
SELECT title,
       count(*)::bigint AS job_count
FROM jobs
WHERE id >= sqlc.arg(from_id) AND id < sqlc.arg(to_id)
  AND closed_at IS NULL
  AND duplicate_of IS NULL
  AND NOT is_private
GROUP BY title;

-- name: GeoGapReportBounds :one
-- The id span cmd/report-geo-gaps walks. Same shape as ClassifyDriftReportBounds.
SELECT COALESCE(MIN(id), 0)::bigint AS min_id,
       COALESCE(MAX(id), 0)::bigint AS max_id
FROM jobs;

-- name: ListLocationsForGeoGap :many
-- One chunk of the geo-gap report: every distinct (location string, LLM country list)
-- pair among PUBLISHABLE postings in an id range, with how many postings in THIS CHUNK
-- carried it. dictgap.GeoGapCandidates re-runs internal/dict/location on each location
-- and compares; the stored jobs.countries is deliberately not read, since it is the
-- answer of whichever dictionary last derived the row.
--
-- Publishable for the reason ListTitlesForUnclassifiedReport states: a location only
-- closed or duplicate postings carry places nothing in the catalogue.
--
-- The country list is sorted inside the LATERAL so that two postings naming the same
-- countries in a different order group together. The CASE keeps
-- jsonb_array_elements_text total on a missing or non-array `countries` key, the same
-- guard ListJobSkillsForGapReport documents; a posting whose list is empty is dropped,
-- since it states no opinion to compare.
--
-- Reads no description column. Deliberately NO row LIMIT, for the reason
-- ListTitlesForClassifyDrift states: the GROUP BY caps the output, and the caller sums
-- a location that spans more than one chunk.
SELECT j.location,
       c.llm_countries,
       count(*)::bigint AS job_count
FROM jobs j
CROSS JOIN LATERAL (
    SELECT ARRAY(
        SELECT x
        FROM jsonb_array_elements_text(
            CASE WHEN jsonb_typeof(j.enrichment -> 'countries') = 'array'
                 THEN j.enrichment -> 'countries'
                 ELSE '[]'::jsonb END) AS x
        ORDER BY x
    )::text[] AS llm_countries
) c
WHERE j.id >= sqlc.arg(from_id) AND j.id < sqlc.arg(to_id)
  AND j.closed_at IS NULL
  AND j.duplicate_of IS NULL
  AND NOT j.is_private
  AND j.location <> ''
  AND cardinality(c.llm_countries) > 0
GROUP BY j.location, c.llm_countries;
