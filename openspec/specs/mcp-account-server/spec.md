# mcp-account-server Specification

## Purpose
TBD - created by archiving change mcp-oauth-signin. Update Purpose after archive.
## Requirements
### Requirement: Bearer-Gated Signed-In MCP Endpoint
The system SHALL serve an MCP server at `/api/v1/mcp/account` that requires a valid `Authorization: Bearer <token>` header resolving to a live OAuth grant (per the `mcp-oauth-authorization` capability), responding `401 Unauthorized` with no MCP framing attempted when the header is missing, malformed, or does not resolve to a live grant.

#### Scenario: A request with no Authorization header is rejected
- **WHEN** a request reaches `/api/v1/mcp/account` with no `Authorization` header
- **THEN** the response is `401 Unauthorized`

#### Scenario: A request with an expired or revoked token is rejected
- **WHEN** a request presents `Authorization: Bearer <token>` for a grant that has expired or whose session-generation no longer matches
- **THEN** the response is `401 Unauthorized`

#### Scenario: A request with a live token is served
- **WHEN** a request presents `Authorization: Bearer <token>` for a currently valid grant
- **THEN** the MCP server responds to the request as that grant's owning user

### Requirement: Account-Acting Tool Parity With the In-App Assistant
The signed-in MCP server SHALL expose the same tool implementations the in-app assistant uses for its own user — not a separate implementation — covering: resolving who the signed-in account is, searching job postings, reading one posting in full, reading one employer, scoring market fit, saving a job, unsaving a job, marking a job applied, setting a tracking stage and/or note on a job, and listing the account's tracked jobs.

#### Scenario: A tool call and the assistant's equivalent action agree
- **WHEN** the MCP server's `save_job` tool is called for a posting, and separately the in-app assistant's `save_job` tool is called for the same posting by the same user
- **THEN** both calls produce the same stored interaction, because both call the same underlying function

#### Scenario: Every capability named in the request is reachable
- **WHEN** the MCP server's tool list is inspected
- **THEN** it includes tools covering identity ("who am I"), job search, one-posting read, one-employer read, market fit, save, unsave, mark-applied, stage/note setting, and listing tracked jobs

### Requirement: Tool Calls Are Scoped to the Resolved Account Only
Every tool call on the signed-in MCP server SHALL execute as the user id resolved from the request's bearer token, with no mechanism for a request to address or affect any other account's data.

#### Scenario: A tool call only reads or changes the caller's own data
- **WHEN** a tool call is made with a valid bearer token belonging to user A
- **THEN** the call's effects (reads and writes alike) are scoped to user A's own records, regardless of any id-like value present in the tool's arguments
