-- migrate: no-transaction
--
-- 'board_gone' records a guarded close after repeated explicit platform evidence, a minimum
-- age, and a recent successful sibling board crawl. It differs from board_unreachable (which
-- cannot identify why the board failed) and feed_empty (which requires successful empty reads).
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
        'board_gone'::text
    ])) NOT VALID;

ALTER TABLE public.jobs
    VALIDATE CONSTRAINT jobs_closed_reason_check;
