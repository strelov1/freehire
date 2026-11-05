# Infrastructure authentication

`AUTH_INTROSPECTION_SECRET` enables `GET /internal/auth/introspect` for trusted
infrastructure. The secret must contain at least 32 bytes and arrive in
`X-FH-Auth-Secret`. The endpoint is disabled when the setting is empty. Keep it
internal at the reverse proxy and never expose or log the shared secret.

The endpoint accepts API keys, OAuth access tokens, and signed sessions through
the existing cookie or Bearer carriers. A valid cookie takes precedence over a
Bearer credential, matching the ordinary REST authentication flow. Bearer-only
consumers must omit the cookie. Database lookups confirm credential liveness and
session generation; these read-only queries do not change `last_used_at`.

The response contains the account ID as a string, credential kind, original
scope, signed or stored expiry, and the account's entitlement timestamps as Unix
seconds. It contains no credentials or personal profile data. A narrow-scope key
can be introspected successfully without acquiring additional permissions.
Unknown or revoked credentials return 401; infrastructure failures return 503.

An infrastructure cache belongs to `freehire-ops`. Protected application routes
continue to verify their original credentials and operation permissions, so a
cached infrastructure identity cannot authorize a private action.

For attribution only, trusted infrastructure may forward `X-FH-Auth-User` with
the shared secret. The application verifies this metadata and strips its headers
before ordinary handlers. It never sets the authenticated user from this value.
The private MCP endpoint's own verified account takes precedence over external
attribution metadata.
