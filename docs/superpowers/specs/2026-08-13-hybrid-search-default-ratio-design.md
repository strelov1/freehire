# Enable hybrid search by default on the main jobs search

## Problem

The main jobs search bar (`SPA` "search" tab, `searchJobs()` in `web/src/lib/api.ts`)
hardcodes `semantic_ratio=0`, forcing pure keyword search even though the backend and
Meilisearch index support blending in semantic (embedding-based) ranking. All
infrastructure for hybrid search is already live and unused by default:

- `jobs_semantic` Meili index has 100% embedding coverage (1,021,471 / 1,021,471 docs
  as of 2026-08-13).
- `semantic_ratio` is wired end-to-end: SPA → `/api/v1/jobs/search?semantic_ratio=` →
  `internal/handler/search.go` → `search.SearchParams.SemanticRatio` →
  `internal/search/client.go`'s `Search`.
- Already used elsewhere (unaffected by this change): `/jobs/:slug/similar` and
  `/recommendations`, both pure semantic (ratio effectively 1) for their own use case.

## Constraint discovered via live testing (2026-08-13)

Querying prod (`https://freehire.me/api/v1/jobs/search?q=devops&semantic_ratio=<r>`)
found a hard cliff, not a gradual degradation:

| ratio | total hits for "devops" |
|---|---|
| 0.0 – 0.5 | ~28,800–29,700 (comparable to keyword-only) |
| 0.5 → 0.55 | jumps to 1,021,471 — literally every doc in `jobs_semantic` |
| 0.55 – 1.0 | stays at the full-catalogue count |

Above 0.5, hybrid ranking stops filtering anything — every embedded document scores
above zero and is returned, matching the risk flagged in the existing code comment
(`web/src/lib/api.ts`): a naive full-semantic blend makes a short exact query like
"devops" return the whole catalogue reranked by similarity, reading as "search is
broken."

No case was found in ad hoc testing (`devops`, `ml engineer`) where a low ratio (0.2–0.3)
visibly surfaced different top results than keyword-only — those queries already had
exact keyword matches, so hybrid had nothing to add. A synonym-style query (e.g. "SRE"
vs "site reliability engineer") was not tested; the value-add of blending at low ratio
is assumed, not proven, but the safety ceiling is measured.

0.5 itself does not trip the cliff, but it sits with zero margin against a threshold
that is a property of today's data/model and could shift (catalogue growth, embedder
model migration — see `internal/embed/AGENTS.md`). It also was not evaluated for
ranking-order quality (only total-hit count), so it would be a much larger behavior
change than its "still under the cliff" status suggests. Chose the smaller, safer nudge
instead — see Decision.

## Decision

Flip the default from `0` to a fixed **`0.2`** for the main search only — a wide safety
margin below the measured 0.5 cliff, and a small enough blend that it nudges rather
than reorders results. No config flag, no env var — a bare constant, changed the same
way it is today (`params.set('semantic_ratio', '0')` → `'0.2'`), because there is
exactly one call site and no current need to tune it without a code change (YAGNI).

Out of scope: `/similar`, `/recommendations`, the agent search endpoint
(`/agent/jobs/search`, which already forwards whatever ratio a caller passes and needs
no change) — none of these use `searchJobs()`.

## Rollback

One-line revert (`'0.2'` → `'0'`) if search quality regresses after shipping — no
migration, no data change, nothing to undo on the Meilisearch side.

## Testing

- Manual: repeat the prod curl comparison (`?q=<query>&semantic_ratio=0` vs `=0.2`)
  for a handful of real queries post-deploy, including at least one synonym-style query,
  to confirm 0.2 doesn't visibly worsen relevance for exact matches and ideally
  improves recall for near-matches.
- No new automated test: this is a single literal change to an existing, already-tested
  parameter path (`SearchJobs` handler and `search.Client.Search` are covered by
  existing tests); the risk is search *quality*, which automated tests in this repo do
  not assert on.
