## ADDED Requirements

### Requirement: The role-type facet is documented as a one-sided signal

`web/static/openapi.yaml` SHALL declare `role_type` on every endpoint that accepts
the string facets, and the generated docs SHALL list it among the facet params.

Its description SHALL state that the vocabulary holds a single value and that its
absence means only "no people-management marker in the title" — NOT that the posting
is an individual-contributor role. An integrator reading `role_type_exclude` as a
positive IC filter would build on a claim the catalogue cannot support, so the
contract has to close that reading explicitly rather than leave it to inference.

#### Scenario: The contract declares the facet

- **WHEN** an endpoint in `web/static/openapi.yaml` accepts the string facets
- **THEN** it declares `role_type`, with `people_manager` as its enumerated value

#### Scenario: The documentation states what the absence means

- **WHEN** a reader looks up `role_type`
- **THEN** the description says the absence of the value means no marker was found,
  and explicitly not that the posting is individual-contributor work
