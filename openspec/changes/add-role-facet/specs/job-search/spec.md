## ADDED Requirements

### Requirement: Role facet on the jobs index and search endpoint

The jobs index SHALL carry a multi-valued `roles` attribute (derived at index
time — see the `role-facet` capability) and SHALL declare it as a **filterable +
facetable** attribute. The public `GET /api/v1/jobs/search` and
`GET /api/v1/jobs/facets` endpoints SHALL accept a `role` facet filter mapped to
the index `roles` attribute. Multiple `role` values SHALL be ORed within the
facet by default and SHALL be AND-combined when `role_mode=and` is set;
`role_exclude` SHALL exclude the listed roles; and the `role` facet SHALL
AND-combine with the other facet filters — identical semantics to the existing
multi-valued `skills` facet. The `role` filter SHALL be available to every
consumer of the shared filter builder (search, facets distribution, and the
saved-search notification matcher) without special-casing.

#### Scenario: Filtering by a single role

- **WHEN** a client requests `GET /api/v1/jobs/search?role=founding_engineer`
- **THEN** only jobs whose `roles` include `founding_engineer` are returned

#### Scenario: Multiple roles are ORed within the facet

- **WHEN** a client requests
  `GET /api/v1/jobs/search?role=senior_backend&role=lead_frontend`
- **THEN** jobs whose `roles` include `senior_backend` OR `lead_frontend` are
  returned, and a job tagged only `senior_frontend` is not

#### Scenario: Role excludes and ANDs with other facets

- **WHEN** a client requests
  `GET /api/v1/jobs/search?role=founding_engineer&regions=eu&role_exclude=fractional_cto`
- **THEN** only jobs whose `roles` include `founding_engineer`, do not include
  `fractional_cto`, and whose top-level `regions` include `eu` are returned
