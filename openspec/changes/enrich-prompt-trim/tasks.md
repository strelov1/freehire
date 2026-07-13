# Tasks

## 1. Prompt trim: stop requesting the nine dict-backed facets

- [x] 1.1 **RED** — Rewrite `TestSystemPromptRelaxesDiscoveryFacets` in
  `internal/enrich/langchain_test.go` to encode the new contract: the built
  system prompt MUST NOT request `work_mode`, `seniority`, `category`, `skills`,
  `employment_type`, `education_level`, `english_level`, `posting_language`, or
  `experience_years_min`; it MUST still request `countries`/`regions` (with the
  "concise lowercase label of your own" allowance) and the retained served keys
  (`summary`, `salary_min`/`salary_max`/`salary_currency`/`salary_period`,
  `visa_sponsorship`, `timezone_note`, `company_type`, `company_size`, `domains`,
  `relocation`). Assert both directions (removed absent AND retained present).
  Confirm it fails against the current prompt.
- [x] 1.2 **GREEN** — Edit `buildSystemPrompt` in `internal/enrich/langchain.go`:
  remove the `enum(...)` lines for `work_mode`, `seniority`, `category`,
  `employment_type`, `education_level`, `english_level`; drop `skills`,
  `posting_language`, and `experience_years_min` from the "Other keys" line;
  narrow the discovery-facet exception paragraph so its novel-label allowance
  names only `countries`/`regions`. Keep `relocation`, `salary_period`,
  `company_type`, `company_size`, `domains` enums and the geographic guidance
  intact.
- [x] 1.3 Sweep the `internal/enrich` tests for any other assertion tied to a
  removed field being present in the prompt (e.g. `enrichment_test.go`,
  `provider_test.go`); update or remove them to match the new prompt. Do NOT
  touch `Validate`/`Sanitize` behavior or their tests — those stay unchanged.
- [x] 1.4 **Verify green** — `go test ./internal/enrich/...`, then
  `go build ./... && go vet ./...`. All pass.
