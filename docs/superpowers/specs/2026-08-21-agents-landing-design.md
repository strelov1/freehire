# `/agents` — the agent-friendly landing

**Date:** 2026-08-21
**Status:** approved

## Problem

freehire already has four agent surfaces, and every one of them is a separate
page a visitor has to already know about:

| Surface | Page |
|---|---|
| CLI (Go binary, agent skills, Claude Code plugin) | `/cli` |
| MCP server (`freehire-mcp`) | `/cli#mcp` (`/mcp` is a 308 to it) |
| Custom GPT / ChatGPT Actions | `/chatgpt` |
| Raw HTTP API | `/docs/api`, `/openapi.yaml`, `/llms.txt` |

Nothing answers the question a visitor with an agent actually has: **"I have an
agent — how do I point it at freehire?"** `/llms.txt` answers it for the agent;
there is no human-facing equivalent.

## What we build

A new landing at **`/agents`**. It is a **hub that links out** — `/cli` and
`/chatgpt` keep their content, their URLs and their inbound links (MCP
directories link to `/mcp` → `/cli#mcp`; breaking that is not worth it).

The page's central claim, and the reason it is not three equal cards: **the
surfaces are not equivalent.** A local harness driving the CLI gets the whole
surface. Claude Desktop over MCP gets nearly all of it. ChatGPT gets public
reads plus tracking behind a pasted key. The page ranks them and says why.

### The one action

The primary action is not reading — it is **copying one block of text and
pasting it into an agent**. That block is the hero:

```
Install freehire and use it to run my job search.

  curl -fsSL https://freehire.me/install.sh | sh

Then read https://freehire.me/llms.txt for the conventions,
and `freehire --help` for the commands.
```

It works in any local harness, needs no plugin registry, and hands the agent
two URLs that already exist and are already maintained.

### Sections

1. **Hero** — pitch left, the copyable handoff card right (dotted `grid-bg`
   background and staggered `.reveal`, same as `/cli` and `/chatgpt`, so the
   page reads as the same product).
2. **Three tracks, visually unequal.** `01` is a wide card with a terminal;
   `02` and `03` share a narrower row below it. The asymmetry carries the
   ranking without a sentence having to claim it.
   - `01` Local harness (Claude Code, Codex, Cursor, Cline, …) → `/cli`
   - `02` Claude Desktop / any MCP host → `/cli#mcp`
   - `03` ChatGPT → `/chatgpt`
   - a quiet fourth line: roll your own → `/docs/api`, `/openapi.yaml`,
     `/llms.txt`
3. **What your agent can do** — five tasks, each linking to the feature page
   that owns it (search, tracking, market fit, CV tailoring, mail triage).
4. **Why local wins** — the honest paragraph: the full tool set including CV
   edits and PDF render, access to your own files, and no per-conversation
   cap on tool calls.
5. **FAQ** — six questions, rendered from `agentsFaq.ts` so the visible block
   and the `faqPageJsonLd` payload share one array (Google drops the rich
   result when they disagree).

### Cross-linking

Discovery is the part a new landing usually gets wrong. Edits:

- `Footer.svelte` — "Agents" joins the Resources group, above CLI.
- `sitemap.ts` — `/agents` joins `STATIC_PATHS`.
- `CliView.svelte`, `ChatGptView.svelte` — a backlink to `/agents`, so the
  three pages form a triangle rather than three dead ends.
- `static/llms.txt` — `/agents` named in the prebuilt-clients section.

## Non-goals

- No new API, no backend change. Every URL the page cites already resolves.
- No absorbing `/cli` or `/chatgpt`. They stay where they are.
- No screenshots. Previews are markup, per the feature-landing recipe.

## Testing

- `agentsFaq.test.ts` — the FAQ array is non-empty and every entry has both a
  question and an answer, which is what `faqPageJsonLd` requires.
- `sitemap.test.ts` already asserts `STATIC_PATHS` has no duplicates; add
  `/agents` to the list it checks.
- `pnpm run check` and `pnpm run lint` for the Svelte surface.
