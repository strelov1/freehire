## ADDED Requirements

### Requirement: Role type is a filterable facet on the jobs index

The searchable jobs index SHALL declare `role_type` as a filterable attribute. Like
`roles` and `ai_archetype`, it is derived at index time and carried **top-level** on
the document, so it filters on the plain attribute rather than an `enrichment.*` dot
path.

The public search endpoints SHALL accept a `role_type` facet param carrying the
values of `vocab.RoleTypeValues`, with the same grammar every string facet has: a
comma-separated or repeated value list, a `role_type_exclude` twin, and AND-ing
against the other facets.

Because the vocabulary holds a single value, excluding it is how a caller asks for
postings with no management marker. That is not the same as asking for
individual-contributor postings, and the documentation SHALL say so rather than
letting the exclusion be read as a positive claim.

#### Scenario: Filtering by role type narrows to management postings

- **WHEN** a client requests `GET /api/v1/jobs/search?role_type=people_manager`
- **THEN** only postings whose title resolved to `people_manager` are returned

#### Scenario: Excluding the value drops management postings

- **WHEN** a client requests
  `GET /api/v1/jobs/search?role_type_exclude=people_manager`
- **THEN** postings that resolved to `people_manager` are omitted, and postings with
  no resolved role type are returned

#### Scenario: An unknown role-type value matches nothing and is not an error

- **WHEN** a client requests `GET /api/v1/jobs/search?role_type=cat_herder`
- **THEN** the request succeeds and matches no postings, consistent with every other
  facet's handling of an unknown value

#### Scenario: The facet composes with the others

- **WHEN** a client requests
  `GET /api/v1/jobs/search?role_type=people_manager&category=security`
- **THEN** only security postings that are also people-management roles are returned
