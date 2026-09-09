-- Candidate-reported facts about how a company hires (migration 0156). One row per
-- (user, company, kind); withdrawal sets retracted_at rather than deleting, so the
-- uniqueness bound survives a retraction and cannot be used to file repeatedly.
--
-- The domain layer runs file-or-retract and then the recount in ONE transaction, so
-- a reader never sees the label without the count that qualifies it.

-- The company row's lock that serializes concurrent writes with the recompute below
-- is LockCompanyForVote, reused here rather than duplicated under a second name —
-- the same call companyfeedback makes for the same reason. It locks the companies
-- row; nothing about it is specific to votes.

-- name: FileCompanyProcessReport :one
-- File a report, or revive the caller's own retracted one.
--
-- The ON CONFLICT branch is guarded on retracted_at IS NOT NULL, so an already-live
-- report updates nothing and RETURNS NO ROW — which is exactly how the service tells
-- "filed" from "you already reported this" (409) without a second read. A revival
-- reuses the row rather than inserting a second, keeping the uniqueness bound whole.
INSERT INTO company_process_reports (user_id, company_slug, kind)
VALUES ($1, $2, $3)
ON CONFLICT (user_id, company_slug, kind) DO UPDATE
    SET retracted_at = NULL
    WHERE company_process_reports.retracted_at IS NOT NULL
RETURNING id;

-- name: RetractCompanyProcessReport :one
-- Withdraw the caller's own live report. Guarded on retracted_at IS NULL so a second
-- withdrawal returns no row (404) rather than silently restamping the timestamp, and
-- so the row is never deleted.
UPDATE company_process_reports
   SET retracted_at = now()
 WHERE user_id = $1 AND company_slug = $2 AND kind = $3 AND retracted_at IS NULL
RETURNING id;

-- name: RecountCompanyProcessReports :one
-- Recompute the company's materialized counter from the rows and return it. Run as
-- its own statement AFTER the write within one transaction, the same shape as
-- RecountCompanyFeedback and RecountCompanyVotes.
--
-- The kind is named in the statement rather than passed in because the counter is a
-- column, and a column holds one kind. A second kind gets its own column and its own
-- line here — which is the point at which the cost of another kind becomes visible,
-- instead of a widening jsonb nobody can filter on.
UPDATE companies SET
    ai_interview_reports = (
        SELECT count(*) FROM company_process_reports r
         WHERE r.company_slug = $1 AND r.kind = 'ai_interview' AND r.retracted_at IS NULL
    )
WHERE slug = $1
RETURNING ai_interview_reports;

-- name: SyncJobsAIInterviewReports :exec
-- Copy the company's counter onto its postings, so a job card carries the label
-- without the read path joining companies. Run in the SAME transaction as the report,
-- not on a schedule: a report is filed in real time, and a company page showing the
-- label while that company's own job cards say nothing reads as a bug.
--
-- IS DISTINCT FROM keeps a no-op report from touching a single row, and updated_at is
-- bumped for the same reason SyncJobCollections bumps it — `reindex --since` is what
-- carries the change into the facet index.
UPDATE jobs
   SET ai_interview_reports = $2, updated_at = now()
 WHERE company_slug = $1
   AND ai_interview_reports IS DISTINCT FROM $2;

-- name: MyLiveCompanyProcessReportKinds :many
-- The kinds this user currently has live against one company. The write surface needs
-- it to open in the right state: without it a returning reader cannot be shown that
-- they already reported, and the only way to find out would be to try and be refused.
SELECT kind FROM company_process_reports
 WHERE user_id = $1 AND company_slug = $2 AND retracted_at IS NULL
 ORDER BY kind;

-- name: CountRecentCompanyProcessReports :one
-- How many reports this user has filed since `since` — the rate-limit check, mirroring
-- CountRecentCompanyFeedback. A revival takes the ON CONFLICT branch and leaves
-- created_at untouched, so this only grows on genuinely new rows.
SELECT count(*) FROM company_process_reports WHERE user_id = $1 AND created_at >= $2;
