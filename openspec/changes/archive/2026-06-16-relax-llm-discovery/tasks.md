## 1. Partition Sanitize/Validate (TDD)

- [x] 1.1 Add failing tests in `internal/enrich`: an out-of-vocabulary `work_mode`/`seniority`/`category` and a non-vocab `regions` element SURVIVE `Sanitize` and PASS `Validate` (captured raw); an out-of-vocabulary served field (`employment_type`, `english_level`, `company_type`, `domains`) is still blanked/filtered by `Sanitize` and still fails `Validate`; salary clamping unchanged.
- [x] 1.2 In `enrichment.go`, remove `work_mode`/`seniority`/`category` from the `Sanitize` and `Validate` scalar lists and `regions` from their multi-value lists, keeping the served scalars + `domains` + salary clamping. (`countries`/`skills` are already non-enum.)
- [x] 1.3 Run `go test ./internal/enrich/` green.

## 2. Loosen the prompt for the discovery facets (TDD)

- [x] 2.1 Add a failing test asserting the built system prompt (a) permits a novel/own label for the discovery facets (work_mode, regions, seniority, category) and (b) still instructs "exactly one of the allowed values" for the served enum fields.
- [x] 2.2 In `langchain.go`, split the enum instruction: a relaxed line for the discovery facets (prefer allowed, else a concise lowercase label) and the strict line for the served fields. Keep listing allowed values for all as guidance.
- [x] 2.3 Run `go test ./internal/enrich/` green.

## 3. Verify

- [x] 3.1 `go build ./... && go vet ./... && go test ./...` green; `gofmt -l` clean; confirm no other package regressed. Do NOT bump enrich.Version.
