## Context

The brainstormed and approved design is
`docs/superpowers/specs/2026-08-02-show-apply-questions-design.md`. This records the
technical shape and the reasoning that is specific to fitting it into the codebase.

`apply_forms` was shipped by `collect-ats-apply-forms` and is filling: Recruitee is
complete at ~36k, Greenhouse and Ashby are draining a ~214k queue at roughly 8k an
hour. The store keeps each platform's own vocabulary verbatim — field identifiers,
option values, question text — because those tokens exist to be handed back to the
platform that issued them.

Reading it for display is the first consumer, and it wants almost none of that. It
wants the question text and one word about the answer.

## Goals / Non-Goals

**Goals:**

- Show a candidate what a posting's application will ask, before they open it.
- Cost nothing on the pages where no form is known — the majority.
- Leave a future filter as a clean, separate piece of work.

**Non-Goals:**

- Any derived fact: counts, time estimates, "they will ask about salary".
- Any change to the list endpoint, the search index, or `internal/jobview`.
- Any fallback for a provider whose form cannot be read.

## Decisions

### The projection lives in `internal/applyform`, beside the capture

`applyform` owns the shape of a captured form. A display projection over it belongs
there rather than in `internal/handler`, for the reason the capture's own mappers
live there: the knowledge of what a `Field` means is one thing, and splitting it
across the handler would put half of it where nothing else can reach it.

It is a pure function — stored `Form` in, display shape out. No database, no HTTP,
so the exclusions are unit-testable without a fixture server.

*Alternative rejected:* project in the handler. Cheaper by one file, and it would
put "a demographic field is not an employer question" in the same package as route
wiring, where the next reader would not look for it.

### The endpoint follows `JobCopies` exactly

Resolve the slug to an id (`GetJobIDBySlug`), then load the form. Two queries rather
than one join, because that is what the sibling endpoint on the same resource does,
and because it separates the two 404s cleanly: an unknown slug fails at the first
query exactly as `/copies` does, while a known posting with no stored form fails at
the second with its own message.

`RenderError` already maps `pgx.ErrNoRows` to 404, so neither case needs handling
written for it.

### The page fetches it as a fourth parallel request

`+page.server.ts` already runs three requests in parallel and degrades two of them
to empty on failure, with the comment explaining why: a discovery aid must not break
the page. The form is the same class of thing and takes the same treatment — a
`.catch(() => null)` beside the other two.

*Consequence, accepted:* one more API round trip per job page render. It is parallel
with the existing three, so it costs latency only if it is the slowest, and it is a
single indexed lookup by primary key.

### The answer-kind vocabulary is a display concern, not a stored one

`applyform.FieldType` already normalizes each platform's control names into ten
values. The projection maps those to the words a reader sees, and maps the ones that
are not questions — `hidden`, `info` — to exclusion rather than a word.

An empty `FieldType` (a control the capture could not normalize, its `RawType` kept)
yields no word. This is the capture's dict-only rule carried through to display: the
question is real and is shown, but nothing is invented about what answering it
costs.

## Risks / Trade-offs

- **The block is absent on most job pages** (8.8% of the open catalogue carries a
  readable provider, 15.8% of technical postings) → Accepted rather than mitigated.
  Where it appears it is exact, and the alternative — saying something generic about
  the platform — was considered and dropped as noise.

- **A form captured weeks ago may no longer match the live page** → The capture is
  gated on "this job has no stored form", so it is written once and never refreshed;
  an employer editing their form afterwards goes unnoticed. For telling a candidate
  roughly what awaits them this is overwhelmingly right, and a freshness policy
  belongs to whoever needs one — autofill will, this does not.

- **Showing employer-authored text is republication** → A deliberate reversal of the
  capture change's display decision, made by the product owner with the trade-off
  stated. Recorded in the proposal so the next reader does not "fix" it back.

## Migration Plan

None. No schema change, no index change, no worker change. The endpoint is additive
and the page degrades to today's behaviour without it, so the deploy has no ordering
constraint and rollback is reverting the code.

## Open Questions

- Whether the block belongs near the apply button or lower on the page is a layout
  question the implementation can settle by looking at it; the approved design says
  beside the apply action, and nothing downstream depends on that choice.
