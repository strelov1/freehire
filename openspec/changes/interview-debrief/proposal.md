## Why

The most truthful evidence a candidate ever produces passes through the product and is
lost: what they actually said to an employer, out loud, under pressure. A CV is an
edited version of someone; a rehearsal is an improvisation. A real interview is neither,
and an hour after it the memory is still sharp — but nothing asks for it, so the
experience bank never sees it and the next application starts from the same thin evidence
as the last one.

The rehearsal preset already covers the hour before an interview. The hour after it is
empty, and it is the more valuable one.

## What Changes

- A new assistant preset, `debrief`, held against one application the candidate has
  already interviewed for. The agent asks what was asked and what they answered, records
  what they confirm into the experience bank, and tells them where each answer fell short.
- An opened application offers a debrief alongside the rehearsal. The debrief is offered
  from the `interview` stage onward; the server checks only that the caller owns an
  application against the vacancy, so a candidate who never moved the stage is not blocked.
- The debrief reuses the rehearsal's context tool and its tool set unchanged: the vacancy,
  the application's stage, the fit analysis' requirements with the bank's evidence for
  each, and the employer's invitation carrying its untrusted marking.
- The agent speaks first under a server-supplied brief, as the rehearsal does.
- The critique lives in the transcript and nowhere else. No debrief report, no readiness
  score, no per-round record is persisted.
- A candidate may debrief the same application as many times as they have rounds.

Out of scope, deliberately: drafting the follow-up email (it depends on the unfinished
`application-followup-draft`), and generating a preparation list for the next round (a
rehearsal already does that, and is one button away).

## Capabilities

### New Capabilities

- `interview-debrief`: the review of an interview that has already happened — how it is
  offered, what the agent reasons from, what reaches the experience bank and under what
  agreement, and what the session leaves behind.

### Modified Capabilities

- `assistant-sessions`: the session rail's requirement enumerates the conversations it
  spans; a debrief is a conversation the candidate returns to and belongs in it.

## Impact

- `internal/assistant`: `PresetDebrief`, `debriefPrompt`, and the preset switches in
  `NormalizePreset` and `SystemPrompt`.
- `internal/handler`: `creatablePreset` admits `debrief`; the rehearsal's vacancy
  resolution, session labelling, opening endpoint and interview tool set are registered
  for it. `internal/handler/assistant.go` gains a second preset that binds to an
  application, which is the point at which the rehearsal's one-off branches are worth
  generalising.
- `migrations/`: one migration widening the `assistant_sessions.preset` CHECK. The next
  free number is 0069 — 0068 is taken, and the numbering has collided before, so it is
  verified against production before the file is written.
- `web/`: a second action on the application card, and the debrief in the session rail.
- No new table, no new column, no change to `internal/experience`.
