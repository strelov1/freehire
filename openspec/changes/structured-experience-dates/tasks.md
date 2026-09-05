## 1. Shared `perioddate` package

- [ ] 1.1 Create `internal/candidate/perioddate/perioddate.go`: `type PeriodDate struct { Year, Month int }` (`Month == 0` = year-only), `Sanitize()` (year clamped 1900–2100, month clamped 0/1–12 else 0), `Format() string` (e.g. "Mar 2021", "2018").
- [ ] 1.2 Add `Parse(s string) (*PeriodDate, bool)` to the same package: port `period_sort.go`'s existing `parsePeriodLabel`/`parseYearMonth`/`parseMonthYear`/`monthNumber` logic (currently returning a sortable int) to instead return a `*PeriodDate`. Recognize the same formats it does today (`YYYY-MM`/`YYYY/MM`, `Month YYYY`/`Mon YYYY`, bare `YYYY`).
- [ ] 1.3 Add `IsPresentLabel(s string) bool` to the same package: port `period_sort.go`'s existing `isPresentLabel`.
- [ ] 1.4 `PeriodDate.MarshalJSON`/`UnmarshalJSON`: marshal always emits `{"year":Y,"month":M}` (omit `month` key when 0); unmarshal accepts that object shape OR a legacy JSON string (delegates to `Parse`, `nil` on failure — never errors on unparseable legacy input).
- [ ] 1.5 Unit tests: port every case from `internal/candidate/experience/period_sort_test.go`'s `TestParsePeriodLabel` (`"2024"`, `"2023-09"`, `"2023/09"`, `"Jan 2018"` .. `"October 2018"`, plus `"Present"`/`""`/`"sometime"` failing to parse as a date) as `perioddate.Parse` tests; add `UnmarshalJSON` tests for both the object and legacy-string shapes, and for genuinely garbage legacy strings (must decode to `nil`, not error).
- [ ] 1.6 Add `internal/candidate/perioddate` to the layering table (`internal/platform/arch/layering/blocks.go`), same layer as `experience`/`resumeextract`/`cv`.

## 2. `experience_employments`: schema + backend

- [ ] 2.1 New migration: add `period_start_year int`, `period_start_month smallint`, `period_end_year int`, `period_end_month smallint` (all nullable) to `experience_employments`, additive alongside the existing `period_start`/`period_end text`. Replace `experience_employments_user_idx` with one ordering on the new columns (`user_id, is_current DESC, period_start_year DESC NULLS LAST, period_start_month DESC NULLS LAST`).
- [ ] 2.2 Update `internal/platform/db/queries/experience.sql`: employment insert/update/select statements read/write the four new columns instead of the two text ones; `ListEmployments`'s `ORDER BY` uses the new columns natively. Run `make sqlc`.
- [ ] 2.3 `internal/candidate/experience/experience.go`: `Employment.Start`/`End` become `*perioddate.PeriodDate`; update `Sanitize()` to call `PeriodDate.Sanitize()` on each instead of `clip()`-ing a string.
- [ ] 2.4 `internal/candidate/experience/repository.go`: adapt `CreateEmployment`/`FillEmploymentBlanks`/read paths to the new sqlc-generated column shape (four nullable ints in, `*PeriodDate` out).
- [ ] 2.5 `internal/candidate/experience/import_resume.go` (`EntriesFromResume`): copy the now-structured `Start`/`End` straight from `resumeextract.Experience` (both are `*perioddate.PeriodDate` after task 3); `Current` derivation (`role.End == "" || isPresentLabel(role.End)`) becomes `role.Current` read directly (see task 3.3).
- [ ] 2.6 Delete `internal/candidate/experience/period_sort.go` and `period_sort_test.go`; remove `store.go`'s call to `sortEmploymentsChronological` in `ListEmployments` (now just what the SQL `ORDER BY` returns, verify a still-needed stability tie-break if `period_sort_test.go`'s fixtures had one beyond the date itself).
- [ ] 2.7 `internal/api/handler/assistant_experience_tools.go:637`: replace the `Start + " – " + End` string concatenation with `perioddate.Format` on each side (matching the `daterange`-style " – " join only when both are present).
- [ ] 2.8 `cmd/backfill-experience-dates` (new, following `cmd/backfill-*` conventions — idempotent, chunked, needs only `DATABASE_URL`): for every employment row with the new columns still null, parse `period_start`/`period_end` via `perioddate.Parse`; on failure, use the year of `created_at`. Add its entry to `AGENTS.md`'s worker table.
- [ ] 2.9 Follow-up migration (separate file, applied only after 2.8 has run and the new code is deployed): drop `period_start`/`period_end text` and the now-superseded index, if any remains.
- [ ] 2.10 Go tests for 2.3–2.7 green (`go test ./internal/candidate/experience/...`).

## 3. `resumeextract`

