-- One row per posting cmd/quality-audit asked the remote judge about: what the posting's own
-- text says about where the work is done, as a model read it and the grounding check let
-- through (internal/job/qualityaudit). The audit judges ~200 postings a week across ~10
-- sources, so one week is no precision for any one source; the gauges are computed over the
-- trailing 56 days of these rows, which is the whole reason the judgements are kept.
--
-- It records a MEASUREMENT and nothing reads it to act: no posting is retagged from it.
-- Small by construction (~10k rows a year), so it is never pruned and needs one index, for
-- the window read.
CREATE TABLE IF NOT EXISTS public.remote_audit_judgements (
    id            bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    -- No foreign key: jobs are never hard-deleted (AGENTS.md, "Job deletion"), and a
    -- measurement must not be able to block or cascade anything on the catalogue's hottest table.
    job_id        bigint      NOT NULL,
    source        text        NOT NULL,
    -- The judge's reading of the posting's arrangement.
    work_mode     text        NOT NULL CHECK (work_mode IN ('remote', 'hybrid', 'onsite', 'unclear')),
    -- Whether the posting says the candidate may work from any country.
    worldwide     text        NOT NULL CHECK (worldwide IN ('yes', 'no', 'unclear')),
    -- Whether the posting carried regions 'global' when it was judged — the population the
    -- worldwide answer is a precision for.
    tagged_global boolean     NOT NULL,
    judged_at     timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS remote_audit_judgements_judged_at_idx
    ON public.remote_audit_judgements (judged_at);
