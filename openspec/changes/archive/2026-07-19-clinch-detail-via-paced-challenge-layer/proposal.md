## Why

`clinch` job postings carry a title and location but no description: ClinchTalent
fronts every detail page with an AWS-WAF *Challenge* action, so `clinch.go` is
sitemap-only (`Description: ""`) and every description-derived facet stays blank.
A spike (memory `hire-clinch-awswaf-challenge-spike`) established the gate is a
**rate-based** challenge, not a hard block: a cold IP is served ~6 clean HTTP 200
full pages (with `div.job-description`) before the WAF flips to HTTP 202 with
`x-amzn-waf-action: challenge`. That headroom is enough to harvest descriptions
if requests are paced and a trip is handled gracefully — no proxy, no token
solver, no new dependency.

## What Changes

- **Transport correctness fix**: `Client.do` currently classifies HTTP 202 as
  success (`>=200 && <300`), so a WAF challenge shell (~2464 B) would be parsed
  as if it were the posting. Detect the `x-amzn-waf-action: challenge` response
  and surface it as a typed `ChallengeError` instead of decoding the shell.
- **Optional challenge-aware pacing layer**: extend the existing `pacer.go`
  `rateLimitedHTMLGetter` seam with a conservative paced getter that any source
  behind an AWS-WAF challenge can opt into — the reusable "plug in where needed"
  layer.
- **clinch detail hydration**: `clinch` fetches each `/jobs/` detail through the
  paced getter and extracts the description from `div.job-description`. On the
  first `ChallengeError` it **latches off** detail hydration for the rest of the
  run (remaining postings keep the empty description they have today).
- **Non-regression guarantee**: clinch is never worse than today's sitemap-only
  behavior. Expected outcome is *partial* description coverage that back-fills
  across runs; the pacing interval is tunable from observed convergence, matching
  the `careerspage` / `hh` / `vagas` precedent.

## Capabilities

### New Capabilities
- `waf-challenge-pacing`: the sources HTTP transport recognizes an AWS-WAF
  challenge response and surfaces it as a typed error rather than content; an
  optional rate-limited HTML getter lets a source pace its requests under the
  challenge's per-IP window and react to a trip.
- `clinch-source`: the clinch adapter hydrates job descriptions through the
  paced challenge-aware getter with latch-off on a WAF trip, guaranteeing no
  regression from the current sitemap-only reconstruction.

### Modified Capabilities
<!-- None: no existing spec covers the clinch adapter or the sources HTTP transport. -->

## Impact

- **Code**: `internal/sources/http.go` (challenge detection + `ChallengeError`),
  `internal/sources/pacer.go` (paced clinch getter), `internal/sources/clinch.go`
  (detail fetch + description extraction + latch), `internal/sources/source.go`
  (wire clinch to the paced getter). Tests alongside each.
- **Behavior**: clinch postings gain descriptions where the per-run WAF budget
  allows; no change to any other source. No new dependency, proxy, or config.
- **Ops**: a full clinch run makes more requests (one detail GET per new
  posting, paced); bounded by the shared limiter so aggregate rate stays under
  the WAF window.
