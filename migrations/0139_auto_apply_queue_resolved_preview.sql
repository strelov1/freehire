-- Persists the answer-preview snapshot the tailor-and-review orchestrator resolves right
-- after tailoring, before an entry becomes reviewable. The candidate's review shows exactly
-- this snapshot rather than a value recomputed (or approximated) when they open it — see
-- openspec/changes/auto-apply-review-tracking/design.md.
ALTER TABLE public.auto_apply_queue
    ADD COLUMN resolved_preview jsonb;
