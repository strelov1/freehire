## 1. The draft assembler (pure, no I/O)

- [x] 1.1 Add `internal/followup` with an input struct (role, company, days silent, stage, optional
      strength, optional recipient name) and a function returning subject + body. Table tests cover:
      the same input drafts identically; a missing strength omits the line rather than leaving a gap
      or a placeholder; a missing recipient name falls back to a neutral greeting; the elapsed time
      reads naturally at 21, 24 and 60 days.
- [x] 1.2 Pin the tone rules as tests, since they are the product here: no apology, no "just
      checking in", the ask is a concrete question (is the role still open / what is the timeline),
      and the body stays under a stated length so it is readable on a phone.

## 2. Schema and reads

- [x] 2.1 Add `migrations/0059_user_jobs_followed_up_at.sql`: one nullable timestamptz, no backfill.
      The comment must say why it is NOT part of the last-activity derivation.
- [x] 2.2 Carry `followed_up_at` on the tracking reads (`ListTrackedJobs` and the per-slug detail),
      and add the write that sets it. `make sqlc`.
- [x] 2.3 Integration test the invariant that pays for this change: an application silent for 24 days
      that has been chased still reports 24 days silent and the same silence state. Watch it fail
      first by wiring `followed_up_at` into the `GREATEST(...)` — if the test passes with the column
      in the derivation, it is not testing anything.

## 3. Endpoints

- [x] 3.1 `GET /me/tracking/:slug/followup` assembles the draft: resolve the application owner-scoped
      (a foreign slug is the same not-found as a missing one), refuse anything whose silence state is
      not `silent` via `userjob.SilenceStateFor`, read the cached analysis for the strength, and
      prefill the recipient from the newest linked email's `from_addr` when there is one. Tests:
      draft for a silent application; refusal for active / unconfirmed / terminal; a foreign slug is
      404.
- [x] 3.2 Test both recipient paths against a real DB: an application with linked mail prefills the
      sender; one without returns a draft with no recipient — the commonest case, and the one a
      naive implementation would drop.
- [x] 3.3 `POST /me/tracking/:slug/followup` records the chase, owner-scoped, idempotent enough that
      a double click does not error. Test that it writes the timestamp and that a foreign slug writes
      nothing.
- [x] 3.4 The non-goal is enforced by construction rather than by a test: `inboxHandlers` holds no
      mail client at all (queries, pool, the gmail connector/cipher, origin, cookie flag, mail
      domain, tracking service), so neither endpoint has anything to send with. A test constructing
      a nil mailer would have asserted against a field that does not exist.

## 4. The board

- [x] 4.1 Offer the draft from the silent card in `BoardCard.svelte`, next to the existing badge, and
      show the assembled subject/body with a copy action. The card stopped being one `<button>` — a
      second control cannot nest inside the first — so the open action stretches over the card via an
      `::after` overlay and the follow-up button sits above it on `z-10`. The dialog also offers a
      Gmail and a `mailto:` compose link: both hand the draft to the candidate's own client, which is
      the same handoff the clipboard makes, one click shorter. Copy stays because a `mailto:` on a
      machine with no registered handler fails silently.
- [x] 4.2 A chased card keeps its silence marker and additionally reports when it was chased. Unit
      test the view logic (the repo's convention: logic in `.ts`, thin components) — `$lib/followup.ts`,
      15 tests. This needed the wire shape first: `followed_up_at` reached the sqlc rows but neither
      `jobtracking.TrackedJob` nor the two responses carried it, so the board could not have shown it.
- [x] 4.3 Verify visually at desktop and mobile widths — the board card is dense, and a second line
      of state is exactly where it will overflow. Shot at 1100 and at a real 390 (CDP device
      emulation; `--window-size` clamps the layout viewport at 500). No card or dialog overflows; the
      badge row and the dialog footer wrap rather than clip. Hit-tested the overlay by dispatched
      pointer events: a click on Follow up does NOT open the drawer, and a click anywhere else still
      does — the one failure mode a screenshot cannot show.

## 5. Close out

- [x] 5.1 `go build ./... && go vet ./... && go test ./...`; `go test -tags=integration ./internal/db/
      ./internal/handler/`; in `web/`: `pnpm run lint`, `pnpm run check`, `pnpm test`, `pnpm run build`
      (all four — `svelte-check` catches what the others miss). Run after rebasing onto origin/main,
      which had moved four commits (including design-system changes).
- [x] 5.2 Record in `docs/agents/notifications.md` (or the tracking reference) that `followed_up_at`
      exists, that it is deliberately outside the silence derivation, and what the card shows when
      both are set. Written into `internal/userjob/AGENTS.md`, beside the silence ladder it must not
      join.
