## Why

`internal/` holds 144 packages as a flat list. The tree says nothing about what belongs
with what or what may import what — the root `CLAUDE.md` compensates with a 40-row table
that is the only map. And nothing can be enforced: any package may import any other, so
there is no place to declare that `sources` must not reach into `cv`, and no mechanism that
would catch it.

Now, because the catalogue side of the codebase is approaching a service extraction. The
question "what would it take to run ingest on its own?" is unanswerable while its edges are
invisible.

## What Changes

- Group the 144 packages into **eleven blocks** at `internal/<block>/<pkg>`: `platform`,
  `dict`, `ai`, `identity`, `candidate`, `job`, `application`, `search`, `ingest`,
  `engage`, `api`. **BREAKING** for every import path in the module.
- Add a **layering rule**: the blocks form eight layers, and a block may import only blocks
  strictly below it. Enforced two ways — `depguard` in `.golangci.yml` (fails at the import
  line) and a Go test that asserts the whole graph at once (diagnosable).
- Four prerequisite code edits, each removing a cycle that would make the rule unstateable:
  1. `submission` + `moderation` move to `ingest` (they are manual job intake, not applications).
  2. The `llm.Settings` conversion moves out of `internal/config`.
  3. The provider vocabulary (`Taxonomy`, `AggregatorProviders`, `BoardKeyedProviders`)
     carves out of `internal/sources` into `internal/dict/provider`; `SanitizeHTML` goes to
     `internal/platform/htmltext`.
  4. The silence model (`DaysSilent`, `SilenceSilent`, `SilenceStateFor`,
     `SilenceThresholdDays`, `ValidateAppliedOn`) carves out of `internal/userjob` into
     `internal/job/silence`.
- Repoint every hardcoded string path — including four tests that walk the module by path
  and would otherwise pass over nothing.

**No behaviour changes.** No package gains or loses a capability; nothing on the wire moves.

## Capabilities

### New Capabilities

- `module-layering`: the block assignment, the layering rule, and how it is enforced.

### Modified Capabilities

None. `application-silence-signal` describes behaviour that edit 4 relocates without
altering — the requirements are untouched.

## Impact

**Every Go file with an internal import.** Plus:

- `.golangci.yml` — new `depguard` rules
- `sqlc.yaml` — `queries:` and `out:` both point into `internal/db`
- `.github/workflows/perf.yml` — the change filter hardcodes three `internal/` paths; a
  stale filter silently stops running the perf job
- `cmd/gen-cities/main.go` — `outputPath` string
- `internal/llmkey/scope_test.go`, `internal/normalize/legal_form_rule_test.go`,
  `internal/pgerr/pgerr_test.go` — guards that locate their target by string path and
  pass vacuously if it is stale
- `CLAUDE.md` and `docs/` — 202 links to `internal/<pkg>/AGENTS.md`

**Not affected** (verified): `lefthook.yml` (globs are `*.go`), the ops-side release script.

**Out of scope:** build/test speed (measured: directory moves change zero edges in the
graph, so they cannot affect it), splitting `internal/handler` or `internal/sources`,
multi-module or multi-repo.

**Conflict surface:** 0 open PRs; ~15 unmerged local/fork branches touching ~35 files in
`internal/`, the largest being `dedup-marker-ownership` at 20 files. Conflicts land in
import blocks and are mechanical.
