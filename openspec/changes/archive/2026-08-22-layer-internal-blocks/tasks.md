## 1. The layering checker

Built first, and testable on its own before the repo has any blocks: the checker takes a
graph and a table and returns violations, so it can be driven red-green against fixtures
rather than against the real 144-package tree.

- [x] 1.1 Write `internal/arch/layering` — the block table (package → block) and the layer table (block → layer 1..8), as data, in one file. It is the single source both the checker and the generated `depguard` rules read.
- [x] 1.2 Write the checker: takes a `map[pkg][]importedPkg` plus the two tables, returns a list of violations. Drive it with fixture graphs covering all six spec scenarios — unassigned package, unknown block, upward import, same-layer cross-block import, downward import, intra-block import.
- [x] 1.3 Add the real-graph entry point (reads `go list -f` output including `TestImports` and `XTestImports`). Assert only that it parses and produces a violation list; do NOT yet assert the list is empty.

## 2. Prerequisite extractions

Done on the flat layout, before any directory moves, so each is a small reviewable diff.
Each removes a block cycle that would otherwise make the layering rule unstateable.

- [x] 2.1 ~~Move the `llm.Settings` conversion out of `internal/config`.~~ **Withdrawn — no code change needed.** `internal/llm` and `internal/llmschema` were misclassified as `ai`. Neither imports anything of ours (`llm` imports only `llmschema`) and neither knows the domain: one wraps an OpenAI-compatible endpoint, the other derives a JSON Schema from a Go type. They belong in `platform`, alongside `safehttp` and `blobstore`, which makes `config` → `llm` an intra-block import. The alternatives were to scatter the conversion across eight `cmd/` entrypoints — regressing the property `internal/config/llm.go:72-74` exists to hold — or to add a package containing one function.
- [x] 2.2 ~~Extract the provider vocabulary from `internal/sources` into `internal/provider`.~~ **Withdrawn — not extractable, and not needed.** `Taxonomy()` is literally `All(nil)`: the crawl registry built from every adapter constructor, with a nil transport. It cannot be separated from the adapter set. The edge existed because `catalogstats` was in the wrong block — it imports nothing from `job` at all (only `cache`, `db`, `testdb` from platform and `sources` from ingest), and its two headline figures are facts about the adapter registry. Reclassified to `ingest`.
- [x] 2.3 Move `sources.SanitizeHTML` into `internal/htmltext`, with its tests. Repoint the callers found in 2.2.
- [x] 2.4 Extract the silence model from `internal/userjob` into a new `internal/silence`: `DaysSilent`, `SilenceSilent`, `SilenceStateFor`, `SilenceThresholdDays`, `ValidateAppliedOn`. Move their tests. Repoint `internal/ghost` and `internal/ghostreport`. Verify neither imports `internal/userjob` any more.
- [x] 2.6 Drop `normalize.JobSlug` from `internal/db/jobs_slug_integration_test.go:25,86`. It is an in-package (`package db`) test, so this is a real `db` → `normalize` edge: platform (layer 1) → dict (layer 2). Use a literal slug — a db test that recomputes the slug with the production function cannot catch a slug-format change anyway.
- [x] 2.7 Drop `sources.NamespaceExternalID` and `sources.BoardIDPattern` from `internal/db/jobs_existing_ids_integration_test.go:79,101,188`. Also `package db`, so it is a real platform (1) → ingest (7) edge — the worst one in the repo. Note this is NOT fixed by task 2.2: 2.2 extracts `Taxonomy`/`AggregatorProviders`/`BoardKeyedProviders`, which these tests do not use.
- [x] 2.5 Confirm the extraction plan is complete: `TestPostMoveGraphHasOnlyThePlannedViolations` must be green with `plannedViolations` empty. Strike each entry off as its task lands — the test fails both when a new upward edge appears and when a fixed edge is left on the list.

## 3. The move

