# Exception collection and terminal investigation

The `otel-error-harness` change sends explicit exception logs through the existing
Vector receiver and VictoriaLogs store. It covers Go API errors, recovered panics,
match-analysis stream failures, workers using `worker.Main`, SvelteKit errors and
the extension's own background, content and panel errors. It does not turn every
returned Go error or every application-level SSE message into telemetry.

This is searchable exception aggregation. It does not provide Sentry's issue
lifecycle, automatic grouping or automatic source-map resolution. Sentry remains
an optional fallback while the new path is rolled out.

## Configuration

Runtime Go processes and SSR use `OTEL_EXPORTER_OTLP_LOGS_ENDPOINT`, including the
full `/v1/logs` path, or the base `OTEL_EXPORTER_OTLP_ENDPOINT`. A typical local
receiver is `http://127.0.0.1:4318/v1/logs`. Set
`OTEL_RESOURCE_ATTRIBUTES=deployment.environment.name=production` for Go. Leave
`OTEL_SERVICE_NAME` unset to retain each executable's service identity. Workers on
another host need their own reachable trusted receiver: loopback on the crawl
host does not reach host2, and the public browser route deliberately labels all
events as client-origin events.

The web build declares `PUBLIC_OTEL_LOGS_ENDPOINT=/telemetry/v1/logs`,
`PUBLIC_OTEL_ENVIRONMENT=production` and `PUBLIC_OTEL_SERVICE_VERSION` equal to
the build's git SHA. SSR also needs its private OTLP endpoint in the Node runtime.
The extension build uses `WXT_OTEL_LOGS_ENDPOINT=https://freehire.me/telemetry/v1/logs`
and `WXT_OTEL_ENVIRONMENT=production`; its manifest supplies the version. These
public settings contain no credentials. Changing a build setting requires a new
build; changing only a systemd environment file cannot alter an installed extension.

Without an OTLP endpoint, collection is disabled and existing Sentry configuration
can be used. With an endpoint configured, OpenTelemetry takes precedence. Removing
the OTLP settings and redeploying/restarting the affected components rolls back
collection. Keep valid Sentry settings during the transition if fallback is needed.

On the existing host layout, put Go runtime settings in the environment files its
units already read and Node/SSR settings in `/opt/freehire/env/hire-web.env`.
The release script also reads the public OTLP endpoint/environment from that web
file for builds and stamps the release SHA as the public service version.
When retiring Sentry uploads, remove the entire optional
`/opt/freehire/env/sentry-build.env` file: leaving org/project settings while
clearing only its token still trips the existing credential gate. Remove
inherited Sentry upload variables from the release environment too. Runtime OTLP
configuration alone does not bypass a deliberately configured Sentry upload gate.

## Operations files

The corresponding files live in the private `freehire-ops` repository:

- `provision/host2/vector/otel.yaml` adds separate trusted and public-client OTLP
  receivers and reconstructs an allowlisted event shape.
- `provision/host2/vector/vector.yaml` sends their normalized outputs through the
  existing authenticated, disk-buffered VictoriaLogs sink.
- `provision/host2/systemd/vector.service.d/otel.conf` loads both Vector files.
- `provision/host2/nginx/conf.d/freehire-telemetry.conf` defines request limits;
  `nginx/snippets/freehire-telemetry.conf` exposes only protobuf log ingestion.
- `provision/litellm-host/grafana/provisioning/alerting/application-errors.yaml`
  defines panic and error-burst thresholds. Existing notification policies route
  alerts to Telegram and control grouping and repeat intervals. It also defines
  collector-unavailable and delivery-error alerts using the existing Prometheus
  datasource.
- `provision/host2/env/otel.env.example` documents application settings.
- `scripts/errors.sh` wraps official query endpoints and `vlogscli`.

Install these files in their matching host locations, retaining existing Vector
credentials and Grafana contact points. Validate the combined Vector configuration
with the deployed version, run `nginx -t`, and reload systemd and the affected
services. Source edits alone do not install infrastructure. Do not restart Vector
with its modified main file while omitting the new OTLP file: the sink references
the transforms it defines.

Check the actual host topology before installing: the API and Node services run on
fsn, while host2 also runs crawl workers. The directory name `host2` is historical
and is not evidence that every producer runs there. Each process's loopback
endpoint needs a receiver on that same machine, or an explicitly configured
private transport. Installing only the crawl host's receiver leaves API and SSR
events undelivered.
Set `VECTOR_HOST_NAME=fsn` for the fsn receiver so its OTLP records are not
labelled with the default `host2` identity. Review existing journald host labels
when adapting the historical host2 configuration to another machine.

