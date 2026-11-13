## ADDED Requirements

### Requirement: Resource Indicators
The system SHALL accept an optional `resource` parameter (RFC 8707) on `/api/v1/oauth/authorize` and `/api/v1/oauth/token`, bind the issued grant to it, and refuse a bound grant at the other MCP endpoint. A `resource` must be `<origin>/api/v1/mcp` or `<origin>/api/v1/mcp/account` (one trailing slash tolerated); anything else is `invalid_target`. A grant issued without a resource SHALL remain valid on both MCP endpoints.

#### Scenario: Unknown resource at authorize
- **WHEN** `/authorize` carries a valid client and redirect_uri but `resource=https://evil.example/mcp`
- **THEN** the response redirects to the redirect_uri with `error=invalid_target` and the `state`

#### Scenario: Resource round-trips and binds the grant
- **WHEN** a client authorizes with `resource=<origin>/api/v1/mcp/account` and exchanges the code
- **THEN** the grant is bound to that resource and `/api/v1/mcp` answers 401 `invalid_token` for it while `/api/v1/mcp/account` accepts it

#### Scenario: Token resource differs from the bound one
- **WHEN** the code was bound to one resource and `/token` sends the other
- **THEN** the response is 400 `invalid_target`

#### Scenario: No resource keeps today's behaviour
- **WHEN** a grant has no resource
- **THEN** both MCP endpoints accept it
