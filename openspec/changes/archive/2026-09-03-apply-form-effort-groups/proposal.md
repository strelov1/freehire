## Why

The application-form block already carries the one fact that decides whether a
candidate applies — how much work the form is — and then hides it. Every question
renders as an identical row, so a form of four one-line fields and a form
demanding five essays look the same until the reader has read all fifteen rows.

The answer-kind hint that would carry that fact is pinned to the end of the
question text, where a long question pushes it onto a second line and it reads as
a separate list item rather than as a note about the line above it.

## What Changes

- The block states its own size up front: the number of questions, and the number
  demanding a written answer when there are any.
- Questions are grouped by what answering costs — short answers, then choices from
  a list, then written answers, then attachments — cheapest first, so the reader
  meets the cheap questions before the expensive ones and can stop reading as soon
  as the total is more than they will spend.
- The answer kind moves off each row and into its group's heading, which states it
  once for the whole group. What is left beside a question is whether it is
  optional.
- The provider caption carries the platform's own brand mark where one is
  available, so the source of the questions is recognisable before it is read.
- Not a breaking change: the served form's shape and ordering are untouched. Every
  change here is in how the job page reads what it is already served.

## Capabilities

### New Capabilities

None. The block already exists and is already specified; what changes is how it
presents what it is served.

### Modified Capabilities

- `apply-form-display`: the job page's rendering of the served form gains
  requirements — a stated size, grouping by answering cost, the answer kind named
  once per group rather than per question, and the provider's brand mark. The
  served projection itself (`internal/ingest/applyform`) is unchanged, including
  its promise that questions arrive in the employer's order.

## Impact

- `web/src/lib/components/JobApplyForm.svelte` — the whole of the change's
  rendering. Its `applyFormWorthShowing` export and its index-keyed `{#each}` are
  deliberately untouched; the latter guards against a duplicate-key crash real ATS
  forms have caused.
- `web/src/lib/applyFormGroups.ts` (new, with its test) — the pure grouping and
  counting the component renders. Pure and separate because the web suite runs
  without Svelte compilation, so logic left inside a `$derived` cannot be tested.
- `web/src/lib/atsmarks.ts` (new) — provider slug to brand mark, mirroring the
  existing `web/src/lib/techmarks.ts`. Sourced from `simple-icons`, already a
  dependency of `web/`.
- No Go change, no API change, no migration. `openapi.yaml` and
  `internal/ingest/applyform` are out of scope: the wire contract states that
  questions arrive in the employer's order, and reordering them for one reader's
  convenience would make that statement false for every other consumer.

### Known limits accepted

- `simple-icons` carries a mark for Greenhouse only; Ashby, Workable, Lever and
  Recruitee have none. The mark therefore sits *beside* the provider's name rather
  than replacing it, and a provider without a mark renders as text alone — the
  same partial-coverage fallback `techmarks.ts` already documents for skills.
- Grouping discards the employer's ordering *between* groups. Accepted: the block
  answers "is this worth applying to", and the form itself is filled on the
  platform's own site, where its order is whatever the platform shows.
