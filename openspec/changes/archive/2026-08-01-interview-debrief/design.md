## Context

`internal/assistant` holds four presets today: `chat`, `tailor`, `profile`, `browse`, and
`interview`. Each is a pair of a system prompt and a tool set, and the vocabulary is
pinned by a CHECK constraint on `assistant_sessions.preset` — adding one is a migration
and not only a constant.

The rehearsal (`interview`) is the only creatable preset that binds to an entity, and
building it produced everything a debrief needs: `rehearsalVacancy` treats the
`user_jobs` row as the authorisation, `LabelSession` names the session after the vacancy
at creation, `PostAssistantOpening` lets the agent speak first under a server-supplied
brief, and `interview_context` assembles the vacancy, the application's stage, the fit
analysis' requirements with the experience bank's evidence for each, and the employer's
invitation with its untrusted marking.

The experience bank's publication rule is the constraint everything here bends around:
`cv_import`, `stated_in_chat` and `manual` are the candidate speaking and may reach a CV;
`agent_inferred` is the model speaking and may not. `experience_add` stamps
`stated_in_chat` whoever composed the `said` string, so the distinction between the
candidate's words and the agent's paraphrase is not enforceable at the write path — it
is a prompt rule, made auditable by the transcript.

## Goals / Non-Goals

**Goals:**

- Capture what the candidate actually said in a real interview into the experience bank,
  with the same provenance discipline as every other route into it.
- Tell the candidate where each answer fell short, in terms they can use in the next round.
- Reach both by reusing the rehearsal's machinery rather than duplicating it.

**Non-Goals:**

- Transcript ingestion (pasted or uploaded). Most candidates have no transcript, and one
  that exists carries the interviewer's words too, which are not the candidate's to bank.
- A follow-up email draft. It belongs to `application-followup-draft`, which is unfinished.
- A preparation list for the next round. A rehearsal already is that, and it is one
  button away from the same application.
- Any persisted artefact: no report, no score, no per-question record.
- Any change to `internal/experience`. The bank needs nothing new; STAR-depth atoms
  remain a layer nobody has asked for yet.

## Decisions

### A separate `debrief` preset rather than a mode inside `interview`

The two sessions share their authorisation, their context, their tool set and their
streaming path. What differs is the system prompt — and one rule inside it that is
mutually opposed between the two.

`interviewPrompt` must guard against banking: *"Never record something they hedged,
invented on the spot, or offered as an example of what they might say. A rehearsal is
where people try things out."* Improvisation is the point of a rehearsal, and half the
prompt polices the session against itself. In a debrief the candidate is recalling, not
inventing, and banking is the purpose. The rule inverts.

Alternatives considered:

- **A mode flag inside `interview`.** Saves a migration and three switch branches —
  roughly an hour of work. Costs a single prompt constant containing "if rehearsing do X,
  if debriefing do Y", which is exactly where long branching prompts leak: the model
  starts critiquing mid-rehearsal and interrogating mid-debrief. `prompt.go` deliberately
  holds one prompt per preset, each with a comment arguing why it is its own.
- **Extending `profile`.** The experience interviewer already asks, listens and banks with
  the candidate's words. But it reasons from where the bank is thin, and a debrief reasons
  from where the interview went badly. Those are different lists, and the critique — half
  the value — has nothing to hang on.

This will be the first case of two presets sharing a tool set. That is acceptable and
worth naming: the tools answer "what can this session touch", the prompt answers "what is
this session for", and the two need not vary together.

### The rehearsal's branches get generalised, not copied

`CreateAssistantSession` currently reads `if preset == assistant.PresetInterview` in two
places — resolving the vacancy and labelling the session. A second application-bound
preset makes those conditions wrong rather than merely repetitive. They become a single
notion of "a preset that binds to an application", which both presets satisfy, so the
next one does not add a third branch. `PostAssistantOpening` and the tool registration
are widened the same way.

This is the change reshaping the part it touches rather than special-casing around it.

### The stage gates the offer, not the endpoint

The button appears on an application in `interview` or later. The server checks only that
the caller owns an application against the vacancy, exactly as the rehearsal does.

A candidate who interviewed and never moved their stage is the candidate most in need of
this, and a 403 in that moment is a bug wearing a rule's clothes. The agent sees the real
stage in its context and can ask about the discrepancy itself.

### The critique lives in the transcript

No table, no column, no migration beyond the CHECK. A debrief is a conversation the
candidate can reopen, and the rehearsal set this precedent deliberately: *"No rehearsal
report, readiness score or per-round progress SHALL be persisted."* A structured record
would only be worth its cost if something read it back — nothing does.

### The opening's idempotency gate reads an ANSWERED opening

Copied from the rehearsal, and the reason is worth restating because it is not obvious:
`Runner.Run` records the brief before it calls the model, so a proxy 502 leaves a session
holding exactly one message. A gate testing "is the transcript empty" would lock that
session out of its own opening forever, with no message the candidate wrote and no way to
retry.

### Migration number

The next file is `0069`. Numbers in `migrations/` have collided before — two `0062_*`
files exist — and the production ledger has held a number whose file was still on an
unmerged branch. The number is verified against production before the file is written.

## Risks / Trade-offs

- **Two presets share one tool set** → a change to `assistantInterviewTools` now moves two
  surfaces at once. Mitigated by a test asserting both presets register the same set, so
  a divergence is a deliberate edit rather than a surprise.
- **The banking rule is a prompt rule, not a service rule** → the model can compose a
  claim the candidate never made and stamp it `stated_in_chat`. Unchanged from the
  rehearsal and the experience interviewer; the transcript is what makes it auditable.
  Tightening it means modelling "the candidate confirmed this atom" in the bank's write
  path, which is a change to `internal/experience` and out of scope here.
- **A debrief invents figures the candidate did not give** → the highest-value output
  (a quantified achievement) is also the easiest to fabricate, and the candidate may not
  catch a plausible number attributed to them. The prompt forbids supplying one and a
  prompt test asserts the rule is present; the deeper fix is the same write-path change.
- **Two buttons on one application card confuse people** → "Rehearse" before, "Debrief"
  after. The stage gate means they rarely both read as available.
- **The employer's invitation reaches a preset with no mail tools** → it arrives through
  the context carrying its untrusted marking, as it does for the rehearsal, and the
  warning travels in the payload rather than only in the prompt because the payload is
  re-read every turn.

## Migration Plan

One migration widening the `assistant_sessions.preset` CHECK to admit `debrief`.
Additive, and deployed before the code that writes the value — the standard order for
this repository. Rollback is the reverse CHECK; any `debrief` rows would have to be
deleted first, which is why the constraint change ships in its own migration and not
alongside anything else.
