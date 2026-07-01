# remote-region-filters Specification

## Purpose
Let users filter jobs to the "geography not specified" bucket — postings whose
derived region set is empty — without materializing a redundant column. Combined
with the existing `work_mode=remote` filter it reproduces the former
"remote, location-flexible" bucket, and on its own it is the orthogonal
"region unspecified" filter.

## Requirements
### Requirement: The regions facet accepts an "unspecified" sentinel value

The search API SHALL treat a reserved `regions` facet value (`none`) as a request
for jobs whose derived `regions` set is empty, rather than an equality against a
literal region code. The predicate SHALL be expressed as the search engine's
empty-array test (`regions IS EMPTY`) so it composes with real region values (ORed
within the facet) and requires no stored per-job column. The sentinel SHALL be
built by the same pure filter builder shared by the HTTP search handler and the
saved-search/notification matcher, so both produce an identical filter.

#### Scenario: The sentinel selects jobs with no resolved region

- **WHEN** a search request sets `regions=none`
- **THEN** only jobs whose derived `regions` set is empty are returned (via
  `regions IS EMPTY`)

#### Scenario: The sentinel ORs with real regions

- **WHEN** a search request sets `regions=none` alongside a real region (e.g.
  `regions=none&regions=eu`)
- **THEN** jobs are returned that have region `eu` OR no region at all, as a single
  ORed facet group

#### Scenario: Excluding the sentinel keeps only located jobs

- **WHEN** a search request excludes the sentinel (`regions_exclude=none`)
- **THEN** only jobs whose derived `regions` set is non-empty are returned (via
  `regions IS NOT EMPTY`)

#### Scenario: The sentinel is scoped to the regions facet

- **WHEN** the reserved value appears on a different facet (e.g. `relocation=none`,
  where `none` is a real vocabulary value)
- **THEN** it is treated as an ordinary equality, never as an empty-set test

### Requirement: The "region not specified" filter needs no stored facet

The system SHALL NOT persist a materialized boolean for the "remote, region not
specified" bucket. The bucket SHALL be derivable at query time from the existing
`regions` search attribute (empty array) combined with the existing `work_mode`
filter, so no `jobs` column, read-model field, or dedicated filterable attribute
is required for it.

#### Scenario: The former bucket is reproduced by composition

- **WHEN** a search request combines `work_mode=remote` with `regions=none`
- **THEN** it returns exactly the remote jobs whose geography did not resolve — the
  bucket formerly served by a stored `remote_unspecified` facet

### Requirement: The SPA exposes the sentinel as a Region chip

The web frontend SHALL present the "region not specified" option as a pill within
the jobs filter sidebar's Region facet (labelled `Not specified`), appended after
the real macro-region pills, so it selects, ORs, and excludes like any region. The
filter model SHALL serialize the selected chip to `regions=none`, parse it back
from the URL, and count it toward the active-filter total like any facet value. The
company filter's Region facet SHALL NOT offer the sentinel (the companies list
filters by array overlap, which has no empty-set test).

#### Scenario: The chip is shown inside the Region facet

- **WHEN** the user opens the jobs filter sidebar's Region facet
- **THEN** a `Not specified` pill appears among the region pills, and selecting it
  sets the `regions=none` search param
