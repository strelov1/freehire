-- Paycor boards named only by their 32-hex clientId show "Employer via Paycor" instead of the
-- id (#4548). The company slug stays the id's, so each board remains its own company page; only
-- the display name changes. New and changed postings get the label from the adapter
-- (sources.PlaceholderLabel); this moves the postings a crawl would leave untouched, queues the
-- open public ones for search, and renames their companies rows.
WITH moved AS (
    UPDATE jobs
    SET company = 'Employer via Paycor', updated_at = now()
    WHERE source = 'paycor' AND company ~ '^[0-9a-fA-F]{32}$'
    RETURNING id, company_slug, COALESCE(posted_at, created_at) AS eff_posted_at,
        closed_at IS NULL AND duplicate_of IS NULL AND NOT is_private AS indexed
), queued AS (
    INSERT INTO search_outbox (job_id, job_posted_at)
    SELECT id, eff_posted_at FROM moved WHERE indexed
    ON CONFLICT (job_id) DO UPDATE
        SET id = DEFAULT, created_at = now(), attempts = 0, last_error = '', failed_at = NULL
)
UPDATE companies SET name = 'Employer via Paycor', updated_at = now()
WHERE slug IN (SELECT company_slug FROM moved) AND NOT is_reference;
