## Why

`experience.Employment`, `resumeextract.Experience`, and `cv.ExperienceItem` each
store a work-history period's start/end as a free-form string ("October 2018",
"2024", "Present") by deliberate design — no parsing at write time, the same
convention repeated three times. This has a real, already-documented cost:
`experience_employments`' own database index sorts these strings
lexicographically, which is wrong whenever a candidate's roles mix formats (a
bare year sorts as if it were January; month names sort alphabetically, not
calendar order) — `internal/candidate/experience/period_sort.go` exists purely
to re-sort the database's wrong-order result in application code afterward. The
same free-text fields are also plain, unstructured text inputs on `/my/profile`'s
Work history editor and the CV section form, with no date-picker.

## What Changes

- New shared value type `internal/candidate/perioddate.PeriodDate{Year, Month}`
  (`Month == 0` means year-only), replacing the free-text `string` fields
  wherever the free-form convention is used today: `experience.Employment`,
  `resumeextract.Experience`, `cv.ExperienceItem`, and their `Education`/
  `Certification` siblings' year fields.
- `experience_employments` gets real structured columns
  (`period_start_year`/`period_start_month`/`period_end_year`/`period_end_month`)
  replacing the free-text `period_start`/`period_end`, sorted natively in SQL.
  `period_sort.go` and its Go-side re-sort are deleted.
- A one-off backfill (`cmd/backfill-experience-dates`) parses every existing
  employment's free text into the new columns; a row that fails to parse falls
  back to the year of its own `created_at` rather than being left null.
- `resumeextract`'s LLM extraction schema changes from a free string to a
  `{year, month?}` object, with `current: true` for an ongoing role (a field
  `resumeextract.Experience` gains, matching `experience.Employment`/
  `cv.ExperienceItem`, which already have one).
- `resume_structured`/`cv_documents` (jsonb) need no bulk data migration —
  `PeriodDate.UnmarshalJSON` accepts either the old free-text string (best-effort
  parsed) or the new object shape, self-healing on the next read/write of each
  row; `MarshalJSON` always emits the new shape.
- **BREAKING (internal only for the typst interface; a one-time cosmetic
  change for existing data)**: the CV renderer
  (`internal/candidate/cv/renderer.go`) formats the structured date into
  display text via `perioddate.Format` before building `data.json` — the 9
  typst templates still receive a plain string and are unchanged. `Format`
  emits one canonical style ("Mar 2021"); an existing entry whose stored free
  text used a different style (e.g. "October 2018") renders in the canonical
  style after this change, not its original wording — a one-time
  normalization, not a functional regression or a change to what information
  is shown.
- `/my/profile`'s Work history editor and the CV section form get a real
  month/year picker (`<input type="month">` plus a "year only" toggle) in place
  of a plain text input.
- **BREAKING (internal only)**: `experience.Employment`'s hand-maintained
  frontend mirror (`web/src/lib/types.ts`) and the two `cmd/gen-contracts`-
  generated interfaces (`resumeextract.Experience`, `cv.ExperienceItem`) all
  change from `start?: string` to a structured `start?: { year: number; month?:
  number }`.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `experience-bank`: adds a requirement that an employment's period boundaries
  are a structured year/(optional)month value rather than free text, and that
  listing sorts on it natively rather than via an application-side re-sort.
- `resume-structured-profile`: the "work-experience entries (... dates ...)"
  requirement's `dates` becomes the same structured `{year, month?}` value
  (plus `current`), replacing today's implicit free-text string.
- `cv-builder`: the CV document's experience/education entries carry the same
  structured date value; the renderer still formats it to a plain display
  string before the typst template ever sees it, but that string is now the
  renderer's one canonical style, which can differ from an existing entry's
  original free-text wording (see What Changes above).

## Impact

- Schema: new migration adding structured columns to `experience_employments`
  (additive), a follow-up migration dropping the old free-text columns once the
  backfill and new code have both landed. No schema change to
  `resume_structured`/`cv_documents` (jsonb, self-healing on read).
- Backend: `internal/candidate/perioddate` (new), `internal/candidate/experience`,
  `internal/candidate/resumeextract`, `internal/candidate/cv` (struct fields,
  repository/store queries, `Sanitize`/`Validate`), `internal/candidate/cv/renderer.go`
  (format-before-render), `internal/platform/db` (sqlc regeneration),
  `cmd/backfill-experience-dates` (new one-off worker).
- Frontend: `web/src/lib/types.ts` (hand-maintained mirror), `cmd/gen-contracts`
  output (`web/src/lib/generated/contracts.ts`, regenerated, not hand-edited), a
  new shared date-picker component, `ExperienceBankView.svelte` and
  `CvSectionForm.svelte` (consumers of the new component).
- No change to any of the 9 typst templates in `internal/candidate/cv/templates/`.
