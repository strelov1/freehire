# CI gates and linters

What each check beyond `go build`/`go vet`/`go test` enforces, how to pass it, and its
escape hatch. Read the entry before touching what it guards. The root
[AGENTS.md](../../AGENTS.md) keeps a one-line summary of each.

## Migrations: `pnpm check:sql`

Never edit an applied migration; add a new file. `pnpm check:sql`
(`scripts/check-migrations.mjs`) fails when a change modifies, deletes or renames a migration
main already holds, unless a commit in the change carries a `Migration-Edit: <reason>`
trailer — for the rare edit that is right (a comment, a file that never ran anywhere). A
migration the branch itself added is new at the merge base, so editing it is never caught.

The same script lints (squawk) only the migrations a change ADDS — the applied history holds
many findings that stay, because rewriting an applied file is a worse hazard than any of
them. Whether a file runs inside a transaction is a property of the FILE (the
`migrate: no-transaction` marker, which must be a comment line of its OWN — quoted inside a
sentence it does not count), so the check runs two passes; `.squawk.toml` carries the
argument. Suppress a rule with `-- squawk-ignore <rule-name>` on the line before the
statement, with the reason beside it.

## Generated SQL layer: `make sqlc`

`internal/platform/db/` is generated: edit `internal/platform/db/queries/*.sql` and run
`make sqlc`. The pre-commit hook and the `sqlc` CI job regenerate and diff, so a query
edited without regenerating fails. Both use `make sqlc`, which holds the only version pin —
a second pin would drift and look exactly like stale code.

## Dead JS code: `pnpm check:dead`

knip (the `dead-code` CI job) gates **everything it reports — files, dependencies, binaries
AND exports** across `web/`, `design-system/` and the root scripts. An exported name nothing
imports fails the build, an exported *type* included; `knip.config.js` argues why. The
deliberate exemptions carry a JSDoc `@public` tag beside the code rather than an entry in the
config. CI only: knip needs both packages' dependencies installed, and a fresh worktree
usually has neither.

## Dead Go code: `deadcode`

`deadcode -test -tags=integration,llmlive ./...` (the `dead-code-go` CI job) builds the whole
call graph and reports what no binary and no test can reach. **Both flags are
load-bearing.** Without the tags it does not parse the build-tagged tests and calls their
helpers dead — the same blindness `.golangci.yml`'s `run.build-tags` documents for depguard.
Without `-test` every test-only helper is dead. It cannot see a function whose only caller is
its own test; that still needs a reviewer. Locally:
`go install golang.org/x/tools/cmd/deadcode@v0.52.0` — with a toolchain at least as new as
go.mod's `go` directive, or it reports "package requires newer Go version" instead of
findings.

## Workflows: actionlint and zizmor

`.github/workflows/` is linted by `actionlint` (the `workflows` CI job, which also runs
shellcheck over every `run:` block) and audited by `zizmor --offline` in the same job. Every
`uses:` is pinned to a commit SHA with a `# vX.Y.Z` comment, which Dependabot moves together;
a new action goes in the same way or zizmor fails. `actions/checkout` takes
`persist-credentials: false` unless a later step pushes. A finding wrong for one line carries
`# zizmor: ignore[<audit>]` with the reason beside it. Nothing else reads a workflow until
GitHub runs it, so a mistake in one that fires on a tag or a schedule surfaces late.

## Instruction files and Markdown: agnix, rumdl, `pnpm check:links`

- **agnix** `--strict` (the `docs` CI job) lints every AGENTS.md: an unclosed code fence
  turns the rest of the file into one code block, invisible to a reader skimming the
  rendered file. CDX-AG-009 caps each AGENTS.md at 32 KiB. `.agnix.toml` argues every rule
  it turns off.
- **[rumdl](https://github.com/rvben/rumdl)** (the `docs` CI job, and a pre-commit hook on
  staged `*.md` that steps aside when the binary is missing; `.mise.toml` pins the version)
  catches an unclosed fence, a skipped heading level, a malformed table or list, a duplicate
  heading. `.rumdl.toml` excludes `openspec/changes/archive/**` and the generated
  `docs/API*.md` and argues every rule it turns off; turn a rule off there, with the reason,
  not per file. `rumdl fmt` repairs only some findings and gets others wrong (it turns a
  wrapped line beginning `#659` into a heading), so run `rumdl check` and read the diff of a
  `fmt`.
- **`pnpm check:links`** (the `docs` CI job and a pre-commit hook) resolves every relative
  Markdown link in the repository. A dead link in the module table sends a reader somewhere
  confidently. The hook carries no glob on purpose: what breaks a link is renaming its
  **target**, usually a `.go` or `.svelte` file.

