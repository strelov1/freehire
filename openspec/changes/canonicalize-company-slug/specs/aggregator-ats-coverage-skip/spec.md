## MODIFIED Requirements

### Requirement: An aggregator posting is not saved when its company already has non-aggregator coverage

The system SHALL, when ingesting a posting from a provider `sources.ProviderKind` classifies
as `KindAggregator`, skip saving that posting when the company already has at least one OPEN
posting from a source that is NOT in `sources.AggregatorProviders(sources.Taxonomy())`. The
company SHALL be matched by EXACT `company_slug` string equality — no hyphen-folding at query
time; see the "exact match only" scenario below for why. Both sides of that comparison SHALL be
the alias-resolved slug taken from the board run's single shared resolution map, so the value
the gate asks about is the value the upsert would write. A skipped posting SHALL be counted in a
dedicated `Stats.ATSCovered` counter, distinct from `Stats.Rejected`.

Sharing one map is the requirement, not merely sharing one function. The gate and the upsert
previously agreed only because each independently applied the same pure derivation; once the
slug depends on registry state, an independently derived value can silently disagree, and a gate
that silently stops matching looks exactly like a gate that found nothing to skip.

#### Scenario: Aggregator posting for a covered company is skipped

- **WHEN** an aggregator-provider board yields a posting for a company that already has an
  open posting from a non-aggregator source
- **THEN** the posting is not saved, and `Stats.ATSCovered` is incremented for that board

#### Scenario: Aggregator posting for an uncovered company is saved normally

- **WHEN** an aggregator-provider board yields a posting for a company with no open posting
  from any non-aggregator source
- **THEN** the posting is saved exactly as it is today, and `Stats.Ingested` is incremented

#### Scenario: A streaming aggregator source is gated the same as a buffered one

- **WHEN** an aggregator provider that fetches via a streaming source (postings emitted one at
  a time rather than as one fetched batch) emits a posting for a covered company
- **THEN** the posting is not saved, and `Stats.ATSCovered` is incremented — the gate applies
  regardless of whether the provider's adapter buffers or streams

#### Scenario: Coverage matches EXACT company_slug only, unlike the reindex suppression pass

- **WHEN** an aggregator posting's resolved `company_slug` and an existing non-aggregator
  posting's `company_slug` differ only by hyphenation (e.g. `cfoinsights` vs `cfo-insights`)
  and no `company_slug_aliases` row joins them
- **THEN** this gate does NOT treat the company as covered and does NOT skip the aggregator
  posting — the live lookup compares `company_slug` values exactly as resolved, with no folding
  (a live Meili filter cannot compute the reindex pass's `replace(company_slug, '-', '')` fold
  at query time, and folding the query value instead would break exact matches too — see
  design.md's "Coverage definition"). `aggregator-ats-dedup`'s periodic reindex pass remains the
  mechanism that catches an unregistered spelling pair, on its own schedule

#### Scenario: A registered spelling pair IS treated as one company

- **WHEN** an aggregator posting's derived slug folds to a `company_slug_aliases.folded_key`
  whose `canonical_slug` is the slug of a company with open non-aggregator coverage
- **THEN** the posting resolves to that canonical slug before the lookup, the gate finds the
  company covered, and the posting is skipped — the match is still exact, on a value that was
  canonicalized first

#### Scenario: The gate reads the same map the upsert writes from

- **WHEN** a board run resolves its distinct company slugs through the alias registry
- **THEN** the coverage lookup is asked about the resolved slugs, and any posting that survives
  the gate is stored under the same resolved slug — no code path re-derives it
