## Why

CV tailoring currently runs inside the generic `/my/assistant` chat: cramped under the /my
chrome, boxed in rounded cards, with the CV preview bolted on and no room for the job
description or the fit verdict. It should be a first-class, full-width surface where the
candidate sees the vacancy, the verdict, and their CV update live as the agent reframes it.

## What Changes

- Add a dedicated top-level route **`/tailor/[slug]`** with its **own full-width layout** (no
  /my account rail, no /jobs chrome), styled like roy-web: no rounded-card boxes, thin
  dividers, a centered chat column.
- **Extract the chat** from the 936-line `/my/assistant/+page.svelte` into a reusable
  `<AssistantChat>` component (transport, session lifecycle, message list, composer). Use it on
  **both** `/tailor/[slug]` and `/my/assistant` — one implementation, no duplicated chat logic.
- **Tabbed artifact panel** on the right of `/tailor`: **CV** (live ATS PDF, refreshed each
  turn) · **Job description** (the vacancy text) · **Verdict** (the fit analysis — overall
  score, `missing_have`/`missing_gap`, recommendation). A **draggable splitter** resizes it.
- On load `/tailor/[slug]` **bootstraps** the tailored CV (`POST /me/cvs/tailor`), starts a
  **seeded agent session** (`createSession` with the tailoring context), and **auto-starts** the
  agent — no empty chat, no manual first message.
- Re-point the existing **"Tailor my CV" CTA** on `/jobs/[slug]/fit` to `/tailor/[slug]`.
- **Beta-gated.** **No backend changes** — every endpoint already exists (bootstrap, patch,
  tailor-context, `cvPdfUrl`, `GET /jobs/:slug` for the JD, the cached analysis for the verdict).

## Capabilities

### New Capabilities
- `tailor-surface`: the dedicated `/tailor/[slug]` route and its full-width own layout, the
  reusable `<AssistantChat>` component (shared with `/my/assistant`), the tabbed + resizable
  artifact panel (CV / Job description / Verdict), and the bootstrap-and-autostart entry flow.

### Modified Capabilities
<!-- No requirement-level changes to existing capabilities. The /my/assistant page is
     refactored to consume <AssistantChat> (implementation, not a spec-level behavior change),
     and job-fit-analysis / cv-tailoring endpoints are only consumed, not changed. -->

## Impact

- **Frontend (web/):** new `web/src/routes/tailor/[slug]/+page.svelte` + `+layout.svelte` +
  `+page.ts`; new `web/src/lib/assistant/AssistantChat.svelte` (extracted); refactor
  `web/src/routes/my/assistant/+page.svelte` to use it; new tab/panel + splitter components;
  re-point the fit-page CTA. Reuses `$lib/assistant/*`, `$lib/api` (`tailorCv`, `cvPdfUrl`),
  `$lib/assistant/api` (`createSession`).
- **No Go / API / DB changes.** **No migration.**
- **Tests:** pure logic (splitter clamp, tab state, verdict projection) → vitest; components →
  `svelte-check` + visual verify (per web conventions).
- **Risk:** extracting the chat from a large monolith — mitigated by keeping `/my/assistant`
  behaviorally identical (same component, same props) and svelte-check + visual verifying both.
