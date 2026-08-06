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
  the step cap, cancellation, and failure — and so does the handler on the one path
  that never reaches the loop, a queued message whose wait ran out.
- **A turn is bounded two ways**: tool-calling rounds and the LLM client's per-call
  timeout. Zero/negative bounds fall back to defaults rather than meaning
  "unbounded" — an unbounded loop on a metered gateway is a runaway bill. The round
  ceiling is `RunnerConfig.MaxSteps` unless the turn names its own through
  `TurnConfig`; that value is always chosen server-side, because a ceiling a client
  can raise is not a ceiling.
- **A turn outlives its reader, and stops only when asked.** A failed SSE write means
  this reader is not listening — a phone freezing a backgrounded tab, a slept laptop
  — and nothing more; the turn runs to its own end and its transcript is stored
  whether or not anyone reads it. Treating that write as "the user left" is what once
  lost an unattended run its report after twenty-five committed CV edits. Stopping is
  a request of its own (`POST /assistant/sessions/:id/cancel`), because a dropped
  connection cannot be told apart from a deliberate Stop.
- **One turn at a time per session.** `turnRegistry` (`internal/handler/assistant_turns.go`)
  holds each running turn's `CancelFunc` — the only handle on a turn no request owns
  any more — and makes a second message wait rather than run beside the first: two
  turns of one tailoring session would edit one CV from two conversations that cannot
  see each other. One message may wait, a further one is refused. The registry is
  per-process, so a turn started before a blue/green flip cannot be cancelled and ends
  at its step cap.
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

**Presets.** A session records `chat`, `tailor`, `profile`, `browse`, `interview` or `debrief`. The vocabulary
is pinned by a CHECK constraint on `assistant_sessions.preset`, so adding one is a
schema change and not just a Go constant. The preset selects the system prompt and the
registered tools and nothing else, which is why one chat component serves
`/my/assistant`, the CV-tailoring workspace, the experience interview and the
extension's side panel. A tailoring session's CV tools close over the CV and vacancy
ids from the session binding, so the model cannot address another CV even by guessing
an id.

The preset is answered TWICE — once for the prompt, once for the tool set — and both
switches read `NormalizePreset`, never the constants directly. They used to disagree on
the only case neither names: `SystemPrompt` fell back to `chat` for an unrecognised
preset while the registry compared `== PresetChat` and matched nothing, so such a session
would have been instructed at length about tools it had not been given.
`TestPromptOnlyNamesToolsThePresetHas` is the other half of that guard: every tool a
preset's prompt backticks must be registered for that preset. The reverse is not
required — a tool may go unnamed, its own description is what the model reads.

**Interview rehearsal** (`handler/assistant_interview_tools.go`) is a mock interview against
one application. It is minted from `POST /assistant/sessions?preset=interview&job=<slug>`,
binds to a vacancy and to NO CV, and opens itself: `POST /assistant/sessions/:id/opening`
runs one turn under a server-side brief, the way autopilot does, because the candidate
arrived from an application with nothing to type. That endpoint refuses a session that
already has a transcript — a reload replays the conversation rather than restarting the
interview.

Three decisions carry it:

- **The application row is the authorisation.** `user_jobs` holds one row per (user,
  vacancy), so its absence answers "not yours" and "no such thing" with the same 404. The
  same read supplies the stage.
- **The invitation is placed by the server, not fetched by the agent.** `interview_context`
  carries the employer's most recent `interview_invitation` for the application, flagged
  untrusted in the payload itself. Registering the seven inbox tools to retrieve one
  predictable fact would be paid for on every turn — and `inbox.Service.InterviewInvitation`
  keeps the read inside the package whose rule it is, so `read_at` stays untouched.
- **The bank gate is a prompt rule, deliberately.** `experience_add` takes `said` as a
  string and stamps `stated_in_chat` whoever composed it, so the service cannot tell the
  candidate's words from the model's paraphrase. The prompt requires their explicit
  agreement before recording anything; what makes that checkable is the transcript, where
  both the offer and the "yes" are visible. A rehearsal is where people improvise, and an
  improvisation banked as evidence is a claim they never made.

The preset carries the discovery, tracking and bank tools plus `interview_context`, and
NOT the CV tools, the mail tools or `read_current_page`. It runs on the ordinary step
ceiling: a rehearsal is a dialogue, not an unattended pass.

**Interview debrief** is the rehearsal's mirror: the review of an interview that has
already happened, minted from `POST /assistant/sessions?preset=debrief&job=<slug>`. It
shares everything with the rehearsal but the prompt — the same binding, the same
`interview_context`, the same tool set, the same self-opening endpoint. Both are covered
by one notion in the handler (`bindsToApplication`), and `openingBriefFor` is where the
set of presets that speak first is written down; a preset that returns no brief is one
`PostAssistantOpening` refuses.

