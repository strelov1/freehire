-- The company slugs a merge retired, and the canonical slug each was merged into.
--
-- WHY THIS TABLE IS NOT DERIVED, unlike everything else around companies. `companies` is
-- rebuilt from `jobs` by SyncCompaniesFromJobs, and DeleteOrphanCompanies removes any row no
-- job references. A canonical decision recorded there would therefore disappear the day an
-- employer's last posting closes — and the next posting spelled the other way would open a
-- fresh company, restoring the duplicate on a timer. The decision has to outlive the row that
-- motivated it, so it lives here and nothing rebuilds it.
--
-- For the same reason there is deliberately NO foreign key from canonical_slug to
-- companies(slug). It would be the intuitive constraint to add and it would delete exactly the
-- rows this table exists to keep.
--
-- ONE TABLE, READ FROM BOTH ENDS. They are the same relation, not two concerns:
--
--   ingest (pipeline.Runner)   folded_key = ANY($1)  -> which canonical slug this spelling is
--   GET /companies/:slug       alias_slug = $1       -> where to 301
--
-- folded_key is the alias's slug with hyphens removed — normalize.CompanyKey, the same fold
-- jobs.company_slug_folded stores (migration 0109). It is what lets a NEVER-SEEN spelling
-- reach the canon: "DollarTree" has no alias row of its own, but it folds onto the key
-- "dollartree" already registered against dollar-tree.
--
-- WHY reason IS NOT DECORATION. The two merge classes rest on different evidence. `legal_form`
-- is a pure rule (normalize.CompanySlug) and `spelling` is a judgement elected by job count at
-- merge time. If the rule later proves to have over-stripped, the fix must be able to reverse
-- one class without touching the other; without this column that means re-deriving which rows
-- were which, from a catalogue the merge has already rewritten.
--
-- The catalogue is the reason the rows exist at all: measured on prod 2026-08-17, 5,451 folded
-- groups covering 11,211 companies and 333,497 open jobs — 24% of the open catalogue — are one
-- employer split across spellings its sources happened to use.
--
-- This is a new, empty table, so the index is a plain CREATE INDEX: there is nothing to scan
-- and no lock worth naming. Do NOT copy the CONCURRENTLY dance from the migrations that add an
-- index to `jobs`.
CREATE TABLE IF NOT EXISTS company_slug_aliases (
    alias_slug     text PRIMARY KEY,
    canonical_slug text NOT NULL,
    folded_key     text NOT NULL,
    reason         text NOT NULL,
    created_at     timestamptz NOT NULL DEFAULT now(),

    -- An alias to itself is not a merge; it is a resolver that returns its own input and a
    -- 301 that loops. Refuse it at the boundary rather than defending against it in two
    -- read paths.
    CONSTRAINT company_slug_aliases_not_self CHECK (alias_slug <> canonical_slug),
    CONSTRAINT company_slug_aliases_reason CHECK (reason IN ('legal_form', 'spelling'))
);

CREATE INDEX IF NOT EXISTS company_slug_aliases_folded_key_idx
    ON company_slug_aliases (folded_key);

COMMENT ON TABLE company_slug_aliases IS
    'Retired company slug -> the canonical slug it merged into. The one company-adjacent '
    'table that is NOT derived from jobs: DeleteOrphanCompanies would drop a canon stored '
    'in companies as soon as the employer went quiet. Read by folded_key on ingest and by '
    'alias_slug to serve a 301.';

COMMENT ON COLUMN company_slug_aliases.folded_key IS
    'alias_slug with hyphens removed (normalize.CompanyKey), so a spelling never merged '
    'before still resolves to the canon its folded form already owns.';

COMMENT ON COLUMN company_slug_aliases.reason IS
    'legal_form (a pure normalize.CompanySlug strip) or spelling (a job-count election at '
    'merge time). Recorded so one class of merge can be reversed without the other.';