## Syntax rules: ast-grep

`sgconfig.yml`, the `ast-grep` CI job, a pre-commit hook, `mise run check:ast-grep`. A rule
the code's SHAPE can show — no types needed — gets a rule in `ast-grep/rules/<id>.yml` and a
test in `ast-grep/tests/<id>-test.yml` with `valid` and `invalid` snippets;
`ast-grep test -U` refreshes the snapshots beside them. The rule's `message` names what is
wrong and how to fix it, its `note` says why. A rule is turned on with every violation fixed,
never with a baseline. A `.svelte` file is read as HTML whose `<script>` is TypeScript, so a
TypeScript rule covers the components too. A rule that needs types stays in golangci-lint or
a Go test (the `llmkey` binding, the layering table).

## Public API contract: oasdiff

`scripts/check-openapi-breaking.sh` (the `api-breaking` CI job) runs oasdiff over
`web/static/openapi.yaml` against main and fails on a change that would break an existing
client — a removed field, a narrowed bound, a weaker response promise. A deliberate break
passes with an `API-Break: <reason>` commit trailer.

## Changed-line coverage: diff-cover

Pull requests only. `.github/scripts/diff-cover-gate.sh` runs
[diff-cover](https://github.com/Bachmann1234/diff_cover) against `origin/main` and fails when
too small a share of the lines the PR changes is executed by a test; the old uncovered
backlog blocks nothing.

- Go runs in the `backend` aggregator job, the only place every shard's profile is visible:
  the unit and integration profiles are each converted to Cobertura and merged, so a line
  covered in any shard counts. Web runs as a step of `web-work`, on vitest's lcov.
- Excluded: `internal/platform/db` and files with a "Code generated" header,
  `cmd/*/main.go`, and on the web side tests, specs, `.d.ts`, `generated/` and `fixtures/`
  (the vitest `coverage.exclude`).
- A changed line missing from a report counts as not measured, not as uncovered, so both
  sides list every source file: Go includes a package with no test files at 0%, and
  `web/vitest.config.ts` sets `coverage.include` so a file no test imports shows at 0%. Keep
  both true.
- Thresholds: `GO_FAIL_UNDER` (60) and `WEB_FAIL_UNDER` (0, report-only) at the top of the
  script, picked from the last ~20 merged PRs (nine measurable for Go: three would have
  failed at 60 and at 70, five at 80); web stays at 0 because three of the four measurable
  PRs failed even at 60, so it needs a larger sample.
- No commit-trailer escape hatch: write the test, or, when the lines cannot honestly be
  tested, change the threshold or the exclusions in the script in the same PR with the
  reason in review.
- diff-cover is installed from `.github/requirements/diff-cover.txt` (hash-pinned, in a
  virtualenv); `COVERAGE_FAIL_UNDER=<n>` overrides both thresholds for one local run.

## Python: pii-filter and scripts

The `pii-filter` CI job runs `services/pii-filter`'s tests with `python -m unittest` and no
install step (the service's tests use only the standard library; an install would hide a
test that started needing a package), then every `scripts/test_*.py` as a script — they
have their own runner, so `unittest discover` would report "Ran 0 tests" and pass.

## Artifacts we hand to others

The `artifacts` CI job: `web/static/install.sh` is served at `freehire.me/install.sh` to be
piped into `sh`, so shellcheck covers every tracked `*.sh`; both Dockerfiles go through
hadolint (`.hadolint.yaml` argues its two exclusions, and a finding wrong for one line
carries `# hadolint ignore=` beside it); and `web/static/openapi.yaml` is validated against
the OpenAPI specification, since it is a static asset nothing else parses.

## Shared JS dependencies: `pnpm check:deps`

See [design-system/AGENTS.md](../../design-system/AGENTS.md) — `scripts/check-shared-deps.mjs`
(the `shared-deps` CI job) holds `web/`, `extension/` and `design-system/` to the same
declared version for every package the design system imports as a type.

## Secrets: gitleaks

The one thing the hooks *prevent* rather than report: `gitleaks` (`brew install gitleaks`)
scans the staged index on every commit and fails hard if the binary is missing, because a
scanner that quietly skips makes a commit look examined. The `gitleaks` workflow scans the
whole history on top, since `--no-verify` walks past the hook and a credential removed in a
later commit is still leaked. A finding that survives `.gitleaks.toml` means **revoke the
credential** — rewriting history does not un-leak anything. Suppressions in that file
allowlist a *line shape*, never a path: gitleaks ORs `paths` with `regexes` and prunes the
file before reading it, so a path entry switches the scanner off for that file entirely.
