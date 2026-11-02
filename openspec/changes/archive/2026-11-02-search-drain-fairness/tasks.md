## 1. Fair queue scheduling
- [x] 1.1 Reproduce starvation, short-wave loops, separate budgets, and deferred-wave accounting in tests.
- [x] 1.2 Implement alternating waves and wire the command; preserve timeout/fallback behavior.

## 2. Reopening safety
- [x] 2.1 Reproduce reopened-before/during/after-deletion behavior against real Postgres and the deletion adapter.
- [x] 2.2 Filter deletion candidates, atomically repair reopened acknowledgments, enqueue TouchJob reopens, and regenerate sqlc.

## 3. Closure status updates
- [x] 3.1 Reproduce status hiding, existing-only updates, vector preservation, counts, and reopening with a real engine.
- [x] 3.2 Implement status updates and mandatory filtering across search, facet/count, and sitemap paths; retain hard removal for missing/private/noncanonical jobs.

## 4. Delivery
- [x] 4.1 Update affected conventions and simplify the final diff.
- [x] 4.2 Run unit/integration checks and review the final implementation.
- [x] 4.3 Commit the isolated change, open a PR, and archive/sync the specification.
