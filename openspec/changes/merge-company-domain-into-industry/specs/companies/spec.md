## ADDED Requirements

### Requirement: The company industry facet resolves through two sources

The `industries` facet on `GET /api/v1/companies` SHALL match a company through
either of two independent sources: the curated `companies.industries` array, or the
job-derived `companies.domains` array translated through a curated domain→industry
mapping. A company matches the facet when **either** source yields one of the
requested industry values; the facet otherwise behaves exactly like the other array
facets (OR within the facet, AND across facets, composing with `q`).

The mapping SHALL be dict-only, in keeping with every other dictionary in the
system: a domain value that names no curated industry honestly — including `other`,
and any value absent from `vocab.DomainValues` — SHALL map to nothing and SHALL
contribute no industry. The mapping SHALL never invent a canonical value: every
industry it produces must already exist in the curated vocabulary.

Both query backends SHALL implement this identically. `GET /api/v1/companies` is
served by the Meilisearch companies index or by Postgres depending on the request,
and the rendered list and its `meta.total` may be produced by different paths within
one page; a company matching on one backend and not the other would make a page
contradict its own count.

The `domains` query parameter SHALL keep filtering on the domain column directly,
unchanged. Widening `industries` removes a duplicate *control*, not a contract.

#### Scenario: A company matches on its curated industry

- **WHEN** a client requests `GET /api/v1/companies?industries=fintech` and a company
  has `industries` containing `fintech`
- **THEN** that company is in the response, regardless of its `domains`

#### Scenario: A company matches on its job-derived domain alone

- **WHEN** a client requests `GET /api/v1/companies?industries=developer-tools` and a
  company has an empty `industries` but `domains` containing `devtools`
- **THEN** that company is in the response

#### Scenario: An unmapped domain yields no industry

- **WHEN** a company has an empty `industries` and its `domains` holds only values the
  mapping does not cover — `other`, `media`, `mobility`, or a value absent from
  `vocab.DomainValues` such as `saas`
- **THEN** that company matches no `industries` value and is absent from every
  `industries`-filtered response

#### Scenario: Both backends agree on the matched set

- **WHEN** the same `industries` filter is served once by the Meilisearch path and
  once by the Postgres path
- **THEN** both return the same set of companies, so a page's list and its
  `meta.total` cannot disagree

#### Scenario: The domain parameter is unaffected

- **WHEN** a client requests `GET /api/v1/companies?domains=devtools`
- **THEN** the response contains exactly the companies whose `domains` array contains
  `devtools`, as before this change
