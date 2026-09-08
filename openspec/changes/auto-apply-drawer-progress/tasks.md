## 1. Drawer banner: tailoring / approved

- [x] 1.1 `web/src/lib/autoApplyReview.test.ts`: add failing cases — `autoApplyReviewBanner('tailoring')` returns `{kind: 'tailoring'}`; `autoApplyReviewBanner('approved')` returns `{kind: 'approved'}`.
- [x] 1.2 `web/src/lib/autoApplyReview.ts`: extend `AutoApplyReviewBanner`'s union and `autoApplyReviewBanner()` to cover `tailoring`/`approved`; update the function's doc comment (it currently says both render nothing).
- [x] 1.3 `web/src/lib/components/JobDrawer.svelte`: add the `tailoring` banner branch (status text only, no action) and the `approved` banner branch (status text + the existing "View tailored CV" link to `/tailor/[slug]`, no Approve/Decline buttons) alongside the existing `pending_review`/`blocked`/`declined`/`failed` branches.
- [x] 1.4 Manual check (no Svelte component-test harness exists in `web/` today — this app tests pure logic and verifies UI in the browser, per AGENTS.md): confirmed live against Postgres/Redis + `go run ./cmd/server` + `pnpm dev`, with a seeded user and one `auto_apply_queue` row per status (`tailoring`/`pending_review`/`approved`/`blocked`). Screenshotted via Playwright: the `tailoring` banner shows no action, `approved` shows the same "View tailored CV" link `pending_review` shows (same `/tailor/[slug]` href), the board card's red dot renders next to the "Review" text for `pending_review`/`blocked` only, and the "Needs attention" toggle narrows the board to exactly those two cards (2 of 4) and combines with search. Zero browser console errors. Scratch DB/server/screenshots torn down after.

## 2. Outcome notifications deep-link to the specific application

- [x] 2.1 `internal/engage/nudge/transports_test.go`: add failing assertions that a single-message `KindAutoApplySubmitted`/`Blocked`/`Failed` render (both `TelegramNotifier.render` and `EmailNotifier.render`) contains `/my/tracking/<slug>`, and that the batch (2+ messages) case for the same kinds still contains the bare `/my/tracking` with no slug. Add/confirm assertions that `KindFollowUp`/`KindInterviewPrep` single-message rendering still contains the bare `/my/tracking` (unchanged).
- [x] 2.2 `internal/engage/nudge/transports.go`: in `renderOne`, build `trackingURL` per-kind — `n.origin+"/my/tracking/"+m.Slug` for the three outcome kinds, `n.origin+"/my/tracking"` otherwise. Mirror the same change in `EmailNotifier.render`'s single-message branch.
- [x] 2.3 `web/src/lib/notificationTarget.test.ts`: add failing cases — `nudge_auto_apply_submitted`/`blocked`/`failed` with a `public_slug` now resolve to `{kind: 'tracking', slug: <slug>}` instead of `{kind: 'tracking'}`.
- [x] 2.4 `web/src/lib/notificationTarget.ts`: move the three kinds out of the no-slug `{kind: 'tracking'}` branch into the slug-carrying branch alongside `auto_apply_ready_for_review`; rewrite the file's explanatory comment to describe the split as deliberate (these three now deep-link; `follow_up`/`interview_prep` still don't).

## 3. Board filter: "Needs attention" toggle

- [x] 3.1 `web/src/lib/board.test.ts`: add failing cases for a new `needsAttention(item)` predicate — true for `pending_review`/`blocked` auto-apply statuses, false for every other status and for no attempt at all.
- [x] 3.2 `web/src/lib/board.ts`: add `needsAttention(item: MyJob): boolean`, delegating to `autoApplyNeedsReviewBadge(item.auto_apply_status)` (import from `$lib/autoApplyReview`) rather than re-deriving the pending_review/blocked check.
- [x] 3.3 `web/src/lib/components/JobBoard.svelte`: add a boolean `UrlSyncedState` toggle (own query param, e.g. `?attention=1`) mirroring the existing `search` pattern; AND it into the `shown` derived alongside `matchesQuery` when the toggle is on.
- [x] 3.4 `web/src/lib/components/JobBoard.svelte`: render a "Needs attention" toggle control next to the search input, following the existing search field's styling; reflect its on/off state visually.

## 4. Board card: red dot marker

- [x] 4.1 `web/src/lib/components/BoardCard.svelte`: add a small decorative (`aria-hidden`) colored-dot element alongside the existing "Review" text badge, rendered under the same `{#if needsAutoApplyReview}` condition — no change to the badge's existing text or `aria-label`.

## 5. Verification

- [x] 5.1 `gofmt -l .` (must print nothing for touched files), `go vet ./...`, `go test ./...`.
- [x] 5.2 `pnpm --filter web test` (or the project's equivalent vitest invocation) for the touched `web/src/lib` files and any JobDrawer/JobBoard/BoardCard component tests.
- [x] 5.3 `pnpm --filter web check` (svelte-check) if touched Svelte files are covered by it.
