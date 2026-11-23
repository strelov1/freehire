---
type: Reference
title: Application architecture
description: Find the module that owns a Freehire change.
generated:
  by: codex
  at: 2026-11-23T11:11:34+00:00
sources:
  - resource: https://github.com/strelov1/freehire/blob/c0fc3518d5f25c66f39b069c943fe7a1fab840b9/AGENTS.md
  - resource: https://github.com/strelov1/freehire/blob/c0fc3518d5f25c66f39b069c943fe7a1fab840b9/internal/platform/AGENTS.md
  - resource: ../../internal/ai/assistant/AGENT-REFERENCE.md
  - resource: ../../internal/candidate/cv/AGENT-REFERENCE.md
  - resource: ../../internal/api/handler/feed.go
  - resource: ../../internal/platform/db/queries/feed_state.sql
  - resource: ../../internal/platform/db/queries/employer_analytics.sql
  - resource: ../../internal/application/viewlog/AGENTS.md
  - resource: ../../internal/platform/db/queries/user_profiles.sql
  - resource: ../../internal/identity/userprofile/userprofile.go
  - resource: ../../internal/api/handler/me_profile.go
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

The Matches API resolves the caller's active source before querying the job index.
PostgreSQL stores the active choice separately from the per-source New watermark.
The API resolves saved searches by owner and leaves a deleted selection in place so
Matches can offer a return to profile matching.

The profile's companies to avoid have one writer: the owner-scoped
`POST /me/profile/excluded-companies` and `DELETE /me/profile/excluded-companies/:company_slug`
(`userprofile.Service.ExcludeCompany`/`UnexcludeCompany`). No profile save assigns the column,
so `PUT /me/profile` reports a legacy `excluded_companies` field as ignored and a stale
autosave cannot erase a hidden company. The append runs as one guarded upsert that holds the
row lock, checks the 200-entry bound and creates a minimal profile when none exists; a real
change advances `updated_at`, the filter version Matches cursors bind. The feed adds the full
list as a `company_slug NOT IN` group for both sources and both phases, outside the character
budget of the shared profile query.

Assistant merge consent is checked by the owner-scoped handler against the transcript;
requirement report normalization belongs to the CV store. Follow the current
[assistant contract](../../internal/ai/assistant/AGENT-REFERENCE.md) and
[CV report contract](../../internal/candidate/cv/AGENT-REFERENCE.md). These relative sources
describe the working checkout; they do not assert production deployment.

## Employer vacancy analytics

The private vacancy report scopes reads to the active company team. It combines
the [page-view rollup](../../internal/application/viewlog/AGENTS.md), Apply button
clicks from the public vacancy page, and delivered Freehire applications over
the same 30 completed UTC days. The [SQL queries](../../internal/platform/db/queries/employer_analytics.sql)
read daily unique page views and store only aggregate click counts.
