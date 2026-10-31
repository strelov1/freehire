## Why

The company page's "Time a posting stays open" (`insights_company_lifetime`) counts the
wrong closures. It includes `expired` — the age rule that closes postings from sources
with no close signal at 45 days whatever the employer did, so it publishes our cutoff as
a named employer's hiring speed. And it leaves out `probe_expired` — the liveness probe
reading the source page as expired twice, which is exactly "the posting left its
source". The rollup's own comment read `expired` as "its own deadline passed", which is
not what the label means (docs/agents/job-lifecycle.md, mechanism 4).

## What Changes

- The lifetime rollup counts `unseen`, `feed_removed`, `probe_expired` and
  `employer_closed`, and no longer `expired`.
- Private postings (a member's URL import, which carries the employer's slug and can be
  closed by the orphan liveness probe as `probe_expired`) no longer count, as the company
  page's own reads already exclude them.
- A guard test pins every `closed_reason IN (...)` in `insights.sql` to one list, so the
  role pulse (#3952) and this rollup cannot drift apart. sqlc cannot share a SQL
  fragment, so the list lives once in the test and the queries are held to it.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `company-hiring-signal`: the posting-lifetime rollup's set of counted closures.

## Impact

- `internal/platform/db/queries/insights.sql` (one predicate, its comment), regenerated
  `internal/platform/db`, a new unit test and the existing integration tests.
- Values on the company Hiring tab move on the next rollup run; no shape change, no
  migration, no reindex.
