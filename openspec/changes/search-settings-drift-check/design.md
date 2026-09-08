## Context

See proposal.md - Why. `internal/search/search/AGENTS.md` already names the exact
hazard shape twice (once for `sortableAttributes`/`filterableAttributes`, once for the
skill embedder) and explicitly says there is no operator script for it. `facetSettings()`
and `companySettings()` are the two functions that already state, in code, what this
binary expects the live indexes to declare — this change adds nothing new to state, it
reads those two functions back and compares them against what Meilisearch's
`GET /indexes/<uid>/settings` actually returns.

## Goals / Non-Goals

**Goals:**
- Make an already-drifted live index visible on a schedule, closing exactly the gap
  AGENTS.md names ("There is no operator script for this").
- Reuse the existing settings source of truth (`facetSettings()`, `companySettings()`)
  rather than maintaining a second, driftable copy of the expected attribute/embedder
  list.
- Match the established shape of this codebase's other observe-and-publish workers
  (`cmd/llm-probe`, `cmd/queue-metrics`): read-only, no-op without its required
  configuration, never a red unit for the condition it observes.

**Non-Goals:**
- Fixing the deploy-ordering hazard itself. Settings-before-binary stays a decision
  made when sequencing a deploy and a reindex; this worker is the read side, not an
  enforcement mechanism, and does not gate a deploy or block a rollout.
- Alerting rules or dashboards — this worker publishes a Prometheus gauge the same way
  `llm-probe` and `queue-metrics` do; wiring an alert on it is an operational follow-up,
  same as every other gauge those workers publish.
- Any change to how a Meili settings patch is applied (`ensure`, `EnsureIndex`,
  `Rebuild.Prepare`) — this only reads settings back, it never writes them.

## Decisions

**A pure `diffSettings` function, not an inline comparison inside the network call.**
The interesting behavior — which attribute is missing, which direction is safe to
ignore — has nothing to do with HTTP or Meilisearch's client library. Separating it
means the comparison is unit-tested against fabricated `*meilisearch.Settings` values
(five cases: match, missing sortable, missing filterable, missing embedder, and the
one-directional rule), while `SettingsDrift` itself gets a narrower integration test
that only has to prove the real `GetSettingsWithContext` call and the real
`facetSettings()`/`companySettings()` wire into `diffSettings` correctly — mirroring how
this package already separates `facetSettings()` (pure, unit-tested) from the embedder
ordering hazard (`ensure`, integration-tested against a real engine, per
`embedder_integration_test.go`'s own doc comment: "This file tests the index, not the
struct").

**A count gauge, not a per-attribute label.** `llm-probe`'s render doc explains why a
rate over three probes is not published as a rate — the caller who writes the alert
should choose the shape. The same reasoning applies here in reverse: the set of
attributes and embedders this binary could ever be missing is small and changes with
every code change to `facetSettings()`/`companySettings()`, so a label per attribute
would mean a Prometheus label cardinality that drifts with the source code. The count
is what an alert needs (`> 0` for some duration); the specific gap is in the run's log
line, exactly like `llm-probe` logs the failing probe's detail rather than trying to
give the gauge itself that texture.

**Both indexes in one worker, not two.** `SettingsDrift` checks jobs and companies in
one call and one gauge. The alternative — a `cmd/search-settings-drift` and a
`cmd/company-settings-drift` — would be two copies of the same gate-before-bootstrap,
no-op-without-config, publish-a-count shape for a hazard that is identical in kind
between the two indexes; splitting them would not change what either can express, only
double the boilerplate `llm-probe` already set the template for.

**No gauge per index.** A single combined count keeps the shape simple and matches
what an operator actually needs to know first ("is anything wrong"); the log line names
which index and which attribute for the case where the answer is yes. Splitting the
gauge by index is a straightforward follow-up if the combined count ever proves too
coarse in practice — noted here rather than built ahead of that need.

## Risks / Trade-offs

- **[Risk] A gap that exists between two runs (drift is introduced and then patched
  within the 5-minute window) is never observed.** → [Mitigation] Same trade-off
  `llm-probe`'s timer accepts explicitly (`OnUnitActiveSec=5min`, "a minute's
  resolution buys nothing... five minutes still turns a half-day discovery into a
  quarter-hour one"). Settings drift is even less likely to self-heal within a window
  than a flaky provider is — it stays wrong until a human patches it — so missing a
  same-window fix costs nothing an operator would have wanted paged for.
- **[Risk] The worker adds a Meilisearch round trip on every scheduled run, on a host
  where a rebuild can already be running.** → [Mitigation] `GetSettingsWithContext` is
  a metadata read, not a search or a write, and carries none of the task-queue
  contention this package's AGENTS.md warns about for `swapIndexes`/rebuild pushes; the
  systemd unit sets the same lowest `CPUWeight`/`IOWeight` as `queue-metrics` and
  `llm-probe` so it is never the reason a real caller waits.
- **[Risk] The worker cannot distinguish "the live index is missing this attribute"
  from "Meilisearch is unreachable" from the gauge alone.** → [Mitigation] Not this
  worker's problem to solve twice — an unreachable Meilisearch is a network/connectivity
  failure that surfaces as `run()` returning 1 (a red unit, unlike an observed drift,
  which stays green), and the log line carries the underlying error. The gauge itself
  is only ever published on a successful comparison.

## Migration Plan

No schema change, no change to any existing read or write path. The new worker and its
systemd unit are purely additive; installing the unit and timer on the host is a manual
step (per `deploy/AGENTS.md`: "Nothing here deploys itself"), same as every other worker
addition, and is out of scope for this change. Until installed, nothing changes.
Rollback is deleting the unit files and the binary — no data was written.
