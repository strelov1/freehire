## 1. Anonymous CV parse (backend)

- [x] 1.1 Handler test: anonymous `POST /api/v1/onboarding/cv` with a PDF returns first name, headline, location/country, total years, ≤4 companies, skills, categories, seniority and an ATS report; nothing persisted (no resume store / S3 call)
- [x] 1.2 Handler test: model failure or timeout degrades to dictionary fields with `structured_available: false`; non-PDF is a 4xx; contact fields (email, phone, links, surname) never appear in the answer
- [x] 1.3 Handler test: a guest call uses the service credential (key resolver never read); a signed-in call goes through the binding; both run under `plan.WithFreeAI`
- [x] 1.4 Implement the route (`mw.optional`, hand-written Fiber, 45 s extraction deadline, `tagCVOnboard` feature tag), register it, add it to `openRoutes` in `paid_ai_test.go`
- [x] 1.5 Document the fourth free-AI mark: `WithFreeAI` doc comment, `requirePaidCall` comment, `internal/ai/plan/AGENTS.md`, root `AGENTS.md` paid-gate bullet

## 2. Anonymous match search (backend)

- [x] 2.1 Handler test: `POST /api/v1/onboarding/matches` with skills + query ranks by skill match, returns `match_percent` per job and the total, limit 10, reports unknown params in `meta.ignored_params`, caps skills at 50
- [x] 2.2 Add the operation to TypeSpec (`typespec/onboarding.tsp`), run `make api-contract`, implement it on the shared search core with an explicit skill vector

## 3. Gateway budget (freehire-ops)

- [x] 3.1 Add the `onboarding-cv-guest` budget (guest_ip_only, 5 / 86400 s, auth_mode optional so signed-in callers are not counted) and the route binding to `policy.json`; run the policy validator/tests in that repo (247 passed); branch `feat/onboarding-cv-budget` in a worktree, not yet committed
- [ ] 3.2 Deploy the policy to host2 and confirm nginx reload is clean

## 4. Web: flow logic

- [x] 4.1 Unit tests + implement `onboardingFlow.ts`: phase transitions, role choices from categories × seniority, where choices (country only when resolved), answers → search query params, answers + parse → profile, the 7→30-day widening rule
- [x] 4.2 Unit tests + implement `onboardingDraft.ts` (IndexedDB: save/load/clear, 24 h expiry, missing-IndexedDB fallback)
- [x] 4.3 API client methods + wire types for the two new routes (`api-methods`), with a 429 mapped to a "daily limit" error

## 5. Web: shared sign-up form

- [x] 5.1 Sign-up from the results goes to `/signin` (returnTo `/onboarding`) after marking the draft; the embedded form extracted at first was dropped and `/signin` restored (register subtitle updated)

## 6. Web: the page

- [x] 6.1 Rewrite `/onboarding/+page.svelte`: drop → parsing → profile summary → three questions → searching (catalogue snapshot, unmeasured figures omitted) → compact results (logo stack, top 5 one-line matches, feed link, ATS scorecard) → one closing row (start over, Save my matches → /signin; signed-in save)
- [x] 6.2 Carry-after-auth: on authenticated + draft, run CV upload, profile save, email alert, complete onboarding independently; report failed parts; clear draft; go to the feed
- [x] 6.3 Component specs (`*.spec.ts`): anonymous visitor sees the drop step (no redirect); results show open job cards; 429 shows the limit message; carry runs all steps and reports a failing one
- [x] 6.4 Layout gate: remove the anonymous redirect and the completed-account reverse bounce; keep routing incomplete accounts once; update `routes/layout.test.ts`

## 7. Removal and cleanup

- [x] 7.1 Delete the old step components, `onboardingSteps.ts`, `onboardingSave.ts`, `onboardingImport.ts` and their tests; remove anything they alone used (knip/`check:dead` clean)
- [x] 7.2 Update `.size-limit.mjs` budget for `/onboarding` to the measured size; run `pnpm run size`

## 8. Verification

- [x] 8.1 `gofmt -l .`, `go vet ./...`, `go vet -tags=integration ./...`, `go test ./...`; web `pnpm check`, lint, format:check, vitest
- [x] 8.2 Drive the flow in a real browser: anonymous drop → answers → Save my matches → /signin email registration (local Go server + own DB + MinIO; catalogue via prod) → carried: CV stored in S3, profile, email alert on the profile search, survey challenges, onboarding complete, landed on the profile feed. Found and fixed a silent draft-save failure (a $state proxy in the answers is refused by IndexedDB)
