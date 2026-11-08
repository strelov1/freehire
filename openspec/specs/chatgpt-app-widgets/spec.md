# chatgpt-app-widgets Specification

## Purpose

TBD - created by archiving change chatgpt-app-widgets. Update Purpose after archive.

## Requirements

### Requirement: One MCP Apps UI resource

The ChatGPT app server SHALL list and serve exactly one UI resource, with MIME type
`text/html;profile=mcp-app`, whose URI is `ui://freehire/jobs-<hash>.html` where `<hash>` is
derived from the page's bytes.

The resource SHALL declare `_meta.ui.csp.resourceDomains` as `https://logo.freehire.me` and
SHALL declare no `connectDomains` and no `frameDomains`.

#### Scenario: A host reads the widget

- **WHEN** a client calls `resources/list` and then `resources/read` on the listed URI
- **THEN** it receives the HTML page with MIME type `text/html;profile=mcp-app`
- **AND** the CSP metadata admits images from the logo proxy and nothing else

#### Scenario: The page changes

- **WHEN** the embedded HTML changes
- **THEN** the resource URI changes with it

### Requirement: Two tools render through it

`search_jobs` and `get_job` SHALL carry `_meta.ui.resourceUri` naming the listed resource.
No other tool SHALL carry it. The tools' inputs, outputs and annotations SHALL be unchanged.

#### Scenario: A tool names a resource that exists

- **WHEN** a client lists tools and resources
- **THEN** each tool's `resourceUri` is the URI of a listed resource

#### Scenario: A client without MCP Apps

- **WHEN** a client ignores `_meta.ui`
- **THEN** it receives the same tool results as before this change

### Requirement: What the widget shows

For a `search_jobs` result the page SHALL render one card per job with logo, title, company,
location, work mode, salary when stated, seniority, up to five skills and the source, plus a
link to the posting on freehire and, when present, to the employer's own posting. It SHALL
name any `ignored_params`.

For a `get_job` result the page SHALL render the same card header, the liveness signals
(`reality`, `ghost_level`), the requirements split by priority, the applicant tracking
system, and the full description collapsed behind a "show all" control.

#### Scenario: A logo is missing

- **WHEN** the logo proxy answers an error for a company
- **THEN** the card shows a monogram instead

### Requirement: The widget treats results as untrusted

The page SHALL insert result values as text only, SHALL use a URL only when it parses with
the `https:` scheme, and SHALL make no network request other than loading logo images.

#### Scenario: A posting carries markup

- **WHEN** a title or description contains HTML
- **THEN** the page shows it as literal text and no element is created from it

### Requirement: The widget reads only fields the wire shape has

Every result field the page reads SHALL exist as a JSON field of the wire types in
`internal/api/mcpapp/wire.go`, enforced by a test over the page's source.

#### Scenario: A wire field is renamed

- **WHEN** a field the page reads is renamed or removed in `wire.go`
- **THEN** the test fails naming the field
