# MCP and bearer authentication diagnostics

The API generates a fresh `X-Request-ID` for every HTTP request. Caller-supplied
values are ignored. The API request log and structured authentication/tool logs
share this ID. Nginx access logs still describe the HTTP transport only.

An MCP tool failure can be an HTTP 200 with `isError: true`. Its result includes
`_meta["freehire.me/diagnostics"]` with `request_id` and `error_code`; search the
API service journal for that request ID rather than searching only for HTTP 4xx.
The metadata describes a failure without changing MCP's transport status.

Structured `authentication refused` records contain the route pattern, status
and a fixed reason, such as `insufficient_scope`, `missing_bearer_token`,
`invalid_or_expired_credential` or `credential_lookup_failed`.
When a credential resolves, records also identify its type (`api_key`,
`oauth_grant` or `session`) and user ID. API keys additionally carry their database
`key_id` and `scope`. An unknown or expired token cannot be attributed to a key;
the authentication query deliberately returns no row for either case.

`mcp tool started` and `mcp tool finished` records carry the same request and
resolved identity. The per-process `call_id` pairs a tool's start and finish;
use `request_id` to correlate across processes. The finish record distinguishes
`success`, `tool_error`, `error`, `canceled` and `deadline_exceeded`.
`error_code` classifies known error types and sentinels; unclassified failures
use `internal_error`. It never derives a code from an error's text.

These structured records exclude credentials, credential hashes, tool arguments,
result bodies and raw error text. Do not add those fields to investigate a
specific incident. This logging helps diagnose future calls; it cannot recover
information that older deployments did not record.
