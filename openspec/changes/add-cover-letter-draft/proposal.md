## Why

The tailoring workspace takes a candidate all the way to a CV shaped for one vacancy and then
stops, one step short of the application. Of the 402,117 open postings whose apply form we have
captured, **209,297 (52%) ask for a cover letter**, and **172,783 of those (83%) accept it as
text** rather than only as an uploaded file — and the two things that decide whether that
letter is any good are things only this platform holds: the fit analysis already knows which
requirements the candidate meets but fails to show (`missing-have`) versus which are genuine gaps
(`missing-gap`), and the experience bank already knows which achievements the candidate has
actually asserted versus which a model inferred. A general-purpose chat tool has neither, so it
invents achievements to fill the gap. That is the difference this change sells.

The letter is also the cheapest way to widen what a Pro plan is worth: the fit analysis and the
tailoring context it reads are already computed and cached by the time the candidate asks.

## What Changes

- **A cover letter can be drafted for a (candidate, vacancy) pair.** A fixed three-stage
  prompt-chain — select evidence, draft, skeptic pass — not an autonomous agent, for the same
  reason `ai-fit-analysis` is a chain: deterministic, typed, cacheable.
- **The letter's job is to close the gap the CV cannot.** Stage 1 selects atoms against
  `missing-have` specifically, so the letter reframes evidence the CV leaves implicit.
- **The third stage is a skeptic, and it is what enforces brevity.** It cuts any sentence not
  supported by a cited atom, and cuts to the length ceiling. A "be concise" instruction inside the
  drafting prompt is not the mechanism.
- **The letter is written in the language of the VACANCY, not the candidate's profile.** This
  inverts the rule `ai-fit-analysis` follows, and the inversion is deliberate: an analysis is the
  candidate reading themselves, a letter is read by the employer.
- **Only publishable-provenance atoms may be cited** — `manual`, `cv_import`, `stated_in_chat`.
  An `agent_inferred` atom may not reach a letter, the same gate `cv-builder` applies, enforced in
  the service and not in a prompt.
- **One current draft is stored per (user, job)**, staleness-stamped by model and language. There
  are no revisions and no undo: a letter is a paragraph of prose, not a structured document.
- **Three entry points share one chain**: `POST /me/cvs/:id/cover-letter` runs it, `GET` serves the
  stored draft and never calls a model, and an assistant tool `cover_letter_draft` calls the same
  code so the chat path cannot drift from the button path.
- **The draft consumes a daily allowance.** `plan-limits` landed on main in #2271 while this
  change was being built, so the meter is no longer deferred: drafting adds one value to
  `internal/ai/plan`'s metered-feature vocabulary and reserves against it on the write path only.

Out of scope for this change: filling the letter into a live apply form via the extension,
exporting it as a file, revision history, and tone/voice settings. Each is a seam, not a feature
withheld.

## Capabilities

### New Capabilities

- `cover-letter-draft`: what a cover letter is drafted from, the three stages and what each one
  owns, the provenance gate on cited evidence, the language rule, the storage and staleness of the
  single current draft, the read path that never calls a model, and the assistant tool that shares
  the chain.

### Modified Capabilities

- `tailor-workspace`: the workspace gains a cover-letter surface alongside the CV, and the
  requirement describing the columns has to say where it sits and how it behaves while a draft is
  in flight.

## Impact

**New code**

- `internal/candidate/coverletter` — the chain, the wire shape, the sanitize pass, the draft store.
  **Must be registered in `internal/platform/arch/layering/blocks.go`**; a package in neither the
  table nor a block fails `depguard` and the `layering` test.
- `migrations/0120_cover_letters.sql` — the draft table. `0119` is taken twice already
  (`0119_jobs_requires_clearance.sql`, `0119_user_llm_key_id.sql`), so `0120` is the next free
  number. Requires `make sqlc` after the queries land.
- `internal/api/handler` — the two endpoints and the assistant tool registration.
- `web/src/routes/tailor/[slug]/` — the surface.

**Reads, unchanged**

- `internal/candidate/fitanalysis.TailoringContext` — the vacancy, the verdict, and the
  `missing-have` / `missing-gap` split. One projection already serves both the HTTP reader and the
  agent tool; this becomes a third reader of the same shape.
- `internal/candidate/experience` — `Retrieve` for candidate atoms, and `Provenance` for the gate.
- `internal/candidate/resumeextract.Professional` — the sole candidate context that reaches the
  model. Raw CV text never does.

**Layering**

`internal/candidate` (layer 4) may not import `internal/job` (layer 5). The vacancy therefore
arrives as a `db.Job` parameter supplied by the caller, exactly as `matchanalysis` takes it.

**Spend**

Model calls made for a signed-in candidate go out on that candidate's own gateway credential and
carry a `feature:` tag (`llm-spend-attribution`); this change adds one more tag value and changes
nothing about how attribution works.

**Depends on**

`add-plan-limits` for the daily allowance and the 402 body. It landed on main in #2271 during this
change's implementation, so `internal/ai/plan` is available and the metering task is no longer
sequenced behind another change — it gains one `plan.Feature` value and reserves on the write path.
