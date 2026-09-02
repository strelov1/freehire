# Design

## The approach this change does NOT take, and why

The obvious fix for a false company-identity match is to stop matching on identity-by-name and
match on the board instead: read the aggregator posting's apply URL, run it through
`internal/ingest/atsboard` to get `(source, board)`, and count the company covered only when
we already crawl that exact board. `atsboard` is pure, network-free, and already shared by
three consumers, so this looked like the cheap correct answer.

**It was measured and it does not work.** No aggregator hands us an ATS URL:

```text
himalayas.app/jobs/api, applicationLink field, 20 of 20 postings:
  https://himalayas.app/companies/<slug>/jobs/<slug>       — equal to the posting's own guid

jobs.url as stored on prod, 20 postings each:
  himalayas → himalayas.app      remotive       → remotive.com
  remoteok  → remoteok.com       nodesk         → nodesk.co
  weworkremotely → weworkremotely.com           jobicy → jobicy.com
  workingnomads → workingnomads.com             remotedotcom → remote.com
  jobspresso → jobspresso.co                    arbeitnow → arbeitnow.com
```

An aggregator's business is the click, so its feed points at its own page by construction. The
real ATS link (`job-boards.eu.greenhouse.io/finnapp/jobs/4837468101` in the issue's second
report) lives one HTTP request behind that page — a detail fetch per posting across a 102,326-
posting catalogue, for a source that today takes its bodies inline and makes no detail call at
all. Recording this here because "just use the apply URL" is the first idea anyone has.

There is no cheap identity signal in an aggregator feed. What there IS, is a way to stop
trusting a coverage claim that has gone stale.

## Coverage definition

A company is covered when it has at least one posting where all of:

- `closed_at IS NULL`
- `source` is not in `sources.AggregatorProviders(sources.Taxonomy())`
- `last_seen_at > now() - coverageFreshness`
- `NOT is_private`
- `company_slug_folded` equals the asked slug's fold

The freshness clause is the change. `NOT is_private` is a clause the search-backed lookup got
for free and this one has to state: `cmd/reindex` drops `is_private` rows from the index
entirely, so the old gate could never see one. A private posting is the jd-tailor-intake path —
a job description a single user pasted in, visible only to them and crawled from nowhere — so
it can never be evidence that we still crawl an employer, and counting it would let one user's
pasted JD for "Acme" silently discard every aggregator posting for every other Acme. The
index's other exclusions (`duplicate_of`, unresolved category, missing body) are NOT mirrored:
each of those is still a posting crawled from the employer's own board, absent from the index
because it is not worth SEARCHING, which is a different question. The last line restates today's behaviour in the form
Postgres can express directly — and collapses it: `company_slug_folded` is
`replace(company_slug,'-','')` written by the same `UpsertJob` that writes `company_slug`
(verified on prod 2026-09-02: zero open rows with a non-empty slug and a NULL fold), so an
exact match implies a folded match and the exact clause is redundant. `jobs.sql` already has
precedent for filtering on the stored column directly (`company_slug_folded = ANY(...)` in the
aggregator-suppression queries).

## Why Postgres, not Meilisearch

`last_seen_at` cannot be carried by the search index, and this is structural rather than an
omission:

- The incremental drain pushes a document only when its `content_hash` moved
  (`internal/search/searchdrain`). `last_seen_at` is not in that hash.
- The write that stamps it on the common path is `RefreshUnchangedJob`, which by design
  "writes NOTHING else" and enqueues nothing. A crawl that re-sees an unchanged posting
  therefore produces no index push at all.

So an index field would freeze at whatever the last content change wrote, and would be most
wrong for exactly the actively-crawled rows the gate needs to credit. The lookup has to read
the table.

Moving to Postgres also removes a workaround rather than adding one. Meili's filter language
matches a stored value literally and cannot compute `replace(company_slug,'-','')` at query
time, which is why the current implementation stores a second field and ORs two `IN` clauses
over it, then walks the batch by hand to avoid crediting slugs the folded clause dragged in.
In SQL the fold is a stored column with its own partial index and the whole apparatus goes away.

**Do not add an index on `last_seen_at`.** `RefreshUnchangedJob`'s comment states the column is
deliberately in no index so the update stays heap-only on the hottest write path in ingest. The
lookup does not need one: `jobs_open_company_slug_folded_col_idx` selects the few open rows per
slug, and the freshness test is a heap recheck over that handful. The query is written as a per-slug `EXISTS` so a large
employer short-circuits on its first fresh row instead of aggregating thousands.

