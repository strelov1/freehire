## Context

`/tailor/[slug]` bootstraps a NEW tailored CV + agent session on every open, and CV editing
lives in a separate `/my/cvs` builder. The user wants one workspace and a `/my/cvs` that
re-opens the *existing* session for a CV. That requires a durable CV→session link, a resume
mode, and the editor moved into the workspace. Backend persistence was chosen over localStorage
(robust, cross-device).

## Goals / Non-Goals

**Goals:** persist `cvs.agent_session_id`; `/tailor` resume mode (`?cv=<id>`); CV editor as a
workspace tab; `/my/cvs` re-open list with no create.

**Non-Goals:** re-minting the CLI token on resume (noted seam); changing the roy agent; multi-CV
composition.

## Decisions

**D1 — Store the session on the CV (`cvs.agent_session_id text`).** Migration `0028`. Set via a
small owner-scoped endpoint after the frontend creates the session; returned by the CV reads.
Chosen over localStorage for cross-device durability.

**D2 — Set-session endpoint, not fold into PATCH.** `PUT /me/cvs/:id/session {session_id}`
(cookie or key, owner-scoped). Keeps the document PATCH surface clean; the session id is
metadata, not document content.

**D3 — `/tailor/[slug]` has two modes.**
- No `?cv`: bootstrap (create tailored CV + session + kickoff), then `PUT` the session id.
- `?cv=<id>`: GET the CV (`agent_session_id`, `job_id`) + the job + the analysis; attach the
  existing session with NO kickoff. Resume needs no new token — the session already holds the
  one injected at creation (short-lived; see risk).

**D4 — CV editor as a tab.** `ArtifactPanel` gains an Edit tab that reuses the existing
`CvEditor` (loads the CV doc, saves via the existing update endpoint). The standalone
`/my/cvs/[id]` editor route redirects into the workspace so there is one editing surface.

**D5 — `/my/cvs` re-open list.** `ListCVsByUser` joins `jobs` for the public slug and returns
`agent_session_id`; `CvList` drops the create button and links each row to
`/tailor/<slug>?cv=<id>`. Only tailored CVs (`job_id NOT NULL`) are listed.

## Risks / Trade-offs

- **Token TTL vs resume** → the CLI token minted at bootstrap is 2h; resuming later means the
  agent's `freehire cv edit` 401s. Mitigation (follow-up): re-mint + re-inject on resume, or
  lengthen the tailoring key TTL. For now resume works within the session's life.
- **roy session liveness on resume** → re-attach replays the journal; if the harness process was
  reaped the agent restarts fresh in the same cwd (its `freehire` auth env may be gone). Same
  token seam; acceptable for MVP.
- **Editing in a tab while the agent also edits** → both go through the same CV; last write wins.
  The preview refreshes on turns; the editor re-loads on tab open. Acceptable.

## Migration Plan

Additive column (`0028`), nullable — no backfill. Deploy backend (migration applied first per
prod ops), then the web. Rollback: the column is unused if the web is reverted.

## Open Questions

- Whether `/my/cvs` should also surface base CVs (job_id NULL) — leaning no (the list is the
  tailoring re-open surface); a base CV is an implementation detail seeded on first tailor.
