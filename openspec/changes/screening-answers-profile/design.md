## Context

Prod `apply_forms` (443k captured ATS forms) shows six screening questions repeating
across employers, independent of the vacancy: work authorization / eligible country, visa
sponsorship needed, salary expectations, notice period, 18+ confirmation, willingness to
relocate. `internal/autofillagent`'s own prompt already anticipates the gap — its
`choosePrompt` tells the model "questions about visa sponsorship, notice periods, salary or
willingness to relocate are NOT answered by a profile — declining is the correct answer",
because until now nothing populated a profile with those answers.

Three existing domains were considered and rejected as the home for this data, each for a
reason documented in its own AGENTS.md: `internal/userprofile` is search/targeting
preferences with a different lifecycle; `internal/experience` is an accumulating evidence
bank for CV content; `internal/resumeextract` is CV-derived data, and none of these six
facts can be derived from a CV — they exist only because the candidate states them
directly.

## Goals / Non-Goals

**Goals:**
- Let a candidate state these six answers once, edit them any time, and have the browser
  extension's agent-driven autofill (`/me/autofill/run`) use them to fill matching questions
  on real application forms.
- Support both a manual edit surface and assistant-driven edit from a chat turn, without
  building two independent write paths.

**Non-Goals:**
- Wiring these answers into `internal/hardconstraint`'s job-compatibility blockers or
  match-analysis scoring. The candidate side of a visa check does not exist after this
  change either — only the vacancy side does today. Connecting them is a follow-up change.
- A generic, user-extensible list of custom Q&A pairs. The six fields are fixed; a form
  question this store cannot answer is left for the candidate, exactly as today.
- Deterministic (non-agent) autofill for these fields. The extension's direct field-id
  mapping works because standard contact fields sit at identifier or id every ATS gives
  the same semantic meaning; screening questions are each an arbitrary employer question
  with an opaque per-posting id (`internal/applyform`'s own documented reasoning for
  keeping ATS vocabulary verbatim), so only the LLM-planned fill can place them by matching
  question *meaning*, not identifier.

## Decisions

**A new package, `internal/screeninganswers`, owning one row per user.** Six typed nullable
columns on a `screening_answers` table (`user_id bigint PRIMARY KEY REFERENCES users(id)`),
not a jsonb blob and not free-text Q&A pairs: the field set is fixed and small (confirmed:
all six ship in v1), so a schema that names each fact is simpler to validate, edit, and
read than a generic key-value store would be, and matches the shape `cv`/`resume_structured`
already use for one-row-per-user domain data.

Columns:
- `authorized_countries text[]` — ISO-3166 alpha-2 codes, reusing `internal/location`'s
  existing country dictionary (dict-only: an unrecognized code is rejected, never stored).
  Empty/null means unstated, not "authorized nowhere".
- `visa_sponsorship_needed boolean`
- `desired_salary_amount integer`, `desired_salary_currency text`, `desired_salary_period
  text` — the same three-part shape `job_submissions` already uses for salary. Period is
  validated against the existing closed enum `vocab.SalaryPeriodValues`
  (`year`/`month`/`day`/`hour`) — the same one `job_submissions.salary_period` and jobs'
  enrichment already use. Currency has no closed dictionary to validate against:
  `internal/vocab`'s own doc comment lists `salary_currency` among the ISO-standard fields
  deliberately left without a bundled vocabulary, so it is validated as a well-formed ISO
  4217 code (three uppercase letters) rather than a dictionary lookup — format-valid, not
  dict-recognized, which is a narrower guarantee than the country-code validation below and
  is called out as such in the spec.
  A single amount, not a min/max range: the six ATS labels sampled ask for one number
  ("What is your desired salary?"), and a range is a speculative refinement with no
  evidence behind it yet (YAGNI).
