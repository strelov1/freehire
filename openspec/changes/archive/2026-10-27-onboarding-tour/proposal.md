## Why

A new user told us they could not find how to sign in and that the site would not let them see a job until they had uploaded a CV and answered questions. The account wizard asks eight questions, three of which feed nothing, routes an unfinished account back to itself on every visit, and never turns on the alert the answers describe. The one survey question worth keeping ("what is hardest") has 10,554 answers on prod, and its free-text "something else" answers name needs the closed list does not offer (jobs that fit, remote work, visa sponsorship, a first job).

## What Changes

- Replace the eight-step `/onboarding` wizard with a seven-step tour: CV, feed (specialization, level, skills, location, live match count), alert channels (email, Telegram, push app) with an email preview, AI-assistant setup over MCP, the challenge question (several answers, a "What helps" panel), CV tailoring (Pro) and the browser extension (form filling is Pro).
- **BREAKING (API)**: the survey's single `biggest_challenge` becomes the list `biggest_challenges`; the challenge vocabulary gains `job_fit`, `remote_flexible`, `visa_sponsorship`, `first_job`, `just_starting`. Migration copies every stored answer into the list. Only the web client reads the survey.
- Stop asking for profile links, years of experience, desired salary, current income and search stage during onboarding. Their stores and other editors are unchanged.
- Existing accounts are routed through the tour once more; a step whose answer is already stored is skipped and pre-filled, and an account missing nothing is marked complete without seeing a screen.
- The tour marks the account complete when it is shown, so the redirect fires at most once.
- Shared components extracted rather than duplicated: `McpSetup` (from `/mcp`), `TailorWorkspacePreview`, `ExtensionPanelPreview`, `JobAlertDigestPreview`.
- `RemoteSearchSelect`: optional close-on-pick and a clear button. `TECH_CATEGORY_VALUES` generated from `vocab.TechCategories`.
- A labelled "Sign in" button in the header for signed-out visitors at every width.

## Capabilities

### New Capabilities
- `onboarding-tour`: the signed-in account's first-run tour — its steps, what each saves, and when it is shown.

### Modified Capabilities
- `header-navigation`: a signed-out visitor sees a labelled Sign in control in the header on every viewport.

## Impact

- Go: `internal/dict/vocab` (challenge values), `internal/candidate/survey` (list field), `queries/candidate_survey.sql`, new migration, `cmd/gen-contracts`.
- Web: `routes/onboarding`, `lib/onboardingSteps.ts`, `lib/onboardingSave.ts`, `lib/surveyOptions.ts`, `lib/components/onboarding/*`, the four extracted previews, `RemoteSearchSelect`, `HeaderProfileMenu`, `McpView`, `TailorLandingView`, `ExtensionLandingView`, `NotificationsLandingView`.
- Data: every account sees the tour at most once more; nothing is deleted.
