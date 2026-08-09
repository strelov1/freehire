## ADDED Requirements

### Requirement: Job detail page links to related collections

The job detail page (`GET /jobs/:slug`) SHALL render a bounded "see also"
block of internal links into existing `/collections/:slug` landing pages,
built from the viewed job's own data with no additional HTTP request. Link
candidates SHALL be drawn from two sources: the job's own facets (role,
region, skills) matched against the `FILTER_COLLECTIONS` registry, and the
job's `collections` field matched against the company-collection registry.
Every rendered link SHALL target a slug that exists in one of these two
registries — the block SHALL NOT construct or link to a slug that maps to no
collection.

#### Scenario: A job's skill facet produces a matching link

- **WHEN** a job detail page is rendered for a job whose `skills` include
  `react`, and a `react` filter collection exists
- **THEN** the see-also block includes a link to `/collections/react`

#### Scenario: A job's company collection produces a matching link

- **WHEN** a job detail page is rendered for a job whose `collections` field
  includes `yc`
- **THEN** the see-also block includes a link to `/collections/yc`

#### Scenario: Job-facet matches are ordered before company-collection matches

- **WHEN** a job matches both a filter collection (via its own facets) and a
  company collection (via its `collections` field)
- **THEN** the filter-collection link appears before the company-collection
  link in the block

### Requirement: The see-also block is always full and never links to a dead slug

When a job's own facet and company-collection matches fall short of the
block's target size, the system SHALL pad the block with a fixed fallback
list of popular collections until the target size is reached or the
available collection pool is exhausted, whichever comes first. The block
SHALL de-duplicate by slug across both sources and the fallback list. The
system SHALL NOT render a link to a collection slug that does not exist in
`FILTER_COLLECTIONS` or the company-collection registry.

#### Scenario: A job with no facet or collection matches still shows links

- **WHEN** a job detail page is rendered for a job whose facets and
  `collections` field match no existing collection
- **THEN** the see-also block renders the fallback list of popular
  collections instead of an empty block

#### Scenario: A duplicate match is not shown twice

- **WHEN** a job's own facets and the fallback list would both produce a link
  to the same collection slug
- **THEN** that slug appears only once in the rendered block

#### Scenario: The block never exceeds the available collection pool

- **WHEN** the combined set of matched and fallback collections is smaller
  than the block's target size
- **THEN** the block renders exactly that smaller set rather than a
  placeholder or a broken link
