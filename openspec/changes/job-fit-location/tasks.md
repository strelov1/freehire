## 1. Migration collision fix

- [x] 1.1 `git mv migrations/0009_user_profile_location.sql migrations/0010_user_profile_location.sql` (SQL unchanged); note it is still unapplied on prod (applies on #554's next deploy)

## 2. Backend — sixth Location dimension

- [x] 2.1 RED: extend `internal/jobfit` tests — six dimensions, rebalanced weights (Title 20 / Exp 25 / Seniority 15 / Skills 15 / Company 10 / Location 15) sum to 100, `verdictFor` unchanged, `sanitizeVerdict` clamps the new `location_fit`
- [x] 2.2 GREEN: add `DimLocationFit` + spec/weight to `dimensionSpecs`, add `LocationFit dimScore` to `recruiterVerdict`, wire it through `buildAnalysis`/`sanitizeVerdict`
- [x] 2.3 RED: test the prompt carries job geography + profile location preferences (Stage 2 scores location_fit)
- [x] 2.4 GREEN: add `JobGeo` + `LocationPreferences` (raw JSON string) to `jobfit.Input`; extend the Stage 2 (and audit) prompt with the geography + preferences and the location scoring key
- [x] 2.5 Handler: pass `job` geography (work_mode/remote/regions/countries/location) and the profile's `location_preferences` JSON into `jobfit.Input` from `PostJobFit`

## 3. Contract & frontend (detailed render)

- [x] 3.1 Regenerate the TS contract (`make gen-contracts`) — the `Dimension`/`Analysis` shape is unchanged (the sixth dimension rides the existing array), confirm no drift
- [x] 3.2 Enhance `web/src/lib/components/JobFitAnalysis.svelte`: always show each dimension's rationale, ensure the location dimension renders, tighten the visual hierarchy for a fuller detailed read
- [x] 3.3 Extend `web/src/lib/jobFit.test.ts` if any pure logic is added (e.g. a location tone/label helper)

## 4. Verify & finish

- [x] 4.1 `go build ./... && go vet ./... && go test ./...`; integration `-tags=integration` for the fit endpoints
- [x] 4.2 Frontend: `svelte-check` + `vitest` + eslint + prod build
- [x] 4.3 Update `AGENT.md` job-fit convention (sixth Location dimension, weights) and note the migration rename
