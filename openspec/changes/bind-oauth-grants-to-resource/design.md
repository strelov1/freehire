## Decisions

- **Identifiers**: `<origin>/api/v1/mcp` and `<origin>/api/v1/mcp/account`, exactly what the two protected-resource documents advertise. A single trailing slash is tolerated on input (clients canonicalise inconsistently) and normalised away before storage; no other variant (case, query, fragment) is accepted.
- **Multiple `resource` values**: a binding is to one resource, so several distinct values are `invalid_target`; repeats of the same value are one.
- **/authorize error**: once client and redirect_uri are validated, `invalid_target` goes back as a redirect (`error`, `error_description`, `state`) per RFC 6749 4.1.2.1. At /token it is a 400 `{"error":"invalid_target","error_description":...}`.
- **/token**: a `resource` is validated before the code is consumed. If the code was bound, a differing `resource` is `invalid_target`; if the code was unbound, the `resource` sent at /token binds the grant.
- **Refresh**: this server issues no refresh tokens (`grant_types_supported` is `authorization_code`), so there is nothing to carry over.
- **Enforcement**: only the two MCP endpoints compare the binding. A grant with NULL resource is valid on both (legacy and clients that do not send it).
- **REST**: unchanged. An OAuth grant is admitted wherever an API key is (`RequireAuthOrKey`); the binding scopes which MCP server a token is for, and REST is not an MCP server. Revisit if a grant should be narrowed further.
- Clients that connected to `/mcp/account` through the root metadata send `resource=<origin>/api/v1/mcp/account` and keep working there; they are refused at `/mcp`.
