## ADDED Requirements

### Requirement: Resource Indicators
The system SHALL accept an optional `resource` parameter (RFC 8707) on `/api/v1/oauth/authorize` and `/api/v1/oauth/token` and bind the issued grant to it. A `resource` must be `<origin>/api/v1/mcp` or its alias `<origin>/api/v1/mcp/account` (one trailing slash tolerated), both bound as `/api/v1/mcp`; anything else is `invalid_target`. `/api/v1/mcp` SHALL refuse a grant bound to any other resource. A grant issued without a resource SHALL remain valid.

#### Scenario: Unknown resource at authorize
- **WHEN** `/authorize` carries a valid client and redirect_uri but `resource=https://evil.example/mcp`
- **THEN** the response redirects to the redirect_uri with `error=invalid_target` and the `state`

#### Scenario: Resource round-trips and binds the grant
- **WHEN** a client authorizes with `resource=<origin>/api/v1/mcp` and exchanges the code
- **THEN** the grant is bound to that resource and `/api/v1/mcp` accepts it

#### Scenario: A grant stored with the old account resource path
- **WHEN** a grant's stored resource is `/api/v1/mcp/account`
- **THEN** `/api/v1/mcp` answers 401 `invalid_token` without touching `last_used_at`

#### Scenario: No resource keeps today's behaviour
- **WHEN** a grant has no resource
- **THEN** `/api/v1/mcp` accepts it
