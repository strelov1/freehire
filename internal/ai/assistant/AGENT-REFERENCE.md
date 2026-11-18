# The in-app assistant

## Scope

The `internal/ai/assistant` package — the agent's turn loop, its tool contract, the
session/transcript store, and the prompts each preset runs under. The tools
themselves live in `internal/api/handler` (`assistant_*_tools.go`), because they are
built from the same services the HTTP handlers use.

## Always true

- **The agent runs in this process.** There is no external runtime, no shell, and
  no credential: a tool receives the session owner's `userID` and calls a Go
  service directly. Anything a tool must not reach is a tool that does not exist.
- **A tool failure is not a turn failure.** `Registry.Call` never returns a Go
  error — an unknown tool, malformed arguments and a failing service all come
  back as `{"error": "..."}` for the model to read and correct within the same
  turn. Repeating a failed call with the same arguments ends the loop with
  `tool_loop`; changing only a `cv_edit` note does not count as a correction. A successful
  intervening tool clears this guard because it may change the state behind the refusal.
  Only a model/transport failure ends a turn as errored.
- **Every turn ends with exactly one `result` event.** A client that receives no
  terminal event waits forever, so the loop emits one on every path: an answer,
  the step cap, cancellation, and failure — including a turn that dies BEFORE the
  loop, on the first transcript write. And so does the handler on the two paths that
  never reach the loop at all: a queued message whose wait ran out, and a panic on the
  stream's own goroutine (`recoverStream`, `internal/api/handler`), which the server's
  recover middleware cannot see. A client only raises on a read failure, so a stream
  that simply stops reads to it as a turn that ended cleanly — and it then never ends
  the turn, queueing every later message behind one that is already over.
- **A turn is bounded two ways**: tool-calling rounds and the LLM client's per-call
  timeout. Zero/negative bounds fall back to defaults rather than meaning
  "unbounded" — an unbounded loop on a metered gateway is a runaway bill. The round
  ceiling is `RunnerConfig.MaxSteps` (default 8) unless the turn names its own through
  `TurnConfig`; interactive tailor uses 16 and autopilot 30. That value is always
  chosen server-side, because a ceiling a client can raise is not a ceiling. The
  max-steps wrap-up call keeps the tools in the request and forbids calling them
  (`llm.WithoutToolCalls`, tool_choice `none`): withholding them would change the
  prompt prefix and recompute the whole turn uncached. If a provider still emits tool
  calls (Haiku on Bedrock has), they are executed before the turn stops so the
  transcript never ends on a dangling `tool_use`.
- **A turn outlives its reader, and stops only when asked.** A failed SSE write means
  this reader is not listening — a phone freezing a backgrounded tab, a slept laptop
  — and nothing more; the turn runs to its own end and its transcript is stored
  whether or not anyone reads it. Treating that write as "the user left" is what once
  lost an unattended run its report after twenty-five committed CV edits. Stopping is
  a request of its own (`POST /assistant/sessions/:id/cancel`), because a dropped
  connection cannot be told apart from a deliberate Stop.
