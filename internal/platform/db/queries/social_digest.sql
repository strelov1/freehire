-- Queries behind the daily social digest (internal/engage/socialdigest, cmd/social-digest).
--
-- The division of labour here is deliberate: SQL answers what is CHEAP and
-- unambiguous — which day has data, which postings are eligible at all, which
-- postings went out recently — and the editorial shaping (the view floor, the cap on
-- postings per company, the final ten) happens in Go, where it is a pure function
-- over a slice and can be tested without a database. Those are the rules most likely
-- to be argued about and changed; keeping them out of SQL keeps that argument cheap.

-- name: LatestJobViewDay :one
-- The freshest day the view rollup has produced. The digest asks for this rather than
-- computing "yesterday" from the clock: cmd/rollup-views fires at 02:30 UTC and reads
-- the rotated access log, so whether the freshest complete day is yesterday or the day
-- before depends on when logrotate runs on the host. A digest that assumed the answer
-- would fail by publishing a stale list silently, which is the worst way to fail.
--
-- Returns NULL when the table is empty; the caller treats that as a broken pipeline,
-- not as an empty day.
SELECT max(day)::date AS day FROM job_daily_views;

-- name: TopPageViewedJobsForDay :many
-- The day's candidates, most-viewed first, ranked on page_uniques — NOT on uniques.
-- uniques fuses page opens with API reads and API reads carry no bot filtering, so on
-- a host whose traffic is mostly crawlers it answers "what did robots fetch". See
-- migration 0138 and internal/application/viewlog.
--
-- The predicates are the same open-posting shape every public listing uses
-- (closed_at IS NULL AND duplicate_of IS NULL AND NOT is_private), plus ats_absent_at:
-- a posting the source's own ATS has stopped listing must never be promoted. The full
-- ghost verdict (internal/job/ghost) is a hedged classification needing evidence this
-- query has no reason to gather; ats_absent_at is the strongest single column of it.
--
-- is_tech IS TRUE, not "IS NOT FALSE". This is an IT job board publishing under its
-- own name, and the unfiltered list measured on 2026-09-02 led with "Manager
-- Operations", "Strategic Account Executive" and "Social Media Marketing" — the
-- catalogue carries non-tech postings and the most-viewed of them are exactly the ones
-- that rise. NULL means the dictionary could not decide, and "we are not sure this is
-- a tech job" is not a good enough reason to put it in front of people. The same
-- predicate gates enrichment spend (internal/platform/db/queries/enrichment.sql).
--
-- OVER-FETCHES on purpose. The caller drops rows for the company cap and the
-- quarantine, so a LIMIT of exactly ten would return fewer than ten publishable
-- postings on any day where one company had a good morning.
SELECT
    j.id,
    j.public_slug,
    j.title,
    j.company,
    j.company_slug,
    j.location,
    j.remote,
    v.page_uniques
FROM job_daily_views v
JOIN jobs j ON j.id = v.job_id
WHERE v.day = sqlc.arg('day')
  AND v.page_uniques > 0
  AND j.closed_at IS NULL
  AND j.duplicate_of IS NULL
  AND NOT j.is_private
  AND j.ats_absent_at IS NULL
  AND j.is_tech IS TRUE
ORDER BY v.page_uniques DESC, j.id
LIMIT sqlc.arg('lim');

-- name: RecentlyDigestedJobIDs :many
-- The quarantine set: postings that appeared in a digest in the days [since, before),
-- in ANY channel. Across channels on purpose — the list is the editorial unit and the
-- channel is only how it is delivered, so a posting shown on Discord yesterday should
-- not lead another channel's post today.
--
-- `before` is the digest's OWN day and the bound is exclusive, which is the whole
-- reason it exists. Without it the digest quarantines itself: once one channel has
-- published day D, a second channel building the same day reads back its own ten ids
-- and drops every one of them, so it publishes a different list under the same day —
-- or, on a day where barely ten postings clear the floor, publishes nothing and
-- reports it as a quiet day. `-day` replay breaks identically, which would defeat
-- treating this table as the archive of what actually went out.
SELECT DISTINCT job_id
FROM social_digest_posts
WHERE day >= sqlc.arg('since') AND day < sqlc.arg('before');

-- name: DigestPublishedForChannel :one
-- The publish-once check. Keyed on the channel and not on the day alone: a run that
-- posted to Discord and then failed on LinkedIn must, next time, skip Discord and
-- retry LinkedIn.
SELECT EXISTS(
    SELECT 1 FROM social_digest_posts
    WHERE day = sqlc.arg('day') AND channel = sqlc.arg('channel')
);

-- name: RecordDigestPost :batchexec
-- Ledger write, one row per posting in the published list. Written only AFTER a
-- channel has published; a dry run never reaches here. ON CONFLICT DO NOTHING so a
-- retry that races itself cannot fail the run over a row that already says what we
-- were about to say.
INSERT INTO social_digest_posts (day, channel, job_id, slot)
VALUES (sqlc.arg('day'), sqlc.arg('channel'), sqlc.arg('job_id'), sqlc.arg('slot'))
ON CONFLICT (day, channel, job_id) DO NOTHING;
