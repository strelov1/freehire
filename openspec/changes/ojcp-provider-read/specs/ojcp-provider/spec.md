## ADDED Requirements

### Requirement: Job Manifest at the well-known path

The system SHALL serve an OJCP Job Manifest at `/.well-known/ojcp.json` over HTTPS with
`Content-Type: application/json`. The manifest SHALL declare `ojcp_version`, a `provider`
object carrying at minimum a `name`, and a `tools` array naming exactly the tools this
deployment implements. It SHALL additionally declare `feed_endpoints`, `mcp_endpoint`,
`auth` and `rate_limits`.

The declared `rate_limits` SHALL match the limits actually enforced in front of the OJCP
endpoints. A declared limit the deployment does not honour is a defect, not a nicety.

#### Scenario: Agent discovers the provider

- **WHEN** an agent fetches `/.well-known/ojcp.json`
- **THEN** it receives HTTP 200 with `Content-Type: application/json`
- **AND** the body validates against the published OJCP manifest schema
- **AND** `tools` names only tools this deployment answers

#### Scenario: Manifest never advertises an unimplemented tool

- **WHEN** the manifest is generated
- **THEN** every name in `tools` resolves to a registered handler on both transports

### Requirement: search_jobs tool

The system SHALL implement `search_jobs` with a conforming input and response schema. The
response SHALL carry `ojcp_version`, `query`, `total_results`, `returned`, `offset` and a
`jobs` array of OJCP `JobPosting` objects.

The result set SHALL be produced by the same search core that serves the public job search,
so an agent and a browser asking the same question receive the same postings.

#### Scenario: Agent searches for postings

- **WHEN** an agent calls `search_jobs` with a query and pagination
- **THEN** the response validates against the OJCP `search-jobs` response schema
- **AND** `returned` equals the length of `jobs`
- **AND** `total_results` reports the full match count, not the page size

#### Scenario: Unreadable filter widens the answer

- **WHEN** an agent passes a filter value the search vocabulary does not recognise
- **THEN** the response still validates and the unread parameters are reported back
- **AND** the answer is never silently narrowed to nothing

### Requirement: get_job_detail and get_employer_context tools

The system SHALL implement `get_job_detail`, returning one OJCP `JobPosting` with the
posting's full description, and `get_employer_context`, returning the employer projection
for a company we hold.

#### Scenario: Agent reads one posting

- **WHEN** an agent calls `get_job_detail` with an `ojcp_id` the catalogue holds
- **THEN** the response carries that posting with its full description
- **AND** the response validates against the OJCP `job-detail` response schema

#### Scenario: Agent asks for a posting we do not hold

- **WHEN** an agent calls `get_job_detail` with an unknown `ojcp_id`
- **THEN** the system answers with an OJCP error envelope and a not-found status
- **AND** it does not answer with an empty posting object

### Requirement: JobPosting projection

The system SHALL project a catalogue posting into an OJCP `JobPosting` such that:

- `ojcp_id` is the posting's stable public slug,
- `url` is our own page for the posting,
- `official_job_url` is the source employer's own link for it,
- `skills_required` carries the posting's resolved skill facets,
- `agent_notes` carries our posting-reality verdict when one exists.

The projection SHALL be a pure function of an already-loaded domain value. It SHALL NOT
read the database, and it SHALL NOT depend on the HTTP framework.

#### Scenario: Source attribution travels with the record

- **WHEN** a posting originating from an external source is projected
- **THEN** `official_job_url` names the source's own URL for that posting
- **AND** `url` names our page, so the two are never conflated

#### Scenario: Posting-reality verdict reaches the agent

- **WHEN** a posting carries a ghost verdict
- **THEN** `agent_notes` states that verdict
- **AND** a posting with no verdict omits the field rather than asserting a neutral one

### Requirement: ApplyPath projection from captured forms

The system SHALL project a posting's stored application form into an OJCP `ApplyPath`
carrying `type`, `ats_provider`, `required_fields` and `supports_agent_submission`.

`supports_agent_submission` SHALL be true only for postings this deployment can actually
submit to unattended. A posting on an ATS whose form we can read but not submit — one
gated by a challenge we do not solve — SHALL report `false`.

A posting with no captured form SHALL report an `external_redirect` path rather than
omitting `apply_paths`.

#### Scenario: Posting with a captured, submittable form

- **WHEN** a posting has a stored apply form on a platform auto-apply supports
- **THEN** its `ApplyPath` names the ATS provider and lists the form's required fields
- **AND** `supports_agent_submission` is true

#### Scenario: Posting on a challenge-gated platform

- **WHEN** a posting's ATS gates submission behind a challenge this deployment does not solve
- **THEN** `supports_agent_submission` is false
- **AND** the required fields are still published, so an agent can still prepare a human hand-off

#### Scenario: Posting with no captured form

- **WHEN** a posting has no stored application form
- **THEN** its `apply_paths` carries a single `external_redirect` entry pointing at the source URL

### Requirement: Two transports, one answer

The system SHALL expose each read tool over both a REST transport under `/ojcp/v1/` and an
MCP transport at the manifest's `mcp_endpoint`. Both transports SHALL be thin adapters over
one shared implementation, so that the same call through either returns the same projected
payload.

Errors SHALL be rendered in the transport's own idiom: a JSON-RPC error with the OJCP error
envelope in its `data` field on MCP, and an appropriate HTTP status with the envelope as the
body on REST.

#### Scenario: Same question, same answer

- **WHEN** the same tool call is made over REST and over MCP
- **THEN** the projected payload is identical

#### Scenario: Error rendering follows the transport

- **WHEN** a tool call fails
- **THEN** the REST caller receives the matching HTTP status with the error envelope as the body
- **AND** the MCP caller receives a JSON-RPC error carrying the same envelope in `data`

### Requirement: Visibility matches the public catalogue

The OJCP surface SHALL publish exactly the postings the public job search publishes — open,
canonical, and not private — using that same predicate rather than a second copy of it.

#### Scenario: A private posting is never projected

- **WHEN** a private posting exists in the catalogue
- **THEN** no OJCP tool returns it, on either transport

#### Scenario: A suppressed duplicate is never projected

- **WHEN** a posting is marked as a duplicate of another
- **THEN** OJCP returns the surviving posting and not the suppressed one

### Requirement: Unrecognised fields are ignored, not rejected

The system SHALL ignore input fields it does not recognise rather than failing the call, per
the OJCP extensibility rule.

#### Scenario: Agent sends a field from a later spec version

- **WHEN** a tool call carries a field this implementation does not know
- **THEN** the call succeeds and the unknown field is ignored
