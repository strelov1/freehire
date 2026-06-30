## 1. Structured facet seam

- [ ] 1.1 Add `Seniority`, `Category`, `Skills`, `ExperienceYearsMin` fields to `sources.Job` (`internal/sources/source.go`) with a docstring matching `WorkMode`'s "structured signal only, never a heuristic" contract.
- [ ] 1.2 Add the same fields to `jobderive.Input` (`internal/jobderive/jobderive.go`).
- [ ] 1.3 In `jobderive.Derive`, apply precedence: `seniority` source→title→description; `category` source→title; `experience_years_min` source→`jobfacts`; `skills` = union(source, dictionary).
- [ ] 1.4 In `internal/pipeline/pipeline.go`, pass the new `Job` fields into `jobderive.Input` (beside `WorkMode: j.WorkMode`).
- [ ] 1.5 Unit-test `jobderive`: source beats dict (seniority/category/experience), dict fills when source silent, skills union, multi-tier seniority order.

## 2. getmatch mapping

- [ ] 2.1 Extend `getmatchOffer` to decode `seniority`, `specializations`, `skills_objects`, `required_years_of_experience` from the detail response.
- [ ] 2.2 Map the scalar grade to `enrich.SeniorityValues` (explicit map; unknown dropped).
- [ ] 2.3 Canonicalize `skills_objects` names via `skilltag.Parse` (drop noise).
- [ ] 2.4 Map `specializations` codes to `enrich.CategoryValues` via an explicit subset map; resolve to a single category or empty on conflict (mirror `getmatchWorkMode`).
- [ ] 2.5 Set `required_years_of_experience` into `ExperienceYearsMin` (nil when absent).
- [ ] 2.6 Populate the structured `Job` fields in `toJob` from the detail response (reuse the already-fetched detail; avoid a second request).
- [ ] 2.7 Unit-test the getmatch mapping with detail fixtures: grade→seniority, skills canonicalized + noise dropped, specialization map (mapped/unmappable/conflict), experience set/absent.

## 3. Verification & rollout

- [ ] 3.1 `go build ./... && go vet ./... && go test ./...` green.
- [ ] 3.2 Live-smoke the getmatch adapter against the real API; confirm a sample of offers now carry structured seniority/skills/category/experience.
- [ ] 3.3 Document the rollout note (next getmatch crawl re-ingests; run `make reindex` to surface facets) in the change/PR description.
