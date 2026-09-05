## 1. Schema

- [x] 1.1 Add `migrations/0134_candidate_survey.sql`: `candidate_survey` table (user_id PK → users ON DELETE CASCADE, job_search_stage, biggest_challenge, biggest_challenge_note, current_income_amount, current_income_currency, current_income_period, updated_at) plus `ALTER TABLE users ADD COLUMN onboarding_completed_at timestamptz`. Header comment states why named columns over jsonb and why no extra index.
- [x] 1.2 Verify the migration passes `pnpm check:sql` (squawk) and that `TestEveryUserForeignKeyIsIndexed` still passes.

## 2. Vocabularies

- [x] 2.1 Add `JobSearchStageValues` to `internal/dict/vocab` beside `SalaryPeriodValues`, with the four members and a comment on what each means.
- [x] 2.2 Add `JobChallengeValues` to `internal/dict/vocab`, including the explicit `other` member the free-text note is gated on.

## 3. Survey package

- [x] 3.1 Register `internal/candidate/survey` in the block table in `internal/platform/arch/layering/blocks.go` (a package in neither list fails the guard).
- [x] 3.2 Add sqlc queries in `internal/platform/db/queries/` for get/upsert on `candidate_survey`, then `make sqlc`.
- [x] 3.3 Implement `survey.Answers` with independently-nullable fields, plus `Sanitize`/`Validate`: vocabulary membership for stage and challenge, ISO 4217 shape for currency, `SalaryPeriodValues` membership for period, and rejection of a note paired with any challenge other than `other`.
- [x] 3.4 Implement the repository: owner-scoped read that reports all-unstated rather than failing when no row exists, and a partial upsert where an omitted field keeps its stored value. (No clear operation — see the decision recorded in design.md; a JSON null and an omitted field are indistinguishable to `encoding/json` without presence-detection machinery in every caller, and the adjacent screening answers settled the same way.)

## 4. Survey and onboarding API

- [x] 4.1 Add `GET/PUT /api/v1/me/survey` in a new `internal/api/handler/survey.go`, mirroring `screening_answers.go` including its middleware split (read admits an API key, write is cookie-only).
- [x] 4.2 Add `POST /api/v1/me/onboarding/complete`, cookie-only, idempotent — a second call on an already-marked account is a success, not a conflict.
- [x] 4.3 Expose `onboarding_completed_at` on the existing `/me` read so the layout's gate needs no second request.
- [x] 4.4 Document the three new routes. NOT `web/static/openapi.yaml` — that schema is the PUBLIC read-only contract and carries no `/me/*` path at all (see its own `info.description`). The authenticated surface is `docs/API.md`, generated from `web/src/lib/docs/api-spec.ts` and ratcheted by the `docs` CI job; edit the source and run `pnpm --dir web run gen:api-docs`.

## 5. Owned overlay: years and links

- [x] 5.1 Add `TotalYears int` and `TotalYearsSet bool` to `resume.Owned`, following the `HeadlineSet` convention so an explicit zero is distinguishable from unstated, and make `Sanitize`/`ApplyBody` handle it (flag only ever turns on).
- [x] 5.2 Overlay `TotalYears` onto the composed structured CV the same way the other owned body fields are overlaid, so a résumé re-upload cannot overwrite a candidate-corrected figure.
- [x] 5.3 Add a profile-link classifier mapping a URL to `linkedin` / `github` / `other` by host, in `web/src/lib/profileLinks.ts` — NOT in Go. It decides which input box a link pre-fills, and the backend stores one flat list either way, so a Go classifier would mean a new wire shape whose only consumer is one wizard step. It carries the same rule as `internal/candidate/linkedinprofile`'s matcher — exact host match against a small set, never a suffix test — with tests for `linkedin.com.evil.example` and `github.com.evil.example`.
- [x] 5.4 Regenerate the web contract types so `Professional`/`Owned` carry `total_years`.

## 6. Wizard: step extraction

- [x] 6.1 Extract each existing step of `web/src/routes/onboarding/+page.svelte` into its own component under `web/src/lib/components/onboarding/`, leaving the page with step sequencing only. No behaviour change.
- [x] 6.2 Replace the stage-everything-then-`finish()` model with save-on-leaving-a-step, where a failed save keeps the user on that step with the error rather than advancing.
- [x] 6.3 Make the step list skip any step whose owning store already holds an answer, so an existing account sees only what it has not answered.

## 7. Wizard: new steps

- [x] 7.1 Add the profile-links fields to the `confirm` step, pre-filled from the classified `Structured.Links`, saving through `PUT /me/resume/contacts` while retaining unrecognised links.
- [x] 7.2 Add the `experience` step, pre-filled from `Structured.TotalYears`, saving through `PUT /me/resume/contacts`.
- [x] 7.3 Add the `money` step: two sliders (current income, desired salary) over one currency and one period selector, each yielding an exact integer with step 500. Desired salary saves through `PUT /me/screening-answers`, current income through `PUT /me/survey`.
- [x] 7.4 Add the `stage` step (single select from the stage vocabulary), saving through `PUT /me/survey`.
- [x] 7.5 Add the `challenge` step (single select, with the free-text note revealed only for `other`), saving through `PUT /me/survey`.

## 8. Gate

- [x] 8.1 Change the root layout's redirect condition from CV presence to `onboarding_completed_at IS NULL`, keeping `onboardingGate.dismissed` as the within-visit guard and its sign-out reset.
- [x] 8.2 Call `POST /me/onboarding/complete` on reaching the last step and on an explicit decline, and confirm navigating away does neither.
- [x] 8.3 Update `web/src/lib/onboarding.ts`'s lifecycle comments: local storage now carries only the feed banner's nudge, never completion.

## 9. Verification

- [x] 9.1 `gofmt -l .` clean, `go vet ./...`, `go test ./...`, `go vet -tags=integration ./...`.
- [x] 9.2 `pnpm --dir web lint` and `pnpm --dir web test` (run `svelte-kit sync` first in this fresh worktree).
- [x] 9.3 Exercise the wizard end to end in the browser against a local server: a fresh account through all eight steps, and an account with a CV seeing only the unanswered ones.
