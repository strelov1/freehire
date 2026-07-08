## Why

We already compute how well a candidate's skills cover the live market (the
`resume-verdict` capability), but it is cookie-only and reads its skills from a
stored profile + parsed CV. An agent driving `freehire-cli` holds only a flat
list of skills and no browser session, so it cannot use it. We need a stateless,
API-key endpoint that takes a skill list plus a market filter and returns the
coverage.

## What Changes

- New `POST /api/v1/market/coverage` endpoint (behind `RequireAuthOrKey`): body
  carries the caller's `skills`; the market filter is the full facet vocabulary
  as query params. Returns the market-coverage result (total, covered, coverage
  percent, ranked skill gaps, and the role's top in-demand skills scored against
  the passed skills).
- Refactor: extract the shared "role filter + skill sets → three facet queries →
  `verdict.Compute`" step so the new endpoint and the existing CV verdict share
  one implementation. Behavior of the existing verdict is preserved.
- `coherence_percent` (a CV-section metric) is omitted from the stateless
  response — a flat skill list has no declared/body sections.
- `freehire-cli`: new `market-fit` command and full facet-filter support (a
  generic `--facet key=value` plus high-traffic named flags), extended to the
  `search` command too. *(Consumer of the API; lives in the separate
  `freehire-cli` repo.)*

## Capabilities

### New Capabilities
- `market-coverage`: a stateless, API-key endpoint that scores a supplied skill
  list against the live open-vacancy market for a facet-filtered role and returns
  coverage, ranked skill gaps, and the role's top in-demand skills.

### Modified Capabilities
<!-- None. The resume-verdict refactor is behavior-preserving (implementation
     detail), so its requirements do not change. -->

## Impact

- **New:** `internal/handler/market_coverage.go`, route in
  `internal/handler/handler.go`.
- **Refactored (behavior-preserving):** `internal/handler/resume_verdict.go`
  (extract shared `coverageFor`).
- **Reused unchanged:** `internal/verdict`, `internal/search` (filter vocabulary),
  `buildSearchFilter`.
- **Client (separate repo):** `freehire-cli` — new `Coverage` client method,
  `market-fit` command, generic `--facet` flag on `market-fit` + `search`.
- No DB migration, no reindex — pure read/compute over the live search index.
