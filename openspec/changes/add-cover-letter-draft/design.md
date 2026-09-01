## Context

The tailoring workspace already assembles everything a cover letter needs and then stops at the CV.
`fitanalysis.TailoringContext` holds the vacancy, the verdict, and the requirement split into
`missing-have` (the candidate meets it but the CV does not show it) and `missing-gap` (a genuine
hole). `internal/candidate/experience` holds achievement atoms stamped with who asserted them.
`resumeextract.Professional` is the de-identified candidate projection that already serves the fit
chain. Nothing new has to be gathered; what is missing is the stage that writes prose from it.

Two neighbours set the pattern this change follows. `matchanalysis` is a fixed three-stage chain
whose own documentation opens with "Fixed prompt-chain, NOT an autonomous agent — deterministic,
typed, cacheable". `atscheck` is the same shape at a smaller size, and its file layout
(`<name>.go` for the wire shape and sanitize, `analyzer.go` for the chain, `flexdecode.go` for
tolerant JSON) is the one this package copies.

The relevant constraint from the layering table: `internal/candidate` sits at layer 4 and
`internal/job` at layer 5, so this package can never import the job domain. `matchanalysis` solves
this by taking `db.Job` as a parameter from a caller that has already loaded and authorized it, and
this package does the same.

## Goals / Non-Goals

**Goals:**

- A cover letter whose every claim about the candidate traces to an atom the candidate asserted.
- Brevity produced by a stage that cuts, not by an adjective in a prompt.
- One chain behind every entry point, so the chat path and the button path cannot drift.
- The letter written for its reader — in the vacancy's language.

**Non-Goals:**

- Filling the letter into a live apply form through the extension. The extension can already write
  a `textarea` via `fillByLabel`, and `apply_forms` already records whether a posting exposes
  `cover_letter_text`; this is a seam left open, not a capability withheld by accident.
- Exporting the letter as a file. `cover_letter` as a file upload is a different mechanism from
  `cover_letter_text` and needs the upload path, not a text path.
- Revision history and undo. `cvedit` earns that machinery because a CV is a structured document
  edited over months; a letter is a paragraph regenerated in seconds.
- Tone and voice settings. Two length bands are the whole surface until there is evidence that a
  third axis is wanted.
- Implementing the daily allowance. That is `add-plan-limits`' shape to define.

## Decisions

### A chain, not an agent

Considered giving the assistant a free hand: it already holds the tailoring session, and a chat
loop can iterate ("shorter", "drop the Python part"). Rejected on two grounds.

The first is that the agent has no context the chain lacks.
`internal/candidate/fitanalysis/tailoring.go:66` states outright that one projection serves both the
HTTP endpoint and the agent tool "so neither can drift into showing a different shape of the same
analysis". The agent does not read the vacancy more deeply; it spends turns deciding to fetch what
a chain is handed.

The second is that an agent turn is a tool-calling loop — measured at 7.1 model calls and $0.48 in
`add-plan-limits`' own findings, against roughly one cent for a three-call chain. The cost is not
in the letter's length, it is in re-sending the system prompt, the 6000-character posting and the
candidate projection on every round.

The agent still gets a way in: a `cover_letter_draft` tool that calls this chain. That preserves
the conversational path without making the letter's quality depend on how a loop happened to
unfold.

### One package, not two

`matchanalysis` and `fitanalysis` are split because the fit chain has four entry points, two of
which run from a detached goroutine with no Fiber context, and the cache, staleness stamp, credit
rule and coalescing therefore could not live on a handler. The letter has two entry points and both
are request-scoped. A second package would be a boundary drawn for a problem this feature does not
have.

`internal/candidate/coverletter` therefore holds the chain, the wire shape, the sanitize pass, and
a narrow `Repository` port for the draft — the same shape `experience` uses, where `store.go` is the
owner-scoped domain surface and `repository.go` adapts `*db.Queries` to it. If a third entry point
ever appears that cannot reach a request, the split can be made then, against a real constraint.

### The provenance gate is a filter on the input, not a check on the output

The atoms are filtered before the chain starts. Checking the letter afterwards would mean matching
prose back to atoms, which is exactly the fuzzy problem the gate exists to avoid; and a model that
never sees an inferred atom cannot cite one.

This follows `experience/AGENTS.md`: "Provenance decides publication, and the check is in the
service." The CV evidence gate learned the same lesson — a gate implemented by routing alone was
laundered by banking an inference, editing it, and reading it back as `manual`.

### The audit stage has a floor

Stage 3 cuts. A stage whose only instruction is to cut can cut everything, and an empty letter is a
worse failure than a slightly long one. The server therefore compares the audited body against a
minimum; below it, the Stage 2 draft is served instead. This is the same shape as the existing rule
that an unparseable Stage 3 degrades to Stage 2 — a third stage may improve the result, never
destroy it.

### Language comes from `jobs.posting_language`, and falls back to English

The column exists and is populated: on production, of 4.5M open rows, 3.49M are `en`, 296k `ru`,
151k `de`, and 233k (about 5%) are empty. An empty value falls back to English, **not** to the
candidate's profile language. A wrong-but-plausible English letter to a German employer is a normal
outcome in this market; a Russian letter to a German employer reads as a mistake.

