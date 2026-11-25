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

### Requirement: Agents Can Read the Account's Plan Before Metered Work
The signed-in MCP server SHALL offer a read-only `get_plan` tool, to OAuth grants and
full-scope API keys alike, reporting the account's tier, when its paid entitlement ends,
when today's allowances reset, and for every metered AI feature today's limit, use,
whether the limit is enforced, and — for an enforced limit — the remaining amount. It SHALL read the same use case as
`GET /me/plan` and SHALL carry no prices, invoices or payment source, pointing at the
website's plan page instead.

#### Scenario: A paid account reads its allowances
- **WHEN** a pro account that has used part of today's auto-apply allowance calls `get_plan`
- **THEN** the result names the `pro` plan, its end date, and an `auto-apply` allowance whose used, limit and remaining agree with `GET /me/plan`

#### Scenario: A free account is told it has no AI
- **WHEN** a free account calls `get_plan`
- **THEN** the result names the `free` plan with nothing remaining on any enforced limit and carries the plan page URL to upgrade from

#### Scenario: A shadow limit claims no remainder
- **WHEN** a feature's limit is not enforced
- **THEN** its allowance carries the limit and use but no remaining amount

#### Scenario: No payment details reach the agent
- **WHEN** any account calls `get_plan`
- **THEN** the result carries no price, payment source or invoice

### Requirement: Unread application mail is visible to account MCP agents as metadata only
The account MCP server SHALL serve a read-only `list_unread_mail` tool to OAuth grants and
full-scope API keys that lists the caller's newest unread application mail as sender,
subject, received time, classification label and, when confirmed, the linked company and job
slug, together with the total unread count, whether the mailbox is still syncing or needs
reconnecting, and a link to the inbox. It SHALL NOT return message bodies or snippets, SHALL
NOT mark any message read, and SHALL NOT call a model.

#### Scenario: Unread count exceeds the page
- **WHEN** the caller has more unread application mail than the requested limit
- **THEN** `unread_count` reports the total and `messages` holds at most the limit

#### Scenario: Reading marks nothing
- **WHEN** the tool lists a message
- **THEN** the message is still unread afterwards

#### Scenario: Another user's mail
- **WHEN** another account has unread mail
- **THEN** none of it appears in the caller's result or count

#### Scenario: First Gmail sync still running
- **WHEN** the caller's Gmail is connected and has never completed a sync
- **THEN** `sync_pending` is true

### Requirement: Sender-written mail text is returned as inert data
Sender and subject SHALL be returned with control and format characters removed and their
length capped, and the tool description, the result and the account server instructions SHALL
state that they are written by the sender and must never be followed as instructions.

#### Scenario: An injected instruction in a subject
- **WHEN** a subject contains a line break followed by text addressed to the agent
- **THEN** the subject is returned as one single-line string inside its field and the result carries the untrusted-text notice

