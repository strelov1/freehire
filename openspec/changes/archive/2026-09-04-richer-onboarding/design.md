## Context

The wizard at `web/src/routes/onboarding/+page.svelte` runs four steps — `cv`, `confirm`,
`skills`, `location` — and commits them all at the end through `PUT /me/profile`. Everything
it asks is a search facet. The gate that routes a user into it lives in the root layout and
reads "does this account have a CV", with `onboardingGate.dismissed` (a module-level `$state`
singleton, reset on sign-out) stopping the redirect from firing repeatedly inside one visit.

Three stores already hold candidate facts, and two of them already hold facts this change
wants:

- `user_profiles` — search preferences. Carries `CHECK (cardinality(skills) > 0)` and
  `specializations` bounded 1..5, so a row cannot exist for a user who skipped those steps.
  It is strictly the search profile and cannot be the home for anything optional.
- `screening_answers` (migration 0092) — the six facts an ATS application form asks and no CV
  supplies. Already has `desired_salary_amount`, `desired_salary_currency`,
  `desired_salary_period`, written through `PUT /me/screening-answers`.
- `users.candidate_contacts` — a JSONB blob deserialised as `resume.Owned`: the overlay of
  fields the candidate edited directly, which a fresh CV upload must not silently overwrite.
  Already carries `Links []string`. Written through `PUT /me/resume/contacts`, whose body is
  `resume.Owned` verbatim.

`resumeextract.Structured` — what the CV parser returns — already carries both
`TotalYears int` and `Links []string`, so the two new pre-fills need no new extraction work.

## Goals / Non-Goals

**Goals:**

- Capture years of experience, current income, desired salary, job-search stage, biggest
  challenge, and LinkedIn/GitHub links.
- Route every existing account through onboarding once, without asking anyone a question
  they have already answered.
- Add exactly one migration, and no second copy of any fact an existing store owns.

**Non-Goals:**

- Versioning question waves so a future wave can re-ask. The seam is a single column; it is
  not built now.
- Feeding the new fields into ranking, matching, or the feed's filters.
- Changing the onboarding e-mail sequence (`cmd/onboarding`), which keys off account age and
  subscription state, not off these answers.
- Any analytics surface over the questionnaire. The data lands in Postgres; reading it is a
  query, not a screen.

## Decisions

### Desired salary reuses `screening_answers`; current income does not

Desired salary is the same fact whether it filters a feed or fills an ATS form, and
`screening_answers` already holds it under a validated shape. A second "expected salary"
column on the profile would give the product two numbers with equal claim to being the
answer, and nothing to break the tie. *Alternative rejected:* a `user_profiles.desired_salary`
— blocked anyway by that table's skills/specializations CHECKs, and duplicative regardless.

Current income is a different fact: it is what the candidate has, not what they are asking
for, and no employer ever sees it. It goes to the questionnaire. It keeps the same
amount/currency/period triple so the two figures compare without conversion.

### Years of experience and links go to `resume.Owned`, not a new column

`Owned` exists precisely for "fields a fresh CV upload would otherwise silently overwrite",
which is exactly the hazard for a candidate-corrected years figure. It is JSONB, so both
additions ship without a migration, and `PUT /me/resume/contacts` already accepts `Owned`
verbatim — no new endpoint, no new wire shape beyond the field itself.

`TotalYears` needs a companion `TotalYearsSet bool`, following the existing `HeadlineSet`
convention: zero is a legitimate answer ("less than a year") and is indistinguishable from
unstated without it. `Sanitize` only ever turns such a flag on, never off, so an explicit
clear survives.

### Links stay one flat list, classified by host

`Owned.Links` remains `[]string`. LinkedIn and GitHub are recognised by parsing each URL's
host, not stored as two named fields. Naming them would create a third and fourth place a
LinkedIn URL can live (CV-extracted `Structured.Links`, owned `Links`, owned `LinkedInURL`),
and nothing would keep them agreeing.

The classifier lives in the frontend (`web/src/lib/profileLinks.ts`), not in Go. What it
decides is which input box a link pre-fills — a presentation question — and the backend
stores one flat list whichever way it goes, so a Go classifier would mean a new wire shape
with exactly one consumer: one wizard step. It carries the same rule the Go side uses in
`internal/candidate/linkedinprofile`: an exact host match against a small set, never a
suffix test, because `linkedin.com.evil.example` ends in nothing that set accepts. GitHub
gets the identical treatment, and both are tested against their spoofed hosts.

*Alternative rejected:* typed `LinkedInURL`/`GitHubURL` fields on `Owned`. Simpler to read,
but it forks the storage of one fact and leaves the CV-extracted copy unreconciled.

### The completion gate becomes an explicit column

`users.onboarding_completed_at timestamptz` (nullable, no default). NULL routes the user in;
non-NULL never does. The current rule infers completion from CV presence, which was true
while the wizard was about the CV and is false the moment it asks anything else.

Marked on reaching the last step and on an explicit decline. Not marked on navigating away —
that user is asked again next visit, and `onboardingGate.dismissed` keeps the redirect from
firing twice inside one visit, exactly as today.

*Alternative rejected:* an `onboarding_version smallint`, so a future wave re-asks everyone.
That is infrastructure for a need that does not exist yet; the column can be added in one
migration when it does.

### The questionnaire is a new table with named columns

