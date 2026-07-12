## ADDED Requirements

### Requirement: Role-cluster copies are listable from a job

The system SHALL expose `GET /jobs/:slug/copies` returning the OPEN postings sharing the
addressed job's role cluster (`company_slug` + `role_fingerprint`), each carrying its
location, apply URL, and public slug, ordered by location and paginated. The anchor job
itself is included. A job whose fingerprint is empty clusters with no one and returns an
empty list. The endpoint is public.

#### Scenario: Copies of a collapsed role list every open city

- **WHEN** a client requests the copies of a canonical job whose cluster has N open
  postings across cities
- **THEN** the response lists all N, each with its own location and apply URL, ordered
  by location

#### Scenario: A closed posting and unrelated roles are excluded

- **WHEN** the cluster has a closed member and the company has other unrelated roles
- **THEN** neither appears in the copies list

### Requirement: The detail page shows the openings-by-location section

The job detail page SHALL render an "N openings across locations" section listing the
copies with links, shown only when the role has more than one open copy, and degrading
to nothing when the list is empty or the fetch fails.

#### Scenario: Mass-posted role shows the section

- **WHEN** a job's role cluster has multiple open copies
- **THEN** the detail page shows the openings-by-location list linking to each city
