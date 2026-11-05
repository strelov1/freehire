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

## Grafana analytics

MCP analytics uses local Prometheus metrics and VictoriaLogs events. There is no
PostHog MCP client, project token, or external event delivery. Public argument and
response snapshots from the former integration are no longer captured. All four
surfaces (`public`, `mixed`, `account`, `ojcp`) use `ObserveServer` and observed tool
registration. The receiving middleware records the final SDK result, including
input and output validation. Unknown tool names become `unknown`; caller-authored
strings never become metric labels. OJCP invalid-params protocol errors are caller
failures rather than internal failures.

- `freehire_mcp_tool_calls_total{surface,tool,outcome,error_code}` counts final outcomes.
- `freehire_mcp_tool_duration_seconds{surface,tool}` measures dispatch through SDK validation.
- `freehire_mcp_client_calls_total{surface,client}` uses a fixed client-family vocabulary.
  The first recognized family wins across request metadata, handshake metadata,
  and User-Agent. Generic names do not hide a recognized header. These unverified
  client signals are not authentication.
- `freehire_mcp_response_bytes{surface,tool}` measures serialized tool-result bytes,
  including duplicate structured/text content, excluding the JSON-RPC envelope.
  Finite buckets cover sizes through 64 MiB. Grafana shows overflow separately
  and omits p95 when that percentile would fall in the unbounded bucket.
- `freehire_mcp_search_quality_total{surface,signal}` counts `all`, `empty`,
  `ignored_filters`, `semantic_requested`, and `semantic_fallback` signals for
  successful job searches, including research search. Signals overlap. Empty means
  the returned page is empty, including a page requested beyond the last match.
- `freehire_mcp_problem_reports_total{surface,category}` counts accepted report calls
  by their fixed category. A deduplicated repeat still counts as a call, not a new issue.

Finish events add the same safe metadata and counts, never bodies or raw headers.
To correlate calls across stateless requests, a client may send a UUID in the
`conversation_id` field of request metadata `_meta["freehire.me/analytics"]`.
Only its SHA-256 prefix (32 hexadecimal characters) enters logs. The same UUID
correlates across replicas; different conversations need different UUIDs. This is
an unverified correlation hint, not an identity. Filter by the resolved `user_id`
as well when inspecting account activity. The hash is not a Prometheus label.

Clients that do not send a handle remain visible as individual requests; neither
IP addresses nor an account-wide inactivity window stand in for a conversation.
Tool schemas are unchanged. Successful `apply_job` calls record tracking, and
`auto_apply_start` queues an attempt: neither establishes employer submission.
Free-form intent, model self-reporting, and automatic intent clustering are not
part of this local telemetry.
