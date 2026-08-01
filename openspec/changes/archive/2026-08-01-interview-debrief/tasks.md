## 1. The preset vocabulary

- [x] 1.1 Verify against production which migration number is free (0068 is taken on disk and the ledger has held numbers from unmerged branches), then add the migration widening the `assistant_sessions.preset` CHECK to admit `debrief`
- [x] 1.2 Add `PresetDebrief` and admit it in `NormalizePreset` and `SystemPrompt`, with the preset test covering the new value
- [x] 1.3 Admit `debrief` in `creatablePreset`, including the message naming the presets a client may mint

## 2. The prompt

- [x] 2.1 Write `debriefPrompt`: the agent asks what was asked, maps each recalled question onto the context's requirements, and critiques against self-vs-team, concrete outcome, and unsaid figure
- [x] 2.2 State the banking rule in its debrief form — record only what the candidate confirms, in their own words, never a figure the agent supplied — and cover it with a prompt test
- [x] 2.3 State the untrusted-invitation rule and cover it with a prompt test
- [x] 2.4 Write the server-supplied opening brief, and assert it is not client-supplied

## 3. Handler wiring

- [x] 3.1 Generalise `CreateAssistantSession`'s two `preset == PresetInterview` branches into one notion of an application-bound preset, keeping rehearsal behaviour unchanged
- [x] 3.2 Register the interview tool set for `debrief`, with a test asserting both presets carry the same tools and that neither carries an inbox tool
- [x] 3.3 Widen `PostAssistantOpening` to the debrief, preserving the gate that refuses only an ANSWERED opening
- [x] 3.4 Cover creation: bound to the vacancy with no CV binding, labelled after the vacancy, 404 without an application, created regardless of stage, and repeatable for a second round
- [x] 3.5 Confirm the session rail lists debriefs

## 4. The web surface

- [x] 4.1 Add a `createDebrief` call beside `createRehearsal` in the assistant API client (`ChatPreset` turned out to be the URL-mintable set, not the session presets — the rehearsal is already created by its own function)
- [x] 4.2 Add the Debrief action to the application card, shown from the `interview` stage onward and absent when the catalogue no longer holds the posting, reusing the rehearsal's in-flight and error handling
- [x] 4.3 Verify the card visually at mobile and desktop widths — two actions in the strip beside the badges

## 5. Documentation

- [x] 5.1 Update `internal/assistant/AGENTS.md` with the fifth preset and why the banking rule inverts between rehearsal and debrief
- [x] 5.2 Run the full test suite in both modes (`go test ./...` and `go test -tags=integration ./internal/db/`), plus `go vet ./...` and the web checks
- [x] 5.3 Offer a changelog entry on `/blog` (offered; the announcement is deliberately held back)
