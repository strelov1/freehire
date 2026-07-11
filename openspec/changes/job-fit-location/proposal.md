## Why

The user profile now carries **location & work-mode preferences** (#554: `work_modes`, remote
reach, current base, relocation willingness), but the on-demand job-fit analysis ignores them —
it scores title/experience/seniority/skills/company only. Geographic and work-mode fit is exactly
what a recruiter and an ATS weigh when deciding whether a candidate can actually take a role
(onsite-in-Berlin vs remote-only-in-Brazil), so it must participate in the fit verdict. The result
should also be presented in fuller detail than the compact bar.

## What Changes

- Add a sixth scored dimension **Location & work-mode fit** to the fit analysis, fed by the job's
  geography (`work_mode`/`remote`/`regions`/`countries`/`location`) and the caller's profile
  `location_preferences`. Rebalance the weighted `overall_score` so the six dimensions sum to 100
  (Title 20 / Experience 25 / Seniority 15 / Skills 15 / Company 10 / Location 15) — title and
  experience stay the heaviest.
- Feed `location_preferences` into the prompt-chain so the recruiter/audit stages reason about
  geographic and work-mode fit and surface location gaps in `gaps`/`recommendation`.
- Render the fit result in **fuller detail** in the SPA: every dimension's rationale visible, the
  new location dimension included, a clearer visual hierarchy.
- **Fix the migration-number collision**: rename the profile-location migration
  `0009_user_profile_location.sql` → `0010_user_profile_location.sql` (the job-fit
  `0009_user_job_analysis.sql` merged first and is already applied to prod).

## Capabilities

### Modified Capabilities
- `job-fit-analysis`: the scored verdict gains a sixth Location & work-mode fit dimension over the
  profile's location preferences, the overall weighting is rebalanced, and the SPA presents the
  result in fuller detail.

## Impact

- `internal/jobfit` (new dimension, weights, prompt), `internal/handler/job_fit.go` (pass
  `location_preferences` + job geography into the input), `cmd/gen-contracts` regen,
  `web/src/lib/components/JobFitAnalysis.svelte` (richer render).
- `migrations/0009_user_profile_location.sql` → `0010_` (rename only; the SQL is unchanged).
- No new config. The location dimension degrades gracefully when the profile set no preferences
  (the model scores on the job geography alone / neutral).
