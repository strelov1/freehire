# AI coding readiness assessment

Assessment date: 2026-11-15. Scope: the developer workflow in this checkout,
including navigation, local environments, checks, generated artifacts and review.
This is not a production security audit or a benchmark of model cost.
Findings below describe the starting snapshot; follow-up status is recorded at the end.

The repository already has useful foundations: enforced Go layers, generated SQL
and HTTP contracts, per-checkout databases, reusable integration-test templates,
staged secret scanning and application verification helpers. Improve these before
adding another agent framework.

## Priority 1: make verification trustworthy

The original [verification stack](../../.agents/skills/verify-freehire/stack.sh)
recorded only Git HEAD when building the API. A dirty Go edit could leave the
running binary stale while `doctor` still passed. Journeys also shared temporary
spec and evidence paths, so concurrent runs could overwrite or delete each
other's work. This change fixes those cases; five helper regression tests pass.

Acceptance: editing a build input makes `doctor` refuse the stale server;
simultaneous journeys keep separate specs, logs, screenshots and traces; each
journey checks the stack before driving it.

Next, keep one dependable start/check/drive/cleanup path. Preserve diagnostics on
failure. The existing helpers are the place for this work.
An actual run also lost its Vite process after the startup tool session ended;
the browser reported `ERR_CONNECTION_REFUSED`, and the next journey's preflight
refused the missing web process. Test process lifetime across the agent runtime's
tool sessions as well as service health. A same-session retry does not establish
that unattended launches survive a session boundary.

## Priority 2: expose one local check plan

Required commands are distributed across the root instructions,
[Makefile](../../Makefile), [hooks](../../lefthook.yml), package scripts and
[CI](../../.github/workflows/ci.yml). An agent must reconstruct which commands
apply and which generate files. For example, ordinary Go tests do not compile
integration-tagged callers; linked design-system dependencies must be installed
before the web checks.

Add a small local command that prints the applicable check plan and can run it.
Reuse the existing [CI scope classifier](../../.github/scripts/ci_changes.py)
and package scripts. Include committed changes against an explicit base,
unstaged/staged changes, untracked files, deletions and both sides of renames.
Unknown inputs select the full plan. Explain each selected check.

Use focused tests while editing, then the required final gates. Keep whole-module
Go guards: some deliberately inspect other files and documents. Do not replace
them with changed-package-only testing. Existing CI already selects backend/web
work and caches dependencies; introducing a second unrelated selector would add
drift.

Acceptance: representative SQL, TypeSpec, Go signature, Svelte, shell and docs
changes produce correct plans; a failed command remains a failed run; one final
record identifies revision, dirty inputs, commands and outcomes. Do not claim a
speedup until task timings show one.

## Priority 3: reduce navigation and instruction cost

The tracked checkout contains 93 `AGENTS.md` files totaling 883,218 bytes. This is
not the amount automatically loaded on every task. The root file alone is 28,435
bytes; several local instruction files approach the 32 KiB instruction limit.
Large files are a reason to inspect relevance and duplication, not a reason to
delete safety constraints.

More directly, [.agnix.toml](../../.agnix.toml) disables `CDX-AG-010`, the
instruction-chain size check, because the root already consumes most of its
budget. The API handler path inherits root and block instructions as well as its
own large file. Shorten those chains before re-enabling the check; per-file caps
alone do not bound inherited context. Actual loading limits depend on the agent
runtime. The config's statement that nobody uses Codex is also stale.

Keep the root as a compact routing map: invariants that apply everywhere,
environment setup, validation entrypoint and links to owners. Move detailed
subsystem policy to its owning instructions or a linked reference. Preserve the
working distinction between [knowledge navigation](../knowledge/index.md) and
instructions. The server inventory injected into the conversation is not part of
the tracked root file and needs a separate change at its actual source.
Keep explicit cross-module triggers for paid AI, auth and SQL: directory
inheritance alone will not load a sibling module's rules. Use
[source adapter references](../../internal/ingest/sources/AGENTS.md) as an
existing example of loading details only when needed.

Regular source search also includes archived OpenSpec changes and generated
code. For example, an unrestricted `UpsertJob` file search matched 248 files,
including 97 archived specifications and 46 generated files in this snapshot.
Those are file counts, not measured token savings.

Provide an explicit source-search command or documented scope that omits frozen
history and generated outputs by default, with an obvious full-search option.
Retain SQL queries and generation inputs. Prefer this over a global ignore that
could hide relevant files from existing checks. Search the archive when history
is the question and generated output when checking contract drift.

Acceptance: ordinary implementation lookup finds the source of truth first;
historical and generated material remains available intentionally; moving
instructions preserves their discoverability from the root and affected paths.

## Priority 4: make familiar database commands safe to repeat

`make migrate` loops over every SQL file with `psql`, while
[cmd/migrate](../../cmd/migrate/main.go) already supplies version tracking and an
advisory lock. `make psql` and `make migrate` also select the Compose `hire`
database rather than the per-checkout database supplied by
[dev-env.sh](../../scripts/dev-env.sh). An agent following these familiar targets
can operate on a different database from `make run`.

