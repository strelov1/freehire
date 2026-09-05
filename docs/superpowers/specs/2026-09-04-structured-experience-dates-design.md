# Structured dates for work-history/CV periods

## Problem

`internal/candidate/experience.Employment`, `internal/candidate/resumeextract.Experience`,
and `internal/candidate/cv.ExperienceItem` (plus their `Education`/`Certification`
siblings) all store a period's start/end as free-form strings ("October 2018",
"2024", "Present") by deliberate, documented design — no parsing is attempted at
write time. This has a real, already-documented cost: `experience_employments`'
own SQL index sorts `period_start` lexicographically, which is wrong whenever a
user's roles mix formats (a bare year sorts as if it were January, month names
sort by letter not calendar order), and `internal/candidate/experience/period_sort.go`
exists purely to re-sort the SQL's wrong-order result in Go afterward.

The same free-text convention is intentionally shared across all three types (per
migration `0047_experience_bank.sql`'s own comment), and repeated as three
independent Go struct definitions and three independent frontend type
declarations (two generated via `cmd/gen-contracts`, one hand-maintained in
`web/src/lib/types.ts`). This change replaces the convention everywhere it
appears with one shared structured type, and gives `/my/profile`'s Work history
editor a real date-picker.

## Goals / Non-Goals

**Goals:**
- One shared value type for a CV/work-history period boundary, used by all three
  Go packages and reflected in all three frontend type declarations.
- `experience_employments` (the only *accumulating* store among the three) gets
  real structured columns, sortable natively in SQL — `period_sort.go` is deleted.
- `resumeextract`'s LLM extraction returns the structured shape directly
  (schema-constrained output), not a string for the model to format itself.
- `/my/profile`'s Work history editor gets a real month/year picker that also
  accepts direct keyboard entry.
- Existing stored data (the `experience_employments` text columns; already-stored
  `resume_structured`/`cv_documents` JSON) keeps working — no user-visible data
  loss beyond the explicitly-approved backfill fallback below.

**Non-Goals:**
- No day-of-month precision — CVs never carry one, and none of the three existing
  free-text conventions or their tests ever show one.
- No new free-text precision beyond year/month (no quarters, seasons, or custom
  ranges) — confirmed absent from real data via `period_sort_test.go`'s fixtures
  and every doc comment describing the convention.
- No change to the 9 typst CV templates — see Decisions.
- No change to how `Current`/`is_current` ("Present") is modeled — it stays a
  separate boolean, as it already is in `experience.Employment` and
  `cv.ExperienceItem` (this change only adds the same field to
  `resumeextract.Experience`, which lacks it today).

## Decisions

### A shared `PeriodDate` type, one new package

```go
// internal/candidate/perioddate
type PeriodDate struct {
    Year  int
    Month int // 0 = year-only
}
```

Used as `*PeriodDate` wherever the field may be entirely absent (an employment
with no dates at all). A new package rather than putting it in any one of
`experience`/`resumeextract`/`cv` — all three are peers in the same layer
(`candidate`) and none should own a type the other two also need; a small shared
package one import away from each is the same shape as the layering doc's own
`internal/dict` sitting below packages that all need normalization helpers.
`perioddate` exports the type, `Parse` (the free-text best-effort parser — see
Migration Plan), `Format` (renders back to display text, e.g. "Mar 2021" /
"2018"), and validation (`Sanitize`/range-check, 1900–2100 matching
`period_sort.go`'s existing bound).

**Alternative considered:** keep three independent structs (status quo) and only
add a derived, database-only sort key column. Rejected: it satisfies the sorting
bug but not the actual ask — the API/storage layer would still expose free text,
and the date-picker would have nothing structured to bind to.

### `experience_employments`: real columns, not jsonb

New migration replaces `period_start text` / `period_end text` with
`period_start_year int`, `period_start_month smallint` (nullable),
`period_end_year int` (nullable), `period_end_month smallint` (nullable). Plain
integer columns rather than a `date` type: a SQL `date` cannot represent
"year-only" without an arbitrary day-of-month placeholder that would lie about
precision. `ORDER BY period_start_year DESC NULLS LAST, period_start_month DESC
NULLS LAST` replaces the lexicographic index; `period_sort.go` and its test file
are deleted, and `ListEmployments`'s Go-side re-sort goes with them.

### `resumeextract`/`cv`: same type, JSON-native, self-healing on read

`resume_structured` and `cv_documents` are jsonb blobs (migrations `0011`,
`0024`) — no schema migration needed there, only the Go struct's field type
changes (`Start *perioddate.PeriodDate` instead of `Start string`). Backward
compatibility for rows already stored in the OLD (free-text) shape is handled by
`PeriodDate.UnmarshalJSON` accepting **either** a JSON string (parsed via the
same best-effort parser as the backfill, falling back to `nil` on failure) or the
new `{"year":2018,"month":3}` object; `MarshalJSON` always emits the new object
shape. A user's next resume upload or next tailored CV open/save naturally
upgrades their stored JSON — no bulk rewrite migration needed, consistent with
these two stores being regenerated/thrown-away snapshots rather than
accumulating records (per `internal/candidate/experience/AGENTS.md`'s own
boundary table).

### `resumeextract`'s LLM contract changes shape, not just prompt wording

The extraction schema (`internal/platform/llm`'s schema-constrained output,
already used for `resumeextract`) requires `start`/`end` as `{year, month?}`
objects instead of free strings, and the system prompt instruction changes from
"keep dates as written" to: interpret the CV's printed range into year/(optional)
month, and signal an ongoing role via `current: true` rather than a special `end`
string. `resumeextract.Experience` gains a `Current bool` field (mirroring
`experience.Employment`/`cv.ExperienceItem`, which already have one) so this is
expressible.

### Typst templates: untouched

All 9 `.typ` files' `daterange(a, b)` helper takes two already-formatted display
strings; `internal/candidate/cv/renderer.go` builds `data.json` for typst today
by passing `ExperienceItem.Start`/`End` straight through. This change makes
`renderer.go` call `perioddate.Format` on the structured field when building that
JSON, so `data.json`'s shape — and therefore every template — is byte-for-byte
unchanged. The structured type is a storage/API-layer change; the render-time
projection to typst stays string-formatted.

### Frontend: `<input type="month">` plus a "year only" toggle

The native month picker gives both click-to-pick and type-to-enter for free;
checking "I don't remember the month" swaps it for a plain year number input.
One shared Svelte component (`PeriodDateInput.svelte` or similar), used by
`ExperienceBankView.svelte`'s employment start/end fields and
`CvSectionForm.svelte`'s experience/education date fields — replacing today's
plain `<Input>` text fields at `ExperienceBankView.svelte:191-192,209-210` and
`CvSectionForm.svelte:88-89,124-125`.

**Alternative considered:** a fully custom picker component (own dropdowns for
month and year). Rejected: `<input type="month">` already satisfies "pick or
type" natively, with less code and free OS-level accessibility, at the cost of
needing the small year-only toggle for one edge case native month inputs cannot
express.

### Frontend types: fix all three, generated ones follow the Go change

`resumeextract.Experience` and `cv.ExperienceItem` are both in `cmd/gen-contracts`
already — their generated `start`/`end` fields become whatever shape
`cmd/gen-contracts` emits for `perioddate.PeriodDate` (a nested
`{year: number; month?: number}` interface) once the Go field type changes; `make
gen-contracts` regenerates `contracts.ts` mechanically. `experience.Employment`
is NOT in `gen-contracts` (confirmed) — its hand-maintained mirror in
`web/src/lib/types.ts:1277-1293` is edited by hand to match.

## Migration Plan

Deploy order (schema before code, per this repo's `migrate` convention):

1. New migration adds the four new nullable columns to `experience_employments`
   alongside the existing `period_start`/`period_end text` (additive, no data
   loss yet).
2. A one-off backfill (`cmd/backfill-experience-dates`, following the
   `cmd/backfill-*` conventions already in this repo) walks every row: parse
   `period_start`/`period_end` with `perioddate.Parse` (the same logic
   `period_sort.go` has today); on success, write the parsed year/month into the
   new columns. **On failure** (confirmed rare — real data is "2024"/"October
   2018"-shaped, per `period_sort_test.go`'s fixtures — but a free-text field
   accepts anything): fall back to the **year of the row's own `created_at`** as
   the approved lossy default, rather than leaving the row null — an
   approximate date reads better to the row's owner than an empty one, and nulls
   would otherwise sort ambiguously against real dates. `is_current` rows with no
   parseable end are left with `period_end` unset (already the existing
   convention — `end` is meaningless when `current=true`).
3. Deploy the code that reads/writes the new columns (`experience.Employment`,
   its repository/store, the API wire shape, the frontend).
4. A follow-up migration drops the old `period_start`/`period_end text` columns
   once the backfill and the new code have both landed and been verified in
   production (kept as a separate migration file, not squashed into step 1, so
   the drop is reviewable and reversible independently of the additive step).

`resume_structured`/`cv_documents` need no equivalent backfill — see Decisions
("self-healing on read").

## Risks / Trade-offs

- [The LLM extraction contract change (free string → structured object) could
  degrade extraction quality if the model handles the new schema worse than the
  old "copy the string" instruction] → mitigated by keeping the ask conceptually
  simple (year + optional month + a boolean, which is easier for a model to
  reason about than reproducing arbitrary CV formatting verbatim); verify with a
  manual pass over a handful of real CVs before considering `resumeextract` done.
- [Backfilling unparseable dates to `created_at`'s year is a lossy default] →
  explicitly approved; the row was already free text of unknown reliability, and
  `created_at` is a real, non-arbitrary date about that row.
- [Three independent consumers (experience/resumeextract/cv) all changing
  together is a wide, easy-to-partially-miss diff] → the task list (below, once
  written) sequences one consumer at a time with its own tests green before
  moving to the next, rather than one giant cross-cutting commit.

## Open Questions

None — every ambiguous point (UI widget shape, backfill fallback, migration
scope) was resolved with the user before writing this doc.
