## 1. Wire shape on Employment

- [x] 1.1 Add kind-aware `MarshalJSON` / `UnmarshalJSON` on `experience.Employment` (`name` for project, `company` for job; write accepts `name` or legacy `company` for projects)
- [x] 1.2 Unit tests for marshal/unmarshal of job vs project, including legacy project `company` input
- [x] 1.3 Align generated or hand-maintained `ExperienceEmployment` in `web/src/lib/types.ts` (and gen-contracts if it must learn about `name`)

## 2. Call sites that re-emit place labels

- [x] 2.1 Assistant experience tool projections: emit `name` for project places, `company` for jobs
- [x] 2.2 Handler tests for list/create project responses using `name`

## 3. Experience UI

- [x] 3.1 `ExperienceBankView` (and any employment edit form) display/edit project label as name, not company
- [x] 3.2 Spot-check labels copy (“role” vs project wording) so projects are not described as companies

## 4. Verify

- [x] 4.1 `go test` for touched packages + `go vet -tags=integration ./...`
