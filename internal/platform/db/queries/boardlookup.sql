-- Board lookups behind the "contribute a board" flow: does the catalogue already crawl
-- this board, and which board does a pasted job id belong to. Named after
-- link_contributions for historical reasons — that table is gone (migration 0131), and
-- these queries read `jobs`.

-- name: JobsExistForBoard :one
-- Whether the catalogue already crawls this board — any job whose external_id is "<board>:…".
-- Matched with a LIKE-prefix so the (source, external_id text_pattern_ops) index serves it as
-- a range scan; starts_with()/a default-collation LIKE would seq-scan the whole source (37s
-- over greenhouse's ~300k rows). board_pattern is "<escaped board>:%", built by the repository.
SELECT EXISTS (
    SELECT 1 FROM jobs WHERE source = sqlc.arg(source) AND external_id LIKE sqlc.arg(board_pattern)
) AS exists;

-- name: BoardByGreenhouseJobID :one
-- Find the greenhouse board already carrying a job with this Greenhouse job id — for links on
-- a company's own domain that expose only the ATS job id (server-side embeds, no board token
-- in the URL/page). external_id is "<board>:<id>"; served by the
-- (split_part(external_id,':',2)) WHERE source='greenhouse' partial index.
SELECT split_part(external_id, ':', 1) AS board
FROM jobs
WHERE source = 'greenhouse' AND split_part(external_id, ':', 2) = sqlc.arg(job_id)
LIMIT 1;

-- name: BoardByAshbyJobID :one
-- Find the ashby board already carrying a job with this Ashby job id — for company careers
-- pages that embed Ashby via the ashby_jid widget param (the board slug is JS-rendered, absent
-- from the URL/markup). external_id is "<board>:<uuid>"; served by the
-- (split_part(external_id,':',2)) WHERE source='ashby' partial index.
SELECT split_part(external_id, ':', 1) AS board
FROM jobs
WHERE source = 'ashby' AND split_part(external_id, ':', 2) = sqlc.arg(job_id)
LIMIT 1;

-- name: CompanyForBoard :one
-- The tracked company on a board — for the "already tracked" reply: a job's company name and
-- slug so the bot/UI can link to /companies/<slug>. board_pattern is "<escaped board>:%" (same
-- index-backed LIKE as JobsExistForBoard). Only rows with a resolved company_slug qualify.
SELECT company, company_slug FROM jobs
WHERE source = sqlc.arg(source) AND external_id LIKE sqlc.arg(board_pattern) AND company_slug <> ''
LIMIT 1;
