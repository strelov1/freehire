## Context

`internal/assistant` runs four presets today — `chat`, `tailor`, `profile`, `browse`. A
preset is deliberately thin: it selects the system prompt and the registered tool set and
nothing else, which is why one chat component serves the assistant page, the tailoring
workspace, the experience interview and the extension's side panel. The vocabulary is
pinned by a CHECK constraint on `assistant_sessions.preset`, so a fifth preset is a schema
change and not just a Go constant.

Two things this change needs already exist and were verified in the code:

- `tailoringContext(ctx, userID, jobID)` and `cvContextTool(jobID)`
  (`internal/handler/assistant_cv_tools.go`) close over the **vacancy alone**. The fit
  analysis with its requirements is reachable without any CV binding.
- `withBankEvidence` / `evidenceFor` answer, per requirement, what the candidate's bank
  holds — a linear scan over the caller's own atoms with no model call in it. Its comment
  records why it exists: a recorded tailoring session spent ten `experience_search` rounds
  and never reached an edit.
- The autopilot endpoint runs a turn from a **server-side brief** (`autopilotBrief`) passed
  to `streamTurn` as an ordinary user turn. That is the mechanism for an agent that speaks
  first.

`assistant_sessions` already carries `job_id`, so binding a rehearsal to a vacancy needs no
new column.

## Goals / Non-Goals

**Goals:**

- A rehearsal that asks about *this* candidate's recorded experience against *this*
  vacancy's requirements — the question set no general-purpose chatbot can generate.
- Reuse the existing chat surface, transcript, streaming and ownership model unchanged.
- Keep the experience bank clean: a rehearsal must not bank what the candidate improvised.
- Leave a seam for voice input without building it.

**Non-Goals:**

- Voice capture or transcription.
- A persisted rehearsal report, a readiness score, or progress across sessions.
- Metering the turn against AI credits. That gap is the assistant's as a whole
  (`internal/assistant/AGENTS.md`), not this change's to close.
- Coaching the candidate's technical knowledge as an authority. The agent conducts an
  interview; it does not become a reference on the stack.

## Decisions

### A fifth preset, not a mode inside `chat`

**Chosen:** a new `interview` preset with its own prompt and tool set.

**Alternative — a section in `chatPrompt`:** no migration, and a rehearsal could start from
any conversation. Rejected because the session would carry no vacancy binding (the agent
re-establishes the subject every time), because `chatPrompt` already carries the search
playbook plus the whole mail section, and because a preset exists precisely so that the
prompt and the tools are chosen for one job.

### A new `interview_context`, not a reuse of `cv_context`

**Chosen:** a new tool, closing over the session's `job_id`, reusing `tailoringContext` and
`withBankEvidence` underneath.

**Alternative — register `cvContextTool(*sess.JobID)` as-is:** almost no new code, since it
already binds to the vacancy alone. Rejected on the tool's *description*, which is what the
model reads as instruction: it says "the vacancy this CV is being tailored to … reframe an
existing bullet". In a session where no CV is editable that is a false statement of the
task, and the `cv_` prefix names a family the preset does not carry.

### The invitation is placed by the server, not searched for by the agent

**Chosen:** `interview_context` carries the most recent `interview_invitation` message
linked to the application, fetched by a new sqlc query and marked as untrusted in the
payload.

**Alternative — register the inbox tools for this preset:** rejected on cost and on
surface. Seven tools are paid for on every turn of every rehearsal to retrieve one fact we
can predict; and `internal/inbox`'s guarantee that no agent tool opens a message by id
exists because that path marks `read_at`, which means "a human saw this". The new query
reads without touching it, so the guarantee stays structural.

`ListEmails` cannot express this filter — it takes status and link but not a vacancy
(`internal/db/gmail.sql.go`) — and filtering its page in Go would break pagination. Hence
one new query rather than a client-side filter.

**Confirmed links only.** The query matches on `job_id`, which `internal/maillink` sets
only from a deterministic match; a model's confident guess lands in `suggested_job_id` and
waits for the candidate to accept it. So an invitation the matcher was not certain about is
invisible to the rehearsal even while the inbox shows it as a suggestion. That is the right
trade here — an unconfirmed link would put another employer's interview into the context,
which is worse than opening with no invitation — but it is a real coverage limit, not an
oversight.

### The round is chosen in conversation, not in the URL

**Chosen:** the agent proposes a round in its opening, informed by the invitation when
there is one, and holds to the candidate's answer for the session.

**Alternative — a round parameter on session creation:** saves one exchange but needs a
column (or an overloaded label) and a menu in the UI, and it cannot use what the invitation
says about the format. The conversational choice is better informed and costs nothing in
schema.

### The bank gate is a prompt rule, deliberately

`experience_add` accepts `said` from the model, and provenance is recorded as
`stated_in_chat` either way — the service cannot distinguish the candidate's words from the
model's paraphrase of them on the way in. This change therefore cannot enforce the rule in
the service path the way `cv_edit`'s `evidence_id` gate is enforced.

What makes it checkable instead is the *explicit agreement*: the offer and the candidate's
"yes" are both in the transcript, so a wrongly banked achievement can be traced to the
exchange that produced it rather than merely suspected. Tightening this into a service-side
gate would mean modelling "the candidate confirmed this atom", which is a larger change to
the bank's write path and is not justified before the failure is observed.

### The turn keeps the ordinary step ceiling

`MaxSteps` stays at the runner's default 8. A rehearsal is a dialogue — one question costs
a round or two — and the autopilot's raised ceiling of 30 exists for an unattended pass
over every requirement, which this is not.

## Risks / Trade-offs

- **The agent banks an improvisation anyway** → the prompt rule above, plus the transcript
  making it traceable. Watch for it before reaching for a service-side gate.
- **A technical round produces confidently wrong critique** → the prompt confines the agent
  to conducting the interview and to grounding claims about the market or the posting in
  what the tools returned. This is a real limit of the feature, and the reason the round is
  the candidate's choice rather than the default.
- **A prompt injection inside an employer's invitation** → the invitation is labelled
  untrusted in the payload and in the prompt, as the mail section already does. The
  reachable damage is bounded by the tool surface: the preset can write to the experience
  bank and to tracking, and has no outbound channel.
- **Context size** → `interview_context` is replayed into the model's context on every
  later turn. It carries the vacancy, a one-line verdict, requirements with at most three
  pieces of evidence each, and one truncated message — the same discipline
  `agentTailorContext` documents, under the registry's existing result cap.
- **The rehearsal is only as good as the fit analysis** → with no cached analysis the
  requirement list is absent and the rehearsal falls back to the posting. Acceptable: the
  vacancy and the bank still carry most of the value.

## Migration Plan

One migration widens the `assistant_sessions.preset` CHECK to admit `interview`, following
`0048` and `0049` exactly. It is additive and needs no backfill: no existing row can hold
the new value. Deploy order is the usual one — migrate before the code that writes the new
preset. Rollback is the reverse constraint plus deleting any rehearsal sessions created in
the window, which is why the constraint is worth keeping rather than dropping.

## Open Questions

None blocking. Two deferred by choice: whether a rehearsal should eventually leave a report
on the application, and whether the round should become a first-class field once we see
which rounds candidates actually pick.
