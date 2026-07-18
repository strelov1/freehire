## ADDED Requirements

### Requirement: Keyless Crelate portal crawl

The system SHALL crawl a Crelate candidate portal's published jobs through its keyless public API,
returning all postings for one configured board in a single request. The adapter SHALL be
registered under provider key `crelate` in the source registry.

#### Scenario: Fetch returns all published jobs

- **WHEN** the adapter fetches a board whose OrganizationId keys a portal with published jobs
- **THEN** it issues one `GET jobs.crelate.com/api/candidateportal/GetAllJobs` request whose
  `requestEnvelope` carries that OrganizationId, and returns one `Job` per posting in the
  response's `Jobs` array

#### Scenario: Portal-level error is surfaced

- **WHEN** the API responds with `IsError: true`
- **THEN** the adapter returns an error for that board (it does not return a partial or empty
  success)

### Requirement: Two-part board identifier

The board SHALL be `<portalSlug>:<organizationId>`: the OrganizationId GUID keys the API and the
portal slug builds each posting's human job URL. Both parts are required.

#### Scenario: Well-formed board is accepted

- **WHEN** a board is `careertree:ec546cba-84d5-4d8a-97e5-52e8ef47db08`
- **THEN** the adapter queries the API with the OrganizationId and builds job URLs under
  `jobs.crelate.com/portal/careertree/`

#### Scenario: Malformed board is rejected

- **WHEN** a board is missing the colon, the slug, or the OrganizationId
- **THEN** the adapter returns an error identifying the expected `<portalSlug>:<organizationId>`
  form, and does not issue a request

### Requirement: Aggregator employer resolution

Because a Crelate portal commonly fronts many client companies, the adapter SHALL be marked as an
aggregator and SHALL set each job's company from the posting's `CompanyName`, falling back to the
configured company when the posting omits it.

#### Scenario: Employer taken from the posting

- **WHEN** a posting carries `CompanyName` "Career Tree Network (Discovery At Home)"
- **THEN** the resulting `Job.Company` is that value, not the board's configured company name

### Requirement: Posting normalization

The adapter SHALL map each posting onto the catalogue's `Job` shape: `Id` to the external id,
`Title`, a human job URL from the portal slug and job code, structured `City`/`State`/`Country` to
the location, `LastPostedOnDate` to the posted date, and the sanitized `Description`. A posting with
an empty `Id` SHALL be dropped so it cannot collide on the dedup key.

#### Scenario: Structured fields are mapped

- **WHEN** a posting has an `Id`, a `City`/`State`, a `LastPostedOnDate` timestamp, and a `Description`
- **THEN** the `Job` carries that id, a "City, State" location, a parsed posted-at time, a URL under
  the portal, and the sanitized description

#### Scenario: Posting without an id is skipped

- **WHEN** a posting has an empty `Id`
- **THEN** it produces no `Job`
