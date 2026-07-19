## Context

`clinch.go` reconstructs postings from sitemap slugs only, because ClinchTalent
fronts detail pages with an AWS-WAF Challenge action. A spike (memory
`hire-clinch-awswaf-challenge-spike`) measured the gate precisely on
`careers.withwaymo.com`:

- It is **rate-based**, not a blanket block. A cold IP (residential *and* the
  prod datacenter IP `89.167.94.146`) is served ~6 clean HTTP 200 full pages
  (each ~126 KB, with `div.job-description`) before the WAF flips to HTTP 202.
- The 202 shell (~2464 B) carries `x-amzn-waf-action: challenge`, `gokuProps`,
  and the `AwsWafIntegration.getToken()` loader. Its penalty window is long;
  pacing does not recover a tripped IP within a run.
- The free token-solver path was rejected: it works from `sa-east-1` but fails
  deterministically from the prod EU region (region-specific `challenge.js`
  build), i.e. a maintenance treadmill.

The reusable seam already exists: `pacer.go` `rateLimitedHTMLGetter` +
`pacedCareerPageGetter/Vagas/HH`, each a thin constructor over a shared
`golang.org/x/time/rate` limiter. The transport (`http.go` `Client.do`) is where
the 202 classification bug lives.

## Goals / Non-Goals

**Goals:**
- Harvest clinch descriptions where the per-run WAF budget allows, with zero new
  dependency, proxy, or config.
- Make the challenge-aware layer reusable — any AWS-WAF-challenged source can opt
  in later.
- Guarantee non-regression: clinch output is a strict superset of today's.

**Non-Goals:**
- Solving the WAF challenge / minting `aws-waf-token` (free solver rejected;
  CapSolver deferred).
- Full description coverage in a single run — partial, back-filling coverage is
  the accepted outcome.
- Recovering a tripped IP mid-run, or per-tenant token/cookie sessions.

## Decisions

**1. Detect the challenge in the transport, surface a typed `ChallengeError`.**
`Client.do` gains a check *before* the `2xx` success branch: if
`resp.Header.Get("x-amzn-waf-action") == "challenge"`, close the body and return
`&ChallengeError{URL: ...}` (a small typed error next to `StatusError`).
Matching on the header rather than the bare `202` status is precise — 202 is a
legitimate success elsewhere, and the header is AWS's own challenge marker.
Return immediately (no retry): a challenge will not clear on a fixed backoff and
retrying only deepens the penalty. *Alternative considered:* treat 202 as an
error generically — rejected, it would mis-handle any legitimate 202.

**2. Reuse `rateLimitedHTMLGetter`; add a `pacedClinchGetter` constructor.** No
new abstraction — the challenge-awareness lives entirely in the transport
(decision 1), so the pacing layer is exactly the existing wrapper with a
clinch-tuned interval. *Alternative considered:* a bespoke "challenge-aware
getter" type — rejected as redundant once `do` surfaces `ChallengeError`, which
propagates through `GetHTML` unchanged.

**3. Conservative starting interval, tuned from convergence.** Start
deliberately gentle (the WAF trips at ~6 rapid requests and punishes hard), in
the spirit of the `careerspage`/`vagas` comments ("under-shooting only lengthens
a run; over-shooting re-introduces the penalty"). The exact safe sustained rate
could not be measured (spike poisoned both test IPs), so the latch (decision 4)
is the real safety net, not the interval.

**4. One-way latch-off on the first `ChallengeError`.** clinch iterates sitemap
entries; a small `hydrator` helper holds a `latched bool`. Once a detail fetch
returns a `ChallengeError`, set `latched` and skip all further detail fetches
this run, emitting the remaining postings sitemap-only. Mirrors
`tyomarkkinatori.detail(...) rateLimited`. *Alternative considered:* keep trying
every posting — rejected, it wastes requests and prolongs the penalty.

**5. Only fetch detail for postings that need it.** clinch's description is empty
today; the hydrator fetches detail per sitemap entry during the run. Because the
pipeline skips already-seen postings, a mostly-static board (e.g. Waymo)
back-fills across runs: each run spends its ~budget on the newest/first N
entries. This is acceptable for slowly-changing boards and needs no persistence.

## Risks / Trade-offs

- **Partial coverage** → Accepted and documented; the latch + per-run budget
  means a large board hydrates a slice per run. Not a regression (empty
  description is today's state for 100%).
- **WAF trips early, near-zero descriptions** → Non-regressing by construction
  (latch → sitemap-only). Interval is tunable from prod `board_health` / observed
  description-fill rate without code changes to the contract.
- **202-as-success elsewhere** → Guarded by matching the `x-amzn-waf-action`
  header, not the status code, so no other source's legitimate 202 is affected.
- **Detail HTML shape drift** (`div.job-description`) → Extraction failure yields
  an empty description, never a dropped posting; covered by a spec scenario.

## Migration Plan

Pure additive code change, no schema/config. Deploy with the normal ingest
image; the next scheduled `clinch` run begins hydrating. Rollback = revert the
commit (clinch returns to sitemap-only). Tune the interval by observing the
description-fill rate over a few runs.

## Open Questions

- Final interval/burst values — start conservative, confirm from the first prod
  runs' fill rate (no contract change to adjust).