- [ ] 3.1 `internal/candidate/resumeextract/structured.go`: `Experience.Start`/`End` become `*perioddate.PeriodDate`; add `Current bool`; do the same for `Education.Year` (mirror the reasoning — a bare year is already what that field holds, now typed) if a same-shaped change applies cleanly, else leave `Education.Year` as-is and note why in the task's own commit.
- [ ] 3.2 Update `sanitizeExperience()` or its replacement to call `PeriodDate.Sanitize()`.
- [ ] 3.3 Update the LLM extraction schema (wherever `internal/platform/llm`'s schema-constrained output for `resumeextract` is declared) so `start`/`end` are `{year, month?}` objects and `current` is a boolean field, not a string sentinel.
- [ ] 3.4 Update `resumeextract.go`'s `systemPrompt` (currently: "keep dates as written, e.g. '2021-03' or 'Present'") to instruct the model to interpret the CV's printed range into year/(optional month), and to use `current: true` for an ongoing role instead of a special `end` value.
- [ ] 3.5 Go tests green (`go test ./internal/candidate/resumeextract/...`); manually spot-check extraction against a handful of real/sample CVs for date-quality regressions (per design.md's Risk) before calling this task done.

## 4. `cv` (document model + renderer)

- [ ] 4.1 `internal/candidate/cv/cv.go`: `ExperienceItem.Start`/`End` become `*perioddate.PeriodDate` (keep `Current bool`, unchanged); same for `EducationItem.Start`/`End` and `Certification.Year` where they hold the same free-text convention.
- [ ] 4.2 Update `cv.go`'s `sanitizeExperience()` (and education/certification equivalents) to call `PeriodDate.Sanitize()`.
- [ ] 4.3 `internal/candidate/cv/seed.go` (`Seed()`): copy the now-structured fields straight from `resumeextract.Experience`/`Education` instead of string-copying.
- [ ] 4.4 `internal/candidate/cv/renderer.go`: when building `data.json` for typst, format each structured date via `perioddate.Format` into the same plain string the templates already expect — no `.typ` file changes. Add a test asserting the JSON payload's `start`/`end` fields are still plain strings, byte-for-byte matching what the old free-text field would have produced for an equivalent value.
- [ ] 4.5 Go tests green (`go test ./internal/candidate/cv/...`), including a render-output snapshot/comparison test for at least one template confirming PDF text is unchanged for a representative experience entry.

## 5. Frontend

- [ ] 5.1 Regenerate `web/src/lib/generated/contracts.ts` (`make gen-contracts`) — `Experience`/`ExperienceItem`'s `start`/`end` become the generated `PeriodDate`-shaped interface.
- [ ] 5.2 Hand-edit `web/src/lib/types.ts`'s `ExperienceEmployment` (`experience.Employment` is not in `gen-contracts`) to match: `start`/`end` become `{ year: number; month?: number } | undefined`.
- [ ] 5.3 New shared component (e.g. `web/src/lib/components/PeriodDateInput.svelte`): `<input type="month">` bound to the structured value, plus a "I don't remember the month" toggle that swaps in a plain year `<input type="number">`; emits/accepts the `{year, month?}` shape (or `undefined`).
- [ ] 5.4 `web/src/lib/components/ExperienceBankView.svelte`: replace the plain text start/end inputs (employment add/edit form) with `PeriodDateInput`; update the display line that concatenates `employment.start`/`end` to format the structured value instead.
- [ ] 5.5 `web/src/lib/components/cv/CvSectionForm.svelte`: replace the plain text start/end inputs (experience and education entries) with `PeriodDateInput`.
- [ ] 5.6 `web/src/lib/api.ts` (and any other call site constructing/reading `ExperienceEmployment`/`Experience`/`ExperienceItem` start/end as a plain string) updated to the structured shape.
- [ ] 5.7 `pnpm --dir web check` (svelte-check) passes; `pnpm --dir web test` (vitest) passes.

## 6. Verification

- [ ] 6.1 `go build ./... && go vet ./...` and `go vet -tags=integration ./...` pass.
- [ ] 6.2 `go test ./...` passes; run `go test -tags=integration ./internal/platform/db/...` if the sqlc/query changes touch anything that suite covers.
- [ ] 6.3 Manual pass against the running dev stack: create an employment with a year-only start, one with month+year, one marked current; confirm they list in correct chronological order and the picker round-trips each precision correctly.
- [ ] 6.4 Manual pass: render a CV to PDF (or its HTML preview) for a profile with mixed-precision dates; confirm the rendered text is unchanged from before this change for an equivalent value.
- [ ] 6.5 Run `cmd/backfill-experience-dates` against a copy of representative existing data (or the dev stack's seeded data) and confirm every row gets a non-null structured date, with the `created_at`-year fallback exercised for at least one deliberately-unparseable row.
