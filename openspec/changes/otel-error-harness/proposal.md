# OpenTelemetry error harness

## Why

Operators need to find application faults from a terminal or an agent and trigger
notifications without depending on a hosted error tracker or its UI. The operations
repository already provisions Vector, VictoriaLogs and Grafana with Telegram alerts.

## What changes

- Emit exception logs using OpenTelemetry and OTLP from Go, SvelteKit and the extension.
- Keep unexpected HTTP faults, panics, SSE failures and failed worker exits observable.
- Reuse Vector's native OTLP receiver, VictoriaLogs storage/query API, official
  vlogscli and the existing alerting infrastructure.
- Provide deployment configuration, query recipes and an end-to-end smoke check.

## Scope

This is error collection and investigation, not a new observability product. Existing
metrics and system-log delivery remain. Optional Sentry fallback remains during rollout.
Production activation requires installing the operations configuration and deploying
the instrumented applications; source changes alone are not evidence of live delivery.
