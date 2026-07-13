## ADDED Requirements

### Requirement: Deterministic non-tech title detection

The system SHALL provide a deterministic, curated dictionary that identifies confidently non-technical job titles by whole-word match, and MUST NOT guess: a title it cannot confidently place as non-tech yields no signal. The dictionary MUST only match unambiguous non-tech role nouns (e.g. registered nurse, forklift operator, warehouse associate, cashier, housekeeping, electrician, teacher) and MUST NOT contain generic terms that also occur in technical roles (e.g. bare "engineer", "technician", "analyst").

#### Scenario: Confident non-tech title is detected
- **WHEN** a title contains a curated non-tech role noun as a whole word (e.g. "Registered Nurse (RBT)")
- **THEN** the detector reports the title as non-tech

#### Scenario: Technical title is not flagged
- **WHEN** a title states a technical role (e.g. "Software Engineer II, AWS DynamoDB")
- **THEN** the detector reports no non-tech signal

#### Scenario: Ambiguous substring does not match
- **WHEN** a non-tech term appears only as a substring of another word (e.g. "warehouse" inside a longer token) or the title carries a shared term like "engineer"
- **THEN** the detector does not flag the title, matching only on word boundaries

### Requirement: Tri-state is_tech derivation

The system SHALL derive a tri-state `is_tech` signal for every job during facet derivation, from the title and the derived category, with technical evidence taking precedence over non-technical evidence. The value MUST be `true` when the derived category is a recognized technical category, `false` when the derived category is a known non-technical category OR the non-tech detector flags the title, and otherwise unknown (absent). An unknown value MUST NOT be coerced to `true` or `false` — the absence is the honest state used to measure remaining coverage.

#### Scenario: Recognized tech category yields true
- **WHEN** the title resolves to a technical category (e.g. `backend`)
- **THEN** `is_tech` is `true`

#### Scenario: Blacklist non-tech category yields false
- **WHEN** the derived category is one of the non-technical categories (marketing, sales, support, management)
- **THEN** `is_tech` is `false`

#### Scenario: Detector-only non-tech yields false
- **WHEN** the derived category is empty but the non-tech detector flags the title (e.g. "Warehouse Cleaner")
- **THEN** `is_tech` is `false`

#### Scenario: Unresolved job stays unknown
- **WHEN** neither a technical nor a non-technical category resolves and the detector is silent
- **THEN** `is_tech` is unknown (absent), not `true` and not `false`

### Requirement: is_tech persistence and backfill

The system SHALL persist the derived `is_tech` on the job row through the same write path as the other deterministic facets, and MUST re-derive it for existing jobs via the backfill-derive maintenance pass. A job written or re-crawled MUST carry the `is_tech` value its title and category currently derive.

#### Scenario: Ingest persists is_tech
- **WHEN** a job is upserted through the ingest write path
- **THEN** its `is_tech` column reflects the value derived from its title and category

#### Scenario: Backfill re-derives is_tech
- **WHEN** the backfill-derive pass runs over existing jobs
- **THEN** every processed job's `is_tech` is recomputed from its current title and category

### Requirement: is_tech search facet with filter

The system SHALL expose `is_tech` as a filterable search facet: the served job wire shape MUST include the value, the search index MUST make it filterable (an unknown value indexed as absent so it is filterable as empty), the search API MUST report its facet distribution, and the web filter UI MUST offer a Tech / Non-tech control that filters the results.

#### Scenario: Facet distribution is reported
- **WHEN** the jobs search is queried with faceting on `is_tech`
- **THEN** the response reports counts for the tech and non-tech buckets

#### Scenario: Filtering to tech excludes non-tech
- **WHEN** a search filters `is_tech` to tech
- **THEN** only jobs with `is_tech` true are returned, and non-tech and unknown jobs are excluded

#### Scenario: Served job carries is_tech
- **WHEN** a job is returned by any list, detail, company, or search response
- **THEN** its wire shape includes the `is_tech` value (or null when unknown)
