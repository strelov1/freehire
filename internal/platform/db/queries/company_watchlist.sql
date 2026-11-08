-- A person's company watchlist (migration 0248). company_slug carries no foreign key, so a
-- stored slug may belong to a company a merge has since retired. Every read below follows it
-- to the canonical slug through company_slug_aliases — but only while no company row still
-- holds the slug, the same order GET /companies/:slug uses, so a company that came back is
-- never shadowed by the alias that once retired it. Two stored slugs that resolve to one
-- company collapse into a single entry, the older date winning.

-- name: ListWatchlist :many
-- One page of the person's watchlist, newest first, with each company's display name
-- (the slug itself when no company row carries it any more) and the number of entries the
-- name filter leaves. The filter is a substring test on the folded name, not LIKE, so a
-- `%` or `_` typed into it means itself.
WITH entries AS (
    SELECT COALESCE(a.canonical_slug, w.company_slug) AS slug,
           min(w.created_at)::timestamptz AS added_at
    FROM company_watchlist w
    LEFT JOIN company_slug_aliases a
           ON a.alias_slug = w.company_slug
          AND NOT EXISTS (SELECT 1 FROM companies c WHERE c.slug = w.company_slug)
    WHERE w.user_id = @user_id
    GROUP BY 1
),
named AS (
    SELECT e.slug, COALESCE(c.name, e.slug) AS name, e.added_at
    FROM entries e
    LEFT JOIN companies c ON c.slug = e.slug
)
SELECT n.slug, n.name, n.added_at, count(*) OVER ()::bigint AS total
FROM named n
WHERE @q::text = '' OR strpos(lower(n.name), lower(@q::text)) > 0
ORDER BY n.added_at DESC, n.slug
LIMIT @page_limit OFFSET @page_offset;

-- name: ListWatchlistSlugs :many
-- Every company the person watches, resolved as ListWatchlist resolves them: what the
-- alert's search is narrowed to.
SELECT DISTINCT COALESCE(a.canonical_slug, w.company_slug)::text AS slug
FROM company_watchlist w
LEFT JOIN company_slug_aliases a
       ON a.alias_slug = w.company_slug
      AND NOT EXISTS (SELECT 1 FROM companies c WHERE c.slug = w.company_slug)
WHERE w.user_id = @user_id
ORDER BY 1;

-- name: ListWatchlistSlugsForUsers :many
-- ListWatchlistSlugs for a batch of people in one round trip: the notify worker resolves
-- every watchlist alert's owner at once.
SELECT DISTINCT w.user_id, COALESCE(a.canonical_slug, w.company_slug)::text AS slug
FROM company_watchlist w
LEFT JOIN company_slug_aliases a
       ON a.alias_slug = w.company_slug
      AND NOT EXISTS (SELECT 1 FROM companies c WHERE c.slug = w.company_slug)
WHERE w.user_id = ANY(@user_ids::bigint[])
ORDER BY 1, 2;

-- name: IsWatchingCompany :one
-- Whether the person watches this company, by the slug a company page carries. A stored
-- alias of it counts: the person added the company under its old spelling. An alias slug a
-- company row still holds is its own company, as in the reads above, and does not count.
SELECT EXISTS (
    SELECT 1 FROM company_watchlist w
    WHERE w.user_id = @user_id
      AND (w.company_slug = @slug
           OR w.company_slug IN (SELECT a.alias_slug FROM company_slug_aliases a
                                 WHERE a.canonical_slug = @slug
                                   AND NOT EXISTS (SELECT 1 FROM companies c WHERE c.slug = a.alias_slug)))
);

-- name: ResolveWatchableCompany :one
-- The company a slug names, following a retired slug to its canonical one. No row means no
-- such company, which is what a typo in a request looks like. A company with no open job
-- still resolves — it is the one a watchlist is for.
SELECT c.slug, c.name
FROM companies c
WHERE c.slug = @slug
UNION ALL
SELECT c.slug, c.name
FROM company_slug_aliases a
JOIN companies c ON c.slug = a.canonical_slug
WHERE a.alias_slug = @slug
  AND NOT EXISTS (SELECT 1 FROM companies x WHERE x.slug = @slug)
LIMIT 1;

-- name: AddToWatchlist :one
-- Add one company, or find it already there, and return when it was added. No row means the
-- list is at the limit and this company is not on it: the limit never refuses a company
-- already stored, so adding it again stays idempotent on a full list.
INSERT INTO company_watchlist (user_id, company_slug)
SELECT @user_id, @slug::text
WHERE (SELECT count(*) FROM company_watchlist WHERE user_id = @user_id) < @max_entries::bigint
   OR EXISTS (SELECT 1 FROM company_watchlist WHERE user_id = @user_id AND company_slug = @slug::text)
ON CONFLICT (user_id, company_slug) DO UPDATE SET company_slug = EXCLUDED.company_slug
RETURNING created_at;

-- name: AddManyToWatchlist :one
-- Add a batch all-or-nothing: when the new slugs would take the list past the limit nothing
-- is written. Slugs already stored are skipped and do not count toward it. `fresh` is how
-- many were new and `inserted` how many were written, so fresh > 0 with inserted = 0 is the
-- limit refusing the batch.
WITH fresh AS (
    SELECT DISTINCT s.slug
    FROM unnest(@slugs::text[]) AS s(slug)
    WHERE NOT EXISTS (
        SELECT 1 FROM company_watchlist w WHERE w.user_id = @user_id AND w.company_slug = s.slug
    )
),
ins AS (
    INSERT INTO company_watchlist (user_id, company_slug)
    SELECT @user_id, f.slug FROM fresh f
    WHERE (SELECT count(*) FROM company_watchlist WHERE user_id = @user_id)
          + (SELECT count(*) FROM fresh) <= @max_entries::bigint
    ON CONFLICT (user_id, company_slug) DO NOTHING
    RETURNING 1
)
SELECT (SELECT count(*) FROM fresh)::bigint AS fresh,
       (SELECT count(*) FROM ins)::bigint AS inserted;

-- name: RemoveFromWatchlist :exec
-- Remove a company by the slug a page carries, together with any stored alias of it that no
-- company row still holds (that one is a company of its own, and stays).
DELETE FROM company_watchlist
WHERE user_id = @user_id
  AND (company_slug = @slug
       OR company_slug IN (SELECT a.alias_slug FROM company_slug_aliases a
                           WHERE a.canonical_slug = @slug
                             AND NOT EXISTS (SELECT 1 FROM companies c WHERE c.slug = a.alias_slug)));

-- name: ListWatchableCompaniesByFoldedSlug :many
-- The companies — hiring or not — whose slug folds to one of the keys or IS one of the
-- slugs: the candidates a typed company name is resolved among. Unlike
-- ListHiringCompaniesByFoldedSlug it keeps a company with nothing open, since that is the one
-- a watchlist most wants; job_count is returned so the caller can prefer a hiring namesake.
-- The keys are joined through unnest for the same reason as there: one probe of
-- companies_folded_slug_idx each.
SELECT c.slug, c.name, c.job_count
FROM unnest(@folded_keys::text[]) AS k(key)
JOIN companies c ON replace(c.slug, '-', '') = k.key
UNION
SELECT c.slug, c.name, c.job_count
FROM companies c
WHERE c.slug = ANY(@slugs::text[]);