## Why 14 days

The ingest sweep closes a posting unseen for 48h (`sources.DefaultSweepGrace`), so 48h is the
tempting constant. It is the wrong one, because the sweep and the gate ask different questions:
the sweep asks "is this posting still on its board?", the gate asks "do we still crawl this
employer at all?". A board can go legitimately uncrawled for much longer than a posting can go
legitimately unlisted — the fleet skips runs.

Measured on prod 2026-09-02 (1% table sample of open postings), share of rows seen within each
window:

| source | sampled | ≤48h | ≤7d | ≤14d | ≤30d |
|---|---|---|---|---|---|
| workday | 11,127 | 73.6% | 85.1% | **92.8%** | 99.5% |
| smartrecruiters | 2,736 | 74.2% | 90.5% | **90.5%** | 91.8% |
| greenhouse | 2,015 | 98.5% | 99.2% | **99.2%** | 99.4% |
| teamtailor | 827 | 99.4% | 99.8% | **99.8%** | 99.9% |
| lever | 697 | 98.0% | 98.0% | **98.3%** | 98.9% |
| ashby | 687 | 97.5% | 98.1% | **98.1%** | 98.7% |
| recruitee | 659 | 99.7% | 99.7% | **99.7%** | 100% |
| bamboohr | 641 | 96.3% | 96.3% | **97.0%** | 98.4% |
| workable | 576 | 97.6% | 97.9% | **97.9%** | 98.1% |
| trakstar | 61 | 96.7% | 98.4% | **98.4%** | 98.4% |

At 48h the largest provider by volume loses a quarter of its live rows to crawl jitter — the
gate would stop crediting real coverage on 43,649 slugs, most of them correctly covered. At 14
days the worst provider still carries 92.8% and every other carries 97-99.7%, while the window
is far wider than any plausible cadence and so what falls outside it is a board that has
genuinely stopped being crawled.

30 days was considered and rejected: it recovers only 11,151 of the 22,022 zombie slugs, and
the measurement shows nothing between 14 and 30 days that needs the extra room (workday moves
92.8% → 99.5%, i.e. the remaining 7% are not jitter, they are rows that stopped being seen).

The constant is deliberately not an env var. A per-run override would make the gate's behaviour
depend on how a cron happened to be invoked, and the number is a property of the fleet's crawl
cadence, which the code should state once and be corrected in when it is re-measured.

## Direction of error

The two ways this gate can be wrong are not symmetric, and the design leans on that:

- **Claiming coverage wrongly is unrecoverable.** The posting is never written, so it is never
  in the database, never in `/find`, never in search, and it leaves no trace anyone can query.
  It is not even distinguishable from "the aggregator never listed it".
- **Missing coverage is recoverable.** A duplicate row is written and the periodic
  `aggregator-ats-dedup` pass marks it, on the nightly schedule it already runs.

Every judgement call therefore resolves toward saving. A wide freshness window, a lookup error
answering "nothing is covered" (unchanged from today), and a nil lookup disabling the gate are
all the same rule.

## Load

One query per board run, in the batched path exactly where the Meili call is today. Himalayas'
~2,000-posting run resolves to on the order of 1,500 distinct slugs, batched the same way.

`EXPLAIN (ANALYZE, BUFFERS)` on prod 2026-09-02 over a batch of 500 real company slugs confirms
the shape the design assumes:

```text
Nested Loop Semi Join
  -> Function Scan on unnest asked                                    (500 rows)
  -> Index Scan using jobs_open_company_slug_folded_col_idx  (loops=500)
       Index Cond: (company_slug_folded = asked.folded)
       Filter: (source <> ALL (...)) AND (last_seen_at > now() - '14 days')
       rows=0.91 per loop, Rows Removed by Filter: 10
```

One index search per company, ~11 open rows touched each, the freshness test a heap recheck
over exactly those — never a scan, and no index on `last_seen_at` involved. The batch cost
525ms of cold I/O for 500 companies; the same query against a warm cache is a fraction of that,
and it runs once per board run rather than per posting.

The
streaming path (`jobtech`) and the `CoverageGated` probe (`remotedotcom`) keep their existing
shapes — memoized single-slug and mid-crawl batch respectively — since only the port's backing
store changes, not its interface.
