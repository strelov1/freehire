## MODIFIED Requirements

### Requirement: A job from a source with no close signal is closed by age

The system SHALL close an open job by age — once its effective posting date
(`COALESCE(posted_at, created_at)`) is older than a fixed window of 45 days, with reason
`expired` — when the job's source belongs to one of exactly two explicitly enumerated sets,
and SHALL NOT close any other job by age.

**Set one: sources with no close signal at all.** Neither a re-crawl that could stop seeing
the posting, nor a change feed, nor a posting URL a probe could reach a verdict on. `telegram`
is the only member: its stored URL is the Telegram post, which outlives the vacancy.

**Set two: swept sources whose crawl budget cannot re-reach their own tail, and whose URL a
probe cannot read.** The ingest sweep does close these sources on evidence, and keeps doing
so; the age rule covers only what that sweep structurally never revisits. A source qualifies
for this set only when BOTH hold:

- its crawl reads a bounded, recency-ordered slice of a catalogue deeper than the budget, so
  a posting that ages past the slice is never offered to the sweep again; and
- its stored URL answers the same regardless of whether the posting behind it is live — a
  network landing page, a tracking redirect, or a host that refuses automated requests — so
  no probe can produce a death verdict from it.

Membership in either set SHALL be an explicit, deliberate opt-in held in one place, never
inferred from a source's shape, so that a newly added adapter can never drift into being
closed by a guess. A source in set two SHALL NOT also be listed in set one: set one's members
are additionally excluded from the liveness probe, and applying that exclusion to a swept
source would silently remove it from a mechanism that still owns its evidence-based closes.
The system SHALL refuse to run rather than proceed when that distinction is violated.

An age close SHALL NOT itself reopen the job. A set-two source's own re-crawl MAY reopen it
by re-listing the posting, which is the ordinary reappearance path and not an exception to
this rule.

Age is the catalogue's weakest close, resting on a guess where every other mechanism rests on
evidence. The window SHALL therefore be shared by both sets rather than tuned per source, and
SHALL be biased toward under-closing: a job exactly at the boundary survives one more run.

#### Scenario: A stale Telegram vacancy is closed

- **WHEN** the liveness worker runs and an open job has `source = 'telegram'` with an
  effective posting date 46 days in the past
- **THEN** the job is closed with reason `expired`

#### Scenario: A recent Telegram vacancy is left open

- **WHEN** the liveness worker runs and an open job has `source = 'telegram'` with an
  effective posting date 44 days in the past
- **THEN** the job stays open

#### Scenario: A swept aggregator's unreachable tail is closed by age

- **WHEN** the liveness worker runs and an open job's source is a set-two member
  (`whatjobs` and its per-country markets, or `adzuna`) with an effective posting date 46
  days in the past
- **THEN** the job is closed with reason `expired`, even though the ingest sweep also covers
  that source

#### Scenario: Age does not close a job an ordinary sweep or probe covers

- **WHEN** the liveness worker runs and an open job has `source = 'greenhouse'` or
  `source = 'manual'` with an effective posting date a year in the past
- **THEN** the age rule does not close it

#### Scenario: A source listed in both sets stops the worker

- **WHEN** the liveness worker starts and a source appears both in the no-close-signal set
  and among the registered crawl providers
- **THEN** the worker refuses to run and reports the source by name, rather than closing that
  provider's postings by age

#### Scenario: A set-two entry matching no registered provider stops the worker

- **WHEN** the liveness worker starts and a set-two entry matches no registered crawl provider
- **THEN** the worker refuses to run and names the entry, so a renamed adapter — or a
  credential-gated source absent from this host's registry — is noticed rather than becoming
  a silent no-op that leaves its postings open forever

### Requirement: A source's close mechanism does not depend on a credential being present

Every set-two source is credential-gated: `adzuna` registers only when `ADZUNA_APP_ID` and
`ADZUNA_APP_KEY` are set, and each `whatjobs` market only when its publisher id is. A host
without the credential therefore builds a registry that omits the source, which would both
drop it from the age rule and admit its postings to the URL probe — two mechanisms changing
hands on the presence of an environment variable that describes crawling, not lifecycle.

The system SHALL make that condition an error rather than a silent reclassification: a
configured set-two member absent from the registry SHALL stop the worker.

This SHALL NOT be softened by resolving set-two membership against the source taxonomy
instead of the live registry. The taxonomy would restore the age rule but leave the probe
exclusion keyed to the registry, so the two halves of the decision would disagree — and the
existing guards exist precisely to keep them one decision.

#### Scenario: A missing crawl credential stops the worker rather than changing the mechanism

- **WHEN** the liveness worker starts on a host where `ADZUNA_APP_ID` is unset, so `adzuna`
  is absent from the registry while still listed as a set-two member
- **THEN** the worker refuses to run, rather than silently leaving every open Adzuna posting
  to the URL probe that cannot reach a verdict on it
