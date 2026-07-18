## 1. Schema & sqlc

- [x] 1.1 Add a new numbered migration in `migrations/`: nullable `skills text[]`, `regions text[]`, `cities text[]`, `work_mode text`, `salary_min int`, `salary_max int`, `salary_currency text`, `salary_period text` on `job_submissions`; nullable `salary_min_manual int`, `salary_max_manual int`, `salary_currency_manual text`, `salary_period_manual text` on `jobs`.
- [x] 1.2 Regenerate sqlc so the new columns land in the models (build green). The per-query text edits (submission insert/select, `UpsertManualJob`, the enrichment apply/update) are folded into the consuming feature tasks below, where each is test-covered.

## 2. Job aggregate: explicit region/city overrides

- [x] 2.1 Add `Regions []string` and `Cities []string` to `jobderive.Input`; in `Derive`, apply the explicit-wins precedence already used for `WorkMode` (explicit value wins over `geo.Regions`/`geo.Cities`, empty falls back to derivation). Cover with a `jobderive` test.
- [x] 2.2 Add `Regions`/`Cities` to `job.Draft` and pass them into `jobderive.Input` from `job.New`; test that an explicit region/city on the draft survives onto `job.Fields` while an unsupplied one still derives.

## 3. Manual salary override

- [x] 3.1 Carry the manual salary (min/max/currency/period) on `job.Draft`/`job.Fields` as base fields (not derived); test they round-trip through `job.New` and the repository load projection.
- [x] 3.2 Make the effective salary prefer the manual salary: when a job has a manual salary, `enrichment.salary_*` (the single projection every consumer reads) reflects it. Overlay the manual salary over the LLM payload in the enrichment apply path before persisting; test that an enrichment pass preserves a manual salary and leaves a manual-less job unchanged.
- [x] 3.3 Seed `enrichment.salary_*` from the manual salary columns at mint so salary displays immediately before any enrichment pass; test the minted job shows the manual salary pre-enrichment.

## 4. Create / submit contract

- [x] 4.1 Add `skills`, `regions`, `cities`, `work_mode`, `salary_min`, `salary_max`, `salary_currency`, `salary_period` to `createJobRequest` and `moderation.CreateInput`; validate `work_mode`/`regions` against the known vocab (drop unknowns) and keep salary sanity checks. Test the contract mapping and validation.
- [x] 4.2 Thread the explicit facets through `moderation.Create`/`Update` into `derive(...)` and set the manual salary on the created job; test that create with explicit facets/salary produces a job carrying them.
- [x] 4.3 Persist and echo the structured facets + salary on the submission (`submission` service/repository, `job_submissions` columns, `submissionResponse`); test a submission round-trips them.
- [x] 4.4 Verify the approve/mint path forwards the submission's structured facets + salary into the minted job (integration test on the submission approve handler).

## 5. Frontend

- [x] 5.1 Extend `SubmissionInput` (`web/src/lib/api.ts`) and `Submission` (`web/src/lib/types.ts`) with the new optional fields.
- [x] 5.2 Provide a rich (markdown) description editor for the form reusing the tracker's `NoteEditor`/EasyMDE approach, and convert its markdown to HTML with the bundled `marked` on submit.
- [x] 5.3 Build the single-value facet inputs (skills chip input via `TokenInput`, region selector, city input, work-mode selector, salary min/max + currency/period) on `$lib/ui` primitives, reusing the shared vocabularies (`REGION_OPTIONS`, work-mode vocab, currency list) and pill/chip style.
- [x] 5.4 Redesign `SubmitView.svelte` within the design system: sectioned card (Basics / Details / Description), field icons, spacing, and an explicit success state; wire all new fields into `api.submitJob`.
- [ ] 5.5 (DEFERRED — pending clarification) Nav change: which "sidebar" to remove the submit entry from is unresolved; the account `/my` sidebar has no submit link and the header burger is the only nav link. Awaiting the user to point at the exact surface.

## 6. Verify & wrap up

- [x] 6.1 `go build/vet/test` green, `gofmt` clean, web `svelte-check` 0 errors, production `npm run build` OK, and the enrichment/mint/e2e SQL behaviour covered by `-tags=integration` (db + handler). Pixel-level visual check of the authenticated form not run (auth gate) — offered to the user.
- [ ] 6.2 Offer a `/blog` changelog entry (write-changelog) — this is a user-facing change.
