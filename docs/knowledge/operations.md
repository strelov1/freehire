---
type: Reference
title: Operations ownership
description: Find authoritative deployment configuration and distinguish it from application code.
generated:
  by: codex/gpt-6.1-sol
  at: 2026-11-14T22:00:00-03:00
sources:
  - resource: https://github.com/strelov1/freehire/blob/c0fc3518d5f25c66f39b069c943fe7a1fab840b9/AGENTS.md
---

Application code is in this repository. Host configuration belongs to the private
`freehire-ops` repository: systemd units, nginx, release scripts and networking.
An application release does not install changed host configuration automatically.

## Before operating a host

Read `freehire-ops/provision/NETWORK.md` in an up-to-date checkout and the affected
host's instructions. Confirm the deployed configuration before choosing a command.
Historical paths such as `fsn`, `host2` and `litellm-host` can refer to renamed hosts.

This public bundle intentionally contains no copied IP inventory, credentials or
private configuration. If the private repository is unavailable or its network
record is absent, report that gap before deriving a deployment command from old
documentation. A successful SSH connection alone does not verify topology.

## Application evidence

- [Worker reference](../agents/workers.md): command requirements and traps.
- [CI checks](../agents/ci-checks.md): validation requirements for a change.
- [Architecture](architecture.md): which application block owns the behavior.

A host change must also update its record in `freehire-ops`.
