# Unified Freehire MCP

Approved scope: serve the public catalogue and personal account tools from
`/api/v1/mcp`, with OAuth for personal capabilities. Preserve the existing
`/api/v1/mcp/account` contract and the plugin currently under review.

## Behavior

- Anonymous and OAuth clients discover the same 38 tool definitions. The eleven
  catalogue tools keep their existing public schemas, previews and widgets.
- Catalogue calls need no credential. Personal calls without a credential return
  an MCP error result carrying `mcp/www_authenticate` before executing business code.
- Per-tool `_meta.securitySchemes` declares `noauth` or `oauth2`.
- OAuth grants resolve an account through the existing validator. Full API keys
  retain their narrower tool set; CV keys remain forbidden. Invalid credentials
  never fall back to anonymous. Database failures remain server errors.
- Authentication removes caller-supplied internal identity headers, even for
  anonymous requests. Browser cookies grant no MCP account access.
- The account URL remains strict bearer-only and retains its legacy schemas.
- Path-specific RFC 9728 metadata identifies the canonical MCP resource. Existing
  root metadata continues to identify the account resource. OAuth endpoints stay
  unchanged. No database, token model or authorization service is introduced.
- Tool logs and diagnostics retain bounded labels and exclude private arguments.

## Acceptance and implementation plan

1. Exercise the anonymous catalogue and private gate through the SDK transport;
   prove the account handler was not invoked and the challenge survives logging.
2. Mount the mixed handler with optional authentication. Test forged identity,
   discovery, OAuth metadata and the unchanged alias through HTTP.
3. Run OAuth, unknown-token, API-key scope and database-failure integration cases
   against both URLs with a migrated PostgreSQL database.
4. Run the required repository checks and review the diff before delivery.

Plugin manifests and the submitted OpenAI release are unchanged. Moving a packaged
plugin to this URL requires a later manifest release using mixed authentication.
