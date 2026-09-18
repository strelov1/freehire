-- Links one user to one company they have claimed and proven a work-email tie to, so that
-- user may edit that company's curated profile and publish vacancies for it directly (see
-- openspec/changes/add-employer-company-accounts).
--
-- WHY THIS TABLE IS NOT DERIVED, like company_slug_aliases (migration 0112) before it:
-- `companies` is rebuilt from `jobs` by SyncCompaniesFromJobs, and DeleteOrphanCompanies
-- removes any non-reference row no job references. A claim recorded only on `companies`
-- would vanish the moment an employer's last posting closed (or before their first one is
-- even published), and the next self-service attempt would silently start over. The claim
-- has to outlive the row that motivated it, so it lives here.
--
-- `user_id` is the primary key, not a separate unique index: one user manages at most one
-- company on this MVP (see the proposal's scope decisions), and making that a schema-level
-- fact rather than a service-level check means a future relaxation is a migration, not a
-- silently-widened invariant.
--
-- `company_slug` is UNIQUE and reserved at the moment a PENDING claim is inserted, not only
-- once it activates: this is what turns two concurrent claims on the same company into an
-- ordinary unique-constraint conflict (one wins, one gets a clean 409) rather than a
-- hand-rolled lock, and what stops a second claim starting while a first one's verification
-- is still in flight.
--
-- `status`: pending -> active (a moderator or the domain-match check flips it) or the row is
-- deleted outright on rejection, freeing the slug for a future claim. `revoked` is an admin
-- kill switch that deliberately keeps the row (and the slug reservation) rather than
-- deleting it — there is no self-service handoff to a new claimant on this MVP, so freeing
-- the slug on revoke would let anyone re-claim a company an admin just decided to shut out.
--
-- `company_name` is fixed at claim time and never re-derived from a later request: every
-- vacancy this account publishes or edits passes this exact string into the same
-- deterministic company-slug derivation ingest uses, which is what keeps company_slug from
-- drifting between two postings if the employer ever typed the name differently.
CREATE TABLE company_accounts (
    user_id      bigint PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    company_slug text NOT NULL UNIQUE,
    company_name text NOT NULL,
    work_email   text NOT NULL,
    status       text NOT NULL DEFAULT 'pending',
    verified_at  timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT company_accounts_status_check
        CHECK (status IN ('pending', 'active', 'revoked'))
);

-- The moderator review queue is exactly this predicate; a partial index keeps it cheap
-- however large the table grows, since active/revoked rows never need to be scanned for it.
CREATE INDEX company_accounts_pending_idx
    ON company_accounts (created_at)
    WHERE status = 'pending';

COMMENT ON TABLE company_accounts IS
    'One user verified to represent one company: claim/verification state and the fixed '
    'company identity every vacancy that account publishes carries. Not derived from jobs, '
    'like company_slug_aliases — see the file header for why.';

COMMENT ON COLUMN company_accounts.company_name IS
    'Locked at claim time. Every employer-authored vacancy passes this exact string into '
    'derivation, so company_slug never drifts across postings from the same account.';
