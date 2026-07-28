## Context

The full design rationale, including the copy alternatives that were weighed, lives in
`docs/superpowers/specs/2026-07-28-experience-bank-assistant-entry-design.md` (approved
2026-07-28). This document records the technical decisions the implementation depends on.

Current state:

- `ExperienceBankView.svelte:120` renders the only entry into the `profile` interviewer:
  a `Sparkles`-prefixed button labelled "Add more with the assistant" linking to
  `/my/assistant?preset=profile`. The empty branch (lines 106-110) renders a bare `States`
  message with no action at all.
- `AssistantChat.svelte` already accepts a `kickoff` prop and auto-sends it into an empty
  session (line 228). Only `/tailor/[slug]` passes it.
- `AssistantChat.svelte:205` already treats a non-`chat` preset as "start a NEW session"
  rather than "resume the newest".
- `/my/assistant` and `/my/assistant/[id]` are two `+page.svelte` files, each mounting its
  own `AssistantChat`.

## Goals / Non-Goals

**Goals:**

- The bank's entry into the interviewer names an outcome and shows an example, in every
  state of the bank.
- Following that entry produces a question from the agent, not a blank composer.
- The opening turn survives the entry URL being rewritten to the session's own address.

**Non-Goals:**

- The chat's thinking-indicator glyphs (`✢ ✳ ✶ ✻ ✽`) stay. Considered and kept.
- No backend change: `PresetProfile`, `profilePrompt` and the session API are untouched.
- No starter-prompt chips in the empty chat — that would serve every entry into the
  assistant, not this one, and belongs to its own change.

## Decisions

### The kickoff is one shared constant, deliberately short

`web/src/lib/assistant/presets.ts` holds the preset → kickoff mapping, so the entry point
and the chat quote one source:

> Walk through my experience with me — start with whatever is thinnest, and help me fill in
> the achievements that are missing.

_Alternative rejected:_ writing the interview method into the kickoff ("call get_profile,
find a role with no achievements, ask one question…"). That method already lives in
`profilePrompt` on the backend; duplicating it client-side creates two sources of truth for
the agent's behaviour, and a later edit to the server prompt would silently diverge from
what the frontend asserts. The kickoff exists to make a turn start at all — nothing more.

### Merge the two assistant pages into `/my/assistant/[[id]]`

Passing `kickoff` from the current entry page does not work. The sequence is:

```
boot() → createAndOpen() → openSession() → onSessionChange(id)
       → goto('/my/assistant/<id>', {replaceState:true})   [async]
       → phase = 'ready' → dispatch(kickoff)               [SSE turn starts]
       ⋯ navigation lands: the old page unmounts
       → onMount cleanup: cancelTurn() → turn aborted
```

The backend persists the user prompt before the abort arrives, so the newly mounted page
replays a transcript holding the candidate's message and no answer — worse than the silent
composer it replaced. One route node removes the remount, and with it the wasted second
`boot()` that runs on every visit today and the URL-versus-active-session race last patched
in 7a7b70fc.

Behaviour the merged route must preserve exactly:

- No id in the path → open the newest session, else create one; rewrite to that session's
  address with `replaceState` (landing on `/my/assistant` is a redirect, not a history
  step).
- Id in the path → open it; selecting another session from the rail pushes a history entry
  so Back returns to the previous chat.
- A conversation the caller cannot open still renders the dead-link panel.

_Alternatives rejected:_ (a) carrying the kickoff through a second query key
(`?start=profile`) read by the `[id]` page — cheaper, but leaves the duplicated page, the
double boot and a second URL key that means what `preset` already means; (b) not rewriting
the URL at all for preset entries — F5 would then mint another session and another
interview on every reload.

### Re-entry is guarded by an empty transcript, not by a flag

`kickoff` dispatches only when `chat.messages.length === 0`, which is already the guard.
A reload of a session with history sends nothing. A second click of the bank's action
creates a second session and starts a second interview — the intended reading of the
button.

## Risks / Trade-offs

- **Typed `resolve()` call sites break on the route rename** → `resolve('/my/assistant')`
  in `AssistantChat.svelte:453` and `resolve('/my/assistant/[id]')` in the merged page must
  be updated to the optional-parameter form; `svelte-kit sync` + `pnpm build` catches any
  missed one. The plain `'/my/assistant'` href in `accountNav.ts` is a string, not a
  typed resolve, and the optional-parameter route still matches it — `accountNav.test.ts`
  guards this.
- **A kickoff turn costs a model call the candidate did not explicitly ask for** → it is
  spent only on an explicit click of an action that says what it will do, and the assistant
  is behind the beta gate, where there is no per-turn metering to distort.
- **Merging the pages touches routing shared by every assistant entry** → the merged page
  is a near-copy of the two it replaces; `accountNav.test.ts` and manual Back/Forward
  checks cover the addressing behaviour that has no automated coverage.