Vector exports its own metrics on loopback port 9598 by default. Before making
the metrics available to the existing Prometheus scraper, restrict both host and
provider firewalls to the monitoring host, then set `VECTOR_METRICS_ADDRESS` in
the existing Vector environment file to the intended listen address. The scrape
targets must match the hosts where receivers are installed. A stopped or missing
target alerts after two minutes; delivery-error counters expose failures to reach
the VictoriaLogs sink. Do not expose the metrics port unrestricted to the Internet.

The public ingress is unauthenticated, rate-limited and body-limited. Browser
events cannot claim server origin. VictoriaLogs query endpoints remain private.
The receiver accepts protobuf, not OTLP JSON. Extension content scripts relay their
own errors through the background worker rather than fetching from a host page.

## Query from a terminal

Use an SSH tunnel to the machine running VictoriaLogs; do not expose its query API
publicly. With `freehire-ops/scripts/errors.sh` available:

```sh
VLOGS_SSH_HOST=litellm-host ./scripts/errors.sh tunnel
# In another terminal; VLOGS_URL defaults to this tunnel's port 19428.
./scripts/errors.sh recent
./scripts/errors.sh query '_time:1h log_source:otel service:freehire-api | limit 50'
./scripts/errors.sh query '_time:1h log_source:otel | stats by (service, version) count() as errors'
./scripts/errors.sh check
./scripts/errors.sh shell
```

`check` returns 0 for a numeric zero count, 1 when exceptions exist, and 2 for a
failed query or malformed response. An empty response is a failure, not a healthy
check. Interactive mode requires the official `vlogscli` binary; the other modes
use `curl`, with `jq` additionally required for `check`.

The direct official API is also suitable for scripts:

```sh
curl --fail --silent --show-error --max-time 30 \
  http://127.0.0.1:19428/select/logsql/query \
  --data-urlencode 'query=_time:15m log_source:otel | stats count() as errors'
```

Normalized fields include `service`, `environment`, `version`, `origin`,
`exception_type`, `message`, `stacktrace`, `error_source` and `event_name`.
Go events can additionally carry a route template, method, worker and exit code.
Use `origin:server` for server alerts and `origin:client` for browser/extension
alerts. Do not use an arbitrary user-controlled URL as a stream identity.

Optional agent access is available through the official
[VictoriaLogs MCP server](https://github.com/VictoriaMetrics/mcp-victorialogs).
Point it at the private query endpoint through the same tunnel and keep access
read-only. MCP is optional; terminal investigation requires no agent plugin.

## Acceptance and rollout

Before disabling Sentry, verify each instrumented build end to end. Record the
build SHA, environment, service and a unique synthetic error message; then query
that message from a fresh terminal session. Exercise an unexpected Go handler
error, recovered request panic, failed match-analysis SSE after headers, a worker
failure and panic, an SSR error, a browser handled error and uncaught rejection,
and the extension panel, background and content contexts. Content must reject
unrelated host-page errors. Check that routine client errors and cancellations
stay excluded, and URLs with query credentials are not retained.

For workers, verify the original nonzero exit/panic and final exception delivery
before shutdown. Check browser navigation and extension suspension separately;
bounded best-effort delivery cannot promise survival of SIGKILL or a browser crash.

In a controlled environment, temporarily lower an alert threshold, inject an
identifiable exception and verify query evaluation, Telegram delivery, grouping,
repeat behavior and resolution. Test a failed datasource query too: its configured
error state must alert rather than pass as no data. Restore thresholds afterward.
Existing collector/storage-health monitoring remains necessary: a healthy query
with no exceptions cannot prove that a stopped producer is delivering.

Hidden source maps remove the source-map URL from generated JavaScript; they are
not automatic symbolication. Retain maps privately with the exact build/version,
keep them out of public assets and extension distribution archives, and resolve
minified frames locally against those matching artifacts. Until that workflow is
verified, treat JavaScript stacks as raw minified stacks.

Do not mark production activation complete from SDK tests or config validation.
The rollout is complete only after the deployed binaries, installed configuration,
stored events and notification delivery have all been checked.

## Primary references

- [VictoriaLogs 1.52.0 CLI documentation](https://github.com/VictoriaMetrics/VictoriaLogs/blob/v1.52.0/docs/victorialogs/querying/vlogscli.md)
- [VictoriaLogs 1.52.0 OTLP ingestion](https://github.com/VictoriaMetrics/VictoriaLogs/blob/v1.52.0/docs/victorialogs/data-ingestion/opentelemetry.md)
- [Vector 0.57.0 receiver configuration](https://github.com/vectordotdev/vector/blob/v0.57.0/src/sources/opentelemetry/config.rs)
- [Saved investigation](superpowers/specs/2026-11-01-otel-error-harness-research.md)
