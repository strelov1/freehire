-- migrate: no-transaction
--
-- An employer can delete a vacancy it published, and sees the vacancies of its own company
-- that were collected from other sources.
--
-- Delete is soft: direct_applications and other tables reference the jobs row, so the row
-- stays. A deleted vacancy is closed with closed_reason 'employer_deleted'
-- and hidden from jobs_read (migration 0320). The reason is distinct from 'employer_closed',
-- which ReopenEmployerJob can undo; this one is final.
ALTER TABLE public.jobs
    DROP CONSTRAINT IF EXISTS jobs_closed_reason_check;

ALTER TABLE public.jobs
    ADD CONSTRAINT jobs_closed_reason_check CHECK (closed_reason = ANY (ARRAY[
        ''::text,
        'unseen'::text,
        'feed_removed'::text,
        'moderated'::text,
        'probe_expired'::text,
        'expired'::text,
        'board_unreachable'::text,
        'feed_empty'::text,
        'source_misattributed'::text,
        'employer_closed'::text,
        'board_retired'::text,
        'ats_covered'::text,
        'board_gone'::text,
        'native_source_pending'::text,
        'employer_deleted'::text
    ])) NOT VALID;

ALTER TABLE public.jobs
    VALIDATE CONSTRAINT jobs_closed_reason_check;

-- The company an actor acts for, only while its account is active (confirmed): the ONE rule of
-- whose vacancies the employer surface may read, authored by the team or collected from an
-- ATS. NULL for everyone else, so `company_slug = active_employer_company(x)` matches nothing.
-- Reading is by company; writing stays by authorship (created_by in employer_team, source
-- 'employer'). Served by jobs_company_slug_idx.
CREATE OR REPLACE FUNCTION public.active_employer_company(actor bigint) RETURNS text
    LANGUAGE sql STABLE AS $$
    SELECT ca.company_slug FROM public.company_accounts ca
    WHERE ca.user_id = actor AND ca.status = 'active'
    UNION ALL
    SELECT ca.company_slug FROM public.company_account_members m
    JOIN public.company_accounts ca ON ca.company_slug = m.company_slug
    WHERE m.user_id = actor AND ca.status = 'active'
    LIMIT 1
$$;
