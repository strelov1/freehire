-- Add the profile's companies to avoid to every profile-derived saved search, the clause
-- the SPA now seeds (filtersFromProfile in web/src/lib/facetModel.ts).
--
-- A "Jobs matching my profile" alert is a saved query built by filtersFromProfile, and a
-- stored one keeps the clauses it was built with until its owner next saves the profile.
-- excluded_companies was never one of them, so an alert enabled before the SPA seeded it
-- keeps matching companies its owner has ruled out. This file is the one-off bridge, as
-- 0205 was for the experience clause.
--
-- Scope and rule:
--   * derived_from_profile rows only. A manually saved search is the user's own query
--     and is never rewritten.
--   * company_slug_exclude is removed and, when the owner's profile avoids a company,
--     appended again; every other filter survives in order. A missing profile row counts
--     as avoiding nothing.
--   * the list mirrors charBudget with COMPANY_CHAR_BUDGET (400): values in stored order,
--     each costing its UTF-16 length plus one for the joining comma, stopping at the first
--     value that does not fit.
--   * the value is encoded the way URLSearchParams serializes it: UTF-8 bytes, a space as
--     '+', and every byte outside [A-Za-z0-9*-._] as %XX, so the comma becomes %2C.
--   * a rewrite that would leave a non-empty query empty, or push it past savedsearch's
--     4000-rune maxQueryLen, is skipped.
--   * updated_at is deliberately untouched. This corrects a derived value; it is not a
--     user edit, and bumping it would reorder every alert in its owner's saved searches.
--
-- Idempotent: the UPDATE is guarded by IS DISTINCT FROM, so a second run writes nothing.

WITH target AS (
    SELECT s.id,
           s.query,
           COALESCE(p.excluded_companies, '{}'::text[]) AS companies
    FROM public.saved_searches s
    LEFT JOIN public.user_profiles p ON p.user_id = s.user_id
    WHERE s.derived_from_profile
),
costed AS (
    SELECT t.id,
           c.ord,
           c.value,
           char_length(c.value)
               + char_length(regexp_replace(c.value, '[^\U00010000-\U0010FFFF]', '', 'g'))
               + CASE WHEN row_number() OVER (PARTITION BY t.id ORDER BY c.ord) > 1 THEN 1 ELSE 0 END
               AS cost
    FROM target t
    CROSS JOIN LATERAL (
        SELECT btrim(raw.value) AS value, raw.ord
        FROM unnest(t.companies) WITH ORDINALITY AS raw(value, ord)
    ) c
    WHERE c.value <> ''
),
kept AS (
    SELECT id, string_agg(value, ',' ORDER BY ord) AS list
    FROM (
        SELECT id, ord, value, sum(cost) OVER (PARTITION BY id ORDER BY ord) AS used
        FROM costed
    ) running
    WHERE used <= 400
    GROUP BY id
),
encoded AS (
    SELECT k.id,
           (
               SELECT string_agg(
                          CASE
                              WHEN b = 32 THEN '+'
                              WHEN b IN (42, 45, 46, 95)
                                OR b BETWEEN 48 AND 57
                                OR b BETWEEN 65 AND 90
                                OR b BETWEEN 97 AND 122 THEN chr(b)
                              ELSE '%' || upper(lpad(to_hex(b), 2, '0'))
                          END,
                          '' ORDER BY i)
               FROM generate_series(0, octet_length(convert_to(k.list, 'UTF8')) - 1) AS i
               CROSS JOIN LATERAL (SELECT get_byte(convert_to(k.list, 'UTF8'), i) AS b) byte
           ) AS value
    FROM kept k
),
stripped AS (
    SELECT t.id,
           t.query AS original,
           COALESCE(
               (
                   SELECT string_agg(part, '&' ORDER BY ord)
                   FROM unnest(string_to_array(t.query, '&')) WITH ORDINALITY AS parts(part, ord)
                   WHERE split_part(parts.part, '=', 1) <> 'company_slug_exclude'
               ),
               ''
           ) AS query
    FROM target t
),
planned AS (
    SELECT s.id,
           s.original,
           concat_ws('&', NULLIF(s.query, ''), 'company_slug_exclude=' || e.value) AS query
    FROM stripped s
    LEFT JOIN encoded e ON e.id = s.id
)
UPDATE public.saved_searches s
SET query = planned.query
FROM planned
WHERE s.id = planned.id
  AND s.query IS DISTINCT FROM planned.query
  AND NOT (planned.query = '' AND planned.original <> '')
  AND char_length(planned.query) <= 4000;
