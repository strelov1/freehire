## MODIFIED Requirements

### Requirement: A board entry MAY be marked as a community hub whose employer comes from each posting

The system SHALL support an optional `hub` flag on a board-file entry that marks a board-based
provider's board as a community/agency hub: a board that lists vacancies on behalf of many
partner companies rather than a single employer. Unlike a boardless aggregator (one global feed,
no board id), a hub entry still names a board id and still requires a `company` — that configured
company is the hub's own name and the per-vacancy fallback employer. When the flag is absent or
false, the provider's existing behaviour SHALL be unchanged and each job's company SHALL be the
configured entry company.

For a hub entry, the provider SHALL resolve each vacancy's employer from the posting itself
rather than from the configured company, falling back to the configured company when the posting
carries no resolvable employer (for example the hub's own internal roles).

A hub entry MAY additionally carry an optional `tenants` map, from the tenant key the platform
stamps on a posting to that tenant's employer display name. It exists for hubs whose postings
identify the tenant only by an opaque key — a URL path segment or an id — which is not itself a
usable company name. The map SHALL be optional and SHALL be ignored by providers that do not
implement hub resolution, exactly as `hub` and `region` are. A tenant key absent from the map
SHALL fall back to the configured company rather than be turned into a name by transforming the
key, because a plausible-but-wrong employer is worse in the catalogue than the parent brand.

#### Scenario: A non-hub entry attributes every job to the configured company

- **WHEN** a board file lists an entry without the `hub` flag (or with `hub: false`)
- **THEN** each crawled job's company is the entry's configured `company`, exactly as before

#### Scenario: A hub entry attributes each job to the employer named in the posting

- **WHEN** a board file lists an entry with `hub: true`
- **THEN** each crawled job's company is resolved from that vacancy's own payload, and only a
  vacancy with no resolvable employer falls back to the configured `company`

#### Scenario: A hub entry maps an opaque tenant key to an employer name

- **WHEN** a hub entry carries a `tenants` map and a vacancy's tenant key is present in it
- **THEN** the job's company is that map's value

#### Scenario: An unmapped tenant key falls back rather than being invented

- **WHEN** a hub entry carries a `tenants` map and a vacancy's tenant key is absent from it
- **THEN** the job's company is the configured `company`, and the key is NOT transformed into a
  name

## ADDED Requirements

### Requirement: The successfactors adapter resolves a hub board's employer from the job URL

For a `successfactors` board marked as a community hub, the adapter SHALL set each job's company
from the tenant key carried in that job's URL, resolved through the entry's `tenants` map.
SuccessFactors hub sites serve every tenant from one host and one shared `job_sitemap.xml`, and
they identify the tenant only as the FIRST path segment of a job URL
(`/<tenant>/job/<slug>/<id>/`). The job page's own `hiringOrganization` microdata SHALL NOT be
used for this: on a hub it names the corporate parent rather than the employer, and the same
markup emits the property more than once with non-company values.

A job URL whose path does not begin with a tenant segment — the tenant-less
`/job/<slug>/<id>/` shape a hub sitemap also lists — SHALL yield no tenant key, and the literal
`job` segment SHALL NOT be read as a tenant. A missing or unmapped tenant key SHALL fall back to
the configured entry company. A non-hub `successfactors` board's behaviour is unchanged (company
is the configured entry company).

#### Scenario: A mapped tenant becomes the job's employer

- **WHEN** a hub board's job URL is `https://<host>/Riverty/job/Berlin-Software-Engineer-10623/1425618633/`
  and the entry's `tenants` map has `Riverty: Riverty`
- **THEN** the job's company is `Riverty`

#### Scenario: An unmapped tenant falls back to the hub company

- **WHEN** a hub board's job URL carries the tenant segment `Sonopress` and the entry's `tenants`
  map has no `Sonopress` key
- **THEN** the job's company is the entry's configured `company`

#### Scenario: A tenant-less job URL is not attributed to a company called "job"

- **WHEN** a hub board's job URL is `https://<host>/job/Dortmund-Delphi-Entwickler-44369/1431430433/`
- **THEN** the job's company is the entry's configured `company`, never `job`

#### Scenario: A non-hub successfactors board is unaffected

- **WHEN** a `successfactors` entry has no `hub` flag
- **THEN** every job's company is the configured `company`, whatever the job URL's path segments
  are
