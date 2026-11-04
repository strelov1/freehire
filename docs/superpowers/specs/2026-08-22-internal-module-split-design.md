# Grouping `internal/` into layered blocks

Date: 2026-08-22

## Problem

`internal/` holds 144 packages as a flat list. Nothing in the tree says which of them
belong together, which sit below which, or which may import which. Two consequences:

- **Navigation.** Neither a person nor an agent can look at `ls internal/` and form a
  mental model. The root `CLAUDE.md` compensates with a 40-row table of module files;
  that table is the map, and the directory tree contributes nothing.
- **No enforceable boundaries.** Any package may import any other. There is no mechanism
  that could stop `sources` from reaching into `cv`, and no place to declare that it
  shouldn't.

The measured shape today (`go list` over `./internal/...`, 2026-08-22):

| | |
|---|---|
| Go in `internal/` + `cmd/` + `services/` | 362k lines |
| Packages under `internal/` | 144 |
| Binaries under `cmd/` | 68 |
| `internal/handler` | 322 files, 67k lines, one package, imports 120 of the 144 |
| `internal/sources` | 408 files, 69k lines, one package, imports 10 |

## Goals

1. **Navigation** — the directory tree should carry the same information the `CLAUDE.md`
   table carries today.
2. **Boundaries for a future service extraction** — make it cheap to answer "what would it
   take to run ingest as its own service?" by making its edges visible and few.

## Non-goals

- **Build/test speed.** Measured: a one-file change in `internal/handler` rebuilds that
  package in 0.46s; `go build ./...` costs 29.8s wall / 139s CPU, dominated by linking 68
  binaries. Moving directories changes zero edges in the dependency graph, so it cannot
  change either number. Speed is a separate piece of work (splitting `handler`, reducing
  the binary count) and is out of scope here.
- **Splitting `internal/handler` or `internal/sources`.** Both are oversized, both are
  worth doing, neither is this change.
- **Multi-module (`go.work`) or multi-repo.** One `go.mod` stays. Boundaries are declared
  by import path and enforced by `depguard`, not by the module system.

## Design

### Blocks

Eleven blocks. `internal/<pkg>` becomes `internal/<block>/<pkg>`.

| Block | Packages |
|---|---|
| `platform` | `arch/layering`※ `blobstore` `cache` `config` `database` `db` `externalid`※ `flexjson` `htmltext` `isoweek` `linktoken` `llm` `llmschema` `migrate` `modroot`※ `observability` `outbox` `pgconv` `pgerr` `safehttp` `stringset` `testdb` `tokencrypt` `tracerlink` `worker` |
| `dict` | `classify` `companyname` `industrytag` `lang` `location` `normalize` `roletag` `roletype` `skilladjacency` `skillbundle` `skilltag` `vocab` `wordmatch` |
| `ai` | `aiarchetype` `assistant` `autofillagent` `browsertools` `credits` `embed` `enrich` `llmkey` `speech` |
| `identity` | `accountdelete` `accounts` `auth` `auth/apple` `auth/applejobs` `auth/mobileauth` `auth/oauth` `auth/recentauth` `userprofile` |
| `candidate` | `atscheck` `cv` `cvedit` `cvmatch` `cvsection` `experience` `hardconstraint` `hardconstraint/credentials` `headshot` `jobmatch` `matchanalysis` `pii` `resume` `resumeextract` |
| `job` | `applydate`※ `collections` `ghost` `ghostreport` `job` `jobdedup` `jobderive` `jobfacts` `jobhash` `jobreality` `jobview` `liveness` `outboundurl` `privatejob` `silence`※ `verdict` `ycdir` |
| `application` | `appevent` `apptimeline` `calmatch` `calsync` `deliverywindow` `followup` `gmailsync` `ical` `inbox` `jobtracking` `mailbox` `mailclassify` `mailingest` `maillink` `mailmatch` `mailrecall` `mailtpl` `userjob` `viewlog` |
| `search` | `facetsnapshot` `savedsearch` `search` `searchdrain` `searchintent` `similarjobs` |
| `ingest` | `adzunadesc` `applyform` `atsboard` `atsdetect` `boardresolve` `catalogstats` `contribution` `jdresolve` `linkimport` `linksource` `moderation` `pipeline` `screeninganswers` `sources` `submission` `telegram` |
| `engage` | `broadcast` `community` `companyfeedback` `discordbot` `emailnotify` `mailpreview` `notify` `nudge` `onboarding` `pushnotify` `referral` `reminder` `report` `subscription` `telegramnotify` `vote` |
| `api` | `handler` `ogimage` `ratelimit` `realtime` |

