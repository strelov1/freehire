## 1. Backend — widen résumé extraction (dictionaries)

- [x] 1.1 Add a `resumeHeadline(text)` helper that collapses whitespace, drops contact/metadata tokens (email/phone/URL/bare-number/punctuation), and returns a bounded leading window (title + summary top) so the title dictionaries reach the title but not the career history.
- [x] 1.2 Add `classify.Categories(text) → []string` (every category the text names, distinct, precedence order) alongside `Parse`.
- [x] 1.3 Add a `resumeProfile(text) → {skills, seniority, categories}` helper composing `skilltag.Parse` (skills, whole text) and `classify` over the headline (seniority + categories). No LLM; `skills`/`categories` never nil.
- [x] 1.4 Refactor the handler (renamed `ExtractResumeSkills` → `ExtractResumeProfile`, `internal/handler/resume.go`) to call the helper and return `{"data": {skills, categories, seniority?}}` (omit empty seniority); keep the résumé store + `embedResume` side effects and cookie-auth unchanged.
- [x] 1.5 Unit tests (Go): `resumeProfile` resolves seniority/categories/skills from sample text incl. a contact-first layout and a one-token-per-line PDF shape; grade words in the career history don't over-promote; multiple categories are all surfaced; `classify.Categories` dedup + precedence; handler still 401s without auth and 400s on empty input.

## 2. Frontend API + types

- [x] 2.1 `web/src/lib/api.ts`: rename `extractResumeSkills` → `extractResumeProfile` returning `ResumeProfile { skills: string[]; categories: string[]; seniority?: string }`; add the type in `types.ts`.

## 3. Onboarding wizard — CV path (`OnboardingWizard.svelte`)

- [x] 3.1 Add a "Upload CV — autofill" affordance (step 1) alongside the manual pickers, plus `idle`/`parsing`/`error` state and a result note.
- [x] 3.2 On choose: if `!isAuthenticated()` → `openAuthDialog()` (upload only once signed in — no file-across-redirect handoff); else open the file picker.
- [x] 3.3 Make Focus and Seniority multi-select (`specializations[]`/`seniorities[]` on `OnboardingSelection`; shared `toggleIn`/`multiPills`); `selectionsToQuery` maps the arrays to the category/seniority facets.
- [x] 3.4 On file: call `extractResumeProfile`, merge `categories`/`seniority`/`skills` into `sel` (dedup, keep manual picks), stay on the current step for review, and set a note; a generation guard drops a stale parse; error/empty → retryable note, manual path intact.

## 4. Profile-form bonus (`ProfileForm.svelte`)

- [x] 4.1 Merge the returned `categories` into `specializations` (respecting the cap), mirroring the existing skills merge, with an accurate added/capped/nothing note.

## 5. Verify

- [x] 5.1 `go build ./... && go vet ./...` and `go test ./...` green; svelte-check and vitest green; lint clean on changed files.
- [x] 5.2 Visual verification (throwaway route + headless Chrome): the wizard CV affordance; the parsing/error/note states; a signed-in upload pre-fills multiple focus + seniority + stack and stays on step 1 for review; the manual path unaffected.
- [x] 5.3 Confirm the profile page still fills skills and now fills specializations from a résumé; confirm existing `{skills}` behavior is unchanged (additive response).
