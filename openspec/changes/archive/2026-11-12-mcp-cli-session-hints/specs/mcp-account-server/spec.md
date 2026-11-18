## ADDED Requirements

### Requirement: A Session Reached Through the freehire CLI Is Told the CLI Is On Its Shell
When the canonical MCP endpoint is reached by the freehire CLI, identified by a
User-Agent beginning with `freehire-cli/`, the server instructions SHALL end with one
sentence stating that the freehire CLI is available on the agent's shell, naming
`freehire search <query> --json` written to a file for whole results with full
descriptions and `freehire cv render <id> --out <file>` for the CV PDF. The sentence SHALL
NOT exceed 256 characters. It SHALL appear for anonymous and signed-in sessions alike, and
for no other User-Agent. No tool description SHALL change as a result.

#### Scenario: The CLI connects
- **WHEN** an MCP client initializes the canonical endpoint with User-Agent `freehire-cli/1.4.0`
- **THEN** the returned instructions end with the sentence naming `freehire search` and `freehire cv render`

#### Scenario: Another host connects
- **WHEN** an MCP client initializes the canonical endpoint with User-Agent `Claude-User` or `openai-mcp`
- **THEN** the returned instructions are identical to those served without this change and do not mention the CLI

#### Scenario: Anonymous CLI session
- **WHEN** the freehire CLI initializes the canonical endpoint without credentials
- **THEN** the session is served (not refused) and its instructions end with the CLI sentence
