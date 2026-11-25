## Why

The MCP authorization spec (2025-06-18, 2025-11-25) requires resource indicators (RFC 8707): a client sends `resource` on /authorize and /token, and the server binds the token to that resource. Our OAuth server ignores `resource`, so a grant minted for `/api/v1/mcp` is also accepted at `/api/v1/mcp/account` and the reverse.

## What Changes

- `/authorize` (query and the consent form round-trip) and `/token` accept an optional `resource`.
- A `resource` that is not one of the two MCP endpoint identifiers is refused with `invalid_target`.
- The resource is stored (nullable) on the authorization code and the grant.
- The MCP endpoints refuse a grant bound to the other endpoint with 401 `invalid_token`.
- A grant without a resource keeps working on both endpoints, so already-connected clients are unaffected.

## Impact

- Migration: nullable `resource` on `oauth_authorization_codes` and `oauth_grants`.
- `AuthenticateOAuthGrant` also returns the grant's resource.
