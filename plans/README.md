# Improve audit

Audited commit: `9a9c4aead`.
Mode: standard, hotspot-weighted static audit with parallel read-only reviews.
Source: `shadcn/improve`, installed at `/Users/evgenia/.codex/skills/improve`.

The user selected all five findings for implementation. Plans are self-contained;
executors use the existing isolated BB task worktree. The reviewer maintains status.

## Execution order and status

| Plan | Title | Priority | Depends on | Status |
|---|---|---|---|---|
| [001](001-guard-conversation-restoration.md) | Guard conversation restoration | P1 | None | DONE |
| [002](002-cancel-extension-server-turn.md) | Cancel server turns from extension | P1 | None | DONE |
| [003](003-guard-initial-profile-write.md) | Guard initial partial profile writes | P1 | None | DONE |
| [004](004-preserve-query-job-identity.md) | Preserve query-based job identity | P2 | 001 | DONE |
| [005](005-document-search-reconciliation.md) | Correct reconciliation documentation | P3 | None | DONE |

001 and 004 share panel lifecycle code and execute sequentially. Other changes
have independent file ownership. No release, merge, or publication is requested.

## Implementation verification (2026-11-12)

- The extension sends an independent authenticated cancel request before aborting
  its reader. Conversation restoration rejects stale account/page lifetimes, and
  restoration requests cannot invalidate an active message dispatch. Greenhouse
  embedded jobs retain their board and posting identity in conversation keys.
- Partial first profile writes use a normalized insert-if-absent operation;
  conflicts reload and merge through the existing bounded retry. Full saves keep
  their intentional replacement semantics. sqlc generated the new query methods.
- Search documentation now describes marker-only time crossings and unchanged
  re-seen enqueue paths accurately.
- Regression tests were observed failing before their fixes. A fresh full
  extension run passed all 381 tests across 32 files with two workers. Typecheck,
  lint, formatting, and the Chrome MV3 production build passed.
- Focused profile/handler tests and the real Postgres profile integration suite
  passed. Both ordinary and integration-tagged `go vet` passed.
- `GOMAXPROCS=2 go test -p 2 -tags=integration ./...` passed for the entire Go
  module: 270 passing packages, including the full API handler, database, worker,
  and search suites.
- `GOMAXPROCS=2 go test -p 2 ./...` passed: 260 passing packages. An isolated
  warm handler run passed in 4.044 seconds.
- A coordinator rerun of `make sqlc` produced byte-identical generated output.
- Independent review found and resolved a startup/dispatch race, then reported
  no unresolved Important or Critical findings. CodeRabbit CLI was unavailable;
  review used a separate code-review agent plus coordinator inspection.
- The real-stack Chromium jobs journey passed: seeded feed, keyword search,
  empty results, and opening the matching job detail. Screenshots and trace are
  in `/tmp/freehire-improve-evidence`; its isolated stack was stopped afterward.
- Early broad Go runs hit one-second request timeouts in unrelated handler tests
  under host load. Bounded final runs resolved them; no timeout or unrelated
  production code was changed to hide them. The full extension suite similarly
  passed with bounded workers after initial worker-start timeouts.
- The host lacked mise and psql. Integration used the repository's development
  environment script and a temporary Docker-backed psql adapter, preserving
  migration command ordering. No host toolchain or repository workaround shipped.

Extension regression coverage exercises the actual asynchronous helper imported
by the panel. It does not claim a mounted extension or live browser account-swap
reproduction. The site journey is an additional application smoke check.

## Prioritized findings

| # | Finding | Category | Impact | Effort | Fix risk | Confidence |
|---|---|---|---|---|---|---|
| 1 | Reject stale conversation restores across account changes | Security, correctness | A delayed response can restore account A's transcript after sign-out and expose it after signing into B in the same panel | S | Low | High |
| 2 | Make extension Stop cancel the server turn | Correctness | Model calls and tools continue after the panel reports cancellation | S | Low | High |
| 3 | Guard initial partial profile creation against concurrent writes | Correctness | A concurrent save can lose preferences the partial update never intended to change | S | Low | High |
| 4 | Preserve query-based job identity in conversation keys | Correctness | Distinct Greenhouse application forms retain the same conversation | S | Low/medium | High |
| 5 | Correct search reconciliation documentation | Documentation | Troubleshooting guidance incorrectly recommends relying on full rebuilds | S | Low | High |

Effort S means hours including focused regression tests. Runtime symptoms were
inferred from verified control flow; no browser or database race reproduction was run.

## Evidence and remediation sketches

### 1. Stale conversation restoration

- `extension/entrypoints/sidepanel/App.svelte:259-274` captures a token, awaits a
  transcript, and checks only whether `sessionId` is set before adopting it.
- The sign-out flow at lines 416-426 clears chat through `newChat`; `resetChat`
  at lines 496-503 sets `sessionId` to null. The sign-in flow at lines 395-408
  does not clear a transcript restored in the meantime.
- A pending old response can therefore refill cleared state. The same omission
  permits an old page's response to replace the conversation after navigation.
