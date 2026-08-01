## Why

`sources.All` answers two unrelated questions with one map: *can this process crawl the
provider* (a runtime credential fact) and *what kind of source is this provider* (a static,
compile-time fact). Three adapters — `usajobs`, `reed`, `whatjobs` — are registered only when
their environment credential is set, so every taxonomy consumer reads the second question
through the first and silently loses those providers wherever the keys are unset.

That is not hypothetical. `cmd/reindex`'s aggregator-duplicate suppression drops `whatjobs` from
the aggregator set on a keyless reindex host — `whatjobs` is a CPC reseller whose entire
inventory is resold copies of first-party ATS postings, and production holds 6,298 of them, so
those duplicates stay unsuppressed in search. `cmd/ghost-crosscheck` reads the same registry
with the same leak, and `/api/v1/status` reports all three providers as kind `other`.

The generated `SOURCE_VALUES` in `web/src/lib/generated/contracts.ts` is missing the same three
values, because `cmd/gen-contracts` was last run without the credentials in the environment.
That one is cosmetic: the source facet is distribution-driven (`facets.ts:532` marks it
`dynamic: true`, options come from `/api/v1/jobs/facets`), and no web code reads `SOURCE_VALUES`
or the `Source` type at all. It is recorded here because a generated constant that silently
varies with the operator's environment is a trap for whoever does start reading it.

## What Changes

- `sources.All(nil)` — the transport-free listing path — becomes **total** over the provider
  taxonomy: `usajobs`, `reed` and `whatjobs` register with an empty credential there, mirroring
  the `taleo` / `meta` / `bayt` / `gulftalent` treatment six lines below in the same function.
  `All(client)` keeps the credential gate exactly as it is, so `cmd/ingest` still fails fast on
  a board file for an unconfigured provider rather than starting crawls that 410 per board.
- **BREAKING (spec-level, not wire-level):** the stated rule "an unconfigured environment leaves
  the provider absent from the registry" narrows to "absent from the *crawl* registry". Three
  adapter tests and two spec requirements currently assert the wider claim.
- `cmd/reindex`'s comment at `main.go:420` — which asserts `usajobs` is the only credential-gated
  adapter and reasons that its absence is harmless — is deleted along with the condition that
  made it necessary.
- `internal/pipeline/AGENTS.md:11` (stale one-adapter claim) and `internal/sources/AGENTS.md:12`
  are restated around the crawl/taxonomy split.
- `contracts.ts` is regenerated, gaining the three source values so the checked-in file matches
  the registry again.
- Adjacent, not caused by this defect: `SOURCE_LABELS` in `web/src/lib/facets.ts` gains the two
  casing overrides the title-case fallback gets wrong, so the live facet stops rendering
  `Usajobs` and `Whatjobs`.

## Capabilities

### New Capabilities

None. This corrects an existing capability's registration rule.

### Modified Capabilities

- `source-ingest`: the registry's two paths are separated — the transport-free registry is total
  over the taxonomy, the crawl registry stays credential-gated. Reed's "Registered only when the
  key is configured" scenario is restated in those terms.
- `whatjobs-source`: "The publisher id is read from the environment only" keeps its
  MUST-NOT-from-a-board-file rule; its two registration scenarios move from *the registry* to
  *the crawl registry*, and the taxonomy path gains a scenario.

## Impact

- `internal/sources/registry.go` — the three credential branches (~6 lines net).
- `internal/sources/usajobs_test.go`, `reed_test.go`, `whatjobs_test.go` — the registration
  tests assert the crawl/taxonomy split instead of plain absence.
- `cmd/reindex/main.go`, `cmd/ghost-crosscheck/main.go` — no code change needed once `All(nil)`
  is total; the reindex comment goes.
- `internal/handler/status.go` — `/api/v1/status` stops rendering the three as `KindOther`.
  Response shape unchanged; a `kind` value changes for three providers.
- `web/src/lib/generated/contracts.ts` (regenerated) — three values restored to `SOURCE_VALUES`,
  which no runtime code reads today. `web/src/lib/facets.ts` — two label overrides.
- No migration, no new dependency, no change to any crawl path.
