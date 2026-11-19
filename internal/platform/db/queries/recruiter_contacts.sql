-- name: GetJobRecruiterDomain :one
-- A job's id and its company's site host ('' when the company has no site on record).
SELECT j.id, COALESCE(company_site_host(c.company_info), '')::text AS domain
FROM jobs j
LEFT JOIN companies c ON c.slug = j.company_slug
WHERE j.public_slug = $1;

-- name: GetJobRecruiterContacts :one
SELECT contacts, found_at FROM job_recruiter_contacts WHERE job_id = $1;

-- name: PutJobRecruiterContacts :exec
-- found_at is passed, so pruning a suppressed address keeps the original age.
INSERT INTO job_recruiter_contacts (job_id, contacts, found_at)
VALUES ($1, $2, $3)
ON CONFLICT (job_id) DO UPDATE SET contacts = EXCLUDED.contacts, found_at = EXCLUDED.found_at;

-- name: DeleteExpiredJobRecruiterContacts :execrows
DELETE FROM job_recruiter_contacts
WHERE found_at < now() - interval '30 days'
   OR (contacts = '[]'::jsonb AND found_at < now() - interval '7 days');
