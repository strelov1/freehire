## Context

CV tailoring works end-to-end (bootstrap → seeded roy session → agent edits via `freehire cv`
→ live PDF preview), but its UI grew incrementally inside `/my/assistant`: cramped under the /my
layout, boxed in rounded cards, CV preview bolted on. The user wants a dedicated full-width
surface with a tabbed artifact panel (CV / Job description / Verdict), styled like roy-web.

Current state:
- `/my/assistant/+page.svelte` (~940 lines) holds ALL chat logic inline: `RoyClient` transport,
  session lifecycle (`openSession`/`createAndOpen`/`selectSession`), message list rendering,
  composer + queue, labels, and (recently) the CV preview panel + collapsible rail.
- Endpoints all exist: `api.tailorCv` (bootstrap), `createSession(tailoring)`, `api.cvPdfUrl`,
  `GET /jobs/:slug` (JD), the cached analysis (verdict). No backend work needed.

## Goals / Non-Goals

**Goals:**
- Dedicated `/tailor/[slug]` route with its own full-width layout.
- One reusable `<AssistantChat>` component, shared by `/tailor` and `/my/assistant`.
- Tabbed, resizable artifact panel: CV | Job description | Verdict.
- Bootstrap + auto-start on load.

**Non-Goals:**
- Any backend / API / DB change.
- Reworking the roy agent or the tailoring persona (already deployed).
- A general artifacts framework — just these three fixed tabs.

## Decisions

**D1 — Top-level `/tailor/[slug]` with its own layout.** Alternative: `/jobs/[slug]/tailor`.
Chosen top-level so the surface owns its full-width layout without fighting the /jobs public
chrome or needing a layout-reset hack; the slug is self-sufficient (the page bootstraps + fetches
JD + verdict from it).

**D2 — Extract `<AssistantChat>`, don't duplicate.** The chat is lifted out of
`/my/assistant/+page.svelte` into `web/src/lib/assistant/AssistantChat.svelte` with props for the
seed (optional `session` id, `kickoff` prompt, and turn-completion callback so the host can
refresh the CV). `/my/assistant` becomes a thin host that renders `<AssistantChat>` + its session
sidebar; `/tailor` renders `<AssistantChat>` + the artifact panel. One chat implementation. This
is the CLAUDE.md "re-architect freely" path; the risk (touching a 940-line monolith) is bounded by
keeping `/my/assistant` behaviourally identical and svelte-check + visual verifying both.

**D3 — Bootstrap + session live in the route, not the chat.** `<AssistantChat>` stays generic
(it only knows sessions). `/tailor/[slug]/+page.ts` (or the page) does the tailoring bootstrap
(`api.tailorCv`) + `createSession(tailoring)` and passes the resulting session id + kickoff into
`<AssistantChat>`, and the ids (cv/job/analysis) into the artifact panel. Keeps the chat reusable.

**D4 — Artifact panel = fixed 3-tab component with a pointer-drag splitter.** CV = `<iframe>` on
`cvPdfUrl(id)?v=N` (N bumped on turn-complete). JD = the job's description text. Verdict = the
analysis projected (score, recommendation, missing_have/gap lists). Width is a clamped px state
driven by a pointer-capture splitter (min ~360, max ~900).

**D5 — roy-web aesthetic.** No `rounded-xl border bg-card` column boxes; columns separated by
`border-l/border-r` dividers on one background; chat content centered in a max-w column. Applied
in `<AssistantChat>` (so `/my/assistant` gets the cleaner look too).

## Risks / Trade-offs

- **Extracting a 940-line monolith** → regressions in `/my/assistant`. Mitigation: move the code
  verbatim into the component behind the same reactive state; keep the host thin; svelte-check +
  drive both surfaces (send/queue/switch/delete) before shipping.
- **Bootstrap on route load can 409** (no analysis / no résumé) → show the actionable message the
  API returns, and a link back to the fit page. Never a blank surface.
- **Two hosts of one component** could drift → the component owns behaviour; hosts only compose
  layout, so there is one place for chat logic.

## Migration Plan

Additive route + a refactor of one existing page. No data/schema migration. Ship behind the beta
gate; the fit-page CTA repoint is the only user-visible entry change. Rollback = revert the web
change (no state to clean up).

## Open Questions

- Default artifact tab on open — CV (see your CV forming) vs Verdict (see the gaps first). Lean
  CV; cheap to change.
- Whether to keep the collapsible session rail on `/tailor` (probably no rail there — it's a
  single focused session; the sidebar stays on `/my/assistant`).
