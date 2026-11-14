# Activity Auto-apply Implementation Plan

> **For agentic workers:** Use superpowers:executing-plans to implement this plan task by task.

**Goal:** Make the candidate’s automatic applications discoverable in Activity.

**Architecture:** Expose the existing assistant list over an authenticated HTTP route. Use existing posting slugs as Tracking row identifiers; preserve shared status derivation and the 30-day submitted window. Render a new Activity tab with the existing tokens and components.

**Tech Stack:** Go/Fiber, PostgreSQL/sqlc, SvelteKit/Svelte 5, Vitest.

**Spec:** [Approved design](../specs/2026-11-14-activity-auto-apply-design.md)

## Global Constraints

- Scope every read to the authenticated candidate; no raw internal errors.
- Include all live attempts and submissions from the last 30 days.
- Reuse the existing detail surface and action guards; no bulk submissions.
- Follow existing English/Russian localization and responsive Activity navigation.

## Review Focus

- Empty lists serialize as arrays, not null.
- A second user cannot see another candidate’s attempts.
- A missing tracking record links to the vacancy instead of a broken drawer.
- Failed loading can be retried; unmounted views ignore late responses.
- Every state has a readable label; mobile rows wrap without hiding actions.

### Task 1: Authenticated listing

**Files:** `internal/api/handler/auto_apply_list.go`, `assistant.go`, `assistant_auto_apply_tools.go`, `auto_apply_list_integration_test.go`.

**Interfaces:** `GET /api/v1/me/auto-apply` returns `{data: {attempts: [{job_slug, title, company, status, last_activity_at}]}}`.

- [x] Add integration coverage for authentication, caller isolation, empty lists and submitted history.
- [x] Run the test to establish failure before implementing the HTTP handler.
- [x] Expose shared listing, use existing posting slugs for Tracking links without changing SQL.
- [x] Run focused integration tests and compile tagged tests.

### Task 2: Activity view

**Files:** `web/src/routes/my/activity/+layout.svelte`, `auto-apply/+page.svelte`, `web/src/lib/components/AutoApplyList.svelte`, its component spec, `activity.messages.ts`, `api.ts`, `types.ts`.

**Interfaces:** `api.listAutoApply()` consumes Task 1; rows link to `/my/tracking/[id]`, whose existing row identifier is the posting slug.

- [x] Add component tests for empty, error/retry, status rendering and tracking links; watch them fail.
- [x] Implement the typed API client, localized statuses/descriptions, responsive list and route tab.
- [x] Run focused tests, Svelte checking and lint on changed files.
- [x] Inspect desktop/mobile rendering and run a fresh review of the completed diff.

### Task 3: Verification

- [x] Run `go vet ./...`, `go test ./...` and `go vet -tags=integration ./...`.
- [x] Check the diff and report the implemented behavior and any verification limits.
