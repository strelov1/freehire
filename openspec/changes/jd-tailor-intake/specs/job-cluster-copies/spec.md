## MODIFIED Requirements

### Requirement: Role-cluster copies are listable from a job

The system SHALL expose `GET /jobs/:slug/copies` returning the OPEN, non-private postings
sharing the addressed job's role cluster (`company_slug` + `role_fingerprint`), each carrying its
location, apply URL, and public slug, ordered by location and paginated. The anchor job itself is
included when it is not private. A job whose fingerprint is empty clusters with no one and
returns an empty list. The endpoint is public.

A private job (`is_private = true` — the jd-tailor-intake path: a pasted JD or an
unrecognized-URL scrape) SHALL be excluded from every OTHER job's copies list, even when it
coincidentally shares a role cluster with one: a private job is otherwise reachable only by its
own (unguessable, unlisted) slug, and surfacing it through a public job's copies list would
expose it to anyone browsing that unrelated public posting without ever knowing the private
slug.

Its pagination SHALL be bounded the way every other list endpoint's is: a `limit` outside the
endpoint's range is clamped into it, and an `offset` beyond the end of the result set — including
one larger than the paginated column's storage range — SHALL yield an empty page rather than an
error. A caller MUST NOT be able to make the endpoint fail by supplying an out-of-range
pagination value.

#### Scenario: Copies of a collapsed role list every open city

- **WHEN** a client requests the copies of a canonical job whose cluster has N open
  postings across cities
- **THEN** the response lists all N, each with its own location and apply URL, ordered
  by location

#### Scenario: A closed posting and unrelated roles are excluded

- **WHEN** the cluster has a closed member and the company has other unrelated roles
- **THEN** neither appears in the copies list

#### Scenario: An out-of-range offset is an empty page, not an error

- **WHEN** a client requests the copies with an `offset` far beyond the cluster's size —
  including one exceeding the range of the underlying paginated column
- **THEN** the response is a successful empty list, exactly as the other list endpoints answer
  the same request

#### Scenario: A private job never appears in another job's copies list

- **WHEN** a private job shares its role cluster (`company_slug` + `role_fingerprint`) with an
  unrelated public job
- **THEN** requesting the public job's copies does not include the private job
