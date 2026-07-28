## Context

The chat renders an assistant reply through `parseJobSegments` in
`web/src/lib/assistant/unfurl.ts`: a regex finds `/jobs/<slug>` in the settled
markdown, splits the reply into `markdown | job` segments, and swaps each job
segment for a `JobRow`. Everything else the model produced about that vacancy
stays prose.

Three facts about the existing code shape this design.

`internal/assistant/AGENTS.md` already states the rule this change applies:
*"Return structured data, not prose."* Recommendations are the one place in the
assistant where the flow runs the wrong way — the model has structure and emits
text.

`web/src/lib/assistant/chat.ts` accumulates every tool call of a turn into
`ChatMessage.tools[]` with its typed `input`, and `attachResult` sews each
result onto the call it answers. A structured deck therefore needs no new field
in the reducer and no new event type on the wire — the frame it would ride on
already exists.

`web/src/lib/components/JobRow.svelte` already takes a `footer` snippet rendered
inside the card's own border, added for the tracking board. The rationale has a
home without touching the card's internals.

Constraints: the same `AssistantChat` component serves the `chat` and `tailor`
presets; a tool result is also the model's history, so its size is a recurring
context cost on every later turn of the session; and the transcript is replayed
through the same reducer, so anything the renderer needs must be recoverable
from stored turns.

## Goals / Non-Goals

**Goals:**

- A recommendation is a typed artifact whose schema is the extension point:
  adding "matched skills" or "salary framing" later is a field, not a parser.
- One presented set renders as one uninterrupted deck; prose cannot split it.
- The model's rationale renders inside the card, not as a paragraph beside it.
- An invented slug is caught before render and reported to the model, not shown
  to the user as a degraded link.

**Non-Goals:**

- Enriching the card with vacancy facts the model does not author (salary,
  matched/missing skills). The schema is designed to accept them; this change
  does not add them.
- True interleaving of prose and decks within a single message. See Risks.
- Any change to how vacancies are searched, scored or tracked.
- Preserving cards in transcripts recorded before this change.

## Decisions

### A tool call, not a markdown convention

Considered: (a) requiring `- [Title](/jobs/slug) — rationale` and teaching the
parser to take the list item's tail as the note; (b) a fenced ` ```jobs ` block
carrying YAML; (c) a typed tool.

(a) is the smallest diff but leaves the artifact untyped: the note is a `string`
recovered by regex, and every later field is another regex. That is the thing
this change exists to end. (b) invents syntax inside the prose channel, and its
failure modes are the worst of the three — a half-streamed or malformed block is
shown to the user as raw text.

(c) costs more (a tool, a registration, a prompt rule, a renderer) and depends on
the model remembering to call the tool. That dependence is not new: the current
canonical-link rule is the same class of prompt obligation. Its failure mode is
also gentler than (b)'s — a link in prose renders as a link.

### The tool returns a receipt, not the vacancies

The model reaches `present_jobs` immediately after `search_jobs`, whose result
already carries each hit's full structured payload. Returning those vacancies
again would put the session's largest object into history twice, and it grows
with every recommendation the session makes.

So the split is: **the model contributes what only it knows** (which vacancies,
in what order, grouped how, and why), and **the client contributes the facts**
(fetched by slug through the existing `jobCache`). The tool returns
`{presented: [slug], dropped: [{slug, reason}]}`.

The renderer therefore joins two sources — `result.presented` decides *which*
entries survive, `input.jobs` supplies each one's note. This is the one piece of
incidental complexity the receipt design buys, and it is cheap: a slug-keyed
lookup over at most ten entries.

### Slugs resolve in one batched query, behind a narrow interface

`assistantHandlers.queries` is a concrete `*db.Queries`, so a tool that called
`GetJobBySlug` directly could not be unit-tested — which is why no test covers
`get_job` today. `present_jobs` instead takes a one-method resolver interface,
satisfied by `*db.Queries` in production and by a fake in tests, mirroring how
`searchHandlers` isolates its `descriptions` dependency.

The method is the existing `ResolveSlugsToJobIDs`
(`WHERE public_slug = ANY($1::text[])`, already generated for the view-log
worker): it answers the existence question for the whole deck in one round trip
rather than ten. Resolution is existence only — a vacancy that closed between the
search and the recommendation still resolves, and its card renders its own closed
state, which tells the user more than silently dropping it would.

### Unknown slugs are dropped, not fatal — unless all of them are

Failing the whole call on one bad slug would make the model regenerate nine
good rationales to fix one entry. Dropping the bad entry and naming it in
`dropped` lets the model decide whether a replacement is worth another round.

A call where nothing resolves is an error, because there is no deck to show and
silence would read to the model as success.

### The deck renders on the result, not on the call

`tool_use` reaches the client before `tool_result`. Rendering from the arguments
alone would paint a deck from unvalidated slugs, then repaint when the model
self-corrected — the user would watch a wrong answer being fixed. Gating on
`result !== undefined && !isError` costs nothing: `chat.ts` already tracks both.

### `present_jobs` is withheld from the tool-activity list

`ToolGroupList` renders a progress chip per call. A chip reading "Presenting
jobs" above the deck it produced is noise, so `AssistantChat` partitions
`message.tools` into the presenting calls (rendered as decks) and the rest
(rendered as the activity list).

### The unfurl path is deleted rather than kept as a fallback

Keeping it would mean two ways to show a vacancy and a standing question about
which one a given reply used. Deleting it makes the tool the only path, which is
what makes the prompt rule unambiguous ("never write a job link"). The cost is
stated in the proposal and accepted: older transcripts show links.

## Risks / Trade-offs

**The model does not call the tool** → The reply degrades to prose with a plain
link, not to a broken render. The prompt states the rule as an absolute and the
tool description repeats it. This is the same dependence the canonical-link rule
already carries today.

**Prose written before the call renders below the deck** → Tool output is
rendered above a message's text, so a preamble would appear under the cards.
Mitigated by the prompt: the deck comes first, and prose is a short closing note.
The real fix — making `ChatMessage.text` an ordered list of parts so text and
tool output interleave by position — is a larger reducer change and is
deliberately not attempted here. This design notes it as the seam.

**The model pads `why_fits` / `concerns`** → Both are optional and capped (four
and three). Their descriptions instruct omission over invention, matching the
repo's dictionary rule: never guess, emit nothing for unknowns.

**Old transcripts lose their cards** → Accepted. Stored turns contain no
`present_jobs` call, so there is nothing to replay as a deck; those replies show
markdown links.

**A validated slug still fails to hydrate** → The per-card fallback to a plain
link is retained. It is now reachable only through a client-side fetch failure,
not through an invented slug.

## Migration Plan

No schema change, no migration, no worker. The tool reads through the existing
`GetJobBySlug` query and writes nothing.

Deploy is a single release: the backend gains the tool and the prompt rule, the
frontend gains the deck and loses the unfurl path. Rollback is a revert — no
state is written that a previous version could not read, because the change adds
only transcript rows whose tool name an older client would render as an ordinary
tool call.

## Open Questions

None. The tool name (`present_jobs`), the deck cap (ten), and the `why_fits` /
`concerns` limits (four / three) are chosen rather than open — each is a
single-line change if experience argues otherwise.
