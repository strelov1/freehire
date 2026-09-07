## 1. Transport package (`internal/platform/browseruse`)

- [x] 1.1 Define the client type: base URL (default `https://api.browser-use.com/api/v4`,
      overridable for tests), API key, HTTP client, sane timeouts
- [x] 1.2 `CreateRun(ctx, task string, maxCostUSD float64) (runID string, err error)` —
      `POST /runs` with `{"task": ..., "maxCostUsd": ...}`
- [x] 1.3 `PollStatus(ctx, runID) (status string, err error)` — `GET /runs/{id}/status`
- [x] 1.4 `GetResult(ctx, runID) (result RunResult, err error)` — `GET /runs/{id}`,
      mapping `status`, `result` (text), `error`, `totalCostUsd`
- [x] 1.5 `Wait(ctx, runID, pollInterval, timeout) (RunResult, error)` — polls status to a
      terminal state or the given timeout, then fetches the full result once
- [x] 1.6 Unit tests against a `httptest.Server` fake: create/poll/get happy path, a
      non-2xx response surfaces as an error, `Wait` times out cleanly on a run stuck
      `running`/`queued`
- [x] 1.7 (found during implementation) registered `browseruse` in
      `internal/platform/arch/layering/blocks.go`'s platform block — required by the
      layering guard for any new package, not called out as its own task originally

## 2. Browser-use executor in `internal/api/atsapply`

- [x] 2.1 `buildTask(plan Plan, merged []MergedField, applyURL string) string` — pure
      function enumerating every `plan.Fields` entry's label (from `merged`, since
      `ResolvedField` itself carries only the opaque id)/id and exact value, plus the
      fixed "do not act on anything outside this list, do not guess, do not submit unless
      every listed field succeeds, end your report with CONFIRMED: <what you observed> /
      UNCONFIRMED / PARKED: <reason>" instruction block
- [x] 2.2 `parseOutcome(report string) (outcome, detail string)` — pure function
      extracting exactly one of confirmed/unconfirmed/parked from the report's trailing
      marker; any report with no recognizable marker (or the marker not on the last
      non-empty line) returns unconfirmed
- [x] 2.3 `browserUseExecutor.submit` implementing this backend's half of `Submit`:
      `buildTask` → `browseruse.CreateRun` (with the configured per-run `maxCostUsd`) →
      `Wait` → `parseOutcome` → map to `autoapply.StatusApplied` / `StatusUnconfirmed` /
      `StatusParked`; also added `browserUseEligible` (provider + no file-kind field in
      the resolved plan — a résumé-carrying plan for an eligible provider still falls
      through to the ordinary park, per design.md's résumé exclusion) and the
      `dailySpendGuard`/env-config helpers task 3.1-3.2 called for, since they're the
      same file and inseparable from `submit`'s own behavior
- [x] 2.4 Unit tests: `buildTask` output contains every field's exact label/id/value and
      the required instruction clauses; `parseOutcome` correctly classifies a
      confirmed/unconfirmed/parked report, defaults unrecognized text to unconfirmed, and
      rejects a marker not on the report's last line; `browserUseEligible` table-driven
      over provider/file-field combinations; `dailySpendGuard` shadow-vs-enforce behavior

## 3. Wiring into `Client.Submit` and `cmd/auto-apply`

- [x] 3.1 Env-gated enforce flag (`AUTO_APPLY_BROWSERUSE_ENFORCE`), a plain per-call read
      (not memoized), mirroring `add-auto-apply-eligibility-gate`'s fix for the same
      footgun (`sync.OnceValue` breaking `t.Setenv`-based tests) — implemented alongside
      task 2.3 in `browseruse_fill.go` since it's inseparable from the executor's own
      behavior; wired into `Client.Submit`'s gate here
- [x] 3.2 Daily aggregate spend threshold (`AUTO_APPLY_BROWSERUSE_DAILY_CAP_USD`) — also
      landed with task 2.3's `dailySpendGuard`
- [x] 3.3 New branch in `Client.Submit`: when `Plan.FullyResolved()`, provider ∈
      {ashby, workable}, and the executor is configured+enabled+enforced, call the
      browser-use executor instead of returning `StatusParked` for "no fill path";
      unaffected otherwise. **Recruitee dropped from scope, found during this task**:
      `internal/ingest/applyform.Fetchers` has no registered `Fetcher` for it at all, so
      `Client.fetchSchema` already parks a Recruitee attempt with `errNoSchemaFetcher`
      before `Submit` ever reaches a `Plan` to hand this executor — it was never
      reachable, not merely out of scope. Corrected in `browserUseProviders` and in
      proposal.md/design.md/spec.md.
- [x] 3.4 Wired `internal/platform/browseruse`'s client and `NewBrowserUseExecutor` into
      `cmd/auto-apply/main.go`'s construction of `internal/api/atsapply.Client` via a new
      `Client.WithBrowserUse` builder method (added rather than changing `NewClient`'s
      positional signature, to touch none of its other call sites); added
      `AutoApply.BrowserUseAPIKey` to `internal/platform/config` for the one credential
      that must be threaded through a constructor (the rest — enforce flag, cost caps —
      stay plain env reads inside `internal/api/atsapply`, same convention as
      `add-auto-apply-eligibility-gate`'s flag)
- [x] 3.5 Fixture-based unit tests (`browseruse_submit_test.go`) against a fake
      `browseruse` HTTP transport: fully-resolved Ashby plan executes and confirms;
      unresolved plan never calls browser-use; shadow mode (enforce unset) never calls
      browser-use; an unconfigured `Client` (no `WithBrowserUse`) parks exactly as
      before. Greenhouse/captcha/Recruitee exclusion is already covered by the
      pre-existing `TestSubmit_LeverAlwaysParksOnCaptchaWithoutTouchingFetchersOrBrowser`
      / `TestSubmit_ParksHonestlyWhenNoSchemaFetcherIsRegistered` and the new
      `TestBrowserUseEligible` table (task 2.4) — no live browser needed for any of it

## 4. Documentation

- [x] 4.1 Update `internal/api/atsapply/AGENTS.md`: document the new backend, its scope
      (Ashby/Workable only — Recruitee excluded for the structural reason task 3.3 found,
      not a policy choice; fully-resolved plans only), why it does not reopen the
      "chromedp, not a Python/Patchright sidecar" decision (pure HTTP, no new
      language/process), and the confirmed/unconfirmed/parked marker contract
- [x] 4.2 Checked `internal/application/autoapply/AGENTS.md` — no change needed. The
      `SidecarClient` interface/outcome shape (`StatusApplied`/`StatusParked`/
      `StatusUnconfirmed`) is unaffected; that file already speaks of "the real
      implementations" (plural, for Store/AnswerSource/SidecarClient collectively) rather
      than claiming a single physical backend, so nothing there goes stale

## 5. Verification

- [x] 5.1 `go build ./...`, `go vet ./...`, `go test ./...` — all clean after every task
      above (re-verified after the Recruitee correction)
- [x] 5.2 `go vet -tags=integration ./...` — clean; `go test -tags=integration
      ./internal/api/handler/... ./internal/api/atsapply/...` — both pass
- [ ] 5.3 Manual shadow-mode dry run against one real Ashby/Workable posting (fake/test
      candidate data, enforce flag OFF) to confirm the logged projection matches what a
      real execution would have done, before ever flipping enforce on — **left for
      whoever deploys this**: it needs a live `auto_apply_queue` entry and a running
      `cmd/auto-apply` against a real database, which this session has neither of; shadow
      mode makes it a safe, zero-cost check to run before enforce is ever flipped on
