-- Bind an OAuth authorization code, and the grant it becomes, to the MCP
-- resource it was requested for (RFC 8707 resource indicator). NULL means
-- unbound: codes and grants issued before this migration, or by a client that
-- sent no `resource`, stay valid on every MCP endpoint.
ALTER TABLE oauth_authorization_codes ADD COLUMN resource TEXT;
ALTER TABLE oauth_grants ADD COLUMN resource TEXT;