※ Packages that did not exist when this was written. `job/silence` and `job/applydate`
were carved out of `userjob` by edit 4; `platform/externalid` by edit 3, which turned out
to be about the stored key's format rather than the provider vocabulary; `platform/modroot`
and `platform/arch/layering` are the guard itself. `dict/provider` was planned and never
created — see edit 3. `platform/llm` and `platform/llmschema` are reclassifications made
during implementation — see edit 2, and `ingest/catalogstats` likewise.

### Layering

With the four prerequisite edits applied, the block graph is acyclic and settles into
eight layers. A block may import only blocks strictly below it.

```
8. api
7. engage      ingest
6. application search
5. job
4. candidate
3. ai          identity
2. dict
1. platform
```

`ai` sits low because what remains in it — `enrich`, `embed`, `llmkey` — depends only on
`platform` and `dict`. `ingest` sits high because `submission`/`moderation` reach into
`job`, and `linkimport` reaches into `search` and `enrich`.

### Non-obvious block placements

Six packages move to a block other than the one their name suggests. Each move removes a
bidirectional block pair that would otherwise make a `depguard` rule impossible to state.

| Package | From | To | Reason |
|---|---|---|---|
| `ratelimit`, `realtime` | (would be `platform`) | `api` | HTTP middleware; `ratelimit` imports `auth` |
| `ghost`, `ghostreport`, `jobreality`, `liveness` | (would be `application`) | `job` | These describe the **posting's** reality, not an application's. `jobview` reads them |
| `matchanalysis` | (would be `ai`) | `candidate` | Imports `resumeextract`, `jobmatch`, `hardconstraint` |
| `mailpreview` | (would be `application`) | `engage` | Imports 8 `engage` packages; it is the dev preview of outbound mail |
| `facetsnapshot`, `searchintent` | (would be `job` / `ai`) | `search` | Both wrap `search` |
| `submission`, `moderation` | (would be `engage` / `application`) | `ingest` | Manual job intake, not applications. Their package docs say so: "the moderator-authored job use cases", "the public job-submission queue" |

`mail` and `apply` are deliberately **one block** (`application`). They are genuinely
entangled — a classified email advances an application stage (`mailclassify` → `userjob`)
while application tracking reads the classifier (`jobtracking` → `mailclassify`). Splitting
them would require an interface seam that buys nothing here; they are one domain.

### Prerequisite edits

Four code changes must land before the move. Each is a separate commit so it is reviewable
on its own. Without them the block graph has cycles and no `depguard` rule can be written.

**1. Move `submission` and `moderation` into `ingest`.**
Pure relocation, no code change. Removes `engage ↔ application` (back-edge
`submission → moderation`) and part of `job ↔ application`.

**2. ~~Move the `llm.Settings` conversion out of `internal/config`.~~ Withdrawn during implementation.**
`internal/config/llm.go:75` defines `func (l LLM) Settings(model string) llm.Settings`.
That one method is the only reason `config` imports `llm`, which with `llm` in `ai` creates
`ai ↔ platform`.

Every way of moving the method made the code worse. All eight call sites are
`llm.NewClient(cfg.Settings(model), tag)` in `cmd/`, so relocating the conversion there
regresses exactly what the comment at `config/llm.go:72-74` defends — "a field added to
either is a one-line change here rather than seven copies at the entrypoints" — and the
only other option was a package holding one function. `config.LLM` is additionally embedded
in both `config.Settings` and `config.Enrich`, so extracting the struct ripples further.

The classification was wrong, not the code. `internal/llm` imports only `internal/llmschema`;
`llmschema` imports nothing of ours. Neither knows the domain: one wraps an
OpenAI-compatible chat endpoint, the other derives a JSON Schema from a Go type. That is the
category `safehttp` and `blobstore` occupy. Both moved to `platform`, `config` → `llm`
became an intra-block import, and the edge disappeared with no code change.

`llmkey` stays in `ai` — it imports `db` and is about per-user spend attribution, which is
domain rather than transport.

**3. Carve the provider vocabulary out of `internal/sources`.**
`catalogstats` and `privatejob` reach into `sources` for `Taxonomy` (5 uses),
`AggregatorProviders` (3), `BoardKeyedProviders` (3), and `SanitizeHTML` (2). The first
three are the **provider dictionary**, not the crawler — the same taxonomy-vs-crawl-registry
distinction the repo already draws. Move them to `internal/dict/provider`; move
`SanitizeHTML` to `internal/platform/htmltext`. Removes `job ↔ ingest`.

**4. Carve the silence model out of `internal/userjob`.**
`ghost` and `ghostreport` use exactly `DaysSilent`, `SilenceSilent`, `SilenceStateFor`,
`SilenceThresholdDays`, and `ValidateAppliedOn` — how long an application has gone
unanswered, which is the evidence a ghost verdict is built on. Move those into
`internal/job/silence`, below both. Removes `job ↔ application`.

### Enforcement

