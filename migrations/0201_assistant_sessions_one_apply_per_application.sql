-- migrate: no-transaction
--
-- One apply conversation per application (see the auto-apply-chat change). The handler looks
-- for the existing one before creating, but two first opens racing — a double click, two tabs
-- — would both find none and insert two, splitting the numbered questions from the answers
-- given to them. The handler re-reads the existing session when its insert hits this index.
--
-- Partial: every other preset may hold several sessions per vacancy (a rehearsal before
-- each round). No 'apply' row exists before 0199, so nothing can violate it at creation.
-- CONCURRENTLY, hence its own file outside a transaction.
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS assistant_sessions_one_apply_per_job
    ON public.assistant_sessions (user_id, job_id)
    WHERE preset = 'apply';
