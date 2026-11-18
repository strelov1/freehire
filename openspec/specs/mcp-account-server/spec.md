# mcp-account-server Specification

## Purpose
TBD - created by archiving change mcp-oauth-signin. Update Purpose after archive.
## Requirements
### Requirement: Bearer-Gated Signed-In MCP Endpoint
The system SHALL serve its MCP server at `/api/v1/mcp`, and only there, requiring a valid `Authorization: Bearer <token>` header resolving to a live OAuth grant (per the `mcp-oauth-authorization` capability) or a full-scope API key, responding `401 Unauthorized` with a `WWW-Authenticate` challenge and no MCP framing attempted when the header is missing, malformed, or does not resolve to a live credential. No request is served anonymously, whatever its User-Agent.

#### Scenario: A request with no Authorization header is rejected
- **WHEN** a request reaches `/api/v1/mcp` with no `Authorization` header, including the `initialize` a host sends while a connector is added
- **THEN** the response is `401 Unauthorized` carrying `WWW-Authenticate: Bearer resource_metadata="<origin>/.well-known/oauth-protected-resource/api/v1/mcp"`

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

### Requirement: A Session Reached Through the freehire CLI Is Told the CLI Is On Its Shell
When the canonical MCP endpoint is reached by the freehire CLI, identified by a
User-Agent beginning with `freehire-cli/`, the server instructions SHALL end with one
sentence stating that the freehire CLI is available on the agent's shell, naming
`freehire search <query> --json` written to a file for whole results with full
descriptions and `freehire cv render <id> --out <file>` for the CV PDF. The sentence SHALL
NOT exceed 256 characters. It SHALL appear for no other User-Agent. No tool description SHALL change as a result.

#### Scenario: The CLI connects
- **WHEN** an MCP client initializes the canonical endpoint with User-Agent `freehire-cli/1.4.0`
- **THEN** the returned instructions end with the sentence naming `freehire search` and `freehire cv render`

#### Scenario: Another host connects
- **WHEN** an MCP client initializes the canonical endpoint with User-Agent `Claude-User` or `openai-mcp`
- **THEN** the returned instructions are identical to those served without this change and do not mention the CLI

