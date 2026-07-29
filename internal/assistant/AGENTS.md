# The in-app assistant

## Scope
The `internal/assistant` package — the agent's turn loop, its tool contract, the
session/transcript store, and the prompts each preset runs under. The tools
themselves live in `internal/handler` (`assistant_*_tools.go`), because they are
built from the same services the HTTP handlers use.

## Always true
- **The agent runs in this process.** There is no external runtime, no shell, and
  no credential: a tool receives the session owner's `userID` and calls a Go
  service directly. Anything a tool must not reach is a tool that does not exist.
- **A tool failure is not a turn failure.** `Registry.Call` never returns a Go
  error — an unknown tool, malformed arguments and a failing service all come
  back as `{"error": "..."}` for the model to read and correct within the same
  turn. Only a model/transport failure ends a turn as errored.
- **Every turn ends with exactly one `result` event.** A client that receives no
  terminal event waits forever, so the loop emits one on every path: an answer,
  the step cap, cancellation, and failure.
- **A turn is bounded three ways**: tool-calling rounds, the LLM client's per-call
  timeout, and cancellation. Zero/negative bounds fall back to defaults rather
  than meaning "unbounded" — an unbounded loop on a metered gateway is a runaway
  bill. The round ceiling is `RunnerConfig.MaxSteps` unless the turn names its own
  through `TurnConfig`; that value is always chosen server-side, because a ceiling
  a client can raise is not a ceiling.
- **The transcript IS the model's history.** One table holds both, including the
  assistant's tool calls and each tool's result, with the model's argument string
  stored verbatim. Two stores would drift; re-encoding parsed arguments would
  change the bytes the model saw.
- **Ownership is a `WHERE user_id = $1`.** A session the caller does not own is
  reported as missing, never as forbidden.

## How it works

```
POST /api/v1/assistant/sessions/:id/messages
  └─ handler: owner check → registry for the session's preset → SSE writer
       └─ Runner.Run
            ├─ persist the prompt, label the session, emit user_prompt
            ├─ history = system prompt + last N transcript messages
            └─ loop (≤ MaxSteps):
                 llm.Chat(history, tools) ──► text? → emit result(end_turn), stop
                                          └─► tool calls? → Registry.Call each,
                                              emit tool_use/tool_result,
                                              persist, append to history
            └─ cap reached → one final Chat with NO tools → result(max_steps)
```

**Files.** `runner.go` is the loop and the event shapes; `tool.go` the tool
contract, registry and strict argument decoding; `message.go` the stored-message
encoding and its round trip to `[]llms.MessageContent`; `store.go` the
owner-scoped persistence; `prompt.go` the per-preset system prompts.

**Unattended runs.** `POST /assistant/sessions/:id/autopilot` starts a tailoring pass
that walks every requirement of the vacancy in ONE turn — searching the experience
bank per requirement, editing what the evidence supports, asking nothing until it is
done. Everything a client could otherwise dictate is server-owned: the brief, the
raised ceiling (30 rounds against the usual 8), and the pre-run snapshot of the CV
that makes the run undoable. The method itself is a section of `tailorPrompt`, not a
second implementation — the rhythm changes, the rules do not, and `cv_edit` still
refuses a bullet with no `evidence_id`. The run accounts for itself through
`tailor_report`, which replaces the whole report on the CV and returns a receipt
rather than an echo (a tool result is replayed into context on every later turn).

**Presets.** A session records `chat`, `tailor`, `profile` or `browse`. The vocabulary
is pinned by a CHECK constraint on `assistant_sessions.preset`, so adding one is a
schema change and not just a Go constant. The preset selects the system prompt and the
registered tools and nothing else, which is why one chat component serves
`/my/assistant`, the CV-tailoring workspace, the experience interview and the
extension's side panel. A tailoring session's CV tools close over the CV and vacancy
ids from the session binding, so the model cannot address another CV even by guessing
an id.

`browse` is the one preset whose prompt **overrides** the one it extends, rather
than only adding to it. The chat playbook opens with `get_profile` and a question
about what the candidate wants — correct on the website, wrong in a side panel,
where they opened the thing they want to talk about. Because the extension is
appended after that instruction, it has to name it: an override that does not say
what it replaces reads as advice about a different moment.

A `browse` session is one held from the browser extension. It is the only preset whose
agent can see something outside this process: `read_current_page` attaches to the
caller's browser-tool channel (`internal/browsertools`) as an in-process harness for the
length of one call, the same way `/me/autofill/run` does. That tool is deliberately
absent from every other preset — nothing is attached to their channel, and a tool that
can only fail teaches the model to stop calling tools. Reaching no browser is a tool
error naming the remedy, never a failed turn; the call carries its own deadline, because
it is the only tool whose completion depends on a client we do not control.

**History trimming.** `trim` keeps the most recent N messages and then drops any
leading tool results whose originating call was trimmed away — providers reject a
tool result that answers no call in the conversation, so an orphan at the head
turns a context loss into a failed turn.

**Streaming.** `llm.Chat` splits the model's two delta channels: answer text goes
to `OnText`, reasoning to `OnThinking`. The chat renders them apart, so reasoning
is never mistaken for the answer.

## Adding a tool
1. Write it in `internal/handler/assistant_*_tools.go` as an `assistant.Tool`:
   a name, a one-paragraph description the model reads, a JSON schema, and a
   `Run` that decodes with `assistant.DecodeArgs` and calls the same service the
   HTTP handler calls.
2. Register it in `assistantRegistry` under the presets that should offer it.
3. Return structured data, not prose. Include the fields the model needs to act
   (a vacancy's `public_slug`, not just its title; an achievement's id and whether it may
   be written to a CV, not just its text).
3b. Keep the result small. It is persisted in the transcript and replayed into the model's
   context on EVERY later turn — this is why `get_profile` reports the experience bank's
   shape and counts while `experience_search` returns its content per question.
4. Give errors a message the model can act on — name the invalid value and list
   the valid ones. That message is the model's only path to self-correction.

## Limitations
- No metering. The assistant is free behind the restricted-rollout gate;
  `internal/credits` is the seam for a per-turn debit.
- No summarisation: a long session loses its oldest messages to the window rather
  than compacting them.
- One tool round runs its calls sequentially. Parallel execution would need
  per-call result ordering the transcript does not currently model.