```
CREATE TABLE public.candidate_survey (
    user_id                 bigint PRIMARY KEY REFERENCES public.users(id) ON DELETE CASCADE,
    job_search_stage        text,
    biggest_challenge       text,
    biggest_challenge_note  text,
    current_income_amount   integer,
    current_income_currency text,
    current_income_period   text,
    updated_at              timestamptz NOT NULL DEFAULT now()
);
```

Named columns rather than a JSONB blob, following `screening_answers`' stated reasoning: the
field set is fixed and small, so naming each fact validates and reads more simply than a
key-value store, and NULL means "unstated" unambiguously per field. `ON DELETE CASCADE`
satisfies account deletion without a second code path. No extra index — `user_id` is the
primary key and every read is a point lookup by the caller's own id, which is what
`TestEveryUserForeignKeyIsIndexed` checks for.

### Vocabularies validate in Go, not in SQL

`vocab.JobSearchStageValues` and `vocab.JobChallengeValues` join `vocab.SalaryPeriodValues`.
Validation lives in the survey package, matching how `screeninganswers.Validate` checks
`desired_salary_period` — one answer per repository to "where is an enum enforced". A CHECK
constraint would be a second answer, and would make adding a vocabulary member a migration.

`biggest_challenge_note` is accepted only when `biggest_challenge` is the `other` member;
any other pairing is rejected, so a note can never contradict the coded answer.

### Package placement

`internal/candidate/survey` — layer 4. `api` (8) and `engage` (7) may import it, which covers
both the wizard's endpoint and any future use in digests. It must be added to the table in
`internal/platform/arch/layering/blocks.go`; a package in neither list fails the guard.

Note that `screeninganswers` sits in `internal/ingest`, not `internal/candidate` — it was
built for ATS apply-form filling. The wizard reaching both is fine (`api` sees both blocks),
but the split is surprising on first read.

### API surface

| Route | Purpose |
|---|---|
| `GET /api/v1/me/survey` | read the questionnaire; never 404s, reports unstated |
| `PUT /api/v1/me/survey` | partial update; an omitted field keeps its stored value, and there is no clear operation (same contract as the screening answers, for the same reason) |
| `POST /api/v1/me/onboarding/complete` | set `onboarding_completed_at` |

Mirrors `GET/PUT /me/screening-answers` (`internal/api/handler/screening_answers.go`),
including its middleware split: read admits an API key, write is cookie-only.
`onboarding_completed_at` is exposed on the existing `/me` read so the layout's gate has it
without a second request.

Everything else reuses existing routes: `PUT /me/profile` (specializations, skills,
geography), `PUT /me/screening-answers` (desired salary), `PUT /me/resume/contacts` (years,
links).

### The wizard

Eight steps: `cv`, `confirm` (specializations + links), `experience`, `skills`, `location`,
`money`, `stage`, `challenge`. Product-actionable first, segmentation last, so an abandoned
run has answered the questions that change what the user is shown.

Each step persists on leaving it rather than at the end, replacing the current
stage-everything-then-`finish()` model. A step whose store already has an answer is not
presented, which is what keeps an existing account's run down to three or four screens.

The money step carries two sliders — current income and desired salary — over one currency
and one period selector. Each slider yields an exact integer (step 500), not a range:
`screening_answers` stores one number, the `salary_min` filter takes one number, and an ATS
form asks for one number.

At 590 lines the page is already at the edge of what should live in one file; adding four
steps means extracting each step into its own component under
`web/src/lib/components/onboarding/`, with the page keeping only step sequencing and the
save-on-leave dispatch.

## Risks / Trade-offs

- **Every existing account gets interrupted once.** → The run is three or four screens for
  someone who already has a CV, every screen is skippable, and declining ends it permanently.
  This was the user's explicit choice over a passive banner.
- **Per-step saving multiplies the request count and can half-fail.** → Each step writes to
  the store that owns it, so a failed write cannot corrupt another step's data; a failed save
  keeps the user on that step with the error rather than advancing silently.
- **Splitting one wizard's answers across four endpoints is more moving parts than one
  `PUT /me/onboarding`.** → Accepted deliberately: a single endpoint would make the wizard
  the owner of facts it does not own, and the next writer of any of them would have two
  paths to choose between.
- **Host-based link classification can mis-file an unusual URL.** → An unrecognised link is
  retained in `Links` rather than dropped, so mis-classification loses nothing; the LinkedIn
  matcher is the hardened one already in `linkedinprofile`.
- **`users.onboarding_completed_at` starts NULL for everyone, including accounts that
  genuinely finished the old wizard.** → That is the intent: the old wizard asked four
  questions, this one asks eight, and those accounts have five unanswered.
- **The `jobs-onboarding` spec still describes the retired local-filter wizard.** → This
  change touches only the requirements it actually changes; the stale remainder is left alone
  rather than rewritten as a side quest.

## Migration Plan

1. Ship migration `0134_candidate_survey.sql` (new table + new nullable column, both
   additive and non-blocking; `ADD COLUMN` without a default takes no rewrite).
2. Deploy the backend: vocabularies, `internal/candidate/survey`, the two new routes, the
   `Owned.TotalYears` field, the link classifier, `onboarding_completed_at` on `/me`.
3. Deploy the frontend: the new steps and the new gate condition.

Rollback: the frontend reverts independently — the old wizard writes through endpoints that
still exist, and the old gate reads CV presence, which is untouched. The column and table can
stay in place unread.

## Open Questions

None. Scope, storage split, gate behaviour, slider semantics, and single-select challenge
were each decided with the user during brainstorming.
