## MODIFIED Requirements

### Requirement: The detail page shows the openings-by-location section

The job detail page SHALL present "Similar jobs" and "Other locations" as two tabs in one
related-content section. The locations tab shows a bounded preview (the first 10 copies)
plus a "View all N locations" link to a dedicated full-list page when the cluster has more
than the preview; it is shown only when the role has more than one open copy, and the
whole section degrades to nothing when neither similar jobs nor copies exist.

#### Scenario: Mass-posted role shows a bounded locations tab

- **WHEN** a job's role cluster has many open copies
- **THEN** the detail page shows a "Other locations (N)" tab listing the first 10 cities
  and a "View all N locations" link to the full-list page

#### Scenario: Full list lives on a dedicated page

- **WHEN** a client opens `/jobs/:slug/copies`
- **THEN** the page lists the cluster's open postings by location, paginated via
  `limit`/`offset`, with the accurate total