This inverts `matchanalysis`' rule deliberately, and the draft carries a language stamp for the same
reason the fit analysis does — so a candidate whose profile language changes does not silently keep
a letter aimed at the wrong reader.

### The fit analysis is required, and drafting will produce one if absent

The chain reads `TailoringContext`, which needs an analysis. `fitanalysis.Required` already produces
one when absent and is already used this way by the assistant's `interview_context` tool and the
autopilot's run plan, neither of which is charged for it. The letter uses the same call, so a
candidate who asks for a letter on a vacancy they have not analysed gets one rather than an error.

### Storage

One row per `(user_id, job_id)`, upserted:

| Column | Why |
|---|---|
| `body` | the audited letter |
| `cited_atom_ids` | what it is built on — shown in the UI, and the thing a reviewer checks |
| `language` | staleness stamp; also what the UI labels |
| `model` | staleness stamp, invalidates on `LLM_MODEL` upgrade |
| `created_at`, `updated_at` | ordinary |

No `content_hash` stamp on the vacancy. A cover letter is aimed at a role, and a posting edit that
changes a word does not make the letter wrong the way it makes a requirement-by-requirement
analysis wrong.

Migration number is `0120`: `0119` was taken twice by parallel branches
(`0119_jobs_requires_clearance.sql`, `0119_user_llm_key_id.sql`).

## Risks / Trade-offs

- **The skeptic cuts the letter to nothing** → a server-side minimum length; below it, Stage 2 is
  served. Covered by a scenario in the spec.
- **`posting_language` is empty for ~5% of open rows** → fall back to English, never to the profile
  language. A test pins the fallback so a later "helpful" change cannot reach for the profile.
- **A candidate with an empty or entirely inferred bank gets nothing** → the endpoint reports why
  rather than drafting an evidence-free letter. This is the same degradation the fit analysis makes
  when a candidate has no banked experience, and it is the honest outcome: the feature's whole claim
  is that it does not invent.
- **The letter is unmetered until `add-plan-limits` lands** → the metering task is sequenced last
  and named explicitly, and the endpoint stays behind `RequireAuth`, so exposure is bounded to
  signed-in accounts. Spend still lands on the candidate's own gateway credential under a new
  `feature:` tag, so the cost is visible per user from day one.
- **Three model calls where one might do** → accepted. The user's stated priority is quality over
  cost, and the audit stage is the mechanism for the brevity requirement, not an optional polish.
- **17% of the postings that ask for a letter accept only a file** → 36,514 of the 209,297 open
  postings (recruitee, ashby) expose `cover_letter` as an upload with no text field. The letter is
  still useful there — it is copied out by hand — but the seamless path does not reach them, and a
  claim that it does would be false. The remaining 172,783 (greenhouse, workable) accept text.
- **A new package that `depguard` does not know** → registering it in
  `internal/platform/arch/layering/blocks.go` is a task, not an afterthought. A package in neither
  the table nor a block fails the guard, and the guard only sees tagged test files when
  `run.build-tags` names them.

## Migration Plan

1. `migrations/0120_cover_letters.sql` — new table only, no backfill, nothing to reconcile. Safe to
   apply ahead of the code.
2. Deploy the package and endpoints; the tab is the last thing to land, so an incomplete backend is
   never reachable from the UI.
3. The allowance is wired when `add-plan-limits` ships. Rollback is dropping the tab; the table can
   stay, since an unread table costs nothing and re-adding one costs a migration.

### The length bands are a product decision, because the forms do not state a limit

The first task of this change was to read the real `maxlength` of captured `cover_letter_text`
fields and set the bands from the distribution. **That measurement came back empty: `apply_forms`
does not capture `maxlength` at all.** The keys stored per field are `id`, `type`, `raw_type`,
`label`, `required` and sometimes `section`, across all 482,750 captured cover-letter fields.

Widening the capture to record `maxlength` belongs to `apply-form-capture`, not here, and it would
only pay off after a re-capture of the whole queue. So the bands are set as a product decision and
stated as one: **short ≈ 900 runes, standard ≈ 1,800**, the range a recruiter-facing letter
occupies in practice.

They are `DefaultBounds()`, filled in field by field by `Bounds.OrDefault()`. There is
deliberately **no** environment knob and no `SetBounds` equivalent: `matchanalysis` has one
because operators tune its ceilings against live output, and nobody has yet needed to tune a
letter's length without shipping. Changing a band is a release, and the seam for a knob is the
`Bounds` field on `Input`, which the chain already honours.

## Open Questions

- **Whether the tab or the chat is the first surface a user meets.** Both exist from the start; the
  question is which one the empty state points at. Deferred to the UI task.
- **Whether the file-only 17% deserve their own path.** 36,514 open postings (recruitee and ashby)
  expose the cover letter only as a file upload. They can still be served by copying the text out,
  but a rendered file would serve them properly. Out of scope here; revisit with the export seam.
