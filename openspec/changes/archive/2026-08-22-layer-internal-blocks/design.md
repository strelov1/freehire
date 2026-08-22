## Context

The full design, including the complete package→block table and the measurements behind
it, is committed at `docs/superpowers/specs/2026-08-22-internal-module-split-design.md`.
This document records the decisions and their alternatives.

Current state, measured with `go list` over `./internal/...` on 2026-08-22: 144 packages
flat under `internal/`, 362k lines of Go across `internal/` + `cmd/` + `services/`, 68
binaries. `internal/handler` is 322 files in one package importing 120 of the 144;
`internal/sources` is 408 files in one package importing 10.

The block assignment was not chosen by name. It was derived by building the package import
graph, assigning candidates, and iterating until no two blocks imported each other.

## Goals / Non-Goals

**Goals:**

- The directory tree carries the information the `CLAUDE.md` module table carries today.
- An upward import fails CI instead of merging.
- `ingest`'s edges into higher blocks are few enough to be listed, so a service extraction
  is a concrete conversation.

**Non-Goals:**

- **Build or test speed.** Measured: a one-file change in `internal/handler` rebuilds that
  package in 0.46s; `go build ./...` is 29.8s wall / 139s CPU, dominated by linking 68
  binaries. Moving directories changes zero edges in the dependency graph, so it cannot
  move either number. Recorded here because it is the intuitive reason to want this change
  and it is wrong.
- Splitting `internal/handler` (322 files) or `internal/sources` (408 files). Both are
  worth doing. Neither is this change.
- Multi-module (`go.work`) or multi-repo.

## Decisions

### Directories plus a linter, not Go modules

**Chosen:** one `go.mod`; boundaries declared by import path, enforced by `depguard`.

**Alternative — multi-module with `go.work`:** the compiler enforces boundaries, so they
cannot be bypassed. Rejected: each module versions separately, `go build ./...` stops
covering the repo, and `Makefile`, CI, `lefthook`, the release script, and `sqlc` all need
reworking — across 68 binaries. The enforcement gain over `depguard` is real but small;
the cost is not.

**Alternative — separate repositories:** rejected outright at this stage. `migrations/` is
the source for both `sqlc` codegen and Postgres initdb, and every subsystem reads it.

### `mail` and `apply` are one block, `application`

They are genuinely entangled: a classified email advances an application stage
(`mailclassify` → `userjob`) while application tracking reads the classifier
(`jobtracking` → `mailclassify`). Splitting them needs an interface seam that buys nothing
here. **Alternative — keep them separate and write no `depguard` rule between them:**
rejected, because an exception in a boundary rule is the absence of a boundary.

### Six packages move to a block other than the one their name suggests

Each move removes a bidirectional block pair. `ratelimit`/`realtime` → `api` (HTTP
middleware; `ratelimit` imports `auth`). `ghost`/`ghostreport`/`jobreality`/`liveness` →
`job` (they describe the posting's reality, not an application's; `jobview` reads them).
`matchanalysis` → `candidate` (imports `resumeextract`, `jobmatch`, `hardconstraint`).
`mailpreview` → `engage` (imports 8 `engage` packages). `facetsnapshot`/`searchintent` →
`search` (both wrap `search`). `submission`/`moderation` → `ingest` — their package docs
call them "the moderator-authored job use cases" and "the public job-submission queue";
they are manual job intake.

### `llm` and `llmschema` are platform, not ai

Discovered while implementing: `internal/config` imports `internal/llm` for one conversion
method (`config/llm.go:75`), which with `llm` in `ai` is an upward edge from layer 1 to
layer 3. The planned fix was to move the conversion. Every alternative made the code worse —
all eight call sites are `llm.NewClient(cfg.Settings(model), tag)` in `cmd/`, so moving the
conversion there regresses exactly the property the code comment at `config/llm.go:72-74`
defends ("a field added to either is a one-line change here rather than seven copies at the
entrypoints"), and the only other option was a package holding one function.

The classification was wrong, not the code. `llm` imports only `llmschema`; `llmschema`
imports nothing of ours. Neither knows the domain — one wraps an OpenAI-compatible chat
endpoint, the other derives a JSON Schema from a Go type. That is the same category as
`safehttp` and `blobstore`. Moved both to `platform`; `config` → `llm` becomes an intra-block
import and the edge disappears with no code change.

`llmkey` stays in `ai`: it imports `db` and is about per-user spend attribution, which is
domain, not transport.

### Two extractions rather than two exceptions

`catalogstats` and `privatejob` reach into `sources` for `Taxonomy`,
`AggregatorProviders`, `BoardKeyedProviders`, and `SanitizeHTML`. `ghost` and
`ghostreport` reach into `userjob` for five silence functions. Both are cases of a small,
low-level concept living inside a large high-level package.

**Chosen:** carve out `internal/dict/provider` and `internal/job/silence`.
**Alternative — allow the two edges as documented exceptions:** rejected for the same
reason as above, and because both extractions are small and leave the concept easier to
find than it is today.

### Enforcement at two granularities

`depguard` fails at the import line, which is where a developer wants the error. A Go test
that walks the whole graph reports every violation at once, which is what makes a large
violation diagnosable rather than a game of whack-a-mole. Both, not either.

The test must include `TestImports` and `XTestImports`. A test file can create a
cross-layer dependency that the production build never shows, and the original graph
analysis for this change included test imports for exactly that reason.

## Risks / Trade-offs

- **A test that locates its target by string path silently stops testing.** Four exist:
  `internal/llmkey/scope_test.go` (the guard that background entrypoints never spend a
  user's LLM credit), `internal/normalize/legal_form_rule_test.go` (one legal-form
  vocabulary per module), `internal/pgerr/pgerr_test.go`, and `cmd/gen-cities/main.go`.
  These compile and pass whether or not their path is right. → Repoint each in the move
  commit, then break each one deliberately and confirm it fails. This is the only failure
  mode of this change that produces a green CI over a disabled guard.
- **`.github/workflows/perf.yml` filters on hardcoded `internal/` paths.** A stale filter
  silently stops running the perf job. → Same treatment; listed in tasks.
- **Merge conflicts across in-flight branches.** 0 open PRs; ~15 unmerged local/fork
  branches touch ~35 files in `internal/`, the largest `dedup-marker-ownership` at 20. →
  Conflicts land in import blocks and are mechanical. Landing
  `dedup-marker-ownership` first is cleaner but is not a blocker.
- **Analysis drift.** The block assignment was computed against `5cfc6767`; the branch base
  is `403db4ce`. → The layering test is written first and run against the branch base, so
  drift shows up as a test failure rather than a surprise.
- **`internal/handler` remains one 322-file package importing 120 others.** It lands in
  `api`, the top layer, so it violates nothing — but this change does not give it the
  boundary it needs. Accepted; out of scope.

## Migration Plan

No deploy-time migration. The change is source-only: no schema, no wire format, no config
value. Rollback is `git revert` of the merge.

The one ordering constraint is internal to the PR: the four prerequisite edits land as
separate commits before the bulk move, so each is reviewable on its own rather than buried
in a diff that touches every file.

## Open Questions

None.
