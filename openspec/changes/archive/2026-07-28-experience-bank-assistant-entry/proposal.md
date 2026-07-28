## Why

The experience bank's only route into the interviewer that fills it is a button reading
"Add more with the assistant" — it names the tool instead of the outcome, carries the
frontend's only `Sparkles` glyph, and lands the candidate on a blank composer. The
`profile` preset already spells out how to interview (`internal/assistant/prompt.go:69`:
read the bank, find the thinnest spot, ask one question), but no turn runs until a user
message arrives, so the agent sits silent and the person who just clicked "add an
achievement" has to invent an opening line. At zero records the button is not rendered at
all — the state where the interviewer is most useful is the state that hides it.

## What Changes

- The bank's call to action becomes **"Add an achievement"**, followed by an example line
  (`Tell the assistant what you did — "I cut checkout latency by 40% in one quarter."`)
  that shows the expected grain of an answer before the chat opens. The `Sparkles` icon is
  removed.
- The same button and hint render in the bank's empty state, under its existing
  explanation.
- Arriving at the assistant with `?preset=profile` auto-sends a shared kickoff line as the
  candidate's first message, so the interview opens on a real question instead of a blank
  composer. Every other entry into the assistant is unchanged and opens silent.
- `/my/assistant` and `/my/assistant/[id]` merge into one optional-parameter route
  `/my/assistant/[[id]]`. Today they are two route nodes, so rewriting the entry URL to the
  session's own address remounts `AssistantChat` and its `onMount` cleanup aborts the turn
  that had just started — the kickoff cannot survive without this. The merge also removes a
  wasted second `boot()` on every visit and the URL-versus-active-session race last patched
  in 7a7b70fc. Addressing, history behaviour and dead-link handling are preserved exactly.

## Capabilities

### New Capabilities

_None._

### Modified Capabilities

- `experience-bank`: adds a requirement that the bank offers a named way into the
  interviewer in every state, and that entering it starts the conversation rather than
  waiting for the candidate to open it. Delivers the existing "the interviewer opens on a
  real gap" scenario, which today cannot happen until the user types first.

## Impact

- `web/src/lib/components/ExperienceBankView.svelte` — copy, icon, empty state.
- `web/src/lib/assistant/presets.ts` (new) — the kickoff line, shared by the button's
  destination and the chat.
- `web/src/routes/my/assistant/+page.svelte` and `web/src/routes/my/assistant/[id]/+page.svelte`
  — merged into `web/src/routes/my/assistant/[[id]]/+page.svelte`.
- `web/src/lib/assistant/AssistantChat.svelte` — one `resolve()` call site.
- No backend change. `PresetProfile`, `profilePrompt` and the session API are untouched.