- Introduce account/page generation checks around asynchronous restoration,
  including errors and completion state. Add delayed-response tests spanning
  sign-out, subsequent sign-in, and navigation.

### 2. Stop only aborts the reader

- `extension/lib/assistant/client.ts:84` only aborts the local fetch.
  `extension/entrypoints/sidepanel/App.svelte:488-489` calls that handle.
- `internal/api/handler/assistant.go:825` creates a detached turn context, and
  lines 912-920 explicitly continue work after reader disconnects.
- The authenticated cancel route exists at `assistant.go:235`. The web client
  already calls it at `web/src/lib/assistant/client.ts:44-49,296-303`.
- Have explicit Stop send the existing cancel request with the extension's
  Bearer credential and then abort its reader. Cover cancellation before response
  headers as well as during streaming; preserve disconnect behavior deliberately.

### 3. Concurrent first profile writes

- `internal/api/handler/assistant_profile_write_tool.go:143-178` reads preferences,
  merges the partial update, and uses unconditional `Save` when no row existed.
- `internal/platform/db/queries/user_profiles.sql:14-25` implements that save as
  `ON CONFLICT DO UPDATE`, replacing every preference field.
- If another writer creates the row after the read, the partial update replaces
  its fields using an empty snapshot. Existing-row updates already use version
  checks and a bounded retry loop (`assistant_profile_write_tool.go:122-128`).
- Use insert-if-absent for this partial-update creation path, return the existing
  conflict error if another writer wins, then reload and merge on retry. Preserve
  the full-save API's intended replacement semantics. Add deterministic tests for
  simultaneous initial creation and a website save between read and insert.

### 4. Query-based posting identity

- `extension/entrypoints/sidepanel/App.svelte:176-179` drops every query parameter
  from conversation identity; lines 218-227 reset chat only when that key changes.
- Its adjacent lookup helper at lines 190-198 explicitly keeps query parameters
  because Greenhouse embedded forms encode board and job identity in them.
- `internal/ingest/sources/postingurl.go:54-58` reads `for` and `token` to identify
  those postings. Different embedded jobs consequently collapse into one chat key.
- Extract a tested identity helper preserving identity-bearing parameters while
  ignoring tracking-only changes. Test both distinct embedded jobs and tracking
  changes on the same job. Avoid introducing an ungrounded universal ATS parser.

### 5. Search reconciliation guide drift

- `internal/search/AGENTS.md:30-35` says unchanged refreshes never enqueue work and
  unseen postings leave or return only through a full rebuild.
- `cmd/reindex/time_crossings.go:43-47` queues time-based visibility transitions.
- `internal/platform/db/queries/jobs_upsert.sql:312-319` requeues unchanged jobs
  re-seen after the withholding threshold.
- Update the guide to describe these paths. Retain the rationale against relying
  on a stale indexed `last_seen_at` for query-time filtering.

## Recommended selection and dependencies

Recommend plans for 1, 2, and 3 first; 4 is also a small concrete fix. Item 5 is
lower priority. The fixes are independent, although 1 and 4 touch the same panel
and should be implemented sequentially to avoid conflicts. Each implementation
plan must include its own regression-test steps before implementation.

## Original audit verification

- `node scripts/check-shared-deps.mjs`: passed.
- `node scripts/check-doc-links.mjs`: all 362 relative links resolve.
- `PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s .github/scripts -p 'test_*.py'`:
  all 16 tests passed.
- `go test ./internal/platform/arch/layering`: passed.
- `go test ./internal/api/mcpapp ./internal/identity/auth`: passed in the API review.
- `pnpm --dir web audit --prod --audit-level high`: no known vulnerabilities.
- `pnpm --dir design-system audit --prod --audit-level high`: no known vulnerabilities.
- `npm --prefix extension audit --omit=dev --audit-level=high`: zero vulnerabilities.

These are narrow checks, not evidence that the complete product passes its suites.
The reported behavior defects still need deterministic regression reproductions.

## Original audit coverage and limits

Reviewed recent API/MCP/profile and contract hotspots; identity boundaries;
catalogue/source measurements; search visibility/outbox reconciliation; selected
frontend/extension session and transport flows; CI selection and onboarding docs;
shared dependencies, runtime advisory reports, and architecture enforcement.
Read the project's architecture and declined-feature records.

Did not exhaustively audit all adapters, workers, packages, migrations, AI tools,
or frontend routes. Did not start the application, run workers, perform live OAuth
or payment flows, run full unit/integration/E2E suites, or inspect production.
No dependency major-version migration is recommended from manifest age alone.
No new product-direction suggestions were selected from this pass.

## Considered and rejected

- Detached server turns are intentional; the defect is the extension's explicit
  Stop action failing to call the cancellation contract.
- Full search descriptions already have a dedicated endpoint; changing the
  normal search response is explicitly out of scope.
- Catalog/source zero handling: inspected paths distinguish unmeasured values
  and retain previous catalogue totals after a failed refresh; no new runtime
  defect established.
- Architecture and tooling are already enforced; do not propose new general
  infrastructure without a specific demonstrated gap.
