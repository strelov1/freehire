## Why

The onboarding wizard asks four things — CV, specializations, skills, geography — and every
one of them is a search filter. Nothing it collects says how experienced the candidate is,
what they want to be paid, where they are in their search, or what is actually blocking
them, so neither the product nor the operator can tell a browsing senior from a panicking
junior. Two facts the platform already knows how to use are simply never asked for: the
desired salary that `screening_answers` feeds into ATS application forms, and the LinkedIn
profile URL that `POST /me/linkedin/import` consumes and then discards.

The wizard is also unreachable for anyone who already has a CV. Its gate is "redirect every
visit until a CV exists", so the roughly 660 existing accounts — nearly all of which have
uploaded one — would never see a new question. Collection would start from zero.

## What Changes

- The wizard grows from four steps to eight: CV, specializations + profile links, years of
  experience, skills, geography, money, job-search stage, biggest challenge. Every step
  stays skippable and now persists on completion of that step rather than at the end, so an
  abandoned run keeps whatever was already answered.
- **BREAKING (behavioural):** the onboarding gate stops being derived from CV presence and
  becomes an explicit `users.onboarding_completed_at`. NULL means "walk them through",
  non-NULL means "never again". Existing accounts are therefore walked through once, seeing
  only the steps they have not answered.
- Years of experience and profile links (LinkedIn, GitHub) are pre-filled from the uploaded
  CV — `Structured.TotalYears` and `Structured.Links` already carry both — and saved into
  the candidate-owned overlay (`users.candidate_contacts`), which is what protects them from
  being wiped by a later CV re-upload.
- Desired salary is captured into the existing `screening_answers.desired_salary_*` columns.
  No second copy of the number is introduced.
- Current income, job-search stage and biggest challenge are captured into a new
  `candidate_survey` table — segmentation facts that must not be mistaken for search filters.
- Two closed vocabularies (`job_search_stage`, `job_challenge`) join `internal/dict/vocab`.

## Capabilities

### New Capabilities
- `candidate-survey`: the self-reported segmentation questionnaire — job-search stage,
  biggest challenge, current income — its storage, its closed vocabularies, and its
  owner-scoped partial-update API.

### Modified Capabilities
- `jobs-onboarding`: the wizard's step set expands; the completion gate becomes an explicit
  server-side marker instead of an inference from CV presence; each step persists as it is
  answered instead of all at the end.
- `cv-autofill`: the wizard additionally pre-fills years of experience and profile links
  from the uploaded résumé.

## Impact

- **Schema:** one migration — `candidate_survey` (new table) and `users.onboarding_completed_at`
  (new nullable column, no default).
- **New Go package:** `internal/candidate/survey`, which must be registered in
  `internal/platform/arch/layering/blocks.go` or the layering guard fails the build.
- **API:** `GET/PUT /api/v1/me/survey` and `POST /api/v1/me/onboarding/complete` are new.
  `PUT /me/screening-answers`, `PUT /me/resume` and `PUT /me/profile` gain a new caller but
  no new shape.
- **Wire types:** `Owned` gains `total_years`; `GET /me` exposes `onboarding_completed_at`.
- **Frontend:** `web/src/routes/onboarding/+page.svelte` (590 lines today) grows four steps
  and a per-step save; the root layout's redirect effect changes its condition.
- **Not affected:** job ranking and matching do not read the new fields; the onboarding
  e-mail sequence is unchanged; no analytics surface is built for the questionnaire.
