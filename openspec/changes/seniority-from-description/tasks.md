## 1. SeniorityFromDescription (the detector)

- [x] 1.1 Add failing tests in `internal/classify` for `SeniorityFromDescription`: positives per grade (c_level: "head of engineering" via intent anchor, "vp of engineering"; principal: "principal engineer"; staff: "staff engineer"; lead: "lead role", "we are looking for a lead"; senior: "senior-level", "senior position", "we are looking for a senior"; middle: "mid-level"; junior: "entry-level", "junior position"; intern: "internship"); priority (a higher grade wins when two appear); and trap negatives that MUST yield "" ("senior management", "lead the team", "junior colleagues", "our staff", "principal component analysis", "report to the head of product", "5+ years of experience").
- [x] 1.2 Implement `SeniorityFromDescription(desc string) string` in `internal/classify` (new `description.go`): lowercase the input, scan a curated intent-anchored phrase set in priority order c_level > principal > staff > lead > senior > middle > junior > intern, return "" on no match. Canonical values are `enrich.SeniorityValues`.
- [x] 1.3 Run `go test ./internal/classify/` green.

## 2. Wire into jobderive (description as the lower-priority seniority source)

- [x] 2.1 Add a failing test in `internal/jobderive`: description fills `seniority` when the title is silent; the title grade beats a description signal; a noisy description with no anchored phrase yields empty; `category` is unaffected.
- [x] 2.2 In `jobderive.Derive`, capture `class := classify.Parse(in.Title)`, then `seniority := class.Seniority; if seniority == "" { seniority = classify.SeniorityFromDescription(in.Description) }`, and return that seniority (category unchanged).
- [x] 2.3 Run `go test ./internal/jobderive/` green.

## 3. Verify

- [x] 3.1 `go build ./... && go vet ./... && go test ./...` green; `gofmt -l` clean on changed files; confirm no other package regressed.
