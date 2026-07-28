## Why

A recommendation from the in-app assistant is currently a *prose* artifact: the
model writes markdown, and the chat recovers job cards from it by regex-matching
`/jobs/<slug>` out of the finished reply. Everything the model actually knows
about *why* it picked a vacancy — the rationale, the ordering, the grouping —
survives only as free text wrapped around the card, so a recommendation renders
as three blocks per vacancy (a prose heading, a card repeating that heading, and
a loose rationale paragraph), the cards are interrupted by prose between them,
and there is nothing structured to enrich later.

The prose channel also produces defects that are unreachable from the renderer: a
rationale the model happens to indent four spaces becomes a `<pre><code>` block,
and a slug the model invents reaches the user as a bare "View job ↗" link because
nothing validated it before render.

## What Changes

- **NEW** `present_jobs` tool. The model presents vacancies by calling a typed
  tool with `{heading?, jobs: [{slug, note, why_fits?, concerns?}]}` instead of
  writing links into prose. The tool's JSON schema becomes the structure that
  later work enriches (salary framing, matched/missing skills, actions) by adding
  a field rather than a new regex.
- **NEW** the tool validates every slug against the catalogue before the deck is
  shown, reporting unknown slugs back to the model so it self-corrects within the
  same turn. It returns a short receipt (`presented` / `dropped`), not the
  vacancy payload — `search_jobs` has already put those vacancies in the model's
  history, and echoing them would double the most expensive part of the context.
- **NEW** the chat renders a presented set as one contiguous deck: `JobDeck`
  (optional heading + the cards) and `JobDeckCard` (the shared `JobRow` plus a
  footer carrying the model's note, `why_fits` chips and muted `concerns`). The
  rationale now lives inside the card's border, and prose can no longer split a
  deck because the cards no longer live in the prose.
- **BREAKING** the markdown unfurl path is removed (`unfurl.ts`,
  `unfurl.test.ts`, `JobCardUnfurl.svelte`). A job link written in prose renders
  as an ordinary link. Consequence, accepted deliberately: assistant transcripts
  recorded before this change show links where they used to show cards, because
  their stored turns contain no `present_jobs` call to replay.
- **MODIFIED** the system prompt: a vacancy is shown *only* through
  `present_jobs`; a job link in prose is forbidden; `public_slug` is copied from a
  tool result, never constructed; the deck comes first, with no preamble before
  it.

## Capabilities

### New Capabilities
<!-- None. This change replaces the mechanism behind an existing capability
     rather than introducing a new one. -->

### Modified Capabilities
- `assistant-job-cards`: how a vacancy reaches the user changes from "the chat
  unfurls job links found in the reply's markdown" to "the model calls a typed
  tool and the chat renders the deck it authored". Every requirement in the
  capability is restated: the trigger, the hydration contract, the deck layout
  and the prompt obligation.
- `assistant-agent-runtime`: the enumerated tool surface gains `present_jobs`,
  the first tool whose purpose is presentation rather than retrieval or state
  change.

## Impact

**Backend** — `internal/handler/assistant_present_tool.go` (the new tool),
registered in `assistantDiscoveryTools` so both presets offer it;
`internal/assistant/prompt.go` (the presentation rule). No schema or migration:
the tool reads through the existing `ResolveSlugsToJobIDs` query and stores
nothing.

**Frontend** — `web/src/lib/assistant/`: new `JobDeck.svelte` and
`JobDeckCard.svelte`; `AssistantChat.svelte` withholds `present_jobs` from
`ToolGroupList` and renders it as a deck; `unfurl.ts`, `unfurl.test.ts` and
`JobCardUnfurl.svelte` are deleted. `jobCache.ts` and `JobRow.svelte` are reused
unchanged — the deck hydrates cards through the same cache, and `JobRow` already
exposes the `footer` snippet the rationale needs.

**Behavioural risk** — the deck depends on the model calling the tool. The
failure mode is a plain link rather than a broken render, and it is the same
class of prompt dependence the current canonical-link rule already carries.

**Known seam, not addressed here** — tool output renders above a message's text,
so prose written before the call appears below the deck. True interleaving of
prose and decks would require `ChatMessage.text` to become an ordered list of
parts; the prompt sidesteps it by putting the deck first.
