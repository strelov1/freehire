## Why

Ashby and Workable postings already resolve a fully-answered `Plan` through the existing
deterministic pipeline (`applyform.Fetcher` → `Reconcile` → `Resolve`/
`ResolveWithDrafting`) — `internal/api/atsapply`'s own doc says a fully resolved form for
one of these platforms "still parks rather than being submitted through a fill path never
built or verified," because only Greenhouse has a hand-written chromedp DOM-fill path.
Writing per-provider chromedp selectors for each platform is the alternative, and it is
exactly the maintenance burden a live spike found browser-use.com's cloud agent avoids:
it can navigate and interact with an arbitrary live ATS page from a natural-language
instruction, with no per-provider selector code at all.

(Recruitee was initially assumed to be a third eligible provider; found false during
implementation — see the excluded-scope list below.)

## What Changes

- A new package `internal/platform/browseruse` — pure HTTP transport for the browser-use
  cloud API v4 (create a run, poll its status, fetch its result), following
  `internal/platform/llm`'s "transport, not domain" convention.
- `internal/api/atsapply` gains a second `SidecarClient` execution backend: given an
  ALREADY fully-resolved `Plan` (never letting browser-use choose a field's value — this
  preserves the existing sensitive-keyword gate, geography park rule, and "never guess"
  discipline unchanged), it builds a precise fill instruction, runs it via
  `internal/platform/browseruse`, and requires the agent's final report to end with one
  of three strict machine-parseable markers (`CONFIRMED:`, `UNCONFIRMED`, `PARKED:`)
  rather than inferring success from free text.
- `Client.Submit` gains exactly one new branch: when `Plan.FullyResolved()` is true, the
  provider is `ashby` or `workable`, and the browser-use executor is enabled, it executes
  through browser-use instead of returning `StatusParked` for "no fill path." Every other
  case (Greenhouse, white-label, captcha-protected, Recruitee, or browser-use
  unconfigured) is unchanged.
- An env-gated enforce flag, shipping OFF (shadow: logs what it would have attempted),
  mirroring `PLAN_ENFORCE` and the `add-auto-apply-eligibility-gate` rollout, plus a
  per-run `maxCostUsd` cap (a native browser-use v4 field) and a daily aggregate spend
  threshold `cmd/auto-apply` checks before invoking it.

**Explicitly out of scope** (found during design/implementation, deferred to later changes):
- Recruitee — found during implementation to be structurally unreachable, not merely
  deferred: `internal/ingest/applyform.Fetchers` has no registered `Fetcher` for it at
  all (its form arrives free with the ingest crawl and is written directly), so
  `Client.fetchSchema` already parks a Recruitee attempt with `errNoSchemaFetcher` long
  before `Submit` ever reaches a resolved `Plan` to hand this executor. Reaching it would
  need reading its already-captured form from storage instead of `applyform.Fetcher` — a
  real gap, not a decision.
- White-label/unrecognized-layout Greenhouse postings — chromedp's DOM scan fails there,
  so there is no `MergedField` schema to build a `Plan` from at all; closing this gap
  needs a two-phase (scan-then-fill) design this change does not attempt.
- CAPTCHA-protected postings (Lever's captcha short-circuit, `reasonCaptchaProtected`) —
  never routed to browser-use; that would be an attempt to bypass a platform's bot
  protection, not this change's purpose.
- Résumé/CV file upload through browser-use — only individual resolved field VALUES
  (name, email, phone, etc.) ever leave the process for this executor, never the CV file.
  A form whose only remaining unmapped field is the résumé upload still parks.
- A local/self-hosted browser-use runtime (the open-source Python library) — rejected for
  the same reason `internal/api/atsapply`'s own history already rejected a Patchright
  sidecar: it would add a second language, process, and deploy artifact. This change uses
  only the hosted cloud REST API, called via plain Go HTTP.

## Capabilities

### New Capabilities
- `atsapply-browseruse-fallback`: executes an already-resolved application `Plan` for
  Ashby/Workable postings via the browser-use cloud agent when chromedp has no fill path
  for that platform, under a cost cap and a strict confirmation contract.

### Modified Capabilities
(none — `Client.Submit`'s existing behavior for every other case is unchanged, and this
adds a new caller/branch rather than a new requirement on the existing pipeline)

## Impact

- `internal/platform/browseruse` (new): HTTP transport for the browser-use v4 API.
- `internal/api/atsapply`: new browser-use-backed executor file; one new branch in
  `Client.Submit`.
- `cmd/auto-apply`: reads the new enforce flag and daily spend threshold, wires the new
  executor into `internal/api/atsapply.Client`'s construction.
- `internal/api/atsapply/AGENTS.md`: documents the new executor, its scope boundaries,
  and why it does not conflict with the package's existing "chromedp, not a
  Python/Patchright sidecar" decision (no second language/process is introduced).
- New env vars: a browser-use API key/base URL, the enforce flag, the per-run cost cap,
  and the daily spend threshold.
- No schema changes.
