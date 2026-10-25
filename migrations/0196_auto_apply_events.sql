-- Auto-apply over MCP (openspec/changes/auto-apply-over-mcp): an ordered history of every
-- attempt, a marker for the dead letters that must never be retried, and the account switch
-- that lets a connected agent act on auto-apply.

-- Whether an agent connected to the signed-in MCP server may start, review, retry or cancel
-- auto-apply attempts. Off until the person turns it on from a browser session; an agent
-- cannot grant itself the permission it is gated by.
ALTER TABLE public.users ADD COLUMN agents_may_apply boolean NOT NULL DEFAULT false;

-- Set when the worker dead-lettered an attempt because the employer MAY already hold the
-- application (a submission it could not confirm, or one it could not record locally). A
-- retry of such a row risks a duplicate application, so retry refuses it. failed_at alone
-- cannot say this: an ordinary exhausted retry budget sets the same column.
ALTER TABLE public.auto_apply_queue ADD COLUMN submit_uncertain_at timestamptz;

-- The rows already dead-lettered that way before the column existed. They are recognisable
-- only by the reason deadLetterImmediately wrote (internal/application/autoapply/runner.go);
-- left unmarked, the new retry would re-arm them and could send the application twice.
-- Runs before the trigger below exists, so it records no event.
UPDATE public.auto_apply_queue
SET submit_uncertain_at = failed_at
WHERE failed_at IS NOT NULL
  AND (last_error LIKE 'submission unconfirmed%' OR last_error LIKE 'submitted but failed to record%');

