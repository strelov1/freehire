-- name: ListUserJobs :many
-- A user's job interactions joined with a CARD of the job — what a list row draws, and no
-- more. Each subset is ordered by when the job entered *that* list, not by last touch: saved
-- by saved_at, applied by applied_at, the passive history by viewed_at, the board by when it
-- was saved or applied. This keeps a plain re-view from bumping a saved/applied job to the
-- top (viewed_at is refreshed on every view). 'all' keeps the touched-recency timeline.
-- filter narrows to viewed-only/saved/applied subsets; 'viewed' is the passive history (rows
-- neither saved nor applied). Closed jobs stay listed: a user's history must not shrink when
-- a posting closes.
--
-- The columns are named rather than embedded because of what embedding cost: sqlc.embed(jobs)
-- read all 51, description included, and the board renders none of that text. Measured over
-- 500 applications with 5 KB descriptions, it was 2.37 MB of a 2.83 MB response — serialized
-- again into the SSR payload and parsed again by the browser — plus 13 ms of the query's 40,
-- since a description is large enough to be TOASTed and fetched separately per row. The full
-- posting is one read away at GET /me/tracking/:slug, which is what the drawer already calls.
--
-- Only the geography arrives raw: the wire value is a dict-then-LLM hybrid, so both sides have
-- to reach the projection. They are pulled out of the enrichment by key instead of shipping
-- the whole JSONB.
SELECT jobs.id, jobs.public_slug, jobs.title, jobs.company, jobs.company_slug, jobs.closed_at, jobs.application_deadline,
       jobs.location, jobs.work_mode, jobs.seniority, jobs.employment_type,
       jobs.countries, jobs.regions, jobs.skills, jobs.collections,
       jobs.posted_at, jobs.created_at,
       -- The blurb a row shows under the title: the enrichment's own summary when there is
       -- one, else the opening of the description with its tags stripped. Cut here rather
       -- than shipped whole and cut in the browser — the descriptions were 2.37 MB of a
       -- 2.83 MB response, and a row displays about 220 characters of one.
       --
       -- The compact read (the Kanban board) draws no blurb at all, and the fallback is the
       -- expensive half of this row: it de-TOASTs a description per card. CASE evaluates
       -- lazily, so skipping it skips the read — measured on prod at ~75 of a 500-card
       -- board's ~100 ms.
       (CASE WHEN sqlc.arg(compact)::boolean THEN ''
             ELSE COALESCE(
                 NULLIF(jobs.enrichment ->> 'summary', ''),
                 left(regexp_replace(jobs.description, '<[^>]*>', ' ', 'g'), 400))
        END)::text AS blurb,
       ARRAY(SELECT jsonb_array_elements_text(jobs.enrichment -> 'countries'))::text[] AS llm_countries,
       ARRAY(SELECT jsonb_array_elements_text(jobs.enrichment -> 'regions'))::text[]   AS llm_regions,
       uj.viewed_at, uj.saved_at, a.applied_at, a.stage, a.notes,
       -- When tracking was recorded, not the user-editable historical apply date.
       -- Preparing has no applied_at; passive views and notes alone do not join the board.
       LEAST(uj.saved_at, CASE WHEN a.applied_at IS NOT NULL OR a.stage IS NOT NULL
                              THEN a.created_at END)::timestamptz AS added_to_board_at,
       COALESCE(mail.email_count, 0)::bigint AS email_count,
       (a.applied_at IS NOT NULL
        AND COALESCE(mail.suggestion_pending, false))::boolean AS has_pending_suggestion,
       a.followed_up_at,
       -- When a CV of the caller's tied to this job was last opened by a countable visitor. Read
       -- from the denormalised stamp on cvs rather than from the click history, which would mean
       -- joining three tables for one timestamp.
       --
       -- Deliberately NOT an input to last_activity_at below, for the same reason followed_up_at
       -- is not: somebody opening a CV is not a reply. Folding it in would clear the silence
       -- badge at the moment it matters most — they read it and still said nothing.
       (SELECT max(cv.last_click_at)
          FROM cvs cv
         WHERE cv.user_id = uj.user_id
           AND cv.job_id = jobs.id)::timestamptz AS cv_opened_at,
       -- last_activity_at is when this application last moved: its apply date, or the newest
       -- message linked to it, or the employer's newest move in their freehire pipeline
       -- (a stage_set event with source 'employer') — whichever is latest. An employer moving
       -- the application IS a response, as their mail would be; a stage the candidate typed
       -- is not, and stays out. GREATEST ignores a NULL aggregate, so an
       -- application with no mail falls back to applied_at rather than reporting nothing — a
       -- clock that only starts once mail arrives would never fire on the applications ignored
       -- outright, which are the ones worth reporting. NULL on a row that is not an
       -- application: a job merely viewed or saved is not waiting on anyone.
       (CASE WHEN a.applied_at IS NOT NULL
             THEN GREATEST(a.applied_at, mail.newest_mail_at,
                   (SELECT max(ev.occurred_at)
                      FROM application_events ev
                     WHERE ev.application_id = a.id
                       AND ev.kind = 'stage_set'
                       AND ev.source = 'employer'
                       AND ev.retracted_at IS NULL))
        END)::timestamptz AS last_activity_at,
       -- The raw columns behind the card's auto-apply badge. aaq.id is the presence marker:
       -- a LEFT JOIN with no match leaves every column NULL, which is indistinguishable from
       -- a real attempt that has not been tailored yet unless the row's own existence is
       -- read separately. Status is derived from these in Go (autoapply.DeriveStatus),
       -- mirroring has_pending_suggestion's own reasoning above: one derivation, read by both
       -- this list and the drawer's own single-application read (GetUserApplication), so the
       -- badge and the banner can never disagree about what an entry's state means.
       aaq.id AS auto_apply_id, aaq.tailored_cv_id AS auto_apply_tailored_cv_id,
       aaq.review_decision AS auto_apply_review_decision,
       aaq.blocked_at AS auto_apply_blocked_at, aaq.failed_at AS auto_apply_failed_at,
       aaq.preview_failed_at AS auto_apply_preview_failed_at,
       aaq.tailor_failed_at AS auto_apply_tailor_failed_at,
       (aaq.resolved_preview IS NOT NULL)::boolean AS auto_apply_has_preview
