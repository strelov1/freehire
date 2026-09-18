## ADDED Requirements

### Requirement: MCP server at a stable public endpoint

The system SHALL serve an MCP server over the streamable HTTP transport at
`/api/v1/mcp`, reachable publicly over HTTPS without authentication.

The endpoint SHALL accept POST for tool calls and GET for the server-to-client stream, and
SHALL answer `initialize` with a server name and version distinct from the OJCP server's.

#### Scenario: ChatGPT scans the server

- **WHEN** a client sends `initialize` followed by `tools/list`
- **THEN** it receives a protocol-conformant result
- **AND** `tools/list` names exactly the tools this deployment implements

#### Scenario: The OJCP surface is unaffected

- **WHEN** the new server is mounted
- **THEN** `/api/v1/ojcp/mcp` continues to answer with the OJCP server's own name, tools
  and error shape
- **AND** the OJCP manifest names only the OJCP tools

### Requirement: Four read tools over our own wire shapes

The system SHALL expose `search_jobs`, `get_job`, `search_companies` and `get_company`.

Each tool SHALL answer from the same reader the REST handlers use, so the two surfaces
cannot disagree about the catalogue's contents. No tool SHALL build a query of its own.

`search_jobs` SHALL return the search index's truncated description preview. `get_job`
SHALL return one posting's full description.

#### Scenario: A search answers in our shape, not OJCP's

- **WHEN** `search_jobs` is called
- **THEN** each result carries the fields our own job projection publishes, including
  seniority values outside OJCP's vocabulary and salary detail the standard cannot hold

#### Scenario: A search does not return full bodies

- **WHEN** `search_jobs` returns ten postings
- **THEN** each description is the index preview, not the full stored body

#### Scenario: Detail reads the full body

- **WHEN** `get_job` is called with a posting's public slug
- **THEN** the result carries that posting's full description

### Requirement: Tool metadata sufficient for app review

Every registered tool SHALL declare a human-readable title, a description stating when to
use it in terms of what it does, an explicit input schema, an output schema, and the
annotations `readOnlyHint: true`, `destructiveHint: false` and `openWorldHint: true`.

No tool description SHALL contain promotional language.

The server SHALL declare instructions under 512 characters.

#### Scenario: Every tool is annotated

- **WHEN** the registered tools are enumerated
- **THEN** each declares all three annotations
- **AND** each declares a title and a non-empty description

#### Scenario: A write tool cannot inherit a read-only claim

- **WHEN** a tool is registered that is not read-only
- **THEN** the annotation test fails rather than passing by inheritance

### Requirement: A curated filter vocabulary, with unreadable filters reported

`search_jobs` SHALL publish a bounded subset of the search filter vocabulary as its input
schema, covering free text, country, city, work mode, seniority, category, skills,
employment type, salary floor and currency, visa sponsorship, English level, company,
source, posted-within-days, and page bounds.

A filter VALUE the tool receives but cannot honour SHALL be dropped from the query and named
in the result, rather than narrowed on.

This is about values, not parameter names, and the distinction was MEASURED rather than
chosen: the MCP SDK derives the input schema from the Go type with `additionalProperties:
false`, so a parameter the schema does not declare is refused by the transport before any
handler runs. A tool therefore cannot report an unpublished parameter as ignored — it never
sees one. What it can and must report is a value outside a closed vocabulary, which reaches
the handler and would otherwise filter on a value no posting carries.

#### Scenario: An unhonourable filter value is named

- **WHEN** `search_jobs` receives a value outside a closed vocabulary, such as a category we
  hold no facet for
- **THEN** that value is dropped from the query rather than filtered on
- **AND** the result names it as `param=value`
- **AND** the free text still runs, so the answer is wider than asked rather than empty

#### Scenario: An undeclared parameter is refused by the transport

- **WHEN** a caller sends a parameter the tool's input schema does not declare
- **THEN** the SDK answers a validation error before the handler runs
- **AND** nothing in this surface has to report it

#### Scenario: Page bounds are enforced

- **WHEN** a caller requests more results than the tool's maximum page
- **THEN** the tool returns at most its maximum and reports the total separately

### Requirement: Attribution travels with every result

Every posting a tool returns SHALL carry the name of the source it came from and the
employer's own job URL beside freehire's own page URL.

Every company a tool returns SHALL carry its freehire page URL.

#### Scenario: A posting names its source

- **WHEN** any tool returns a posting
- **THEN** the result carries the source name and the employer's own URL when one is stored

### Requirement: Failures are recoverable tool results

A tool failure SHALL be returned as a tool result marked as an error carrying a
human-readable sentence, not as a JSON-RPC protocol error.

A request for something absent SHALL be distinguishable from a deployment failure. The
absent case SHALL name what was not found; the failure case SHALL NOT disclose internal
detail.

#### Scenario: An unknown posting is recoverable

- **WHEN** `get_job` is called with a slug that matches no posting
- **THEN** the result is an error result stating that no posting carries that identifier
- **AND** it is not a JSON-RPC protocol error

#### Scenario: An internal failure says only that it failed

- **WHEN** the reader fails for any reason other than absence
- **THEN** the result states that freehire could not answer, with no internal detail

### Requirement: Visibility matches the public catalogue

No tool SHALL return a posting that the public search would not return: private postings,
non-canonical duplicates and closed postings SHALL be unreachable through every tool.

#### Scenario: A private posting is unreachable

- **WHEN** a private posting's slug is passed to `get_job`
- **THEN** the result is the absent-posting error, identical to an unknown slug

### Requirement: Rate limiting matches the neighbouring agent surfaces

The endpoint SHALL be rate limited by the same limiter applied to `/agent/jobs/search` and
the OJCP endpoints.

#### Scenario: The limit is shared, not re-declared

- **WHEN** the route is registered
- **THEN** it uses the existing agent-search limiter rather than a new figure