-- One row per state transition of an attempt, keyed by (user, job) rather than by the queue
-- row: the queue row is deleted on submission and on cancel, and a re-enqueue makes a new
-- one, while the person asking "what happened" means the pair.
--
-- detail never carries auto_apply_queue.last_error — an internal diagnostic the product
-- never shows a candidate. It carries structured facts: question labels, an attempt number.
--
-- job_id carries no foreign key on purpose. Adding one takes SHARE ROW EXCLUSIVE on jobs,
-- the most-written table in the database, and this migration runs under the runner's 5s
-- lock_timeout behind whatever ingest or a backfill holds there. Postings are soft-closed,
-- not deleted, and every reader joins jobs anyway, so an orphan costs nothing.
CREATE TABLE public.auto_apply_events (
    id         bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id    bigint      NOT NULL REFERENCES public.users (id) ON DELETE CASCADE,
    job_id     bigint      NOT NULL,
    queue_id   bigint      NOT NULL,
    kind       text        NOT NULL,
    detail     jsonb       NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX auto_apply_events_user_job_idx
    ON public.auto_apply_events USING btree (user_id, job_id, id);

-- The worker-side transitions, derived from the OLD -> NEW diff by a trigger rather than
-- written by each writer: the API, both cmd/auto-apply passes and the orchestrator (through
-- the API) all write this table, and a writer that forgot to log would look exactly like an
-- attempt where nothing happened.
--
-- DELETE is deliberately not handled: the trigger cannot tell a submission from a
-- withdrawal. The queries that delete (DeleteAutoApplyEntry, CancelAutoApplyEntry) and the
-- one that re-arms (RetryAutoApplyEntry) record their own event in the same statement, so
-- the person's and the worker's explicit acts are named by the code that performs them.
-- The array a jsonb value holds, or an empty one. unmapped is stored as JSON null — not SQL
-- NULL — by every park that names no field (a captcha page, an unscannable form: Go marshals
-- a nil slice as null), and jsonb_array_elements raises on a scalar. Every detail below is
-- evaluated on every UPDATE, whether or not its event fires, so without this one such row
-- would fail every write made to it.
CREATE OR REPLACE FUNCTION public.auto_apply_jsonb_array(v jsonb) RETURNS jsonb
LANGUAGE sql IMMUTABLE AS $$
    SELECT CASE WHEN jsonb_typeof(v) = 'array' THEN v ELSE '[]'::jsonb END
$$;

CREATE OR REPLACE FUNCTION public.auto_apply_queue_record_events() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    ev record;
BEGIN
    IF TG_OP = 'INSERT' THEN
        INSERT INTO auto_apply_events (user_id, job_id, queue_id, kind)
        VALUES (NEW.user_id, NEW.job_id, NEW.id, 'queued');
        RETURN NULL;
    END IF;

    FOR ev IN
        SELECT * FROM (VALUES
            -- A preview or submit pass took the row. claimed_at moves on every claim, so a
            -- reclaim after an expired lease is recorded too — that is a real second try.
            (NEW.claimed_at IS NOT NULL AND NEW.claimed_at IS DISTINCT FROM OLD.claimed_at
                 AND NEW.review_decision IS NULL,
             'checking_form', '{}'::jsonb),
            (NEW.claimed_at IS NOT NULL AND NEW.claimed_at IS DISTINCT FROM OLD.claimed_at
                 AND NEW.review_decision = 'approved',
             'submitting', '{}'::jsonb),
            (NEW.tailored_cv_id IS NOT NULL AND NEW.tailored_cv_id IS DISTINCT FROM OLD.tailored_cv_id,
             'tailored', '{}'::jsonb),
            (OLD.tailor_failed_at IS NULL AND NEW.tailor_failed_at IS NOT NULL,
             'tailor_failed', '{}'::jsonb),
            (OLD.resolved_preview IS NULL AND NEW.resolved_preview IS NOT NULL,
             'ready_for_review',
             jsonb_build_object('pending', COALESCE(
                 (SELECT jsonb_agg(p -> 'label') FROM jsonb_array_elements(
                      public.auto_apply_jsonb_array(NEW.resolved_preview -> 'pending')) p),
                 '[]'::jsonb))),
            (OLD.review_decision IS NULL AND NEW.review_decision = 'approved',
             'approved', '{}'::jsonb),
            (OLD.review_decision IS NULL AND NEW.review_decision = 'declined',
             'declined', '{}'::jsonb),
            -- A decline sets blocked_at too (DeclineAutoApplyReview reuses the park
            -- columns); that is the decline above, not a form-field park.
            (OLD.blocked_at IS NULL AND NEW.blocked_at IS NOT NULL
                 AND NEW.review_decision IS DISTINCT FROM 'declined',
             'blocked',
             jsonb_build_object('questions', COALESCE(
                 (SELECT jsonb_agg(jsonb_build_object('label', u -> 'label', 'required', u -> 'required'))
                  FROM jsonb_array_elements(public.auto_apply_jsonb_array(NEW.unmapped)) u),
                 '[]'::jsonb))),
            (NEW.attempts > OLD.attempts AND NEW.failed_at IS NULL,
             'attempt_failed', jsonb_build_object('stage', 'submit', 'attempt', NEW.attempts)),
            (NEW.preview_attempts > OLD.preview_attempts AND NEW.preview_failed_at IS NULL,
             'attempt_failed', jsonb_build_object('stage', 'preview', 'attempt', NEW.preview_attempts)),
            (NEW.captcha_attempts > OLD.captcha_attempts AND NEW.failed_at IS NULL,
             'captcha_refused', jsonb_build_object('attempt', NEW.captcha_attempts)),
            (OLD.submit_uncertain_at IS NULL AND NEW.submit_uncertain_at IS NOT NULL,
             'submit_uncertain', '{}'::jsonb),
            (OLD.failed_at IS NULL AND NEW.failed_at IS NOT NULL AND NEW.submit_uncertain_at IS NULL,
             'failed', jsonb_build_object('stage', 'submit')),
            (OLD.preview_failed_at IS NULL AND NEW.preview_failed_at IS NOT NULL,
             'failed', jsonb_build_object('stage', 'preview'))
        ) AS t(fired, kind, detail)
        WHERE fired
    LOOP
        INSERT INTO auto_apply_events (user_id, job_id, queue_id, kind, detail)
        VALUES (NEW.user_id, NEW.job_id, NEW.id, ev.kind, ev.detail);
    END LOOP;
    RETURN NULL;
END;
$$;

CREATE TRIGGER auto_apply_queue_record_events
    AFTER INSERT OR UPDATE ON public.auto_apply_queue
    FOR EACH ROW
    EXECUTE FUNCTION public.auto_apply_queue_record_events();
