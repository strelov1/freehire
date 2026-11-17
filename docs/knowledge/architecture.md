---
type: Reference
title: Application architecture
description: Find the module that owns a Freehire change.
generated:
  by: codex/gpt-6.1-sol
  at: 2026-11-16T17:00:33-03:00
sources:
  - resource: https://github.com/strelov1/freehire/blob/c0fc3518d5f25c66f39b069c943fe7a1fab840b9/AGENTS.md
  - resource: https://github.com/strelov1/freehire/blob/c0fc3518d5f25c66f39b069c943fe7a1fab840b9/internal/platform/AGENTS.md
  - resource: ../../internal/ai/assistant/AGENT-REFERENCE.md
  - resource: ../../internal/candidate/cv/AGENT-REFERENCE.md
---

Freehire aggregates IT jobs through a Go API and workers, with PostgreSQL,
Meilisearch and a SvelteKit web client. Start with the repository's
[architecture map](../architecture.md); use the current [module rules](../../AGENTS.md)
when that narrative and newer package instructions differ.

## Ownership

- HTTP routes and middleware: `internal/api`.
- Crawl adapters and intake: `internal/ingest`; follow the [catalogue flow](catalogue.md).
- Search index, drain and intent: `internal/search`.
- Posting identity, deduplication and wire shape: `internal/job`.
- CV and candidate evidence: `internal/candidate`.
- Enrichment, assistant and plan gates: `internal/ai`.
- Accounts and authentication: `internal/identity`.
- Tracking and mail: `internal/application`; notifications: `internal/engage`.
- Facet vocabulary: `internal/dict`; shared infrastructure: `internal/platform`.

These blocks form eight layers. Same-layer blocks cannot import each other;
read the target block's `AGENTS.md` before changing dependencies. The layer
mapping is enforced by depguard and `internal/platform/arch/layering`.

For deployment ownership, follow [operations](operations.md).

Assistant merge consent is checked by the owner-scoped handler against the transcript;
requirement report normalization belongs to the CV store. Follow the current
[assistant contract](../../internal/ai/assistant/AGENT-REFERENCE.md) and
[CV report contract](../../internal/candidate/cv/AGENT-REFERENCE.md). These relative sources
describe the working checkout; they do not assert production deployment.
