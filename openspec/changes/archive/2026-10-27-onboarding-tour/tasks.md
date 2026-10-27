## 1. Backend

- [x] 1.1 Generate `TECH_CATEGORY_VALUES` from `vocab.TechCategories` in `cmd/gen-contracts`
- [x] 1.2 Vocabulary: add the five challenge values; survey domain takes `BiggestChallenges []string` with its validation rules — tests first
- [x] 1.3 Migration: `biggest_challenges text[]` + backfill, `onboarding_completed_at` reset; queries, sqlc, repository; regenerate contracts and API docs

## 2. Shared web pieces

- [x] 2.1 `onboardingSteps.ts`: step order, specialization order, answer → tips (tests)
- [x] 2.2 `onboardingSave.ts`: feed → profile, challenge → survey (tests)
- [x] 2.3 `plannedSteps` for returning accounts (skip answered data steps, intro only with a data step) — tests
- [x] 2.4 `RemoteSearchSelect` close-on-pick and clear button
- [x] 2.5 Extract `McpSetup`, `TailorWorkspacePreview`, `ExtensionPanelPreview`, `JobAlertDigestPreview`; landings unchanged
- [x] 2.6 Shared profile-alert enable helper used by `ProfileAlertToggle` and the tour

## 3. Tour steps and page

- [x] 3.1 Step components: Cv, Feed, AlertChannels, Agent, Challenge, Tailor, Extension
- [x] 3.2 Rewrite `routes/onboarding/+page.svelte`: plan, pre-fill, complete on show, live count, `cta` Continue
- [x] 3.3 Remove step components and helpers nothing imports any more

## 4. Header

- [x] 4.1 Labelled "Sign in" for signed-out visitors at every width

## 5. Verify

- [x] 5.1 `go test`, `go vet -tags=integration`, `pnpm check`, unit tests, eslint
- [x] 5.2 Visual pass at 390 / 1280, light and dark, signed in through the whole tour
