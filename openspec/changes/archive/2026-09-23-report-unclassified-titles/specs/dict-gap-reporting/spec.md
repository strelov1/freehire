# dict-gap-reporting — delta

## ADDED Requirements

### Requirement: Unclassified title candidates rank what no dictionary places at all

The system SHALL report, ranked by how many postings carry them, the distinct job titles that BOTH the tech-title dictionary and the title-classification dictionary fail to place — the titles that yield neither an `is_tech` signal nor a category.

This population is invisible to the other two reports **by construction, not by omission**: enrichment is gated on `is_tech IS TRUE`, so a title no dictionary places is never enriched, has no LLM opinion recorded against it, and therefore can never appear in a drift report (which compares against that opinion) or in a skill-gap report (which mines it). Measured 2026-09-23, that blind spot held 2,232,773 open canonical postings, 71,314 of them on titles reading as software or IT outright.

The report SHALL be scoped to postings that would be published if they were recognised — open, canonical, non-private — because a title carried only by closed or duplicate postings is not a gap worth a curator's attention.

#### Scenario: A frequent title no dictionary places surfaces

- **WHEN** a distinct title yields no tech-title signal and no category, and open canonical postings carry it
- **THEN** the report includes that title with its posting count, and titles with higher counts are ranked above titles with lower counts

#### Scenario: A title either dictionary already places is excluded

- **WHEN** a distinct title yields a tech-title signal, or resolves to a category, or both
- **THEN** the report excludes that title

#### Scenario: A title carried only by closed or duplicate postings is excluded

- **WHEN** every posting carrying a title is closed, marked as a duplicate, or private
- **THEN** the report excludes that title, since recognising it would publish nothing

### Requirement: Reports answer for the dictionary as it stands, not as it was stored

Every report in this capability SHALL RECOMPUTE the dictionary's answer for each title at report time, and SHALL NOT read a derived column persisted on the posting.

The two are routinely different, and the difference is the whole point: a dictionary change reaches stored rows only through `cmd/backfill-derive`, a pass measured at ~171 rows/s over 12.7M rows, so for the many hours between a dictionary shipping and that pass completing, the stored column states what the OLD dictionary said. A report reading it would rank gaps that have already been closed, and hide gaps the newest terms opened — at exactly the moment a curator is most likely to run it.

#### Scenario: A title the newest dictionary now places is excluded, even before a backfill

- **WHEN** a title's stored posting rows still carry no `is_tech` signal, because no backfill has run since the dictionary gained a term matching it, but the current dictionary does place the title
- **THEN** the report excludes that title
