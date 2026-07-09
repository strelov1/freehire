## ADDED Requirements

### Requirement: A Job is constructed only through the aggregate factory

The system SHALL expose a single construction path for a `Job` domain value — a
factory (`job.New`) taking a source-agnostic draft of the raw posting fields — and
SHALL NOT allow callers to assemble a `Job` from a raw composite literal outside the
`job` package. The factory SHALL run the deterministic derivation (`jobderive`,
wrapping `location`/`skilltag`/`classify`/`roletag` and the public-identity slugs)
internally, so a constructed `Job` always carries facets consistent with its source
fields. Every write path that persists a posting — automated ingest, moderator
authoring, and Telegram extraction — SHALL obtain its `Job` from this factory.

#### Scenario: Every write path derives facets identically

- **WHEN** the ingest pipeline, the moderator authoring path, and Telegram
  extraction each construct a job from the same title, description, and location
- **THEN** all three produce the same dictionary facets (`countries`, `regions`,
  `work_mode`, `skills`, `seniority`, `category`) and the same public/company slugs,
  because each obtains its `Job` through `job.New`

#### Scenario: Facets cannot be omitted

- **WHEN** a caller constructs a `Job` through the factory and never touches the
  facet fields
- **THEN** the resulting `Job` still carries the derived facets, because derivation
  happens inside `job.New` rather than in caller code

### Requirement: The domain Job is loaded and persisted through a repository port

The `job` package SHALL define a `Repository` port (interface) for loading a domain
`Job` by identity and persisting it, and SHALL provide a `QueriesRepository` adapter
implementing that port over the shared `*db.Queries`. The domain `Job` type SHALL NOT
be `db.Job`; the adapter SHALL map between the persistence row and the domain type.
The shared `db.Queries` surface SHALL NOT be split per context by this change — the
port sits on top of the existing generated queries.

#### Scenario: The adapter maps persistence to domain

- **WHEN** the repository loads a stored posting by its `(source, external_id)` identity
- **THEN** it returns a domain `Job` value, not a `db.Job` row, and callers depend on
  the domain type only

#### Scenario: A compile-time contract binds adapter to port

- **WHEN** the package compiles
- **THEN** a `var _ Repository = (*QueriesRepository)(nil)` assertion proves the
  adapter satisfies the port, mirroring the existing `jobtracking` convention

### Requirement: The public wire shape projects from the domain Job

The public read model SHALL be projected from the domain `Job` type
(`jobview.FromDomain`), not built directly from the persistence row. The `jobview`
package SHALL depend on the `job` aggregate for its input shape rather than on the
raw `db.Job` row. The projected wire output SHALL be observably equivalent to the
current `jobview.FromRow(db.Job)` output — this change relocates the source of the
projection, not its result.

#### Scenario: Wire output is unchanged by the projection move

- **WHEN** a stored job is projected to the wire shape through the domain type
- **THEN** the emitted JSON is byte-equivalent to what `jobview.FromRow` produced for
  the same stored row before this change

### Requirement: Lifecycle and enrichment eligibility are guarded aggregate behavior

The domain `Job` SHALL expose its lifecycle transitions and enrichment-eligibility
rule as guarded methods — `Close(at)` (idempotent; preserves `public_slug`,
`enrichment`, and interaction references), `Reopen()`, and `ShouldEnrich()` (open and
below the current enrichment version). The three close mechanisms (the ingest unseen
sweep, the stream-driven self-close, and the liveness probe) and the enrichment
eligibility predicate SHALL express their decision through the aggregate rather than
duplicating the open/closed condition. This change SHALL NOT alter *when* a job closes
or *whether* it is enriched — only where the rule lives.

#### Scenario: Closing is idempotent and non-destructive

- **WHEN** `Close(at)` is called on an already-closed `Job`
- **THEN** the operation is a no-op that leaves `closed_at`, `public_slug`, and
  enrichment intact

#### Scenario: A closed job is not enrichment-eligible

- **WHEN** `ShouldEnrich()` is evaluated on a closed `Job`
- **THEN** it returns false, matching today's `closed_at IS NULL` guard in the
  enrichment queue

#### Scenario: Reopen clears the closed state on re-ingest

- **WHEN** a previously closed posting reappears in its source feed and is upserted
- **THEN** `Reopen()` clears `closed_at` and the job is served on list/search surfaces
  again
