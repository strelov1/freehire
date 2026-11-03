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
--
-- An approved merge whose retiring company still has a row is listed too: its job move
-- stopped part-way (the reconcile deletes that row last), and approving it again resumes it.
SELECT p.id, p.kind, p.company_slug, p.target_slug, p.note, p.created_at, p.status,
       COALESCE(c.name, '')::text      AS company_name,
       COALESCE(c.job_count, 0)::int   AS company_job_count,
       COALESCE(t.name, '')::text      AS target_name,
       COALESCE(t.job_count, 0)::int   AS target_job_count
FROM company_relation_proposals p
LEFT JOIN companies c ON c.slug = p.company_slug
LEFT JOIN companies t ON t.slug = p.target_slug
WHERE p.status = 'pending'
   OR (p.status = 'approved' AND p.kind = 'same' AND c.slug IS NOT NULL)
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

-- name: LockCompanyRelationDecisions :exec
-- Serializes approvals for the transaction (key "fhcr", listed in internal/platform/migrate).
-- Each approval checks the registry and company_parents and then writes them, and two
-- approvals checked against the same state could each pass while together making a chain.
SELECT pg_advisory_xact_lock(1718117234);

-- name: LatestCompanySpelling :one
-- The company name a slug's most recent posting carries: the spelling the next crawl will
-- send, and so the one whose fold ingest will look up. companies.name can be a curated
-- display name that folds differently.
SELECT company FROM jobs
WHERE company_slug = @slug AND company <> ''
ORDER BY created_at DESC, id DESC
LIMIT 1;

-- name: IsCanonicalCompanySlug :one
-- Whether some retired slug already redirects here. Such a slug cannot itself retire: the
-- registry resolves one hop, so its aliases would land on a slug with no jobs.
SELECT EXISTS (SELECT 1 FROM company_slug_aliases WHERE canonical_slug = @slug);

-- name: RepointCompanyParentsOnMerge :exec
-- A merge keeps the subsidiaries the retiring slug had. They move under the canonical slug,
-- or under the canon's own parent when it has one, since one level is the rule. A link that
-- would then point a company at itself is left for the delete below.
UPDATE company_parents p
SET parent_slug = t.slug
FROM (
    SELECT COALESCE(
        (SELECT cp.parent_slug FROM company_parents cp WHERE cp.child_slug = @canonical_slug),
        @canonical_slug::text) AS slug
) t
WHERE p.parent_slug = @alias_slug
  AND p.child_slug <> t.slug;

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

-- name: ListCompanyRelatives :many
-- A company page's parent (role 'parent') and subsidiaries (role 'subsidiary') in one round
-- trip, since every company read pays for it and almost every company has neither. The join
-- through companies hides a relative that has no row any more. Subsidiaries come hiring
-- first, and are bounded because a group can own many.
(SELECT 'parent'::text AS role, c.slug, c.name, c.job_count
 FROM company_parents p
 JOIN companies c ON c.slug = p.parent_slug
 WHERE p.child_slug = @slug)
UNION ALL
(SELECT 'subsidiary'::text AS role, c.slug, c.name, c.job_count
 FROM company_parents p
 JOIN companies c ON c.slug = p.child_slug
 WHERE p.parent_slug = @slug
 ORDER BY c.job_count DESC, c.slug
 LIMIT 100);

-- name: SetCompanyFeedWebsiteIfAbsent :exec
-- The employer's own website as a job feed states it in a structured field (EURES's
-- employer.website). It goes under its own key, feed_website, never `website`. The curated
-- fillers (Wikipedia, YC, the employer's claim) only fill a blank `website` and would keep a
-- feed value there forever, and publish-logo-domains and the company header trust `website`
-- as curated. Only the duplicate report reads this key. Written once: the first feed value
-- stays.
UPDATE companies
SET company_info = company_info || jsonb_build_object('feed_website', @website::text),
    updated_at   = now()
WHERE slug = @slug
  AND COALESCE(company_info ->> 'feed_website', '') = '';

-- name: ListCompanyWebsitesForTwins :many
-- Companies with open jobs and a website, curated or from a feed, for
-- cmd/report-company-website-twins. Grouping happens in Go through logodomain.Domain, the
-- reader the logo map uses, so the two agree on what a shared site is.
SELECT slug, name, job_count,
       COALESCE(NULLIF(company_info ->> 'website', ''), company_info ->> 'feed_website')::text AS website
FROM companies
WHERE job_count > 0
  AND (company_info ->> 'website' <> '' OR company_info ->> 'feed_website' <> '');
