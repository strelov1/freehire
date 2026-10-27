-- Rewrite the experience clause of every profile-derived saved search to the rule the
-- SPA now seeds (freehire#3655; the scope and the splitting are @oguzzkd's, from #3664).
--
-- A "Jobs matching my profile" alert is a saved query built by filtersFromProfile. For a
-- profile whose levels are all intern/junior, #3414 bounded experience AND ticked
-- experience_include_unspecified, which kept every posting stating no years — 2.66M
-- open postings on an intern's alert, the senior ones among them. Dropping that modifier
-- alone overcorrects: most LABELLED internships state no years either (4,071 of 95,645
-- open intern postings state at most a year — prod, 2026-10-26), so an intern would stop
-- receiving internships. The seeder now writes experience_only_seniority_unspecified
-- instead: a posting carrying a picked level passes whatever it states, and one with no
-- level must state a requirement within the cap. A stored alert keeps the old clause
-- until its owner next saves the profile, so this file is the one-off bridge.
--
-- Scope and rule:
--   * derived_from_profile rows only. A manually saved search is the user's own query
--     and is never rewritten.
--   * cap 1 when every selected level is intern; cap 2 when the profile selects only
--     intern and/or junior and at least one is junior; no cap otherwise. A missing
--     profile row is treated as "no cap". This mirrors entryExperienceCap in
--     web/src/lib/facetModel.ts.
--   * the query is a URL-encoded filter string, so it is split on '&' — a value cannot
--     contain a raw '&' — and only the experience params are removed/added. Every other
--     filter survives in order.
--   * a rewrite that would leave a non-empty query empty is skipped: widening a broken
--     derived alert to the whole catalogue is worse than leaving it alone.
--   * updated_at is deliberately untouched. This corrects a derived value; it is not a
--     user edit, and bumping it would reorder every alert in its owner's saved searches.
--
-- Idempotent: the UPDATE is guarded by IS DISTINCT FROM, so a second run writes nothing.
-- saved_searches holds a few thousand rows and the join is on user_profiles.user_id.

WITH target AS (
    SELECT s.id,
           s.query,
           CASE
               WHEN p.seniorities IS NOT NULL
                AND cardinality(p.seniorities) > 0
                AND NOT EXISTS (
                    SELECT 1
                    FROM unnest(p.seniorities) AS lvl(value)
                    WHERE lvl.value NOT IN ('intern', 'junior')
                )
               THEN CASE WHEN 'junior' = ANY (p.seniorities) THEN 2 ELSE 1 END
               ELSE NULL
           END AS cap
    FROM public.saved_searches s
    LEFT JOIN public.user_profiles p ON p.user_id = s.user_id
    WHERE s.derived_from_profile
),
stripped AS (
    SELECT t.id,
           t.query AS original,
           t.cap,
           COALESCE(
               (
                   SELECT string_agg(part, '&' ORDER BY ord)
                   FROM unnest(string_to_array(t.query, '&')) WITH ORDINALITY AS parts(part, ord)
                   WHERE split_part(parts.part, '=', 1) NOT IN (
                       'experience_years_max',
                       'experience_include_unspecified',
                       'experience_only_seniority_unspecified'
                   )
               ),
               ''
           ) AS query
    FROM target t
),
planned AS (
    SELECT id,
           original,
           CASE
               WHEN cap IS NULL THEN query
               ELSE concat_ws('&', NULLIF(query, ''), 'experience_years_max=' || cap,
                              'experience_only_seniority_unspecified=true')
           END AS query
    FROM stripped
)
UPDATE public.saved_searches s
SET query = planned.query
FROM planned
WHERE s.id = planned.id
  AND s.query IS DISTINCT FROM planned.query
  AND NOT (planned.query = '' AND planned.original <> '');
