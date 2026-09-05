## Context

See `proposal.md` for motivation. Full file-level design (every touched file,
per-consumer detail) was worked out and approved with the user beforehand at
`docs/superpowers/specs/2026-09-04-structured-experience-dates-design.md`; this
document summarizes the decisions in OpenSpec's format and is the source the
task list is built from.

Confirmed by direct code inspection: `experience.Employment`,
`resumeextract.Experience`, and `cv.ExperienceItem` are three independent Go
struct definitions sharing one documented convention (migration
`0047_experience_bank.sql`'s own comment: "matching resumeextract.Experience and
cv.ExperienceItem"). `experience_employments` is the only *accumulating* store of
the three (`resume_structured`/`cv_documents` are regenerated/thrown-away
snapshots per `internal/candidate/experience/AGENTS.md`'s boundary table) and the
only one with real columns rather than a jsonb blob.

## Goals / Non-Goals

**Goals:**
- One shared `perioddate.PeriodDate{Year, Month}` type used by all three Go
  packages and reflected in all three frontend type declarations.
- `experience_employments` gets structured, natively-sortable columns;
  `period_sort.go` is deleted.
- `resumeextract`'s LLM extraction returns the structured shape directly.
- A real month/year picker (pick or type) on `/my/profile`'s Work history editor
  and the CV section form.
- Existing stored data keeps working: a backfill for `experience_employments`, a
  self-healing decoder for the jsonb stores — no user-visible data loss beyond
  the approved backfill fallback.

**Non-Goals:**
- No day-of-month precision, no free-text precision beyond year/month (no
  quarters/seasons) — confirmed absent from real data via
  `period_sort_test.go`'s fixtures.
- No change to any of the 9 typst CV templates.
- No change to how "Present"/ongoing is modeled (`is_current`/`Current` stays a
  separate boolean) beyond adding that same field where it's missing today
  (`resumeextract.Experience`).

## Decisions

- **A new package, `internal/candidate/perioddate`, not a field added to one of
  the three existing packages.** All three (`experience`, `resumeextract`, `cv`)
  are peers in the same layer (`candidate`); none should own a type the other
  two also depend on. Exports the type, a best-effort free-text `Parse` (reused
  by both the backfill and the jsonb self-healing decode), `Format` (back to
  display text), and range validation (1900–2100, matching `period_sort.go`'s
  existing bound).
- **`experience_employments`: four plain nullable integer columns**
  (`period_start_year`, `period_start_month`, `period_end_year`,
  `period_end_month`), not a SQL `date` type — a `date` cannot represent
  "year-only" without lying about precision via an arbitrary day. Native
  `ORDER BY ... NULLS LAST` replaces the lexicographic index and
  `period_sort.go`'s Go-side re-sort, which is deleted along with its test file.
- **`resume_structured`/`cv_documents`: no bulk migration.** Only the Go field
  type changes; `PeriodDate.UnmarshalJSON` accepts either the legacy free-text
  string (parsed via the same `perioddate.Parse` as the backfill, falling back
  to `nil` on failure) or the new `{"year":..,"month":..}` object.
  `MarshalJSON` always emits the new shape, so a row upgrades itself the next
  time it's read and re-saved (upload / tailor / save) — consistent with these
  two stores being regenerated snapshots rather than accumulating records.
- **`resumeextract`'s LLM contract changes shape, not just prompt wording**: the
  schema-constrained output (`internal/platform/llm`) requires `{year, month?}`
  objects for `start`/`end` instead of free strings, and the prompt asks the
  model to interpret the CV's printed range rather than "keep dates as
  written". `resumeextract.Experience` gains `Current bool`.
- **Typst stays untouched as an interface; the rendered text is
  canonicalized, not preserved verbatim.** `internal/candidate/cv/renderer.go`
  formats the structured field via `perioddate.Format` when building
  `data.json`; typst still receives a plain string, unchanged as a wire shape.
  The string's content is NOT guaranteed byte-identical to an existing entry's
  original free text: `Format` emits one canonical style ("Mar 2021"), so a
  pre-existing entry stored as "October 2018" renders as "Oct 2018" after this
  change. This is a one-time cosmetic normalization on first read after the
  backfill, not a functional regression — the "CV document" delta spec's own
  scenario for this compares against an already-canonical value rather than
  claiming universal byte-identical output, and a second scenario there covers
  the canonicalization explicitly.
- **Frontend: `<input type="month">` + a "year only" toggle**, one shared
  component replacing the plain text inputs at `ExperienceBankView.svelte` and
  `CvSectionForm.svelte`. Rejected alternative: a fully custom picker — more
  code and its own accessibility work for no capability the native control
  lacks beyond the single year-only case, which the toggle covers.
- **Frontend types**: `resumeextract.Experience`/`cv.ExperienceItem` are both in
  `cmd/gen-contracts` — regenerated mechanically once the Go field type changes.
  `experience.Employment` is NOT in `gen-contracts` (confirmed) — its
  hand-maintained mirror in `web/src/lib/types.ts` is edited by hand to match.

## Risks / Trade-offs

- [The LLM extraction contract change could degrade extraction quality if the
  model handles the new schema worse than "copy the string verbatim"] →
  mitigated by keeping the ask conceptually simple (year + optional month + a
  boolean); verify with a manual pass over real CVs before considering
  `resumeextract` done, per its own task below.
- [Backfilling unparseable employment dates to `created_at`'s year is a lossy
  default] → explicitly approved by the user; the row was already free text of
  unknown reliability, and `created_at` is a real, non-arbitrary date about
  that row, confirmed rare in practice (`period_sort_test.go`'s real-world
  fixtures are all cleanly parseable).
- [`perioddate.Format`'s canonical style ("Mar 2021") does not reproduce every
  existing entry's original free-text wording (e.g. "October 2018"), so any
  CV or CV/experience-bank display rendered after this change can show
  different text for the same date than it did before, for entries that were
  never edited or re-rendered before now] → accepted as a one-time, purely
  cosmetic normalization: the underlying year/month is unchanged, only the
  formatting is; earlier drafts of this document and the delta specs
  overstated this as "no external behavior change" / "rendered output is
  unchanged", which was inaccurate and has been corrected.
- [Three independent consumers changing together is a wide diff] → the task
  list sequences one consumer at a time (bank → resumeextract → cv →
  frontend), each with its own tests green before moving to the next.

## Migration Plan

1. New migration adds the four new nullable columns to `experience_employments`
   alongside the existing free-text ones (additive).
2. `cmd/backfill-experience-dates` (one-off, following this repo's
   `cmd/backfill-*` conventions) parses every row's existing free text into the
   new columns; unparseable rows fall back to `created_at`'s year.
3. Deploy the code that reads/writes the new columns.
4. A follow-up migration drops the old free-text columns once the backfill and
   new code have both landed and been verified.

`resume_structured`/`cv_documents` need no equivalent step (self-healing, see
Decisions).

## Open Questions

None — every ambiguous point (UI widget shape, backfill fallback, migration
scope) was resolved with the user before writing this document.
