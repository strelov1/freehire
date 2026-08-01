## ADDED Requirements

### Requirement: The company list item is a transport projection, not a persistence row

The system SHALL serve `GET /api/v1/companies` as a list of a projection type owned by the
transport layer, and MUST NOT serve a generated persistence row as the endpoint's response shape.
Regenerating the database access layer SHALL NOT be capable of changing the endpoint's public
JSON: a change to a column name or a query alias must either fail to compile or leave the wire
shape untouched.

Both backends of the endpoint — the Postgres read and the Meilisearch read — SHALL project onto
that one type. Neither SHALL construct the other's representation to stay compatible, so a field
added for one backend cannot be silently absent from the other, and the same request cannot
return different bodies depending on which backend served it.

The projection SHALL preserve the endpoint's existing null-ness: a nullable text column with no
value serializes as JSON `null`, and a facet array with no values serializes as `[]` rather than
`null`, whichever backend produced it.

#### Scenario: Regenerating the data-access layer cannot change the response

- **WHEN** a column in the company list query is renamed or given a different alias, and the
  database access layer is regenerated
- **THEN** the endpoint's response fields are unchanged, and any mismatch surfaces as a
  compilation failure rather than as a silently altered public contract

#### Scenario: Both backends serve the same shape

- **WHEN** the same company is returned once by the Postgres path and once by the Meilisearch
  path
- **THEN** the two response bodies are byte-identical, including which fields are `null` and
  which empty arrays are `[]`

#### Scenario: An absent tagline stays null, an absent facet stays an empty array

- **WHEN** a company has no tagline and no industries, served from either backend
- **THEN** `tagline` is `null` and `industries` is `[]`
