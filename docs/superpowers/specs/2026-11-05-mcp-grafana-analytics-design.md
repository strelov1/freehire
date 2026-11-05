# MCP analytics in Grafana

Use the existing Prometheus and VictoriaLogs stack. Preserve tool schemas and business responses. Record final SDK outcomes (including output validation), not only handler returns, for public, mixed, account and OJCP calls. Keep labels bounded: surface, registered tool, outcome, fixed error code, and a separate client-family metric.

Structured start/finish events retain request/user correlation without arguments, result bodies, free-form intent or error text. Accept an optional UUID conversation identifier in request metadata `freehire.me/analytics.conversation_id`; hash it before logging and never use it as a Prometheus label. Calls without this identifier remain individual requests. Client families come from recognized metadata/handshake/header signals and collapse to a fixed vocabulary. Raw client names, versions, models and user agents are not recorded.

For successful search responses, measure returned result counts, ignored-filter presence and semantic fallback. Measure serialized tool-result bytes without retaining the payload. Do not equate successful API calls with completed job applications: a journey shows observed calls, not proof of business completion.

Grafana keeps its existing dashboard UID and adds filters, quality panels, client breakdown and a VictoriaLogs activity/journey view. Alert on sustained internal failures and slow calls only with enough traffic, with no-data treated as idle. OJCP shares the same local instrumentation independent of PostHog configuration or quota. Test through the actual SDK, validate queries against Prometheus/VictoriaLogs, and test alert expressions on synthetic traffic. Application deployment precedes dashboards/alerts that depend on new series.

Automatic intent clustering and full conversation content are outside this change. Missing-capability reports stay in the existing private report workflow.

Remove the MCP PostHog integration, its server configuration and Go dependencies once the local collection replaces it in the same release. Browser analytics is unchanged. The former public argument/result snapshots are intentionally not retained; existing historical PostHog data is not modified.
