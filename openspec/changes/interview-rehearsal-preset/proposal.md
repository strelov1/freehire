## Why

An application that reaches the interview stage is the point where the product currently
stops helping. We took the candidate from discovery through the fit analysis, the tailored
CV and the application mail, and then hand them a calendar invite and nothing else — while
the interview is the step that actually decides the outcome.

The one thing we can do here that a general-purpose chatbot cannot is ask the candidate
about *their own* recorded experience against *this* vacancy's requirements: the fit
analysis already names what the posting asks for, and the experience bank already holds
what they have evidenced. A rehearsal built on those two is a question set nobody else can
generate, and every answer the candidate gives is new evidence the bank keeps.

## What Changes

- A new assistant preset, `interview`, running a mock interview against one application.
  It is a fifth preset alongside `chat`, `tailor`, `profile` and `browse`, and like them
  it changes only the system prompt and the registered tool set.
- A new entry point from the tracking board: an application in the `screening` or
  `interview` stage offers a rehearsal, which creates the session bound to that vacancy
  and opens the conversation with a server-side brief (the mechanism `tailor` autopilot
  already uses), so the agent speaks first and the candidate is not asked to start.
- A new agent tool, `interview_context`, carrying everything the rehearsal reasons from:
  the vacancy, the application's stage, the cached fit analysis' requirements with the
  bank's evidence attached, and the employer's own interview invitation where the mailbox
  holds one.
- The candidate picks the round (screening, behavioural, technical, system design, offer
  negotiation) in the first exchange, and the session holds to it.
- Answers reach the experience bank only when the candidate explicitly agrees to record
  them, in their own words. A rehearsal is where people improvise; an improvisation
  written into the bank is a claim they never made.
- **No** CV tools, **no** mail tools and **no** page tool are offered to the preset.

## Capabilities

### New Capabilities
- `interview-rehearsal`: the mock-interview session — its entry point from an application,
  the context it reasons from, the shape of the rehearsal, and the rule that governs what
  reaches the experience bank.

### Modified Capabilities
- `assistant-agent-runtime`: the preset requirement enumerates which presets carry which
  tools; it gains the interview preset, its bound vacancy, and the exclusions above.

## Impact

- **Schema**: one migration widening the `assistant_sessions.preset` CHECK constraint to a
  fifth value. `assistant_sessions` already carries `job_id`, so no new column.
- **SQL**: one new query returning the latest interview-invitation message for a given
  user and vacancy. It reads only — `read_at` stays untouched, because that column means a
  human saw the message.
- **Go**: `internal/assistant` (preset constant, normaliser, prompt),
  `internal/handler` (the new tool, the registry branch, the creation endpoint).
- **Web**: the rehearsal entry on the tracking board; the chat surface itself is reused
  unchanged.
- **Tests**: `internal/assistant/preset_test.go` uses the literal `"interview"` as its
  example of an *unrecognised* preset and will fail once the preset exists.
- **Not in scope**: voice input, a saved rehearsal report, a readiness score, and metering
  the turn against AI credits (the last is an existing gap across every preset).
