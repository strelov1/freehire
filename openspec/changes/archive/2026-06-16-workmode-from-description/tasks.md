## 1. WorkModeFromDescription (the detector)

- [x] 1.1 Add failing tests in `internal/location` for `WorkModeFromDescription`: positives for each mode (remote: "fully remote", "remote-first", "work from anywhere"; hybrid: "hybrid role", "X days in the office"; onsite: "on-site only", "must be on-site"); priority hybrid > remote > onsite when both present; and trap negatives that MUST yield "" ("distributed systems", "hybrid cloud", "remote server/team", a plain description with no arrangement phrase).
- [x] 1.2 Implement `WorkModeFromDescription(desc string) string` in `internal/location`: lowercase the input, scan a curated, conservative phrase set (separate from `workModeMarkers`) in priority order hybrid > remote > onsite, return "" on no match. Draw modes from `enrich.WorkModeValues`.
- [x] 1.3 Run `go test ./internal/location/` green.

## 2. Wire into jobderive (description as the lowest-priority source)

- [x] 2.1 Add a failing test in `internal/jobderive`: description fills `work_mode` when the location is silent; the location marker beats a description signal; a structured `WorkMode` input beats both; a noisy description with no phrase yields empty.
- [x] 2.2 In `jobderive.Derive`, after the existing structured→location resolution, add the description fallback: `if workMode == "" { workMode = location.WorkModeFromDescription(in.Description) }`.
- [x] 2.3 Run `go test ./internal/jobderive/` green.

## 3. Verify

- [x] 3.1 `go build ./... && go vet ./... && go test ./...` green; `gofmt -l` clean on changed files; confirm no other package regressed.
