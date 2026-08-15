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
  ceiling is `RunnerConfig.MaxSteps` (default 8) unless the turn names its own through
  `TurnConfig`; interactive tailor uses 16 and autopilot 30. That value is always
  chosen server-side, because a ceiling a client can raise is not a ceiling. The
  max-steps wrap-up call offers no tools; if a provider still emits tool calls
  (Haiku on Bedrock has), they are executed before the turn stops so the transcript
  never ends on a dangling `tool_use`.
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
  assistant's tool calls and each tool's result. The model's argument string is stored
  verbatim except for one repair: `EncodeAssistant` runs `healToolArguments` first,
  stripping what a provider appended after valid JSON (Haiku's trailing `</invoke>`)
  so a retry can replay it. Two stores would drift; re-encoding parsed arguments would
  change the bytes the model saw.
- **Ownership is a `WHERE user_id = $1`.** A session the caller does not own is
  reported as missing, never as forbidden.
- **Every system prompt carries a LANGUAGE directive** (`SystemPrompt(preset, language)`,
  `prompt.go`), naming the candidate's saved profile language (freehire#1837) rather than
  guessing from whatever language they type in this message or defaulting to English. It
  governs the assistant's own conversational words only. The tailor preset's directive
  carves out the one exception: a `cv_edit` bullet follows the VACANCY's language instead
  (the honest wall's own instruction to reframe evidence "in the vacancy's language"), since
  a CV is written for the employer reading it, not for the candidate reading the chat beside
  it. Voice mode (`assistant_interview_voice.go`) is outside `SystemPrompt` — its Realtime
  `instructions` are a one-shot rendering, so it appends its own short directive via the same
  `LanguageName` map. `internal/matchanalysis` has an equivalent directive of its own
  (`freeTextLanguageDirective`) for the fit-analysis chain's free-text commentary, which
  follows the SAME rule (profile language, not the vacancy's) since it is the candidate's own
  reading of their fit, not CV content.

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

POST /api/v1/assistant/sessions/:id/retry
  └─ Runner.Continue — same loop, NO new user message (a failed turn already
     recorded the prompt; re-sending it would duplicate context). History rebuild
     still heals dangling tool_use from the interrupted turn.
```

**Files.** `runner.go` is the loop and the event shapes; `tool.go` the tool
contract, registry and strict argument decoding; `message.go` the stored-message
encoding and its round trip to `[]llms.MessageContent`; `store.go` the
owner-scoped persistence; `prompt.go` the per-preset system prompts.

**Unattended runs.** `POST /assistant/sessions/:id/autopilot` starts a tailoring pass
that walks every requirement of the vacancy in ONE turn — searching the experience
bank per requirement, editing what the evidence supports, asking nothing until it is
done. Everything a client could otherwise dictate is server-owned: the brief, the
raised ceiling (30 rounds against the usual 8), and the undo handle — every edit of
the turn is filed under one revision batch, so undoing the run is reverting that
batch (`UndoCVRevisionBatch` → `cvedit.RevertBatch`), newest first. The method itself is a section of `tailorPrompt`, not a
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
arrived from an application with nothing to type. That endpoint refuses only an ANSWERED
opening — a transcript holding an assistant message. The runner records the brief before
it calls the model, so a turn that dies upstream leaves one user line behind, and
refusing on that would strand the session with no way to retry. A reload of a live
conversation replays it rather than restarting the interview.

Three decisions carry it:

- **The application row is the authorisation.** `user_jobs` holds one row per (user,
  vacancy), so its absence answers "not yours" and "no such thing" with the same 404. The
  same read supplies the stage.
- **The invitation is placed by the server, not fetched by the agent.** `interview_context`
  carries the employer's most recent `interview_invitation` for the application, flagged
  untrusted in the payload itself. Registering the seven inbox tools to retrieve one
  predictable fact would be paid for on every turn — and `inbox.Service.InterviewInvitation`
  keeps the read inside the package whose rule it is, so `read_at` stays untouched.
- **Search finds by meaning; only `experience_get` finds by id.** `experience_search` scores
  a query against the owner's atoms and drops zero-scoring matches, so an id passed as a
  query matches nothing — and an empty search result means "no such evidence". Three
  surfaces hand the agent ids (an opening message naming a selection, `get_profile`'s
  `soft_duplicate_clusters`, the id a merge returns), and before the read tool existed the
  interviewer reported the candidate's own achievements as non-existent and answered about
  a different set. `experience_get` resolves ids in one owner-scoped pass, reports
  unresolvable ones instead of failing, and caps a call while naming what it did not read.
  A foreign id reads exactly like a deleted one.
- **The bank gate is enforced in code, not trusted to the prompt.** `experience_add`
  takes `said` as a quote, and `provenanceFor` checks it verbatim against what the
  candidate actually typed in this session (`assistant.UserSaid` over the transcript,
  whitespace-collapsed and case-insensitive, otherwise literal). A quote that appears
  is stamped `stated_in_chat`; one that does not — a paraphrase, a summary, an
  invention — is stamped `agent_inferred` and barred from CVs. An unverifiable
  transcript fails closed the same way. A rehearsal is where people improvise, and an
  improvisation banked as evidence is a claim they never made.

The preset carries the discovery, tracking and bank tools plus `interview_context`, and
NOT the CV tools, the mail tools or `read_current_page`. It runs on the ordinary step
ceiling: a rehearsal is a dialogue, not an unattended pass.

**Voice mode** (`handler/assistant_interview_voice.go`) is a hands-free spoken call
on an `interview` session, over OpenAI's Realtime API via WebRTC — the browser
connects directly to OpenAI, so audio never transits this process. It exists beside
the ordinary tool-calling turn loop, not inside it:

- `POST /assistant/sessions/:id/voice-token` mints a short-lived Realtime client
  secret. Its `instructions` carry a ONE-SHOT rendering of `rehearsalContext`
  (`voiceInterviewInstructions`) rather than a live `interview_context` tool call —
  a voice session has no data-channel tool relay, so what the text preset fetches on
  demand is baked in once, at mint time, and does not change mid-call.
- **No tool calls in a voice turn.** `experience_add` and every other rehearsal tool
  are text-only. Nothing said on a call can be written to the candidate's experience
  bank, regardless of what they confirm out loud — extending this needs a real tool
  relay (client-side JS answering a Realtime function-call event against our REST
  API), not a corollary of shipping the call itself.
- `POST /assistant/sessions/:id/voice-turns` appends one completed exchange through
  the same `Store.Append` `Runner.persist` uses, so the transcript reads the same
  whether a turn was typed or spoken, and a candidate can drop into text mid-call
  without losing what was said out loud. It is NOT how `interview-debrief` learns
  what happened — that preset never reads another session's transcript (see below) —
  persistence is for this session's own history view and for mid-session continuity.
- The credential rides the same `LLM_BASE_URL`/`LLM_API_KEY` gateway as everything
  else here, gated by its own `REALTIME_MODEL` (no default, same posture as
  `STTModel` in `internal/config` — unset means the feature is absent, not billed for
  by accident).
- **`REALTIME_MODEL` must carry the provider prefix** (`openai/gpt-realtime-2.1`, not
  the bare `gpt-realtime-2.1`), unlike every other model name in this file. The
  litellm proxy's realtime WebRTC feature is lazy-loaded and does not resolve a
  `model_name` alias the way the rest of the proxy does — `can_key_call_resolved_model`
  accepts the alias, but the actual upstream call then fails with `litellm.BadRequestError:
  LLM Provider NOT provided`. Confirmed live against the production proxy
  (`root@204.168.137.149:/opt/litellm`) on 2026-08-10: a `model_name: gpt-realtime-2.1`
  entry in `config.yaml` is harmless but unnecessary — what actually works is the
  client passing the provider-prefixed string directly.

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
2. Register it in `registry` (`internal/handler/assistant_tools.go`) under the presets that should offer it.
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
