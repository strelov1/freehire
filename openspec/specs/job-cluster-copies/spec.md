# job-cluster-copies Specification

## Purpose
TBD - created by archiving change job-cluster-copies. Update Purpose after archive.
## Requirements
### Requirement: Role-cluster copies are listable from a job

The system SHALL expose `GET /jobs/:slug/copies` returning the OPEN postings the addressed
job's canonical row REPRESENTS — its duplicate closure — each carrying its location, apply
URL, and public slug, ordered by location and paginated. The canonical row itself is included.
The endpoint is public.

Membership SHALL be the same closure the search document's geography union uses, not the
role-fingerprint cluster. The two must not disagree: a posting whose city the canon claims in
search but whose row the copies list omits is a location the user can filter to and then not
reach. A posting suppressed by the fuzzy-description pass or by aggregator suppression is
therefore listed, where a fingerprint grouping could not list it.

The addressed job MAY itself be suppressed. In that case the endpoint SHALL resolve it to its
ultimate owner and list that owner's closure, so a candidate arriving on a hidden posting sees
the whole group rather than the fragment its own marker belongs to. A canonical row that
represents no other open row returns just itself.

Its pagination SHALL be bounded the way every other list endpoint's is: a `limit` outside the
endpoint's range is clamped into it, and an `offset` beyond the end of the result set — including
one larger than the paginated column's storage range — SHALL yield an empty page rather than an
error. A caller MUST NOT be able to make the endpoint fail by supplying an out-of-range
pagination value.

#### Scenario: Copies of a collapsed role list every open city

- **WHEN** a client requests the copies of a canonical job whose closure has N open
  postings across cities
- **THEN** the response lists all N, each with its own location and apply URL, ordered
  by location

#### Scenario: A fuzzy-suppressed posting is listed among the copies

- **WHEN** a canonical job hides a posting through the fuzzy-description pass rather than
  through a shared role fingerprint
- **THEN** that posting appears in the copies list with its own location and apply URL

#### Scenario: Requesting the copies of a suppressed posting

- **WHEN** a client requests the copies of a posting that is itself marked a duplicate
- **THEN** the response lists its ultimate owner's whole closure, including the owner

#### Scenario: A closed posting and unrelated roles are excluded

- **WHEN** the closure has a closed member and the company has other unrelated roles
- **THEN** neither appears in the copies list

#### Scenario: An out-of-range offset is an empty page, not an error

- **WHEN** a client requests the copies with an `offset` far beyond the closure's size —
  including one exceeding the range of the underlying paginated column
- **THEN** the response is a successful empty list, exactly as the other list endpoints answer
  the same request

### Requirement: The detail page shows the openings-by-location section

The job detail page SHALL render an "N openings across locations" section listing the
copies with links, shown only when the addressed job represents more than one open posting,
and degrading to nothing when the list is empty or the fetch fails.

#### Scenario: Mass-posted role shows the section

- **WHEN** a job's duplicate closure has multiple open members
- **THEN** the detail page shows the openings-by-location list linking to each city
