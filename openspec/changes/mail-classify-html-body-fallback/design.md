## Context

The classify-mail worker (`internal/maillink`) resolves each inbox email to an
application and classifies its status via an LLM (`internal/mailclassify`). The
LLM `Input.Body` is currently fed straight from `emails.body_text`, which the
`ClaimEmailClassificationBatch` query is the only source of. HTML-only ATS mail
has an empty `body_text`, so the LLM classifies from subject alone.

The Gmail sync already extracts and stores both parts (`bodies()` in
`internal/gmailsync/gmailapi.go` → `emails.body_text`, `emails.body_html`). The
gap is purely which column the classifier query reads.

## Goals / Non-Goals

**Goals:**
- The classifier always receives the actual message text when it exists in
  either part, so HTML-only rejections are no longer misread as `screening`.
- Keep the change surgical: no prompt/vocabulary/stage-rule changes, no change to
  what `body_text`/`body_html` mean at rest, no new top-level dependency.

**Non-Goals:**
- Re-classifying already-stored, already-misclassified emails (an ops re-enqueue,
  tracked separately).
- Fixing the same latent gap in the SES/hosted-mailbox ingest path
  (`internal/mailingest`) — that path did not produce the reported bug.
- HTML sanitization for display (the reading pane already renders `body_html`).

## Decisions

- **Fix in the worker, not at ingest.** `readableBody(text, html)` derives the
  classifier body at classify time. This fixes already-stored rows on
  re-classification without a data backfill and leaves `body_text` semantics
  intact (chosen with the user over the ingest-time alternative).
- **Selection order:** prefer `text` when `strings.TrimSpace(text) != ""` (Gmail
  sometimes stores a whitespace-only plain part); else strip `html` via
  `html2text.FromString`; if stripping errors, fall back to the raw `html`
  (text-among-tags beats an empty body); if both are empty, return `""` and the
  classifier keeps working from the subject as today.
- **Length is bounded downstream** by the classifier's `TruncateRunes(...,
  maxBodyRunes)`, so `readableBody` does not truncate.
- **Plumbing:** `ClaimEmailClassificationBatch` returns `e.body_html`;
  `maillink.Claimed` gains `BodyHTML`; the store maps it; the runner passes
  `readableBody(c.Body, c.BodyHTML)`.

## Risks / Trade-offs

- `html2text` output is lossy vs. the original HTML, but it is strictly more
  signal than the empty string the LLM sees today; acceptable and bounded.
- Promoting `html2text` from indirect to direct dependency is intentional and
  low-risk (already compiled into the binary via `enmime`).
