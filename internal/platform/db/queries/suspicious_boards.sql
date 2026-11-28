-- name: ListOpenJobsByCompanyBoard :many
-- Open postings per (company slug, first-party catalog board), with the company name the
-- board row states, for cmd/report-suspicious-boards. A full scan of the open jobs: hand-run
-- only, and against the replica. Aggregator sources are excluded by the caller's list, and a
-- posting no live non-hub board row names (a link import, a retired board) drops out of the join.
SELECT j.company_slug::text AS company_slug,
       j.source::text       AS provider,
       j.board::text        AS board,
       j.open_jobs::int     AS open_jobs,
       b.company::text      AS board_company
FROM (
    SELECT company_slug, source, split_part(external_id, ':', 1) AS board, count(*) AS open_jobs
    FROM jobs
    WHERE closed_at IS NULL
      AND NOT is_private
      AND company_slug <> ''
      AND source <> ALL(sqlc.arg(aggregators)::text[])
    GROUP BY 1, 2, 3
) j
JOIN LATERAL (
    SELECT bd.company
    FROM boards bd
    WHERE bd.provider = j.source
      AND lower(bd.board) = lower(j.board)
      AND bd.status IN ('pending', 'active')
      AND NOT bd.hub
    LIMIT 1
) b ON true;

-- name: ListHiringCompanySites :many
-- Every company with open public jobs and the website it states (curated, else from a feed,
-- else ''), for cmd/report-suspicious-boards. Membership comes from the open postings, not
-- job_count: that column counts the search index, so it can be 0 while a board still feeds the
-- slug, and the company would lose its website-owner signal.
SELECT slug, name,
       COALESCE(NULLIF(company_info ->> 'website', ''), company_info ->> 'feed_website', '')::text AS website
FROM companies c
WHERE EXISTS (
    SELECT 1 FROM jobs j
    WHERE j.company_slug = c.slug AND j.closed_at IS NULL AND NOT j.is_private
);