FROM user_jobs uj
JOIN jobs ON jobs.id = uj.job_id
LEFT JOIN applications a ON a.user_id = uj.user_id AND a.job_id = uj.job_id
LEFT JOIN auto_apply_queue aaq ON aaq.user_id = uj.user_id AND aaq.job_id = jobs.id
-- One pass over this job's mail for all three facts. They were three correlated subqueries
-- over the same rows with the same predicate written three times; the "pending" test in
-- particular has to stay the one the follow-up gate, the ghost signal and the inbox's link
-- filter make, and three spellings of it were three chances to drift.
--
-- has_pending_suggestion is mail the matcher believes belongs to this application that the
-- caller has neither confirmed nor rejected. Such mail contradicts a claim that nobody
-- replied, so the reader downgrades a silence verdict to a question rather than asserting it.
-- "Pending" means the caller has not confirmed it, and confirming is what attaches the
-- application — asking `job_id IS NULL` instead is the same set today only by the coincidence
-- that every writer setting job_id also clears the suggestion.
LEFT JOIN LATERAL (
    SELECT count(*) FILTER (WHERE e.job_id = jobs.id)                AS email_count,
           max(e.received_at) FILTER (WHERE e.job_id = jobs.id)      AS newest_mail_at,
           bool_or(e.suggested_job_id = jobs.id
                   AND e.application_id IS NULL)                     AS suggestion_pending
      FROM emails e
     WHERE e.user_id = uj.user_id
       AND (e.job_id = jobs.id OR e.suggested_job_id = jobs.id)
       AND e.deleted_at IS NULL AND e.direction = 'incoming'
) mail ON true
WHERE uj.user_id = $1
  AND (sqlc.arg(filter)::text = 'all'
       OR (sqlc.arg(filter)::text = 'viewed' AND uj.saved_at IS NULL AND a.applied_at IS NULL)
       OR (sqlc.arg(filter)::text = 'saved' AND uj.saved_at IS NOT NULL)
       OR (sqlc.arg(filter)::text = 'applied' AND a.applied_at IS NOT NULL)
       OR (sqlc.arg(filter)::text = 'board'
           AND (uj.saved_at IS NOT NULL OR a.applied_at IS NOT NULL OR a.stage IS NOT NULL))
       OR (sqlc.arg(filter)::text = 'dismissed' AND uj.dismissed_at IS NOT NULL))
ORDER BY (CASE sqlc.arg(filter)::text
            WHEN 'saved' THEN uj.saved_at
            WHEN 'applied' THEN a.applied_at
            WHEN 'viewed' THEN uj.viewed_at
            WHEN 'board' THEN GREATEST(uj.saved_at, a.applied_at)
            WHEN 'dismissed' THEN uj.dismissed_at
            ELSE GREATEST(uj.viewed_at, uj.saved_at, a.applied_at)
          END) DESC NULLS LAST, uj.job_id DESC
LIMIT $2 OFFSET $3;

-- name: CountUserJobs :one
-- Per-filter row counts for the my-jobs tabs, in one aggregate pass. "all" is
-- every interaction row; "viewed" is the view-only subset (neither saved nor
-- applied), matching the ListUserJobs filter. "board" counts jobs on the Kanban
-- board (saved, applied, or stage set), matching the ListUserJobs board filter.
-- "dismissed" counts jobs the user hid from the feed, matching the ListUserJobs
-- dismissed filter.
SELECT count(*)                                          AS "all",
       count(*) FILTER (WHERE uj.saved_at IS NULL
                          AND a.applied_at IS NULL)        AS viewed,
       count(*) FILTER (WHERE uj.saved_at  IS NOT NULL)    AS saved,
       count(*) FILTER (WHERE a.applied_at IS NOT NULL)    AS applied,
       count(*) FILTER (WHERE uj.saved_at  IS NOT NULL
                            OR a.applied_at IS NOT NULL
                            OR a.stage      IS NOT NULL)   AS board,
       count(*) FILTER (WHERE uj.dismissed_at IS NOT NULL) AS dismissed
FROM user_jobs uj
LEFT JOIN applications a ON a.user_id = uj.user_id AND a.job_id = uj.job_id
WHERE uj.user_id = $1;

-- name: CountMyJobsByStage :many
-- Per-stage application counts for the Pipeline snapshot. An application is any
-- row the user applied to or staged (saved-only rows are excluded); a row with
-- applied_at set but no stage groups under a NULL stage. The Go layer folds these
-- rows into the pipeline buckets.
SELECT stage, count(*) AS count
FROM applications
WHERE user_id = $1
  AND (applied_at IS NOT NULL OR stage IS NOT NULL)
GROUP BY stage;
