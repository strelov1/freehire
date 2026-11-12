# Plan 005: Explain incremental search visibility reconciliation

## Status
- Priority: P3
- Effort: S
- Risk: LOW
- Depends on: none
- Category: docs
- Planned at: commit `9a9c4aead`, 2026-11-11

## Why this matters
The search guide says postings leave or return only on full rebuild, sending operators toward unnecessary rebuilds.

## Current state
`internal/search/AGENTS.md:30-35` says liveness refresh pushes nothing and unchanged re-seen jobs stay hidden until rebuild. In contrast `cmd/reindex/time_crossings.go:43-47` queues time-driven transitions and `internal/platform/db/queries/jobs_upsert.sql:312-319` queues unchanged re-seen jobs crossing the withholding threshold.
Retain the explanation that indexed last_seen_at would freeze and must not be used as a query-time liveness filter. Read call sites and existing job-lifecycle docs to state the exact timer-dependent conditions.

## Scope
Only `internal/search/AGENTS.md`. No source edits, deployment/timer changes, new tests, or unrelated architecture-doc rewrite. Reviewer owns index.

## Drift and workflow
Run `git diff 9a9c4aead -- internal/search/AGENTS.md`. Work in existing isolated BB worktree. No commits/push/merge.

## Steps
1. Read Withheld, time_crossings caller/flags, unchanged-upsert requeued CTE, and job-lifecycle docs.
   Verify: `rg -n 'enqueueTimeCrossings|time-crossings|requeued|withheld_if_seen_before' cmd/reindex internal/platform/db/queries/jobs_upsert.sql` identifies active mechanisms.
2. Replace only stale paragraph with correct explanation of time-crossing enqueue, re-seen enqueue, and drain. Distinguish incremental mode from full rebuild and avoid claiming reconciliation happens without scheduled workers.
   Verify: `git diff -- internal/search/AGENTS.md` shows only the paragraph correction.
3. Run `node scripts/check-doc-links.mjs` and `node scripts/check-file-size.mjs --all`: exit 0. Run local rumdl/agnix if installed, report exact availability; don't install tooling solely for this text edit.

## Done criteria
Documentation matches actual enqueue paths, frozen indexed timestamp rationale remains, links and size checks pass. No regression tests for a documentation-only correction.

## STOP conditions
Report if code no longer implements the stated paths or the deployment schedule cannot be inferred; describe code behavior only, don't guess host configuration.

## Maintenance
Keep the guide aligned when visibility predicates or enqueue triggers change.
