-- migrate: no-transaction
--
-- Withdraw unverified LinkedIn discoveries from the catalogue while retaining identity
-- and application history. This is a catalogue eligibility decision, not a market
-- closure signal. A verified on-site control or native ATS transfer can restore it.
-- Widen the existing check without holding an exclusive lock during validation.
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
        'native_source_pending'::text
    ])) NOT VALID;

ALTER TABLE public.jobs
    VALIDATE CONSTRAINT jobs_closed_reason_check;
