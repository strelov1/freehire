## Context

For the supported multi-tenant ATS (greenhouse, lever, ashby, workable), the company board is
identifiable from the URL alone: the host names the platform and the first path segment is the
board slug — the same for a vacancy URL (`jobs.ashbyhq.com/<board>/<id>`) and a bare
board-listing URL (`jobs.ashbyhq.com/<board>`). That lets a paste-a-link contribution flow run
entirely without a network fetch on the request path.

A separate manual `internal/submission` flow already exists (full-card entry → moderation queue
→ mint). It is left untouched: contributions are lighter (URL only), auto-validated, and
rewarded, not moderated.

Constraints: Go + Fiber v2, sqlc, migrations via Postgres initdb (next number `0025`). Auth is
stateless JWT cookie or API key (`RequireAuthOrKey`).

## Goals / Non-Goals

**Goals:**
- Accept a job link, recognize the company board `(source, board)` with zero network calls, and
  reject a board we already crawl or already recorded.
- The unit of a contribution is the board, not the vacancy: a second link to the same company
  (another vacancy or the board listing) earns nothing.
- Record novel boards and maintain a per-user points balance, atomically.
- Offer the same flow from two front doors: the website and a linked Telegram chat.

**Non-Goals:**
- The background worker that validates a board, onboards it into `sources`, and scrapes its
  vacancies (a `status` seam is provided; the worker is a follow-up).
- Point claw-back for a board that later fails to onboard.
- Moderation queue, public leaderboard, coverage beyond the 4 path-based ATS.

## Decisions

### D1. Board recognition is a small local table, not a linksource seam

`internal/contribution/board.go` holds `recognizeBoard(url) → (source, board, canonical, ok)`:
an `atsBoards` table maps a host (exact or subdomain suffix) to its source, and the board is the
first path segment. For the 4 supported ATS the rule is uniform, so a table row per ATS is the
whole cost of adding one.

- *Alternative — a `BoardKey` method on every `linksource.Source` adapter (interface change):*
  tried and reverted. It threaded a per-vacancy concept through 9 adapters when the rule is
  identical for all of them; a table is simpler to read and to extend. `linksource` stays
  untouched.
- Subdomain-based ATS (recruitee, teamtailor, bamboohr, …) extract the board differently and are
  added when coverage expands — they will need their own rule, not the first-path-segment one.

### D2. New `link_contributions` table + `users.points`, one transaction

```
link_contributions(
  id, submitted_by → users, url, source, board,
  status default 'pending' check in ('pending','onboarded','rejected'),
  created_at, unique (source, board)
)
ALTER TABLE users ADD COLUMN points integer NOT NULL DEFAULT 0;
```

The insert and `UPDATE users SET points = points + 1` run in one pgx transaction. The
`unique(source, board)` constraint enforces "board already contributed" and makes the concurrent
race safe: the loser hits the violation, the repository maps it to `ErrBoardAlreadyContributed`,
and the whole tx rolls back (no point).

- *"already in catalogue"* is a board-tracked check, not a per-vacancy one:
  `starts_with(jobs.external_id, '<board>:')` — any job under the board namespace.

### D3. Ordering of checks (fail cheap first)

1. Auth (`RequireAuthOrKey`).
2. `recognizeBoard` → else 422 "unsupported ATS".
3. board-tracked lookup → 409 "board already in catalogue".
4. Insert board + increment points in one tx; unique violation → 409 "board already contributed".

### D4. Two front doors, one Service.Submit

The website (`POST/GET /api/v1/me/contributions`) and Telegram both call the same
`contribution.Service.Submit`. The Telegram path adds a branch to the existing webhook
(`handleTelegramContribution`): extract the first URL from the message, resolve the chat to its
user (`GetUserIDByTelegramChat` — a reverse lookup on `telegram_links`), submit, and reply with
the outcome. A no-link message is ignored; a link from an unlinked chat prompts the user to link.

- The points balance rides existing `/api/v1/auth/me` (add `points`). Frontend: a SvelteKit page
  with the paste-a-link form, the balance, and the discovered-boards list.

## Risks / Trade-offs

- **Point farming before validation** → A user can paste a well-formed link for a fake board; we
  do not fetch at submit. Mitigation: board uniqueness caps repeats to one point per board; the
  deferred onboarding worker will mark an unreachable board and is the natural place to claw the
  point back. Accepted for MVP.
- **Coverage is only 4 ATS** → Subdomain-based and long-tail ATS are silently unsupported (422).
  Mitigation: the table makes each addition one row + test; expansion is an explicit follow-up.
- **Board slug edge cases** → A non-board first segment (e.g. a greenhouse `/embed/...` vanity
  path) could mis-read the board. Accepted: rare in user pastes; the onboarding worker validates
  before anything is crawled.

## Migration Plan

1. Add `migrations/0025_link_contributions.sql` (table + `users.points`); apply to prod manually
   before deploy (initdb is single-run; matches house rule).
2. Regenerate sqlc (`make sqlc`).
3. Ship backend (additive routes + webhook branch) then frontend.

## Open Questions

- Should the onboarding worker claw back a point when a board is unreachable, or leave awarded
  points immutable? (Deferred with the worker; the `status` column keeps both open.)
- Points-to-reward mapping (what a point buys) is out of scope here.
