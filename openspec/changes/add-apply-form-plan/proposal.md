## Why

Autofill in the side panel is a black box: the user presses one button and gets a sentence
("Autofilled 9 fields — review before submitting"). Nothing says which questions those were,
what is still unanswered, or whether the form can be submitted — so the user scrolls the
whole ATS form to find out, which is the work autofill was meant to remove.

## What Changes

- The Match tab gains an **application-form checklist**: every question the open page asks,
  split into required and optional, each marked answered or not, with a counter and progress
  bar over the required ones.
- **Autofill becomes a walk**: the page scrolls to each question in turn, the control is
  outlined while its value lands, and the panel ticks the item off as it goes. The Autofill
  button reads **Stop** while a walk is running.
- **Unanswered questions stay reachable**: clicking one in the checklist scrolls the page to
  it and puts the cursor in it.
- The counter **follows the user's own typing** — a question they answer by hand on the page
  ticks itself off in the panel.
- The panel's fill primitives gain the page-side affordances this needs: reveal a question
  (scroll + outline + optional focus), fill with reveal, and a change notification from the
  page.

No new server endpoint, no new permission, and no change to what autofill is allowed to
write. The agent keeps filling the form server-side; the panel plays back its report.

## Capabilities

### New Capabilities

- `extension-apply-plan`: what the side panel shows about the application form on the open
  page — the checklist of questions, the required-fields counter, the field-by-field walk
  during autofill, and reaching an unanswered question from the panel.

### Modified Capabilities

<!-- None. `extension-autofill` describes the contact block the server assembles; this change
     adds no field to it and alters no requirement of it. -->

## Impact

- `extension/entrypoints/sidepanel/` — a new `ApplyPlan.svelte`, wired into the Match tab of
  `App.svelte`; the autofill handler becomes a walk with a stop flag.
- `extension/lib/applyPlan.ts` (new) — the pure projection from read form fields to checklist
  items plus the counter.
- `extension/lib/form.ts` — a `revealField` primitive (scroll, outline, optional focus).
- `extension/entrypoints/content.ts` — handles reveal, fills with reveal, and notifies the
  panel when the page's form changes.
- `extension/lib/protocol.ts` — `REVEAL_FIELD` / `REVEAL_RESULT` / `FORM_CHANGED` messages and
  a `reveal` flag on `FILL_BY_LABEL`.

Design already agreed and recorded in
`docs/superpowers/specs/2026-08-15-extension-apply-plan-design.md`.
