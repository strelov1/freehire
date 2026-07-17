## Why

Tailoring now opens a fresh agent session every time and edits the CV in a separate `/my/cvs`
builder. The user wants one coherent workspace: `/tailor/[slug]` is where you chat, view, AND
edit a tailored CV; `/my/cvs` becomes a list that RE-OPENS an existing tailoring session (not a
new one); and CVs are created only from the match page. That needs a durable link between a
tailored CV and its agent session, a resume mode, and the CV editor moved into the workspace.

## What Changes

- Persist the agent session on a tailored CV: `cvs.agent_session_id` (**migration**), set after
  the session starts, returned by the CV reads. So a CV can re-open its exact session.
- **Resume mode** for `/tailor/[slug]?cv=<id>`: reuse the existing tailored CV + its stored
  session (re-attach, **no re-bootstrap, no kickoff**). Absent `?cv` still bootstraps a new one
  (from the match CTA), then stores the session id on the new CV.
- **CV editor as a tab** on `/tailor`: move the structured `CvEditor` form into a workspace tab
  (CV · Edit · Job description · Verdict), so editing happens beside the chat and preview.
- **`/my/cvs` becomes a re-open list**: each tailored CV links to `/tailor/[slug]?cv=<id>`
  (resume). **Remove the "Create" button** — tailored CVs are created only via the match page.
- List CVs returns the job's public slug + the session id so the list can build resume links.

## Capabilities

### New Capabilities
- `tailor-workspace`: the `cvs.agent_session_id` link + its set/read endpoints, the `/tailor`
  resume mode, the in-workspace CV editor tab, and the `/my/cvs` re-open list (no create).

### Modified Capabilities
<!-- The /my/cvs rework (list re-opens sessions, no create) is part of the tailor-workspace
     capability above, not a separate spec-level change to cv-builder's own requirements. -->

## Impact

- **Backend:** migration `0028_cvs_agent_session.sql`; `internal/db/queries/cvs.sql` (return +
  set `agent_session_id`, join job slug in the list); `internal/cv` (Meta/Record fields, store
  method); `internal/handler/cv*.go` (a set-session endpoint; list returns the new fields).
- **Frontend:** `/tailor/[slug]` resume mode + `ArtifactPanel` Edit tab (reuses `CvEditor`);
  `CvList` rework; `/my/cvs/[id]` editor route redirects into the workspace.
- **Tests:** Go unit (store) + integration (session set/read, list-with-slug); web vitest for
  any pure logic; svelte-check + visual for components.
- **Risk/seam:** the minted CLI token is short-lived (2h); resuming after it expires means the
  agent's `freehire cv edit` would 401 — noted; a re-mint on resume is a follow-up.
