-- One-off: re-classify HTML-only emails the pre-fix classifier misread.
--
-- Before the readableBody fix (PR #726), the classifier read only emails.body_text.
-- HTML-only ATS mail (Gem, Ashby, Greenhouse) has an empty body_text, so those
-- emails were classified from the subject alone (e.g. a rejection read as
-- "screening"). This resets the "done" marker for exactly that set so the next
-- cmd/classify-mail run re-enqueues and re-classifies them with the HTML body.
--
-- Run as role hire, and ONLY AFTER the fix is deployed — running it against the
-- old binary would re-produce the same wrong result. Sequence:
--   1. deploy PR #726
--   2. (preview) run the SELECT below to see how many rows are affected
--   3. run the UPDATE
--   4. run `cmd/classify-mail` (its EnqueuePending sweep re-enqueues the reset rows)
--
-- Targeted: whitespace-only body_text WITH a non-empty body_html, already
-- classified. Correctly-classified plain-text mail is left untouched (no LLM spend).
--
-- Caveat: this corrects emails.status_signal and the link, but does NOT un-advance
-- an application stage that the first (wrong) classification already moved forward
-- — stage advancement only moves forward and rejection never auto-applies. Jobs
-- wrongly bumped to "screening" must be corrected by the user in /my/tracking.

-- Preview (run first):
-- SELECT count(*) FROM public.emails
--  WHERE classified_at IS NOT NULL AND btrim(body_text) = '' AND body_html <> '';

UPDATE public.emails
SET classified_at = NULL
WHERE classified_at IS NOT NULL
  AND btrim(body_text) = ''
  AND body_html <> '';

-- Belt-and-suspenders: drop any leftover outbox rows for the target set so the
-- enqueue sweep re-inserts clean entries (classified rows normally have none).
DELETE FROM public.email_classification_outbox o
USING public.emails e
WHERE o.email_id = e.id
  AND btrim(e.body_text) = ''
  AND e.body_html <> '';