- [x] 3.1 Write the move script: read `go list` for the true package list, `git mv` each into `internal/<block>/<pkg>` per the 1.1 table, and rewrite import paths by exact package-path match. A textual find/replace is not acceptable — package names are substrings of one another (`job`, `jobview`, `jobhash`).
- [x] 3.2 Run the move. `provider` → `dict/provider`, `silence` → `job/silence`, `submission` and `moderation` → `ingest`, plus the five other non-obvious placements from the design.
- [x] 3.3 Flip the 1.3 assertion on: the real-graph violation list MUST be empty. This is the task that proves the move.
- [x] 3.4 `gofmt -w` every touched path; `gofmt -l .` must print nothing. `go build ./...` and `go vet ./...` clean.

## 4. Paths that are strings, not imports

These compile and pass whether or not their path is right. For each: repoint it, then break
it deliberately and confirm it fails, then restore.

- [x] 4.1 `internal/llmkey/scope_test.go:29-32` — the map of background entrypoints (`enrich`, `telegram`, `mailclassify`, `embed`) to `../../internal/<pkg>`. This is the guard that background work never spends a user's LLM credit.
- [x] 4.2 `internal/normalize/legal_form_rule_test.go:16` — `canonicalFormList`, the guard keeping one legal-form vocabulary in the module.
- [x] 4.3 `internal/pgerr/pgerr_test.go:117` — the `"internal/pgerr/"` path check.
- [x] 4.4 `cmd/gen-cities/main.go:44` — `outputPath = "internal/location/cities1000.tsv"`.
- [x] 4.5 `.github/workflows/perf.yml:60` — the change filter hardcoding `internal/handler/`, `internal/search/`, `internal/jobview/`. A stale filter silently stops running the perf job.
- [x] 4.6 `sqlc.yaml:5,9` — `queries:` and `out:`. Run `make sqlc` and confirm it regenerates in place with no diff beyond the path.
- [x] 4.7 Sweep for any remaining `"internal/` string literal or `./internal/<pkg>` in a comment; update the `go test -tags=integration ./internal/<pkg>/` header comments.

## 5. Enforcement in CI

- [x] 5.1 Generate the `depguard` rules from the 1.1 layer table into `.golangci.yml` — one rule per block, denying every block at or above its layer. Set `run.build-tags: [integration, llmlive]` in the same file; without it `golangci-lint` never parses the 222 tagged files and the second guard is blind where the first one was.
- [x] 5.2 Prove both guards bite: add a deliberate upward import, confirm `golangci-lint run` fails on the line AND the layering test fails naming the pair. Revert.
- [x] 5.3 `golangci-lint run` clean on the branch. If `--new-from-merge-base` surfaces pre-existing findings on the moved lines, fix them here.

## 6. Documentation

- [x] 6.1 An `AGENTS.md` per block: what the block is, what it may import, what it must not.
- [x] 6.2 Rewrite the 202 `internal/<pkg>/AGENTS.md` links across `CLAUDE.md` and `docs/`. Restructure the root module table by block.
- [x] 6.3 Update `docs/architecture.md` and the `CLAUDE.md` Layout section to describe blocks rather than a flat `internal/`.

## 7. Verification

- [x] 7.1 `gofmt -l .` empty; `go build ./...`; `go vet ./...`; `go test ./...`.
- [x] 7.2 `go vet -tags=integration ./...`, then `go test -tags=integration ./...` (needs Docker).
- [x] 7.3 `go run ./cmd/validate-sources`; `make sqlc` produces no diff.
- [x] 7.4 Spot-run one binary per block-owning worker path to confirm wiring survived: `go run ./cmd/queue-metrics` with no `PROM_TEXTFILE_DIR` (must be a no-op that never opens the pool) and `go build ./cmd/...`.
- [x] 7.5 Recorded the four remaining `ingest` → higher-block edges in `internal/ingest/AGENTS.md` as the service-extraction seam.
