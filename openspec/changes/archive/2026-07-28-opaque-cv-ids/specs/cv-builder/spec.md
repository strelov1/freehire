## ADDED Requirements

### Requirement: A CV id is unguessable

A CV SHALL be identified by a random id rather than by a sequential one, in the
database and on every surface that names it — API paths, web routes, and the
published clients. Ownership already confines every read and write, so the id is
not a capability; but a countable id would publish how many CVs the platform
holds, and it would turn a single missing owner check on any future CV endpoint
into bulk extraction of other people's résumés. An id that is not well-formed
SHALL be reported as a missing CV, so "not a CV" and "not yours" stay one answer.

#### Scenario: Two CVs get unrelated ids

- **WHEN** a user creates two CVs in a row
- **THEN** their ids are independently random, so neither reveals the other nor how many CVs exist

#### Scenario: A malformed id is missing, not invalid

- **WHEN** a request names a CV id that is not well-formed — a number, or anything that is not an id at all
- **THEN** it is refused as not found, indistinguishable from a CV the caller does not own

#### Scenario: The numeric form no longer resolves

- **WHEN** a client built against the previous numeric ids sends one
- **THEN** the request is refused as not found rather than resolving to any CV