`depguard` in `.golangci.yml`, one rule per block, listing the blocks at or above its own
layer as denied. Example:

```yaml
linters:
  settings:
    depguard:
      rules:
        dict:
          files: ["**/internal/dict/**"]
          deny:
            - pkg: github.com/strelov1/freehire/internal/ai
            - pkg: github.com/strelov1/freehire/internal/identity
            # ... every block except platform
        ingest:
          files: ["**/internal/ingest/**"]
          deny:
            - pkg: github.com/strelov1/freehire/internal/api
            - pkg: github.com/strelov1/freehire/internal/engage
```

The rules encode the layering, so they are derivable from the table above rather than
hand-maintained. A future layer violation fails CI at the import line.

CI runs `golangci-lint --new-from-merge-base=origin/main`, which filters to changed diff
lines. Git rename detection means a moved file with only its import block rewritten
presents as a rename plus a few changed lines, and import lines carry no lint findings. If
the ratchet does fire, the report is read and the handful of surfaced findings are fixed in
the same PR.

### Ingest as a future service

After the move, `ingest` depends on `platform`, `dict`, `job` — plus exactly four edges
into higher blocks:

```
linkimport => enrich        linkimport => search
telegram   => llm           telegram   => llmschema
```

Four edges is a concrete conversation about a seam. This is the deliverable of goal 2; the
extraction itself is not in scope.

## Work plan

One PR.

1. Prerequisite edits 1–4, one commit each.
2. `git mv` all packages into their blocks; rewrite import paths with a script driven by
   `go list` (a textual find/replace would corrupt any package name that is a substring of
   another).
3. `.golangci.yml`: add the `depguard` rules.
4. `AGENTS.md` per block. Rewrite every `internal/<pkg>/AGENTS.md` link — there are **202**
   across `CLAUDE.md` and `docs/`.
5. Update the path-bearing config and code listed below.

### Paths hardcoded as strings

These do not fail to compile. Several are tests that walk the module by path, so a stale
path makes the test pass over nothing — a silently disabled guard, not a red build. Each
must be updated in the move commit.

| Location | What breaks |
|---|---|
| `internal/llmkey/scope_test.go:29-32` | Maps `internal/enrich`, `internal/telegram`, `internal/mailclassify`, `internal/embed` to `../../internal/<pkg>`. This is the test enforcing that background entrypoints never resolve a user's LLM credential |
| `internal/normalize/legal_form_rule_test.go:16` | `canonicalFormList = "internal/normalize/company.go"` — the test keeping one legal-form vocabulary in the module |
| `internal/pgerr/pgerr_test.go:117` | Path check on `"internal/pgerr/"` |
| `cmd/gen-cities/main.go:44` | `outputPath = "internal/location/cities1000.tsv"` |
| `.github/workflows/perf.yml:60` | Change filter hardcodes `internal/handler/`, `internal/search/`, `internal/jobview/`; a stale filter silently stops running the perf job |
| `sqlc.yaml:5,9` | `queries: internal/db/queries`, `out: internal/db` |
| `Makefile:46` | Comment on the `gen-cities` target |
| `internal/*/[a-z]*_integration_test.go` | Header comments giving the `go test -tags=integration ./internal/<pkg>/` invocation (cosmetic, but they are the documented way to run each suite) |

Verified as **not** affected: `lefthook.yml` (globs are `*.go`, path-agnostic) and the
release script (ops-side, not in this repo).

## Verification

The compiler is the primary check — the move either builds or it doesn't.

```
gofmt -l .                          # must print nothing
go build ./...
go vet ./...
go test ./...
go vet -tags=integration ./...      # 187 tagged files across 20 packages
go test -tags=integration ./...     # what CI runs; needs Docker
golangci-lint run
go run ./cmd/validate-sources
```

The string-path guards in the table above compile and pass whether or not their path is
correct, so the build says nothing about them. Confirm each one still finds its target —
the cheapest check is to break it deliberately (point it at a nonexistent path) and see the
test fail.

Plus a check that the layering holds: a script that reads `go list -f '{{.ImportPath}}
{{.Imports}}'`, maps packages to blocks, and asserts no block imports a block at or above
its own layer. This duplicates what `depguard` enforces but reports the whole graph at once
rather than per-file, which is what makes a violation diagnosable.

## Risks

- **Merge conflicts.** The diff touches every file with an internal import. Any branch open
  during the move will conflict on its import blocks. Mitigation: land it when few branches
  are open, and rebase rather than merge.
- **Analysis drift.** The block assignment was computed against local `main` at
  `5cfc6767`; `origin/main` was at `403db4ce`. Re-run the layering check against the branch
  base before the move, not just after.
- **`internal/handler` remains one 322-file package** importing 120 others. It lands in
  `api`, the top layer, so it violates nothing — but the boundary it should have is not
  created by this change.
