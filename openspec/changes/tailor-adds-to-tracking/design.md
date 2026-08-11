## Context

See proposal.md — Why. Two gaps blocked the intended UX:

1. **Kanban vs bookmark.** `JobBoard` / `columnOf` only place rows with a stage (or
   `applied_at`) into columns. A bare `saved_at` lives under Activity → Saved and
   never appears on the board — so save-only was the wrong signal.
2. **Resume path.** After the first bootstrap the SPA rewrites to
   `/tailor/[slug]?cv=…`. That resume branch never called `POST /me/cvs/tailor`,
   so opening an existing tailored CV skipped the tracking write entirely.

`TrackJob` already supports stage-without-`applied_at` ("tracked, not recorded as
applied"), which is exactly the prepare-to-apply signal.

## Goals / Non-Goals

**Goals:**

- Opening or starting tailor puts the vacancy in the Applied Kanban column.
- Do not start silence / claim a submitted application (`applied_at` stays null).
- Do not overwrite an existing advanced stage.
- Heal on resume (bootstrap and `?cv=` reopen).

**Non-Goals:**

- No new stage vocabulary (`preparing` / `tailoring`).
- No one-shot SQL backfill (reopen heals).
- No SPA board redesign.

## Decisions

### 1. Stage `applied` without `applied_at`, plus save

**Choice:** `EnsureOnBoard` = `SaveJob` then, if stage is empty,
`TrackJob(stage=applied, source=user)`.

**Why:** Matches Kanban membership (`columnOf`) without starting silence (listing
derives silence only when `applied_at` is set). Save keeps Activity → Saved
consistent if the user later clears the stage.

**Alternatives:** Save-only (invisible on Kanban — what shipped first and failed
the user's check). New `preparing` stage (vocabulary + contracts + UI expansion).
`MarkApplied` (wrong: bumps `applied_count`, sets `applied_at`, starts silence).

### 2. Narrow `jobBoarder` on `cvHandlers`

**Choice:** `EnsureOnBoard(ctx, userID, jobID)` injected from `Register` via
`trackingBoarder` over the jobtracking repository.

### 3. Call sites

- `TailorCV` after CV + session ready (create and idempotent resume).
- `StartTailorSession` (resume minting a missing session).
- SPA `?cv=` resume with an existing session: fire-and-forget `api.tailorCv(slug)`
  so the board side-effect runs without blocking first paint.

## Risks / Trade-offs

- **[Applied column for "not yet submitted"]** → Acceptable: stage means "in the
  pipeline"; `applied_at` is the submit stamp. User can drag / clear stage.
- **[Server must be restarted]** → Local/prod binary must pick up the handler
  change; SPA change needs a refresh.

## Migration Plan

- Deploy backend + web. Reopen any tailored workspace once to heal.
- Rollback: remove EnsureOnBoard calls; leftover stage=`applied` rows remain
  ordinary tracking (harmless).
