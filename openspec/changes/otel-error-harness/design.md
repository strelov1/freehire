# Design

Use official OpenTelemetry log SDKs and OTLP exporters. Vector v0.57.0 already
supports native OTLP log ingestion, so no additional Collector is needed. Its
loopback HTTP receiver accepts server/worker events. nginx exposes only the browser
log-ingestion path with request limits; it does not expose VictoriaLogs queries.
Vector forwards application events through the existing authenticated, disk-buffered
VictoriaLogs sink. Existing journald and nginx sources remain operational.

Resources identify service, deployment environment and build version. Exception
records include type, message and stack, with ERROR severity. Expected client errors,
cancellations and disconnects retain the existing exclusion policy. Reporting must
not block requests or recursively report exporter failures. Short-lived workers flush
after their final fault or panic. No credentials, cookies, request bodies or query
parameters are intentionally collected.

Browser and extension hooks report their own uncaught errors and rejected promises;
extension content scripts must not report unrelated host-page faults. SSR reporting
runs only in the server runtime. Browser exporters are bounded and opt-in. Existing
Sentry remains a fallback, with OpenTelemetry taking precedence when enabled.

Operators use official vlogscli for interactive queries and curl/jq for automation
through an SSH tunnel. Use existing Grafana alerting if its VictoriaLogs datasource
supports it, avoiding a bespoke alert engine. Alert failures must be distinguishable
from an empty result. Query, alert and ingestion examples use the same event fields.

Verify SDK output with local OTLP receivers; verify configuration with the pinned
Vector binary and exercise OTLP ingestion, VictoriaLogs queries and alert evaluation
locally where runtimes are available. Retain private build sourcemaps for investigating
minified JavaScript; raw minified stacks must not be presented as symbolicated.
