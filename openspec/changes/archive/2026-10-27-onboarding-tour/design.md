## Context

The approved product design is `docs/superpowers/specs/2026-10-26-onboarding-tour-design.md`,
signed off step by step against a throwaway prototype. This document records the technical
decisions.

## Goals / Non-Goals

**Goals:** a seven-step tour built from existing stores and components; a multi-answer
challenge question; existing accounts asked once more for only what they lack; a labelled
Sign in for signed-out visitors.

**Non-Goals:** the visa-sponsorship landing page (the tip links to `/collections/visa-sponsors`
until it exists); dropping `candidate_survey.biggest_challenge` (left in place, no longer
written — a later migration can drop it); any change to the screening-answer or contacts stores.

## Decisions

- **The challenge becomes a list: `candidate_survey.biggest_challenges text[]`.** A new
  nullable column, backfilled in the same migration from `biggest_challenge`
  (`ARRAY[biggest_challenge]` where set; ~10.5k rows, so no batching). Go reads and writes
  only the new column; the old one is superseded and documented as such. Validation stays
  in Go (membership in `vocab.JobChallengeValues`, no duplicates, `just_starting` alone,
  the note only when `other` is in the list), matching 0134's "enums are enforced in code".
  The API field is renamed to `biggest_challenges`; only the web wizard reads the survey
  (the mobile app does not), so there is no second client to keep compatible.
  Rejected: keeping the single column with the "biggest" ranking — the user chose several.
- **Vocabulary**: add `job_fit`, `remote_flexible`, `visa_sponsorship`, `first_job`,
  `just_starting`. Existing values keep their identifiers and get new labels in
  `surveyOptions.ts`; note that `english` is now worded "Language requirements", so it means
  more than English for answers given after this change.
- **Re-asking existing accounts**: a data migration sets `users.onboarding_completed_at =
  NULL`. The column was documented in 0134 as "the seam is one more migration" for exactly
  this. The web planner (`plannedSteps`) then decides per account: data steps already
  answered are skipped, intro steps are shown only if a data step is, an empty plan marks
  complete silently (the path the current page already has).
- **Complete on show.** The page calls `completeOnboarding()` once the plan is known and
  non-empty, not on leaving. `onboardingGate.dismiss()` still covers the instant before the
  write lands.
- **Pure step logic in `onboardingSteps.ts`** (plan, specialization order, the answer →
  tips table), unit-tested in Node.
- **`persistStep`**: only `feed` (profile) and `challenge` (survey) write.
- **Technical-first ordering** reads `TECH_CATEGORY_VALUES` generated from
  `vocab.TechCategories`; grouping by `CATEGORY_GROUPS` was rejected because "Engineering"
  holds industrial engineering and the lab sciences.
- **Extracted, shared previews** (`McpSetup`, `TailorWorkspacePreview`,
  `ExtensionPanelPreview`, `JobAlertDigestPreview`) so a landing page and the tour cannot
  drift. Each landing renders exactly what it did before.
- **Alert channels**: email reuses `ProfileAlertToggle`'s create-and-subscribe logic, moved
  into a small shared helper both call; Telegram uses `notifications.link()` then subscribes
  the profile search; push links to the App Store.
- **Match count**: one debounced `searchJobs(params, 1, 0)` over `filtersFromProfile`,
  aborted on change; a failure hides the count.

## Risks / Trade-offs

- [Every existing account sees a screen once more] → only those missing something; the
  rest are completed silently on their next visit.
- [`english` now covers other languages] → historical counts before the deploy date mean
  English only; recorded here and in the survey options file.
- [A labelled Sign in crowds a 360px header] → verify at 360/390; `sm` button.

## Migration Plan

One migration, run by `release.sh` before the flip: add `biggest_challenges`, backfill it,
null `onboarding_completed_at`. The users update touches every completed account in one
statement; run outside the nightly dump window. Rollback: revert the code; the backfill is
additive and the nulled marker only re-shows a skippable tour.
