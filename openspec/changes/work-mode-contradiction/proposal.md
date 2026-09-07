## Why

freehire#2555 reports a posting served as `work_mode = remote` that its own body calls
onsite. The example is NVIDIA's `Senior Structural Test Engineer` (JR2020330), and reading
it end to end shows two independent defects, either of which alone would have produced the
wrong facet.

**The label is the employer's own, and the body is the only thing that contradicts it.**
NVIDIA's Workday publishes `location = "US, TX, Remote"` with `remoteType = null` — a
location bucket, not a work arrangement — while the description states *"This position is
100% on-site based at either our Dallas or Houston Contract Manufacturing (CM) facility …
this role does not offer remote or hybrid arrangements."* Our work-mode precedence
(`internal/job/jobderive`) is source-order only: structured signal → location marker →
description phrase, where each lower source fills only what the higher ones left empty. A
description can therefore never disagree with a location string, only complete it. The
posting reaches the catalogue twice more through aggregators (`echojobs`, `eightfold`),
both of which copy the same "Remote" label — `echojobs` even into the title — so the
mislabel is amplified rather than corrected.

**We would not have read that sentence anyway, because we never re-read a posting.** Our
stored body for that posting is 3 773 characters against the employer's live 3 922: an
older snapshot, taken before NVIDIA added the on-site paragraph. Every hydrating adapter
fetches detail only for a posting the catalogue does not already have (`FetchNew`'s
seen-set); a posting we already hold takes the liveness-only refresh path (`Touch` /
`RefreshUnchangedJob`) and its body is never fetched again for the row's whole life. An
employer's edit — a clarification, a location change, a salary — reaches us never. The
only existing repair is `INGEST_REFETCH_ALL=1`, a manual, provider-wide, one-off pass.

Measured on prod, over 9 813 sampled `remote` postings drawn from two disjoint slices, 22
carry an unambiguous denial of remote work in their own body — about 0.2%, or roughly 340
of the catalogue's 156 626 open remote postings.

## What Changes

- **A description may CONTRADICT a remote label, not merely fill a gap.** A new,
  deliberately small dictionary of unambiguous denials (`location.RemoteContradicted`)
  overrides a `remote` work mode to `onsite` regardless of which tier produced it. It is a
  separate list from `descriptionWorkModePhrases`, which keeps its gap-filling job
  unchanged; a phrase strong enough to fill a blank is not automatically strong enough to
  overrule a stated one.
- **A hydrating crawl re-reads a posting whose stored body has gone stale.** The seen-set
  predicate that already withholds a body-less row for re-hydration gains a second arm:
  a row whose body was last fetched longer ago than `BODY_REFRESH_DAYS` is withheld too,
  so the adapter fetches its detail and the ordinary upsert re-derives every facet from
  the fresh text. A deterministic slot (`BODY_REFRESH_SLICE`) spreads the catalogue's
  re-reads across runs so no single crawl faces the whole backlog.
- **A new `jobs.hydrated_at` column records when we last held a freshly fetched body**,
  written by both write paths. `updated_at` cannot serve: `RefreshUnchangedJob` leaves it
  alone by design, so a re-fetched body that turned out identical would still look stale
  and be re-fetched on every crawl of its slot.

`BODY_REFRESH_DAYS` ships unset, which disables the second arm entirely and leaves crawl
behaviour exactly as it is today. Nothing about the re-read reaches production until an
operator sets it, one provider at a time.

## Capabilities

### New Capabilities

- `job-body-refresh`: when a hydrating crawl re-reads the body of a posting the catalogue
  already holds — what makes a stored body stale, how the re-reads are spread over runs,
  and what records that a body was read.

### Modified Capabilities

- `job-facets`: the work-mode derivation gains a contradiction tier above the existing
  precedence chain.

## Impact

- `internal/dict/location/workmode.go` — the new denial dictionary and
  `RemoteContradicted`.
- `internal/job/jobderive/jobderive.go` — the contradiction check after the precedence
  chain.
- `migrations/` — one new file adding `jobs.hydrated_at`.
- `internal/platform/db/queries/jobs.sql` — `ExistingExternalIDs`,
  `ExistingExternalIDsByBoard`, `UpsertJob`, `RefreshUnchangedJob`; regenerated via
  `make sqlc`.
- `cmd/ingest/main.go`, `cmd/ingest/store.go` — the two new knobs and the widened
  seen-set predicate.
- `AGENTS.md` — the `cmd/ingest` worker note gains the two knobs.

Not in scope: the aggregators' own copies (`echojobs`, `eightfold`) carry the same wrong
label from the same origin and are corrected by the same contradiction check, since it
runs in the shared derivation rather than in any adapter. Recovering `echojobs`' outbound
employer URL, and re-reading bodies for NON-hydrating sources (which already rewrite them
every crawl), stay out.
