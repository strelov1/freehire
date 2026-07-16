## ADDED Requirements

### Requirement: Rate-paced crawl for full single-run coverage

The `careerspage` crawl SHALL hold its aggregate outbound request rate under a bounded
limit shared across all of a run's requests — the paginated listing and every posting's
detail fetch, across every configured board — so that one run stays within
careers-page.com's per-IP rate-limit window and collects every posting. The rate cap MUST
be independent of the detail worker-pool size, so that widening or narrowing concurrency
does not change the request rate. This prevents an earlier or larger board from spending
the window budget and starving later boards in the same run.

#### Scenario: Later boards do not starve behind an earlier board

- **WHEN** a run crawls several careerspage boards over one shared egress and an earlier
  board issues many detail requests
- **THEN** the aggregate request rate stays under the limit and later boards still collect
  their full set of postings in the same run, rather than being rate-limited to a partial set

#### Scenario: Rate is governed by the pacer, not the worker pool

- **WHEN** the detail fan-out runs under its bounded worker pool
- **THEN** the outbound request rate is bounded by the shared pacer regardless of the pool
  size, so no burst exceeds the window budget