- `notice_period_days integer` — stored as days rather than the free-text options ATS forms
  show ("Immediately", "2 weeks", "1 month", "3+ months") because those option sets differ
  per platform; the agent's existing `Choose` step already maps a candidate's stated fact
  onto whichever real options a given form offers, the same way it already maps a location
  string onto a location dropdown.
- `willing_to_relocate boolean`
- `age_18_or_older boolean`

Each column is nullable and independently settable — the "individually optional" proposal
requirement is a schema property, not application logic to maintain.

**No provenance/confirmation state machine, unlike `internal/experience`.** Experience needs
one because the model routinely *infers* achievements from conversation that the candidate
never confirmed, and publishing an unconfirmed inference to a CV would misrepresent them.
Here there is nothing to infer: every field is a scalar fact only the candidate can state
("my notice period is 30 days"), so the write path — manual form or assistant tool — always
carries a value the candidate themselves typed or spoke. The assistant tool follows the same
grounding rule every other write tool follows (only write what the caller actually said),
enforced the same way — by the tool's description and the model's own scoping — with no
extra state needed.

**The store surfaces through the existing autofill assembly, not a parallel one.**
`buildAutofillProfile` (`internal/handler/autofill_profile.go`) already assembles one
`autofillProfile` served to both the deterministic read and the agent-driven run. This
change adds the six fields to that struct as formatted strings (e.g. `"1 month"`,
`"120,000 USD/year"`, `"yes"`), read from the new store alongside the existing CV/résumé
reads. The `autofillagent.Profile` map (already `map[string]string`, already keyed however
the caller decides) gains the corresponding keys. No second endpoint, no second assembly —
the "both entry points share one assembly" invariant `extension-autofill`'s spec already
states extends to the new fields for free.

The `choosePrompt` and `systemPrompt` in `internal/autofillagent/planner.go` are updated:
the line telling the model these questions are never answered by a profile is removed, since
it is no longer true.

**Manual edit lives on the existing profile page** (`web/src/routes/my/profile`), as a new
section beside the existing skills/specializations form, rather than a new route. It is
already where a candidate manages profile-shaped data about themselves, and a second
settings surface for a conceptually similar six-field form would split one mental model
into two pages for no benefit.

**The assistant tool is a single `screening_answers_set` tool**, following
`internal/assistant/AGENTS.md`'s "Adding a tool" convention: registered in
`internal/handler/assistant_tools.go`, implemented in a new
`internal/handler/assistant_screening_tools.go`, calling the same
`internal/screeninganswers` service the manual-edit handler calls. It accepts any subset of
the six fields (a candidate stating just their notice period in passing should not require
restating the other five) and returns the stored result so the model can confirm back what
it wrote.

## Risks / Trade-offs

- **A stale screening answer reaches a real application.** A candidate's notice period or
  salary expectation changes over time; a stored value goes stale silently, the same
  staleness risk the CV/résumé contact fields already carry today, mitigated the same way —
  it is always visible and editable on the profile page, and the agent's plan is reported
  to the candidate before submission (existing `Report`/`Deferred` behavior), never
  submitted unseen.
- **`notice_period_days` as an integer may not map cleanly onto a coarse form option** (e.g.
  a form offering only "Immediately" / "2 weeks" / "1+ month" for a candidate who stored 20
  days) → the existing `Choose` step already resolves a fact onto the nearest real option
  or declines if none fits; no new mechanism needed, but the mapping quality is worth
  watching once this ships.
- **Six new profile fields is scope creep risk on the profile page UI** → keep the new
  section visually and structurally separate from skills/specializations (its own heading,
  its own save action) so it can be split into its own page later without a schema change.

## Migration Plan

Additive: one new migration creates `screening_answers` (next number, `0092`). No existing
table changes. Rollback is dropping the table; nothing else in the system depends on its
existence, since every read degrades to "unstated" the same way an absent CV or résumé
already does.

## Open Questions

- Exact wording/order of the new profile-page section — left to implementation to match the
  existing profile form's style.
