## Context

The assistant chat (`web/src/routes/my/assistant/+page.svelte`) renders each
assistant reply as ONE sanitized blob: `{@html DOMPurify.sanitize(marked(text))}`.
The agent's only tool is the `freehire` CLI, which returns jobs (jobview shape,
incl. `public_slug`). freehire's canonical job card is `JobRow.svelte` — "the
single source of truth for how a job appears in any list" — driven by the `Job`
(jobview) type. `api.getJob(slug)` already fetches that structured `Job` from
`GET /api/v1/jobs/:slug` (public, cacheable). So the data and the card already
exist; what's missing is turning job links in the reply into those cards.

## Goals / Non-Goals

**Goals:**
- Render freehire job links in an assistant reply as real `JobRow` cards,
  hydrated from the structured jobview by slug.
- Keep the reply's prose as markdown, interleaved with cards, in order.
- Reuse `JobRow` (interactive, app-matched); open the job in a new tab.
- A pure, unit-tested parser; graceful skeleton/fallback; a client cache.
- Nudge the agent (skill) to emit canonical job URLs.

**Non-Goals:**
- No new/changed backend API (jobview already suffices). A batch endpoint is a
  noted optimization, not built now.
- No unfurl for company/board/external links (jobs only).
- No broader skill overhaul (separate future change) — just the link convention.
- Not touching how `thinking` renders (markdown-only, unchanged).

## Decisions

**1. Unfurl-by-fetch (approach A), not inline-JSON or model-authored markdown.**
The agent emits ordinary job URLs; the chat fetches the jobview and renders the
card. The card data is thus always the real posting — the model cannot fabricate
or drift a card, and its output stays small. *Alternatives:* inline-JSON (bloats
output, model must copy a large blob faithfully — error-prone) and model-authored
markdown cards (no logo/chips, depends on model discipline) — both rejected.

**2. Segment the reply; render markdown and cards separately.** A pure
`parseJobSegments(text): Segment[]` splits the reply into an ordered list of
`{kind:'markdown', text}` and `{kind:'job', slug, url}`. The renderer maps each
segment to either the existing sanitized `{@html}` (markdown) or a
`<JobCardUnfurl slug url>`. This preserves the DOMPurify guard for prose while
letting real Svelte components render between prose. **All** job links unfurl
(the chosen rule), each as its own block; a link mid-sentence therefore splits
the surrounding markdown into a before/after segment — acceptable because the
skill guides the agent to list jobs as their own lines/bullets.

**3. Detection: freehire job URLs, tolerant.** Match
`https?://(www\.)?freehire.dev/jobs/<slug>` and bare `/jobs/<slug>`, where
`<slug>` is the public-slug charset (`[a-z0-9-]+` plus whatever slugs use).
Strip trailing punctuation (`.`, `,`, `)`, `]`) from the captured slug/URL. A
non-job freehire link (`/companies/…`, `/b/…`) or external link is not matched.
The parser is the security-relevant surface too: the extracted slug is passed to
`api.getJob` (path-encoded) — never interpolated into HTML.

**4. `JobCardUnfurl.svelte` owns fetch + states + cache.** Given `slug`+`url`:
on mount, resolve the jobview from a **module-level `Map<slug, Promise<Job>>`
cache** (so duplicate slugs and re-renders share one request), show a skeleton
while pending, render `<JobRow job newTab>` on success, and a plain `<a>` to
`url` on rejection/404. The cache lives for the page session (cleared on
teardown is unnecessary — it's small and correct to keep).

**5. Reuse `JobRow`, add a minimal `newTab` prop.** Rather than fork a card,
pass a `newTab` (or `target`) prop so the shared card opens `target="_blank"
rel="noopener"` when rendered in chat, keeping the session open. This is the
only change to the shared component and is inert (default same-tab) everywhere
else. *Alternative — a chat-only static preview card:* duplicates JobRow's
markup and drifts; rejected.

**6. Agent emits canonical URLs (skill), optional CLI url.** `using-freehire`
SKILL.md gains a short "presenting jobs" rule: link each job as
`https://freehire.dev/jobs/<public_slug>`. Optionally the CLI's `search` text/
JSON output carries a ready `url` field so the agent copies rather than builds
it (fewer malformed slugs). The skill change is markdown; the CLI change is
optional polish.

## Risks / Trade-offs

- **N fetches for N cards** → cheap (public, cacheable GET) and de-duped by the
  module cache; a batch `?slugs=` endpoint is the noted optimization if a reply
  routinely lists many jobs.
- **A mid-sentence link splits prose into two markdown segments** → accepted per
  the "unfurl all links" decision; the skill steers the agent to list jobs on
  their own lines so this is rare.
- **`JobRow` deps in the chat context** (savedJobs/viewedJobs stores, auth
  dialog) → same SPA, same stores; verified to render standalone. The bookmark
  triggers the auth dialog for signed-out users exactly as on the main site.
- **Closed/removed job** → `api.getJob` still returns it (detail serves closed
  jobs), so the card renders with its state; a truly missing slug 404s → link
  fallback.
- **Streaming**: a card's slug may arrive across streamed chunks; the parser runs
  on the accumulated `message.text` each render, so a partial URL simply isn't
  matched yet and resolves to a card once the full URL has streamed.

## Migration Plan

1. **Frontend (`hire/web`)**: `unfurl.ts` (pure + vitest) → `JobCardUnfurl.svelte`
   → `JobRow` `newTab` prop → segment loop in `+page.svelte`. svelte-check /
   eslint / vitest / build.
2. **Agent (`freehire-agent`)**: `using-freehire` SKILL.md link convention;
   redeploy skills to host-2 (`cp docker/skills → ~/.roy/skills`, per agent-deploy
   doc) — or bake on next agent release.
3. **CLI (optional)**: `url` in `search` output; release if done.
4. **Local run** for live testing before shipping. No DB migration, no env.

## Open Questions

- Batch hydration endpoint (`GET /api/v1/jobs?slugs=`) — build now or defer?
  Defer; per-slug + cache is fine at expected volumes.
- Should the CLI url change ship in this iteration or be deferred with the skill
  nudge alone? Lean: skill nudge is enough; CLI url is optional polish.