Route the targets through the existing versioned runner and per-checkout URL.
Keep an explicitly named Compose database command only if there is a current
need. Verify the target identity before destructive development operations.

Acceptance: repeated migration calls apply each migration once, and migration,
server and SQL console resolve the same checkout database. This observation
comes from the commands; no shared database was modified to demonstrate it.

## Priority 5: remove repeated failure modes through ownership and types

Recent commits `679f38902` and `2dc6c7a2b` both restore account controls on
particular pages or states. The controls belonged to child headings even though
they control the account shell. Moving them into the `/my` layout removes the
need for each page author to remember them. Eight account/profile tests and a
real mobile browser journey passed after the fix.

The higher-impact next design concerns incomplete source listings. The pipeline
instructions document partial-crawl closure incidents, and `627613c43` fixes
Talentbrew pagination. Listing helpers can return successful partial and complete
results with the same shape. Inventory the callers and require affirmative
completeness evidence before allowing missing-job closure. Partial ingestion
should remain useful progress. Do not switch every adapter to strict failure.

Start at [HTML pagination](../../internal/ingest/sources/html.go): represent the
ending explicitly as unknown/partial, exhausted, failed or capped, with the zero
value refusing completeness. The strict wrapper accepts only exhaustion; the
partial wrapper can return useful links without claiming a full listing.
Current production callers are five strict adapters (rexx, hrmos, geekjob,
selfrecruit, careerplug) and seven partial adapters (luxoft, occ, builtin,
pythonorg, careerspage, freshteam, itviec).

Next, carry per-run evidence through fetched-board results and stream completion
to sweep qualification. Account for `Fetch`, `FetchNew`, `FetchNewGated`,
`FetchRecent` and `FetchStream`; recent-only runs remain partial. An explicit
empty-board result needs its own proof. Source capability markers remain an
eligibility check and cannot establish that a particular crawl completed.
Keep existing safeguards until each affected path is migrated.

These commits demonstrate recurring corrections, not who authored the errors.
Prefer changing ownership or representable states over adding another warning.

## Priority 6: separate deterministic verification from live dependencies

The verification skill seeds from a live Greenhouse board. Startup therefore
depends on an external service and current postings before unrelated UI work can
be checked. Its feature map explicitly excludes mail, OAuth, payments and LLM
flows; a successful jobs journey provides no evidence for those features.

Keep the live crawl as an ingestion smoke test. Add a small deterministic fixture
path through supported ingestion boundaries for ordinary application journeys.
Avoid test-only API setters and direct SQL that manufacture the feature's result.
Add journeys when actual tasks need them, prioritizing the user action and its
database/search side effects. Real external-service tests remain separate and
explicit about credentials and cost.

Acceptance: a UI-only journey can run when the public seed board is unavailable;
an ingestion smoke test still detects a broken real adapter; reports state which
dependencies and behaviors were exercised.

## How to measure the next improvements

For the next representative tasks, record setup time, time to the first failing
test, final-check time, number of retries, blocked steps and review-discovered
defects. Record model tokens and cost only where the runtime exposes them.
Use these observations to choose the next change. Fewer instructions or more
agent runs are not useful outcomes by themselves.

## Limits

Read the workflow entrypoints, CI selection, hooks, integration database harness,
verification helpers and representative ownership paths. Did not audit every
adapter, route, auth/payment path, dependency, production host or external agent
runtime. Unchanged workflow proposals above have not been benchmarked. Validation
of implemented changes is reported separately from this assessment.

## Follow-up implementation status

Integrated the four delegated changes into the working checkout: local check
planning, instruction routers with inherited-chain checks, scoped source search,
and database command routing. Integration review fixed source-search omission of
the actual OpenSpec archive and kept live `history` directories visible.

The verification runner now owns startup, journeys and cleanup in one foreground
session. A default three-posting fixture replaces only the external Greenhouse
response and uses the real adapter, pipeline, store and search drain. Live-board
ingestion remains available explicitly. Six helper regressions pass, including
elapsed-time readiness bounds. Actual fixture ingestion and indexing succeeded;
SIGTERM cleanup removed the stack and preserved diagnostics.

Plain and integration-tagged Go vet passed. Earlier full unit tests failed twice on
`TestLogsCollectorFailureIsBounded` in unchanged observability code: exception
capture exceeded its 500 ms limit. Other packages passed. The machine had heavy
concurrent builds and swap activity; this is context, not proof of the cause.
The isolated test and a subsequent full unit run passed without changing the
test or its thresholds. No main-checkout commit was made while the gate failed.

The fixture-backed foreground runner subsequently passed all three browser
journeys: feed/search/detail, signup/save/tracking, and mobile account navigation
without a CV. Cleanup removed the state directory and left no listeners on the
three instance ports. Evidence is in the local `verify-freehire-run-3ua9De`
directory printed by the runner; artifacts are not committed.
Mail, OAuth, payments, LLM features and production deployment remain unverified.
