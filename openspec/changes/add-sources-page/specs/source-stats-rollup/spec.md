## ADDED Requirements

### Requirement: Per-source snapshot table

The system SHALL maintain a `source_stats` table holding, per source key, the figures the
public source catalogue reports: the raw count of that source's open postings, how many of
those the dedup pass matched to a first-party ATS posting, the de-duplicated count
Meilisearch holds for that source, and the instant the row was measured.

The table is a SNAPSHOT, not a ledger: a source's row is replaced on every run. Nothing
reads the history, so keeping one would only invite a reader to trust a figure nobody
maintains.

#### Scenario: A source's figures are replaced on each run

- **WHEN** the rollup runs twice and a source's open-posting count has changed between runs
- **THEN** `source_stats` holds exactly one row for that source, carrying the newer count
  and the newer `measured_at`

#### Scenario: A source that no longer has open postings

- **WHEN** a source that previously had open postings has none at the next run
- **THEN** its row reports zero open postings rather than being deleted, so the page can
  state "we crawl this and it currently carries nothing" instead of dropping the source

#### Scenario: A source with postings but no registered adapter

- **WHEN** the catalogue holds open postings under a source that is not a registered crawl
  adapter
- **THEN** that source gets a row too — the snapshot's spine is the registry together with
  what the scan found and what the previous snapshot held, not any one of them alone

#### Scenario: A non-adapter source whose last posting has just closed

- **WHEN** a source that is in neither the registry nor this run's scan appears in the
  PREVIOUS snapshot
- **THEN** it still gets a row, carrying a freshly measured zero rather than disappearing —
  otherwise the same silent drop the union exists to prevent happens one closure later

### Requirement: The two job counts are measured together

The raw open-posting count and the de-duplicated Meilisearch count SHALL be measured
within the same run and written in the same statement, so the page can never display two
figures that describe different moments.

The raw count is what the ATS-overlap arithmetic is computed from (matched + unmatched =
raw). The de-duplicated count is what the page displays as the source's headline figure,
because that is what `/jobs?source=<key>` will actually show the visitor.

#### Scenario: Both counts land in one row

- **WHEN** the rollup completes a run
- **THEN** each written row carries both counts and one `measured_at`

#### Scenario: The overlap arithmetic closes

- **WHEN** a source's row reports a raw open count and a matched-to-ATS count
- **THEN** the unmatched count implied by the page equals raw minus matched, with no third
  bucket

### Requirement: One aggregate pass over open postings

The per-source aggregate SHALL be computed in a single grouped scan over open postings,
reading no posting description.

A description predicate de-TOASTs every row it touches, which at this catalogue's size is
the difference between a pass that keeps its schedule and one that does not finish. The figures
this rollup needs — the source key and the aggregator duplicate marker — are
narrow columns.

#### Scenario: The pass reads no description

- **WHEN** the per-source aggregate query is executed
- **THEN** it selects and filters on no description column

### Requirement: The Meilisearch leg is best-effort

The de-duplicated counts SHALL come from ONE Meilisearch facet request covering every
source at once. A missing Meilisearch credential or an unreachable Meilisearch SHALL cost
that one figure, never the run: the Postgres-measured figures are still written, and the
de-duplicated count is recorded as absent rather than as zero.

A zero that means "we could not measure this" must never reach a page that will render it
as "this source has no jobs".

#### Scenario: Meilisearch is unreachable

- **WHEN** the rollup cannot reach Meilisearch
- **THEN** the run still writes every source's Postgres-measured figures, records the
  de-duplicated count as absent, and exits successfully

#### Scenario: One request, not one per source

- **WHEN** the rollup measures de-duplicated counts for every source in the fleet
- **THEN** it issues a single facet-distribution request

### Requirement: The snapshot carries no posting URL

The snapshot SHALL NOT sample or store a posting URL.

The first version did, to resolve a source's logo from the host. It was wrong twice and
production said so within an hour of the page going live: an ATS posting's URL usually
lives on the EMPLOYER's domain, so the sample taken for `greenhouse` was `bankrate.com`
and for `successfactors` a staffing agency's — and the page served those companies' marks
under the platforms' names; and where the host really was the platform's it was typically
a per-tenant subdomain the logo service 404s on. A logo is resolved from the source's
DISPLAY NAME instead, which is what that service answers, and which needs nothing from
this table.

#### Scenario: No posting URL is stored

- **WHEN** the snapshot is written
- **THEN** no column holds a posting URL, and nothing downstream can publish one
