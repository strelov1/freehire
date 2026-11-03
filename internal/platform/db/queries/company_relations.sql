-- name: InsertCompanyRelationProposal :one
-- A signed-in user's proposal that two companies are one employer (`same`) or that one owns
-- the other (`subsidiary`). The partial unique index on pending rows makes a duplicate a
-- unique violation, which the service reports as "already proposed".
INSERT INTO company_relation_proposals (kind, company_slug, target_slug, note, proposed_by)
VALUES (@kind, @company_slug, @target_slug, @note, @proposed_by)
RETURNING *;

-- name: ListPendingCompanyRelationProposals :many
-- The moderator queue, oldest first, with both companies' current names and open-job counts
-- so the decision can be made without opening two pages. LEFT JOIN because a company may have
-- lost its last posting since the proposal. The approval then refuses it, and the queue
-- still has to show it to be rejected.
SELECT p.id, p.kind, p.company_slug, p.target_slug, p.note, p.created_at,
       COALESCE(c.name, '')::text      AS company_name,
       COALESCE(c.job_count, 0)::int   AS company_job_count,
       COALESCE(t.name, '')::text      AS target_name,
       COALESCE(t.job_count, 0)::int   AS target_job_count
FROM company_relation_proposals p
LEFT JOIN companies c ON c.slug = p.company_slug
LEFT JOIN companies t ON t.slug = p.target_slug
WHERE p.status = 'pending'
ORDER BY p.created_at, p.id
LIMIT 200;

-- name: GetCompanyRelationProposalForUpdate :one
-- The row a decision is about, locked for the decision's transaction so two moderators
-- cannot approve and reject it at once.
SELECT * FROM company_relation_proposals WHERE id = @id FOR UPDATE;

-- name: DecideCompanyRelationProposal :exec
UPDATE company_relation_proposals
SET status = @status, decided_by = @decided_by, decided_at = now()
WHERE id = @id;

-- name: IsCanonicalCompanySlug :one
-- Whether some retired slug already redirects here. Such a slug cannot itself retire: the
-- registry resolves one hop, so its aliases would land on a slug with no jobs.
SELECT EXISTS (SELECT 1 FROM company_slug_aliases WHERE canonical_slug = @slug);

-- name: RepointCompanyParentsOnMerge :exec
-- A merge keeps the subsidiaries the retiring slug had, under the canonical slug. The one
-- link that would point the canon at itself is left for the delete below.
UPDATE company_parents
SET parent_slug = @canonical_slug
WHERE parent_slug = @alias_slug AND child_slug <> @canonical_slug;

-- name: DeleteCompanyParentLinksOf :exec
-- Whatever links still name the retiring slug: its own parent (the canon may have another,
-- or be a parent itself, and one level is the rule), and a link to the canon it has just
-- become.
DELETE FROM company_parents WHERE child_slug = @slug OR parent_slug = @slug;

-- name: UpsertCompanyParent :exec
-- A subsidiary has one parent, so a newer approved decision replaces the older one.
INSERT INTO company_parents (child_slug, parent_slug)
VALUES (@child_slug, @parent_slug)
ON CONFLICT (child_slug) DO UPDATE SET parent_slug = EXCLUDED.parent_slug, created_at = now();

-- name: CompanyHasParent :one
SELECT EXISTS (SELECT 1 FROM company_parents WHERE child_slug = @slug);

-- name: CompanyHasSubsidiaries :one
SELECT EXISTS (SELECT 1 FROM company_parents WHERE parent_slug = @slug);

-- name: GetCompanyParent :one
-- The parent a company page names. The join through companies hides a parent that has no
-- row any more.
SELECT c.slug, c.name, c.job_count
FROM company_parents p
JOIN companies c ON c.slug = p.parent_slug
WHERE p.child_slug = @slug;

-- name: ListCompanySubsidiaries :many
-- The subsidiaries a parent's page lists, the ones hiring first.
SELECT c.slug, c.name, c.job_count
FROM company_parents p
JOIN companies c ON c.slug = p.child_slug
WHERE p.parent_slug = @slug
ORDER BY c.job_count DESC, c.slug
LIMIT 100;

-- name: SetCompanyWebsiteIfAbsent :exec
-- The employer's own website, as a source states it in a structured field (EURES's
-- employer.website). It is written only where company_info has no website yet, the same
-- fill-only-if-blank rule SeedCompanyAccountWebsite follows: a curated source or the
-- employer's own edit outranks a job feed. The guard also turns the crawl's repeat
-- into a read.
UPDATE companies
SET company_info    = company_info || jsonb_build_object('website', @website::text),
    company_info_at = now(),
    updated_at      = now()
WHERE slug = @slug
  AND (NOT (company_info ? 'website') OR company_info ->> 'website' = '');

-- name: ListCompanyWebsiteTwins :many
-- Companies with open jobs that state the same website host but sit under different slugs.
-- These are merge candidates for cmd/report-company-website-twins, not decisions: a staffing
-- agency's branches share a host and are one employer, while a job board's host can stand
-- behind many employers. A person reads the list and proposes the merges.
WITH hosts AS (
    SELECT slug, name, job_count,
           lower(regexp_replace(
               regexp_replace(company_info ->> 'website', '^[a-zA-Z]+://', ''),
               '^www\.|[:/?#].*$', '', 'g')) AS host
    FROM companies
    WHERE job_count > 0
      AND company_info ? 'website'
      AND company_info ->> 'website' <> ''
)
SELECT host::text AS host, slug, name, job_count
FROM hosts
WHERE host <> ''
  AND host IN (SELECT host FROM hosts GROUP BY host HAVING count(*) > 1)
ORDER BY host, job_count DESC, slug;
