-- A company's team: the people besides the claimant who act for a company account, and the
-- invitations that bring them in. See openspec/changes/add-employer-company-team.
--
-- The claimant's company_accounts row stays the account — its status, its moderation and its
-- slug reservation are unchanged. A member is a user the account's team invited by work
-- email; every member acts exactly as the claimant does (no roles), except that the claimant
-- cannot be removed. user_id is the primary key because a user acts for at most one company,
-- the same rule company_accounts' own primary key states for claimants; the two tables are
-- kept apart from each other by the writes (a claim refuses a member, an acceptance refuses a
-- claimant).
--
-- CASCADE from company_accounts, which goes when a pending claim is rejected (it cannot have
-- invited anyone) or when the claimant deletes their freehire account — and then the team and
-- its invitations go with the account, as its vacancies stop taking applications today: there
-- is no hand-over of the account itself to a member (see add-employer-company-team's design).
-- A revoked account keeps its rows, and its members are refused with it, since every
-- capability checks the account's status.
CREATE TABLE IF NOT EXISTS public.company_account_members (
    user_id      bigint      PRIMARY KEY REFERENCES public.users (id) ON DELETE CASCADE,
    company_slug text        NOT NULL REFERENCES public.company_accounts (company_slug) ON DELETE CASCADE,
    -- The address the invitation was mailed to and the member proved they read by following
    -- it: where their vacancies' applications go and what candidates reply to.
    work_email   text        NOT NULL,
    invited_by   bigint      REFERENCES public.users (id) ON DELETE SET NULL,
    joined_at    timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS company_account_members_company_idx
    ON public.company_account_members (company_slug);
CREATE INDEX IF NOT EXISTS company_account_members_invited_by_idx
    ON public.company_account_members (invited_by) WHERE invited_by IS NOT NULL;

-- An invitation is a single-use link mailed to a work address. Only the token's SHA-256 is
-- stored, like an API key. One live invitation per (company, address): inviting the same
-- address again replaces its token and expiry, so a resend never leaves two links that both
-- work.
CREATE TABLE IF NOT EXISTS public.company_account_invitations (
    id           bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    company_slug text        NOT NULL REFERENCES public.company_accounts (company_slug) ON DELETE CASCADE,
    email        text        NOT NULL,
    token_hash   bytea       NOT NULL UNIQUE,
    invited_by   bigint      NOT NULL REFERENCES public.users (id) ON DELETE CASCADE,
    created_at   timestamptz NOT NULL DEFAULT now(),
    expires_at   timestamptz NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS company_account_invitations_company_email_key
    ON public.company_account_invitations (company_slug, lower(email));
CREATE INDEX IF NOT EXISTS company_account_invitations_invited_by_idx
    ON public.company_account_invitations (invited_by);

-- employer_team is everyone who acts for the company the given user acts for — its claimant
-- and its members — and nobody for a user who acts for none. It is the ONE ownership rule of
-- the employer surface: a vacancy (jobs.created_by) and the applications to it belong to the
-- team, not to whoever happened to publish it. Status is not checked here; every capability
-- checks the account's status before it reads.
--
-- Callers read it as `created_by = ANY(ARRAY(SELECT employer_team(x)))`: the array is an
-- InitPlan evaluated once, so jobs is reached through jobs_created_by_idx exactly as the old
-- `created_by = x` was. A set-returning call in an IN (SELECT ...) list is not inlined, and
-- the planner's default 1000-row guess for it could pick a scan of the whole jobs table.
-- ROWS 3 states a team's real size for any caller that joins against it instead.
CREATE OR REPLACE FUNCTION public.employer_team(actor bigint) RETURNS SETOF bigint
    LANGUAGE sql STABLE ROWS 3 AS $$
    WITH company AS (
        SELECT company_slug FROM public.company_accounts WHERE user_id = actor
        UNION ALL
        SELECT company_slug FROM public.company_account_members WHERE user_id = actor
    )
    SELECT ca.user_id FROM public.company_accounts ca JOIN company USING (company_slug)
    UNION ALL
    SELECT m.user_id FROM public.company_account_members m JOIN company USING (company_slug)
$$;

-- A member's vacancies belong to the team, so a member deleting their freehire account hands
-- them to the claimant first, exactly as RemoveCompanyMember does when a member is removed.
-- It has to run BEFORE the users row goes: jobs.created_by is ON DELETE SET NULL, and that
-- foreign-key action fires before the member row's own cascade, so by then the vacancies no
-- longer say whose they were. A user who is no member matches nothing (one primary-key probe).
CREATE OR REPLACE FUNCTION public.employer_hand_over_member_vacancies() RETURNS trigger
    LANGUAGE plpgsql AS $$
BEGIN
    UPDATE public.jobs j
    SET created_by = ca.user_id
    FROM public.company_account_members m
    JOIN public.company_accounts ca ON ca.company_slug = m.company_slug
    WHERE m.user_id = OLD.id
      AND j.created_by = OLD.id
      AND j.source = 'employer';
    RETURN OLD;
END;
$$;

DROP TRIGGER IF EXISTS users_employer_hand_over ON public.users;
CREATE TRIGGER users_employer_hand_over
    BEFORE DELETE ON public.users
    FOR EACH ROW EXECUTE FUNCTION public.employer_hand_over_member_vacancies();