- **One turn at a time per session.** `turnRegistry` (`internal/api/handler/assistant_turns.go`)
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
  guessing from whatever language they type in this message or defaulting to English. An
  explicit request ("answer in Russian") overrides it for that conversation; typing in a
  language is not such a request. It governs the assistant's own conversational words only. The tailor preset's directive
  carves out the one exception: a `cv_edit` bullet follows the VACANCY's language instead
  (the honest wall's own instruction to reframe evidence "in the vacancy's language"), since
  a CV is written for the employer reading it, not for the candidate reading the chat beside
  it. Voice mode (`assistant_interview_voice.go`) is outside `SystemPrompt` — its Realtime
  `instructions` are a one-shot rendering, so it appends its own short directive via the same
  `LanguageName` map. `internal/candidate/matchanalysis` has an equivalent directive of its own
  (`freeTextLanguageDirective`) for the fit-analysis chain's free-text commentary, which
  follows the SAME rule (profile language, not the vacancy's) since it is the candidate's own
  reading of their fit, not CV content.

## How it works

```text
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
            └─ cap reached → one final text-only Chat → result(max_steps)

POST /api/v1/assistant/sessions/:id/retry
  └─ Runner.Continue — same loop, NO new user message (a failed turn already
     recorded the prompt; re-sending it would duplicate context). History rebuild
     still heals dangling tool_use from the interrupted turn.
```

**Files.** `runner.go` is the loop and the event shapes; `tool.go` the tool
contract, registry and strict argument decoding; `message.go` the stored-message
encoding and its round trip to `[]llms.MessageContent`; `store.go` the
owner-scoped persistence; `prompt.go` the per-preset system prompts (the CV ones in
`prompt_cv.go`, the interview ones in `prompt_interview.go`).

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
rather than an echo (a tool result is replayed into context on every later turn). Its counts
come from the sanitized entries. A named `unverified` part demotes a closed outcome to
`partial`, which the UI counts among remaining gaps. The model still judges semantic coverage;
the server does not infer missing evidence from free-text notes. `cv_edit` distinguishes
`applied` from `unchanged` and refuses empty batches with guidance to use `tailor_report`.

**Merge consent.** `experience_merge` first returns an exact confirmation claim without
mutating the bank. It executes only when the latest user message in the owned session
approves that claim. A plain yes is accepted only after one matching `request_confirmation`
call since the previous user message; a topic change supersedes an earlier approval. The same
unambiguous confirmation rule lets `experience_add` bank an answer to a confirmation card.

**Presets.** A session records `chat`, `tailor`, `base`, `profile`, `browse`, `interview`, `debrief`, `apply` or `support`. The vocabulary
is pinned by a CHECK constraint on `assistant_sessions.preset`, so adding one is a
schema change and not just a Go constant. The preset selects the system prompt and the
registered tools and nothing else, which is why one chat component serves
`/my/assistant`, the CV-tailoring workspace, the experience interview and the
extension's side panel. A tailoring session's CV tools close over the CV and vacancy
ids from the session binding, so the model cannot address another CV even by guessing
an id.

**`base`** is the conversation held against one BASE CV — the candidate's general CV that tailored
copies start from — bound to the CV and to no vacancy (migration 0232 admits the value; it is
minted by `POST /me/cvs/:id/base-session`, which reuses the conversation already bound to the CV).
It is the tailoring workspace's CV tool set without the vacancy tools: `cv_get`, `cv_edit` (same
path policy and evidence gate, so a bullet still needs a banked `evidence_id`),
`request_confirmation`, `check_evidence_fidelity`, `cv_page_count`, plus the shared bank tools. No
`cv_context`, `job_match`, `tailor_report` or cover letter — there is no vacancy to read them
against. Its prompt (`baseCVPrompt`) frames the CV as the candidate's own, kept and reused, and
sends "aim it at this job" to a tailored copy. It is a preset of its own rather than `tailor` with a
NULL job because the tailor prompt and tool set are about a vacancy and `tailor` sessions are
metered as tailoring sessions: a base turn is metered per turn under the assistant allowance, like
a chat, and its messages pass `mw.paidAI(FeatureAssistant)`. Opening the conversation is free — a
free account still opens the workspace and edits by hand. Agent edits on a base set
`cvs.hand_edited` (see `internal/candidate/cv/AGENTS.md`). `turnBounds` gives it the tailoring step
ceiling.

`support` is first-line support, open to every signed-in account, free or paid. It carries the FAQ
inline in `supportPrompt` and exactly three tools: `my_account` (an allowlisted read of the
caller's own plan, expiry, payment source, today's allowances and last five plan events),
`escalate_to_human` (one email to the support inbox per session, at most three a day per
account, claimed in `assistant_sessions.escalated_at` BEFORE the send and released when the
send fails; the summary is flattened to one inert line and Reply-To is the account address
only when it is verified) and `report_problem`. A turn is capped at 3 tool steps. It is
reachable only through `/support/sessions...` (free admission, own meter); every generic
`/assistant/sessions/:id/...` route answers a support session 404, so it cannot be driven
through, or deleted out of, its bounds. Its model is `SUPPORT_MODEL` (falling back to
`LLM_MODEL`) and its tag `support`. `go test -tags=llmlive ./internal/api/handler/ -run
LiveSupport` benches fixed dialogs and counts plan answers given without `my_account`.

The preset is answered TWICE — once for the prompt, once for the tool set — and both
switches read `NormalizePreset`, never the constants directly. They used to disagree on
the only case neither names: `SystemPrompt` fell back to `chat` for an unrecognised
preset while the registry compared `== PresetChat` and matched nothing, so such a session
would have been instructed at length about tools it had not been given.
`TestPromptOnlyNamesToolsThePresetHas` is the other half of that guard: every tool a
preset's prompt backticks must be registered for that preset. The reverse is not
required — a tool may go unnamed, its own description is what the model reads.

**Interview rehearsal, voice mode and the interview debrief** — the application row as authorisation, the server-placed invitation, the bank gate, the voice call through Bifrost, and how the debrief inverts the rehearsal — are in [AGENT-REFERENCE-INTERVIEW.md](AGENT-REFERENCE-INTERVIEW.md).

**The application chat** (`apply`, openspec/changes/auto-apply-chat) is held against one
automatic application, minted from `POST /assistant/sessions?preset=apply&job=<slug>` — and
an application holds ONE: asking again returns it, so the numbered questions and the answers
to them stay in one transcript. Its tools are the MCP server's auto-apply tools bound to the
session's job (`internal/api/handler/assistant_apply_tools.go`): no `job_slug` for the model,
and no `agents_may_apply` switch, which exists for agents acting outside the person's browser.

It is the one preset whose turns do not all start with the candidate. `POST .../sync` narrates
the application's `auto_apply_events` the conversation has not told them about, as a turn
whose input is a **status note**: `RoleNote` (the transcript CHECK's old `system` value),
replayed to the model as a human turn marked "not written by the candidate", never rendered,
never labelling the session, never charged to the allowance. The cursor
(`assistant_sessions.narrated_event_id`) moves only after the turn finished, so a failed
narration is repeated rather than skipped.

`browse` is the one preset whose prompt **overrides** the one it extends, rather
than only adding to it. The chat playbook opens with `get_profile` and a question
about what the candidate wants — correct on the website, wrong in a side panel,
where they opened the thing they want to talk about. Because the extension is
appended after that instruction, it has to name it: an override that does not say
what it replaces reads as advice about a different moment.

**Mail** (`handler/assistant_inbox_tools.go`) is the `chat` preset's alone — seven tools
over `internal/application/inbox`, the same use cases `/me/inbox` and `/me/emails` render. Three rules
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

A `browse` session is meant to be held from the browser extension, but the preset
recorded on the session does not by itself prove that: `internal/ai/browsertools.Hub` is
keyed by user id, not session id, so a browse session reached any other way would still
find a browser attached the moment the caller's extension is open elsewhere. The
`preset` a turn's request carries is therefore not the last word — `effectivePreset`
(`internal/api/handler/assistant.go`) demotes a browse session to plain chat, for BOTH the
prompt and the tool set, unless the turn itself authenticated as the extension's own
Bearer session JWT (`auth.ViaCookie`/`auth.ViaAPIKey` both false). Only a session that
clears that gate is the one preset whose agent can see something outside this process:
`read_current_page` attaches to the caller's browser-tool channel (`internal/ai/browsertools`)
as an in-process harness for the length of one call, the same way `/me/autofill/run`
does. That tool is deliberately absent from every other preset, and from a browse turn
that fails the carrier check — nothing is attached to their channel, and a tool that can
only fail teaches the model to stop calling tools. Reaching no browser is a tool error
naming the remedy, never a failed turn; the call carries its own deadline, because it is
the only tool whose completion depends on a client we do not control.

**Attached documents** (`attachment.go`). A candidate can attach a PDF — a CV, a vacancy,
an offer — to a message, in every preset (the chat and the tailoring workspace share one
composer). `POST /assistant/attachments` reads it with the résumé upload's own extractor
(`resume.ExtractPDFText`; there is one PDF reader in the codebase) and returns the text,
storing nothing; the client sends that text back with the message. It lives in the user
message's transcript row (`userContent.Attachments`), so it is replayed with the message
like anything typed, and `Decode` renders it to the model inside an `<attached_document>`
block flagged as material rather than instructions. Three rules hold it up:

- **It is not the candidate's own words.** `UserSaid` reads only `Text`, never an
  attachment, so a quote from an attached vacancy cannot be stamped `stated_in_chat` and
  reach a CV. Keeping documents out of `Text` is what makes that structural.
- **It is bounded server-side, twice.** `MaxAttachments` (3) per message and
  `MaxAttachmentRunes` per document: extraction truncates and says so, and the turn
  refuses anything larger — a client posting text directly cannot raise either. Because
  the text is replayed every later turn, the per-document cap is a recurring cost bound.
- **It is not masked, on purpose.** The assistant has never masked what a candidate types
  or pastes, and an attachment is a paste with a file picker in front of it. Masking it would
  also strip the very contact details a candidate attached an offer letter to ask about.

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

Moved to [AGENT-REFERENCE-TOOLS.md](AGENT-REFERENCE-TOOLS.md#adding-a-tool).

## Choosing the turn model: the bake-off

Moved to [AGENT-REFERENCE-TOOLS.md](AGENT-REFERENCE-TOOLS.md#choosing-the-turn-model-the-bake-off).

## Limitations

- **Bounded by the allowance, not by spend.** A turn is attributed to the account that ran
  it and its cost is readable (`GET /me/usage`, and per-feature on the gateway). What
  refuses one is `internal/ai/plan`: `assistant_meter.go` consumes `plan.FeatureAssistant`
  per turn against a per-feature, per-UTC-day allowance, and a refusal is HTTP 402 raised
  before the stream opens. The gateway's own per-account ceiling
  (`LLM_USER_MAX_BUDGET`) is a different instrument and stays deliberately unset, because a
  ceiling chosen before the spend distribution is known is a guess — an allowance prices
  the product, a gateway budget is a fuse.
- No summarisation: a long session loses its oldest messages to the window rather
  than compacting them.
- One tool round runs its calls sequentially. Parallel execution would need
  per-call result ordering the transcript does not currently model.
