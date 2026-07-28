# The experience bank's way into the assistant

**Date:** 2026-07-28
**Status:** approved, not yet implemented

## Problem

The experience bank offers one route into the interviewer that fills it — a button reading
"Add more with the assistant", carrying a `Sparkles` icon, linking to
`/my/assistant?preset=profile` (`web/src/lib/components/ExperienceBankView.svelte:120`).

Three things are wrong with it.

1. **It names the tool, not the outcome.** "Add more with the assistant" tells someone
   which machine to talk to, never what to say or in what shape. The `Sparkles` glyph — the
   only one in the whole frontend — adds nothing but the house style of every AI feature
   shipped in the last two years.
2. **The chat opens empty.** `?preset=profile` creates a session under the interviewer's
   system prompt and then waits. The prompt (`internal/assistant/prompt.go:69`) already
   tells the agent to call `get_profile` + `experience_employments`, find the thinnest gap
   and ask one question — but no turn runs until a user message arrives, so the person who
   clicked "add an achievement" lands on a blank composer and has to invent an opening line.
3. **There is no entry at all when the bank is empty.** At zero records the view renders a
   bare `States` message (`ExperienceBankView.svelte:106-110`) with no button — the state
   where the interviewer is most useful is the state that hides it.

## The routing trap

The obvious fix — pass the existing `kickoff` prop (`AssistantChat.svelte:228`) from
`/my/assistant` — does not work, and fails in a way that is worse than the current
behaviour.

`/my/assistant` and `/my/assistant/[id]` are two separate `+page.svelte` files, each
mounting its own `<AssistantChat>`. Arriving with `?preset=profile` runs:

```
boot() → createAndOpen() → openSession() → onSessionChange(id)
       → goto('/my/assistant/<id>', {replaceState:true})   [async]
       → phase = 'ready' → dispatch(kickoff)               [SSE turn starts]
       ⋯ navigation lands: old page unmounts
       → onMount cleanup: cancelTurn() → turn aborted
```

The backend persists the user prompt before the abort lands, so the freshly mounted page
replays a transcript holding the candidate's own message and no answer. This is the same
class of race fixed in 7a7b70fc ("a new chat is no longer evicted by the URL it just
left"); its root is one surface living in two route nodes.

## Design

### 1. Copy

In `ExperienceBankView.svelte`, drop the `Sparkles` import and its element. The button
reads **"Add an achievement"** and is followed by an example line:

> Tell the assistant what you did — "I cut checkout latency by 40% in one quarter."

The example carries the work: it shows the expected grain of an answer (one concrete
result, ideally with a number) before the chat is even open, which is exactly what the
interviewer will ask for. The word "assistant" survives, demoted from the button label to
the hint.

The empty state keeps its explanatory sentence and gains the same button + hint pair
beneath it, so a bank with nothing in it offers the same way out as a bank with twelve
entries.

### 2. The kickoff line

One shared constant in `web/src/lib/assistant/presets.ts`, so the button and the chat quote
the same source:

> Walk through my experience with me — start with whatever is thinnest, and help me fill in
> the achievements that are missing.

Deliberately short. It is not a second system prompt: `profilePrompt` already carries the
method (start from `get_profile` + `experience_employments`, pick ONE gap, ask one question
at a time). The kickoff exists so a turn starts at all.

### 3. Merge the two assistant pages into `/my/assistant/[[id]]`

`web/src/routes/my/assistant/[id]/+page.svelte` and `.../+page.svelte` collapse into a
single optional-parameter route. One route node means the component is no longer
remounted when the entry URL is rewritten to the session's own address, which:

- makes the kickoff survive the rewrite (the whole point);
- removes the second, wasted `boot()` that runs on every visit to `/my/assistant` today;
- removes the source of the URL-versus-active-session race rather than guarding it again.

Behaviour to preserve exactly:

- **No id in the path** → open the newest session, else create one; rewrite the URL to that
  session with `replaceState` (landing on `/my/assistant` is a redirect, not a history
  step).
- **Id in the path** → open that session; switching sessions from the rail pushes a new
  history entry so Back returns to the previous chat.
- `?preset=profile` still forces a NEW session rather than resuming the newest
  (`AssistantChat.svelte:205`), and now also supplies `kickoff`. Every other way into the
  assistant — the nav rail, a bookmarked session, the rail's "New chat" — passes no
  kickoff and opens silent, exactly as today.

Call sites to update: `resolve('/my/assistant/[id]', …)` in both page files (one survives),
and `resolve('/my/assistant')` in `AssistantChat.svelte:453`. The plain `'/my/assistant'`
href in `accountNav.ts` needs no change — the optional-parameter route matches it.

### Re-entry

`kickoff` is dispatched only when `chat.messages.length === 0` (already the guard). A reload
of a session that has history re-sends nothing. A second click of "Add an achievement"
creates a second session and starts a second interview, which is the intended reading of
the button.

## Non-goals

- The spinner glyphs (`✢ ✳ ✶ ✻ ✽`) in the chat's thinking indicator stay. They were
  considered and explicitly kept.
- No new preset, no backend change. `PresetProfile` and `profilePrompt` are untouched.
- No starter-prompt chips in the empty chat. That would serve every entry into the
  assistant, not this one, and belongs to its own change.

## Testing

- `web/src/lib/assistant/presets.test.ts` — the preset map resolves `profile` to a kickoff
  and `chat` to none.
- Existing `sessions.test.ts` and `accountNav.test.ts` must stay green (the latter asserts
  the `/my/assistant` href, which the route merge must not break).
- Manual: `/my/profile` → Experience tab → "Add an achievement" → the chat opens on a new
  session, the kickoff appears as the candidate's message, and the agent answers with a
  single question about a thin part of the bank. Reload mid-answer must not re-send it.
