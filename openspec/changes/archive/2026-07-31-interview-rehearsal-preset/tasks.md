## 1. Schema and SQL

- [x] 1.1 Add the migration widening the `assistant_sessions.preset` CHECK to admit `interview`, following `0048`/`0049` in shape and in the comment that explains why a preset is a schema change
- [x] 1.2 Add the sqlc query returning the most recent `interview_invitation` message for a (user, vacancy) pair — reading only, leaving `read_at` untouched — and run `make sqlc`
- [x] 1.3 Extend `internal/inbox`'s `Queries` interface and its test fake with the new query, keeping the fake's behaviour honest about "no invitation"

## 2. The preset itself

- [x] 2.1 Add the `PresetInterview` constant and admit it in `NormalizePreset`; rewrite `preset_test.go:19`, which uses the literal `"interview"` as its example of an unrecognised preset, and add the preset to the known-preset table
- [x] 2.2 Write `interviewPrompt` and return it from `SystemPrompt`: one round per session, one question at a time, the three critique axes, the explicit-agreement rule before banking anything, and the invitation as untrusted text
- [x] 2.3 Add the preset to the list in `TestOnlyTheChatPresetIsToldAboutMail`, and pin the banking rule and the untrusted-invitation wording the way `TestMailPromptNamesBodiesAsUntrusted` pins the mail one

## 3. The rehearsal context tool

- [x] 3.1 Write `interview_context` in a new `internal/handler/assistant_interview_tools.go`, closing over the session's vacancy: the posting, the application's stage, the verdict and score in one line, and the requirements carrying bank evidence via the existing `withBankEvidence`
- [x] 3.2 Attach the employer's invitation when the mailbox holds one, truncated and flagged as untrusted in the payload itself
- [x] 3.3 Cover the degradations: no cached analysis returns posting and stage without requirements; an unreadable bank returns empty evidence lists rather than an error; no invitation omits the field

## 4. Wiring the session

- [x] 4.1 Register the tool set for the preset in `registry`: discovery, tracking, experience and `interview_context`; assert in a test that no CV tool, no mail tool and no page tool is offered
- [x] 4.2 Add the creation endpoint that resolves the application through the caller's own tracking record, creates the session with the vacancy and no CV, and answers a vacancy the caller has no application against as not found
- [x] 4.3 Run the opening turn from a server-side brief the way the autopilot does, so the agent speaks first; keep the runner's default step ceiling

## 5. The way in

- [x] 5.1 Offer the rehearsal on a tracking-board application in the `screening` or `interview` stage, opening the created session in the existing chat surface
- [ ] 5.2 Verify the entry visually at the widths the board is used at, and confirm the opening turn streams into the chat without the candidate typing anything

## 6. Documentation

- [x] 6.1 Record the preset in `internal/assistant/AGENTS.md` — its binding, its tool set, why the invitation is placed by the server rather than searched for, and that the banking gate is a prompt rule rather than a service one
- [ ] 6.2 Offer a changelog entry on the `/blog` feed once the feature lands

## 7. Known gaps from review (not blocking the first cut)

- [ ] 7.1 The opening brief renders as the candidate's own message. The session label no longer comes from it (a rehearsal is named after its vacancy at creation), but the first bubble still reads as if they typed the server's brief — the spec's "the candidate sees the agent's opening as the first message" is only half met. Suppressing it needs the turn's origin to survive into the transcript, which is a change to the event/message shape.
- [ ] 7.2 The web auto-open fires only from `boot()`. Reaching an unopened rehearsal by clicking it in the session rail (`followAddress` → `openSession`) does not start it. Only reachable for a rehearsal whose opening failed upstream, which is now retryable server-side.
- [ ] 7.3 `BoardCard.svelte` claims the follow-up and rehearse actions never appear together; a `screening` application silent past 18 days shows both, and the row was never laid out for two.
- [ ] 7.4 `rehearsalContext` dereferences `h.stages`, `h.cv` and `h.invitation` without nil guards. Unreachable in production wiring, but `Registry.Call` has no recover, so a partially-wired test harness panics inside the SSE writer.
- [ ] 7.5 `interviewInvitation.From` uses `FromName`, which ATS senders often leave empty; fall back to `FromAddr`.
