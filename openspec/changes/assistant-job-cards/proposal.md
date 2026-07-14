## Why

When the agent recommends jobs in `/my/assistant`, it emits plain text links to
`freehire.dev/jobs/<slug>`. The user scans a wall of blue links instead of the
rich, scannable job cards the rest of freehire uses. Rendering those references
as real job cards — logo, title, tags, skills, "posted N ago" — makes the chat
an actual job-browsing surface, consistent with the app.

## What Changes

- The assistant chat **unfurls freehire job links into real job cards**. Each
  `https://freehire.dev/jobs/<slug>` (or bare `/jobs/<slug>`) in an assistant
  reply is replaced by the existing **`JobRow`** card, fetched by slug via the
  already-structured jobview endpoint (`api.getJob` → `GET /api/v1/jobs/:slug`),
  with a loading skeleton and a plain-link fallback on error.
- Assistant message rendering changes from a single sanitized `{@html}` blob to
  an ordered **segment loop** (markdown segments keep the sanitized `{@html}`;
  job-link segments render a card). The parser is a pure, unit-tested module.
- Cards **reuse `JobRow` as-is** (interactive: working bookmark; the card links
  to the job **in a new tab** so the chat stays open). Cards are constrained to
  the chat column width with vertical spacing so the output reads as one system.
- Agent side (minimal, `freehire-agent`): the `using-freehire` skill instructs
  the agent to present each recommended job as its `https://freehire.dev/jobs/<public_slug>`
  URL so the chat can unfurl it. Optional `freehire-cli` polish: emit the
  canonical `url` in `search` output so the agent copies it verbatim.

## Capabilities

### New Capabilities
- `assistant-job-cards`: rendering freehire job references in the agent chat as
  rich, data-backed job cards (link detection + segment rendering + card
  hydration by slug), plus the agent-side convention that produces the links.

### Modified Capabilities
<!-- None: assistant-sessions covers session management; this is a net-new render concern. -->

## Impact

- **Frontend (`hire/web`)**: `web/src/lib/assistant/unfurl.ts` (new, pure parser
  + vitest), `web/src/lib/assistant/JobCardUnfurl.svelte` (new: fetch-by-slug →
  `JobRow`, skeleton, link fallback, module cache), and the message renderer in
  `web/src/routes/my/assistant/+page.svelte` (segment loop). Reuses `JobRow.svelte`,
  `api.getJob`, and the `Job` (jobview) type — the API already returns the
  structured shape the card needs; no backend change.
- **Agent (`freehire-agent`, separate repo)**: `docker/skills/using-freehire/SKILL.md`
  — a linking convention (markdown-only change; redeploy skills).
- **CLI (`freehire-cli`, optional, separate repo)**: canonical `url` in `search`
  output.
- No DB migration; no new env. Local run at the end for live testing.
