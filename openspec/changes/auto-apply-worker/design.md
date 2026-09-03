## Context

See `proposal.md` for motivation and `docs/superpowers/specs/2026-09-02-auto-apply-worker-design.md`
for the brainstormed design this formalizes (spike findings, architecture diagram, rationale
for each boundary — including the later revision that dropped the Python sidecar). This
document restates only what a task breakdown needs to be unambiguous.

**Already built, unchanged by this document's decisions** (tasks 1-5 of the OpenSpec task
list; see there for what's still open):
- `auto_apply_queue` (migration 0116) and its sqlc queries.
- `internal/autoapply` — the queue-drain domain logic (`Store`/`AnswerSource`/`SidecarClient`
  ports, `Run`), unit-tested with fakes. `SidecarClient` is the seam this document's revision
  changes the implementation of — the interface itself does not change.
- `cmd/auto-apply` — the worker binary: `dbStore` (composes `LockJobForApply` +
  `MarkJobApplied` + queue retirement in one transaction — this is what the spec's "never
  twice" requirement rests on, integration-tested against a real Postgres), and
  `httpSidecarClient`, the HTTP `SidecarClient` implementation this document replaces.
- `internal/candidateprofile` — extracted from `internal/handler` (was unexported and
  handler-scoped) so the extension-autofill path and this worker share one profile
  assembler. `cmd/auto-apply`'s `AnswerSource` wraps it.

Existing pieces this change builds on, unchanged in behavior:
- `internal/outbox.RunPool` — the claim/process/lease loop `internal/applyform`'s
  `cmd/capture-apply-form` already runs on.
- `db.Queries.MarkJobApplied` / `LockJobForApply` — the same statements
  `jobtracking.QueriesRepository.MarkApplied` runs for a manual apply, called directly since
  the queue already carries `job_id` (no slug to resolve).

## Goals / Non-Goals

**Goals:**
- A working, narrow, end-to-end path: queued attempt in → submitted application or a named
  reason it could not be, out.
- Reuse every existing piece that already does part of this job (profile data, tracking
  write path, outbox mechanics) rather than parallel-inventing any of them.
- One language, one deploy artifact. A second spike (below) found the originally-proposed
  Python sidecar bought no stealth advantage over a Go-native browser driver, so this
  revision keeps the whole worker inside `cmd/auto-apply`'s existing binary.

**Non-Goals** (design-level, beyond what proposal.md already excludes):
- Concurrency/throughput tuning. `RunPool`'s existing batch/lease/concurrency parameters are
  reused as-is; no new tuning knobs are designed here.
- Any UI surface. This change has no candidate-facing screen — `auto_apply_queue` rows are
  written and read by backend code only in this change.
- Anti-detection hardening beyond one launch flag. The spikes measured that config leaves
  several headless tells unaddressed under both Patchright and chromedp; closing them further
  is deferred (see Risks).
- A residential/mobile proxy for the datacenter-IP-reputation risk. Wiring one in is a config
  change to `internal/atsapply`'s browser launch, not an architecture decision — deferred
  until real `blocked`/`failed` rates show it is needed.

## Decisions

### Browser automation over an ATS's own submission API — checked directly, not assumed

Before committing to DOM automation at all, this revision checked whether Greenhouse, Lever
or Ashby expose a submission API a third party could call instead. Greenhouse documents
`POST /v1/boards/{token}/jobs/{id}`, but it requires Basic Auth with the *employer's own* Job
Board API key — built for that employer's own careers-site integration into their own
account, not for an unrelated platform applying on a candidate's behalf. Lever and Ashby
expose no submission endpoint at all. So DOM automation is not a stand-in for a simpler path
that exists and was skipped; it is the only path available here.

### chromedp (Go), not a Python/Patchright sidecar — reversed after measurement

The OpenSpec proposal for this change originally specified a Python sidecar
(`services/auto-apply`, Playwright + `patchright`, HTTP-called from `cmd/auto-apply`,
precedent `services/pii-filter`) — see the brainstormed design doc's original text. A
follow-up spike (2026-09-02, same day) tested `chromedp` — pure Go, no second process —
against the same bot.sannysoft.com signals the first spike measured Patchright against:

- Stock `chromedp`, driving a real installed Chrome, already passed `window.chrome`,
  `HEADCHR_CHROME_OBJ`, `HEADCHR_PERMISSIONS`, `HEADCHR_PLUGINS` and `HEADCHR_IFRAME` —
  checks Patchright's bundled `headless_shell` failed. These largely detect "this is a
  stripped-down headless build", not "this is automated".
- Adding one launch flag (`disable-blink-features=AutomationControlled`) also passed
  `WebDriver`, matching Patchright's one fix.
- Net: chromedp + a real Chrome install matched or beat Patchright on every signal this test
  checks. `CHR_MEMORY` and `HEADCHR_UA` failed under both — this test does not fully resolve
  stealth, under either approach.

**Caveat carried forward, not resolved**: this conflates two variables — driver library and
browser binary/channel. Playwright can also run against a real Chrome via `channel: "chrome"`
rather than its bundled build; that combination was not tested. The conclusion that stands
regardless: chromedp was not the weaker option the original proposal assumed, and a Go-native
driver removes a second language, a second process, and a second deploy artifact for no
measured stealth cost on this test.

**Alternative considered and rejected**: keep the Python sidecar for its richer,
already-built Playwright-level API (react-select verification, upload-chooser waits,
submission text-marker matching — all things the reference implementation, ApplYourself,
had already worked out). Rejected because `internal/autoapply`'s `SidecarClient` interface
already isolates this decision behind one seam — nothing upstream of it cares which
implementation is behind it — so there is no compounding cost to building that plumbing
directly against `chromedp`'s lower-level API instead of inheriting it, and doing so avoids
the deploy cost entirely. The plumbing itself (§ below) still has to be built either way.

### `internal/atsapply` is a new Go package, not code inside `cmd/auto-apply`

Mirrors `internal/applyform`'s own split: ATS-specific logic (DOM scan, reconcile, resolve,
fill, submit, per-provider quirks) lives in an `internal/` domain package with no pgx/Fiber
dependency, testable with fixture HTML and no browser in unit tests; `cmd/auto-apply` only
wires it in as the `autoapply.SidecarClient` implementation. **Alternative considered**: put
it directly in `cmd/auto-apply` since (post-revision) there is only one caller. Rejected —
the domain/wiring split is the established convention in this codebase (`applyform`'s
fetchers, `jobtracking`'s repository), and `internal/atsapply`'s scan/reconcile/resolve
logic is exactly the kind of pure, fixture-testable logic that convention exists for.

### `auto_apply_queue`'s `Store` port mirrors `applyform.Store` but is not the same interface

Same shape (`Claim`, a success write, a failure write), but a distinct Go interface in a new
package (`internal/autoapply`), because the success write differs in substance:
`applyform.Save` persists a captured form; this change's success write calls
`MarkJobApplied` and marks the queue row done in the same transaction (mirroring
`LockJobForApply`'s existing serialization, so a double-claim of the same row can't
double-submit — see the spec's "never twice" requirement; verified by an integration test
against a real Postgres). **Scope of that guarantee, found by a PR review pass**: it closes
the double-claim/concurrent-worker race, not every path to a duplicate. A crash between
`SidecarClient.Submit`'s browser click actually succeeding and this transaction committing
leaves the queue row still claimed and un-recorded; once its lease expires a later run
reclaims it as an ordinary pending attempt and can submit again for real. Closing that
window would need the employer's platform to expose its own idempotency or a way to check
"did this candidate already apply" before submitting — none of Greenhouse, Lever or Ashby
does (see proposal.md's Why) — so it stays an accepted, unclosed gap, not a guarantee this
change actually provides. **Alternative considered**: generalize `applyform.Store` into a
shared outbox-with-side-effect interface both packages implement. Rejected as premature — two
implementations is not yet a pattern worth abstracting (AGENTS.md: no infrastructure before a
concrete second need beyond this one), and `internal/outbox.RunPool` itself is already the
shared part. (Already built as described.)

### `parked` is a queue-row marker, never a `user_jobs.stage`

Per the spec's requirement that an unresolved attempt not move the tracked stage: the queue
row's own `blocked_at`/`unmapped` columns are the only place "could not resolve" is recorded.
`user_jobs` is touched exactly once, on success, through `MarkJobApplied` — never on a park.
This keeps the controlled stage vocabulary (`internal/userjob/stages.go`) exactly as it is
today; nothing here adds a value to it. (Already built: the implementation uses
`claimed_at`/`failed_at`/`blocked_at` timestamp columns rather than a literal `status` enum
column, following `apply_form_outbox`'s existing convention more closely than this document's
earlier draft assumed — same semantics, `blocked_at IS NOT NULL` in place of `status =
'blocked'`.)

### DOM is scanned live, per attempt; nothing about the form is persisted by this change

Already decided in the brainstormed design (see linked doc) and restated here because it
drives the data model: `auto_apply_queue` stores no field-level schema, only the attempt's
identity, status, and (on `blocked`) the `unmapped` reasons returned for that one run. A
later attempt for the same job re-scans from scratch. Unaffected by the chromedp revision —
true whether the scan happens in a Python process or a Go one.

## Migration Plan

- One additive migration: `auto_apply_queue` (new table only; no existing table altered) —
  done.
- No new deployable service. `cmd/auto-apply` is one more binary in the existing worker
  fleet, needing a Chrome/Chromium binary on the host (the equivalent requirement a Python
  sidecar would also have carried, for its own bundled or system browser) — not a new class
  of host dependency.
- `cmd/auto-apply` ships disabled-by-default in practice: it only ever processes rows that
  exist in `auto_apply_queue`, and nothing in this change writes to that table (population is
  out of scope), so deploying it is inert until a future change starts enqueueing.
- No rollback beyond the normal revert-and-redeploy — the new table and worker have no
  consumers elsewhere in the codebase to leave dangling.

## Risks / Trade-offs

- **[Risk]** Datacenter IP reputation may cause blocks independent of browser-fingerprint
  cleanliness (carried over from both spikes; no mitigation designed in this change) →
  **Mitigation**: none yet; read `blocked`/`failed` rates with this in mind before assuming
  they are answer-resolution gaps. A residential/mobile proxy is a config change to
  `internal/atsapply`'s launch options if this turns out to matter in practice — not designed
  now, against a guess.
- **[Risk]** Neither Patchright nor chromedp's one stealth flag passes every headless-tell
  check on the test page used (`CHR_MEMORY`, `HEADCHR_UA` fail under both) → **Mitigation**:
  none in this change. Further launch-option hardening is deferred until real submit volume
  against real ATS boards (not a generic bot-detection test page) shows whether it matters.
- **[Risk]** Lever renders a captcha on every posting, so every Lever attempt parks in this
  change's scope (no captcha-solving path exists) → **Mitigation**: expected and acceptable
  for v1 per the spec's "submit only when fully resolved" requirement; solving it
  interactively is the deferred extension-fallback work.
- **[Trade-off]** Live DOM-scanning on every attempt instead of persisting a reconciled
  schema costs a browser session per attempt even for a job seen before → accepted for this
  change's narrow scope; `internal/applyform`'s stored-schema path remains available to build
  on later without this change needing to anticipate it.
- **[Trade-off]** Building `internal/atsapply`'s fill/verify plumbing (react-select-equivalent
  selection assertion, upload confirmation, submit text markers) directly against chromedp's
  lower-level API means not inheriting the reference implementation's already-built
  Playwright-level version of the same logic → accepted: the plumbing is a fixed cost either
  way (Python or Go), and paying it in Go avoids the sidecar/deploy cost entirely.