It is a preset of its own rather than a mode inside `interview` because of one rule that
**inverts** between them. The rehearsal must police itself against banking what the
candidate invented on the spot — improvising is what a rehearsal is for. In a debrief the
candidate is recalling what they already said to an employer, banking it is the purpose
of the session, and the instruction becomes "record what they confirm, never a number you
supplied". One prompt holding both would leak each mode's instinct into the other.

`TestTheDebriefCarriesTheRehearsalsTools` compares the two registries for equality. Two
presets sharing a tool set is new here, and it is the prompt — not the tools — that is
allowed to differ; a tool added to one and forgotten in the other would be a debrief that
cannot read the interview it is reviewing.

The stage governs where the client offers it (`offersDebrief` in `web/src/lib/stages.ts`:
`interview`, `offer`, `accepted`, `rejected`), never what the endpoint accepts. Somebody
who sat an interview and never moved their application's stage is exactly who it is for.

`browse` is the one preset whose prompt **overrides** the one it extends, rather
than only adding to it. The chat playbook opens with `get_profile` and a question
about what the candidate wants — correct on the website, wrong in a side panel,
where they opened the thing they want to talk about. Because the extension is
appended after that instruction, it has to name it: an override that does not say
what it replaces reads as advice about a different moment.

**Mail** (`handler/assistant_inbox_tools.go`) is the `chat` preset's alone — seven tools
over `internal/inbox`, the same use cases `/me/inbox` and `/me/emails` render. Three rules
are load-bearing:

- **No tool opens one message by id.** That endpoint marks the message read, and `read_at`
  means "a human saw this" — an agent sweeping the backlog through it would zero its
  owner's unread count. Bodies come from the listing, which marks nothing, and
  `inbox.Queries` has no read-marking method at all so the guarantee is structural rather
  than a convention.
- **A body-bearing page is capped at 10**, against the 50 an external harness may request
  over HTTP. A harness reads a page once; a tool result is replayed into the model's
  context on every later turn.
- **No tool sends mail.** Bodies are attacker-controlled text, and the surest answer to a
  prompt injection is that it has no outbound channel. The reachable damage is a wrong
  label or a wrong link, both reversible from `/my/inbox`, and `mailclassify.AdvanceStage`
  keeps stage movement monotonically forward.

`inbox_triage` refuses an out-of-vocabulary label where `mailclassify.Sanitize` coerces it
to `other`. The worker sanitizes because it persists raw model output derived from an
attacker's body; the tool carries a judgement the candidate asked for, and silently
rewriting it would record a verdict nobody chose.

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
turns a context loss into a failed turn. It also closes the inverse shape: a
`tool_use` whose `tool_result` never landed (a turn that died after persisting
the calls). Replay synthesises an error result for each unanswered call so the
next turn stays provider-legal — Bedrock in particular rejects the whole request
otherwise.

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

**Whose spend a turn is.** Every turn goes out on the CALLER's own gateway credential,
resolved by `internal/llmkey` and minted on their first AI call. The runner is built once
at boot and cloned per turn (`Runner.With`), because the credential is per-user and the
bounds are not. The call is tagged `feature:assistant` **and** `preset:<preset>`: the
gateway files one spend row per tag, and a rehearsal, an unattended tailoring run and a
question cost wildly different amounts, so the preset is what makes them comparable.

Two rules hold the whole thing up, and both fail silently if broken:

- **Attribution never costs a turn.** An unmintable credential, an unreachable admin API
  and a rejected key all fall back to the service credential and the turn completes. A key
  the gateway has forgotten is additionally retried once and reported, in `internal/llm`'s
  transport — the only layer that sees a status code rather than langchaingo's error prose.
- **A re-credentialed client must not share the schema-model cache.** See the comment on
  `modelCache` in `internal/llm/schema.go`. Sharing it sends one user's schema-bound call
  out on another user's key, successfully.

CV tailoring has no tag of its own on purpose: `/me/cvs/tailor` makes no model call: it
mints a CV and debits credits, and the work is a turn under the `tailor` preset. A second
tag would double-count one spend.

## Limitations
- **Measured, not bounded.** A turn is now attributed to the account that ran it and its
  cost is readable (`GET /me/usage`, and per-feature on the gateway), but nothing refuses
  one. The gateway supports a per-account ceiling and `LLM_USER_MAX_BUDGET` passes one
  through; it is deliberately unset, because a ceiling chosen before the spend
  distribution is known is a guess. `internal/credits` remains the seam for a per-turn
  debit — points price the product, a gateway budget is a fuse, and they are not the same
  instrument.
- No summarisation: a long session loses its oldest messages to the window rather
  than compacting them.
- One tool round runs its calls sequentially. Parallel execution would need
  per-call result ordering the transcript does not currently model.
