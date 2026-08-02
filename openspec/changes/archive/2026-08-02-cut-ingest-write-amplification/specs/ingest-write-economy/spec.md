## ADDED Requirements

### Requirement: An unchanged re-ingest writes only the liveness timestamp

When a crawl re-sees an open posting that matches the stored row on every column the write
would otherwise change, the ingest write path SHALL refresh `last_seen_at` and SHALL write
no other column of that row. It SHALL NOT rewrite the description, the derived facet
arrays, the fingerprints, or any bookkeeping column beyond the liveness timestamp.

The match SHALL be decided by a key that covers every column the full write would set. The
indexed content fingerprint (`content_hash`) alone does NOT: `cities` is written by the
upsert and is not among the fingerprint's inputs, because a caller's structured city list
overrides the location-derived one and can therefore move while every fingerprinted field
stands still. The key SHALL carry `cities` alongside the fingerprint, and a column later
added outside the fingerprint SHALL either join it or join the key.

The refresh SHALL write no column that any index covers, so the update stays eligible for a
heap-only tuple and maintains no index.

The write path SHALL report such a write as neither inserted nor content-changed, so every
consumer already gated on those two signals — the live search push, the role-cluster
lookup — continues to skip it for exactly the reason it skips it today.

#### Scenario: An unchanged posting is re-crawled

- **WHEN** a crawl re-ingests an open posting whose mapped content is byte-identical to the
  stored row
- **THEN** the row's `last_seen_at` advances, every other column of the row is unchanged,
  and the write reports neither inserted nor changed

#### Scenario: A structured city list moves while the fingerprint stands still

- **WHEN** a re-ingested posting carries a different structured city list but is otherwise
  identical, so its `content_hash` is unchanged
- **THEN** the full write path runs and stores the new `cities`, rather than the liveness
  refresh skipping the row

#### Scenario: A changed posting takes the full write

- **WHEN** a crawl re-ingests a posting whose title or description differs from the stored
  row
- **THEN** the full upsert runs, the stored content and fingerprints are updated, and the
  write reports a content change

### Requirement: `updated_at` records when content changed, not when it was crawled

The ingest write path SHALL stamp `jobs.updated_at` only on a write that actually changed
the row — an insert, a content change, or a reopen. A liveness-only refresh SHALL leave
`updated_at` untouched.

This SHALL hold for both liveness-only paths: the unchanged-content refresh, and the
`TouchJob` refresh a hydrating source issues for an offer it re-listed without re-fetching.

A reopen is a change the search reconciler must see, so a write that clears `closed_at`
SHALL stamp `updated_at` even though it wrote no content.

#### Scenario: A liveness-only refresh leaves the change stamp alone

- **WHEN** an open posting is re-crawled with unchanged content
- **THEN** `last_seen_at` advances and `updated_at` keeps its previous value

#### Scenario: A hydrating source's touch leaves the change stamp alone

- **WHEN** a hydrating source re-lists an already-ingested open offer without re-fetching
  its detail
- **THEN** `last_seen_at` advances and `updated_at` keeps its previous value

#### Scenario: A reopen stamps the change stamp

- **WHEN** a posting that had been closed is re-seen by a crawl or a hydrating source's
  touch
- **THEN** `closed_at` is cleared and `updated_at` is stamped, so an incremental reindex
  scoped by `updated_at` picks the row up

### Requirement: A closed posting that reappears is still reopened

The liveness-only refresh SHALL apply to open postings only. A posting carrying a
`closed_at` SHALL fall through to the full write path even when its content fingerprint
matches the stored one, so the reopen — clearing `closed_at`, clearing the close record, and
resetting the liveness strike count — happens exactly as it does today.

#### Scenario: A closed posting reappears unchanged

- **WHEN** a crawl re-ingests a posting that had been closed, with content identical to the
  stored row
- **THEN** the posting is reopened: `closed_at` and the close record are cleared, the strike
  count is reset, and `updated_at` is stamped

### Requirement: The company row is written only when its name changes

The ingest write path SHALL update a company row only when the incoming display name
differs from the stored one. Re-ingesting a posting for a company whose name is unchanged
SHALL leave the company row untouched, including its `updated_at`.

`companies.updated_at` is served as the sitemap `<lastmod>` for a hiring company, so it
SHALL report when the company record last actually changed rather than when it was last
crawled.

#### Scenario: A crawl of an existing company writes nothing to its row

- **WHEN** a board's postings are ingested for a company already stored under the same
  display name
- **THEN** the company row is not updated and its `updated_at` keeps its previous value

#### Scenario: A renamed company still updates

- **WHEN** a posting arrives carrying a display name different from the stored one for that
  company slug
- **THEN** the company row's name and `updated_at` are updated

### Requirement: A derived column may be skipped only when its inputs are covered

The liveness-only refresh skips the recomputation of every derived column, so the write
path SHALL guarantee that each such column's inputs are a subset of the content
fingerprint's inputs. A derived column whose inputs are not fully covered SHALL NOT be
skipped, because a fingerprint match would not imply the column is current.

This guarantee SHALL be enforced by a test rather than by comment, so a field later added
to a derived column but not to the fingerprint fails at build time instead of silently
serving a stale value.

The guarantee covers the posting's own fields, not the dictionaries a derivation consults.
A dictionary is an implicit input the fingerprint cannot observe, so a dictionary edit SHALL
NOT be expected to reach unchanged rows through re-crawling them; the deterministic-column
backfill remains the mechanism that propagates it.

#### Scenario: A field added to the role fingerprint but not the content hash

- **WHEN** the role-identity fingerprint is computed from an input the content fingerprint
  does not read
- **THEN** the test asserting the subset relation fails

#### Scenario: A dictionary edit does not reach an unchanged posting by crawling

- **WHEN** a facet dictionary gains a term that would change a derived column of a posting
  whose own content has not changed
- **THEN** re-crawling that posting leaves the derived column as it was, and the
  deterministic-column backfill is what updates it

### Requirement: The reach of the cheap write path is observable per provider

Each ingest run SHALL report, once per run, what share of its re-seen postings took the
liveness-only refresh. The report SHALL be attributable to the provider that ran, because
the saving is proportional to that share and a provider that varies any fingerprinted field
between crawls takes none of it.

A provider whose share is zero SHALL be visible as such. Without this, a fingerprinted field
churning on every crawl — a session token in the posting URL, a re-serialized posted time —
is indistinguishable from a healthy run, and the same churn has also been forcing a
pointless search-index push on every crawl.

#### Scenario: A run reports its cheap-path share

- **WHEN** an ingest run completes having re-seen postings already in the catalogue
- **THEN** the run logs, for its provider, how many re-seen postings took the liveness-only
  refresh against how many took the full write

#### Scenario: A provider whose rows never match is visible

- **WHEN** a provider re-crawls postings whose content fingerprint differs on every run
- **THEN** its run reports a zero cheap-path share, rather than reporting nothing
