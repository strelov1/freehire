-- name: OpenJobTitlesByFoldedCompany :many
-- Every open posting's title for a batch of folded company keys (normalize.CompanyKey) —
-- cmd/catalog-coverage matches an outside sample against these by jobhash.RoleKey. Duplicates
-- are kept on purpose: a posting we hold twice is still one we hold. Private postings (a
-- user's pasted JD) are not catalogue. Served by
-- jobs_open_company_slug_folded_col_idx, whose predicate the last two conditions repeat: drop
-- one and the planner scans the whole table.
SELECT company_slug_folded::text AS company_key, title
FROM jobs
WHERE company_slug_folded = ANY(sqlc.arg(company_keys)::text[]) AND closed_at IS NULL AND company_slug <> '' AND NOT is_private;

-- name: OpenJobsOnBoard :many
-- One board's open postings: the namespaced id (whose native part an apply link to the posting
-- carries) and the title. board_pattern is externalid.BoardPattern of the stored board name, so
-- the (source, external_id text_pattern_ops) index serves it as a range scan.
SELECT external_id, title
FROM jobs
WHERE source = sqlc.arg(source) AND external_id LIKE sqlc.arg(board_pattern) AND closed_at IS NULL
  AND NOT is_private;
