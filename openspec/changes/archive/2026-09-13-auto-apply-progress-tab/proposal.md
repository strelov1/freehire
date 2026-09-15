## Why

A candidate with a live auto-apply attempt has no dedicated place to see where it stands as a
sequence of stages — today's status banner is read-only prose mixed into the drawer's general
`application` tab, alongside the tailor/notes/view-job controls that have nothing to do with
auto-apply. There is also no visible confirmation, once an attempt succeeds and its queue row is
retired, that a job's ordinary "applied" state actually came from auto-apply rather than a manual
apply — the two are indistinguishable in the tracker today even though the ledger already
distinguishes them.

## What Changes

- A new `Progress` tab is added to the tracker drawer's existing tab set (alongside
  `application`/`fit`/`description`/`emails`), shown only when the job has a live auto-apply
  attempt or has one that already succeeded. It shows a 3-stage progress bar — Tailoring →
  Review → Submitted — derived from the existing seven-value auto-apply status plus one new
  frontend-only signal for "already submitted via auto-apply" (read from the application's
  existing event ledger, no backend change).
- The existing status banner content (the tailoring/pending_review/approved/blocked/declined/
  failed/tailor_failed copy, the approve/decline buttons, the answer-bank inputs, and the
  unmapped-fields list) moves from the `application` tab into the new `Progress` tab — same
  behavior, new home.
- The `Progress` tab offers a "Pause"/"Continue" toggle over a live, non-terminal attempt. It is
  cosmetic only — a candidate-side reminder that they are deliberately not dealing with this
  attempt right now — and has no effect on `cmd/auto-apply`'s claim/process behavior or on the
  queue row at all. Stored in the browser only (not synced across devices).

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `auto-apply-tracker-review`: adds the drawer's dedicated progress-stage view (including the
  post-submission "submitted via auto-apply" signal), the requirement that the existing
  status/decision UI lives in that view rather than the general application tab, and the
  candidate-only pause/continue marker.

## Impact

- `web/src/lib/autoApplyProgress.ts` (new) — pure mapping from auto-apply status (+ the
  post-submission signal) to the 3-stage progress view.
- `web/src/lib/autoApplyProgress.test.ts` (new).
- `web/src/lib/components/JobDrawer.svelte` — new `Progress` tab; the existing status-banner
  markup moves out of the `application` tab into it; new pause/continue control.
- No backend changes: the post-submission signal is read from `TrackedApplication.events`
  (`kind: "applied"`, `source: "auto_apply"`), already returned by `GET /me/tracking/:slug`
  today (`internal/application/apptimeline`, `appevent.SourceAutoApply`).
