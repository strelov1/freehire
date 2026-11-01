## Implementation

- [x] Emit Go exception logs for API, streams and worker failures with bounded lifecycle handling.
- [x] Emit browser, SSR and extension exception logs with filtering and bounded delivery.
- [x] Configure the existing Vector OTLP receiver, nginx ingress and Grafana triggers in freehire-ops.
- [x] Document official CLI/API and optional MCP access, smoke checks, configuration and rollback.
- [x] Run targeted protocol tests and repository checks; review the combined changes.

## Activation

- [ ] Install operations configuration and deploy instrumented builds; verify real end-to-end delivery.

## Recovery notes (2026-11-01)

Continued the interrupted change in the existing checkout and operations repository.
The original BB thread was not present in the available or archived thread list;
its local Codex provider history was subsequently located and the original scope
(existing infrastructure, CLI investigation and triggers) was recovered.
Configuration, CLI access and rollout checks are documented in
[the operator guide](../../../docs/error-telemetry.md).

The final stable full Go suite (`go test -p 1 ./...`), targeted JS tests, the full
extension suite, both Go vet modes, JavaScript type checks, dependency alignment,
dead-code checks and documentation links passed. Local Vector/VictoriaLogs fixtures
preserve service/version/stack and distinguish trusted events from browser events.
Independent review findings were fixed, including credential redaction in Go
exception records. Receiver/delivery alerts, host identity and release build
settings were validated locally in the operations repository.
Production activation remains unchecked: no live deployment or Telegram delivery
has been claimed. Full web testing/build verification needs sufficient free disk;
the broad test attempt hit ENOSPC before some suites could collect.
