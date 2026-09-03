-- name: MineJobTitles :many
-- Raw posting titles with their counts, the source of the suggestion dictionary
-- (cmd/build-suggestions).
--
-- Grouped HERE rather than streamed row by row: the open catalogue is ~1.8M postings
-- written with far fewer distinct titles, so aggregating in Postgres keeps the wire
-- and the worker's memory proportional to the vocabulary rather than to the
-- catalogue.
--
-- Normalisation is deliberately NOT done here, and neither is the frequency floor.
-- `suggest.Title` is one function shared with the query path — a SQL copy would drift
-- — and the floor has to be applied AFTER it: "Product Owner", "product owner" and
-- "PRODUCT OWNER" are three rows here and one suggestion, so a floor applied to these
-- counts would drop a title that clears it comfortably once merged.
SELECT title, count(*) AS count
FROM jobs
WHERE closed_at IS NULL
  AND title <> ''
GROUP BY title;

-- name: SuggestibleCompanies :many
-- Companies worth offering as a suggestion, busiest first. Reads the denormalized
-- companies.job_count (maintained by cmd/recount-companies), so this does not join
-- jobs.
--
-- The floor is what keeps the long tail of one-posting slugs — many of them job
-- titles that landed in an employer column — out of a dictionary meant to name real
-- employers.
SELECT slug, name, job_count
FROM companies
WHERE job_count >= @min_jobs::int
ORDER BY job_count DESC, name;
