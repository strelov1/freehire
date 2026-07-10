## Why

The onboarding wizard turns a fresh `/jobs` visit into a personalized feed, but it still makes the visitor pick their focus, seniority, and stack by hand. Most of that is already written on their résumé. We can pre-fill it: upload a CV once and the wizard's focus/seniority/stack come out filled, ready to review. And we can do it the way every other freehire facet is derived — with our existing deterministic dictionaries, not an LLM.

## What Changes

- Refactor `POST /api/v1/me/resume/extract` to return `{skills, categories, seniority?}` instead of just `{skills}`, deriving the new fields from **existing deterministic dictionaries only**:
  - `skills` — `skilltag.Parse` over the whole résumé (unchanged).
  - `seniority` + `categories` — `classify` over the résumé *headline* (leading title + summary, with contact/number/punctuation tokens dropped and whitespace collapsed, so a one-token-per-line PDF still reaches the title and the career history below can't over-promote the grade). A new `classify.Categories` returns **every** category the headline names (distinct, precedence order) — a person can be several.
  - Best-effort: `skills`/`categories` are always arrays (empty when unresolved), `seniority` is omitted when unresolved (never guessed). No LLM, so the endpoint stays instant and unconditional on `LLM_*`. The existing résumé store + background embed side effects and cookie-auth are unchanged.
- Add a **CV-upload path to the onboarding wizard** (`/jobs`): a signed-in user uploads a résumé and the wizard's focus (categories) and seniority — both now **multi-select** — plus stack (skills) are pre-filled, and the wizard **stays on the current step** so the user reviews the pills (a short note reports what filled). Anonymous visitors are prompted to sign in first (the résumé endpoint is cookie-only); on error the wizard falls back to manual entry.
- **Bonus:** `ProfileForm` on `/my/profile` pre-fills `specializations` from the returned `categories` (it already merges the returned `skills`), respecting the specialization cap, so a CV upload there populates the whole profile.

## Capabilities

### New Capabilities
- `cv-autofill`: Derive a résumé's seniority, categories, and skills via the existing deterministic dictionaries and pre-fill them into the onboarding wizard (and the profile form), so a signed-in visitor can configure their feed from a CV upload instead of by hand.

### Modified Capabilities
<!-- None: this refactors the resume-extract response additively and reuses resume-storage / deterministic-facets without changing their requirements. -->

## Impact

- **Backend** (`internal/`): refactor `internal/handler/resume.go` `ExtractResumeProfile` (renamed from `ExtractResumeSkills`) to run `classify` over a `resumeHeadline` helper and return the extra fields; add `classify.Categories` for the multi-category set. Reuses `internal/skilltag`, `internal/classify`, `enrich` vocabularies, and the existing résumé store + embed.
- **Frontend** (`web/`): `OnboardingWizard.svelte` gains the CV-upload affordance + parsing/error states, makes Focus and Seniority multi-select, and applies the result to `sel`; `web/src/lib/api.ts` `extractResumeProfile` (renamed) returns the widened shape; `ProfileForm.svelte` merges `categories` into specializations; auth gate via `openAuthDialog`.
- **No DB migration, no new endpoint, no LLM dependency.** The `extract` response is widened additively (existing `skills` consumers unaffected).
