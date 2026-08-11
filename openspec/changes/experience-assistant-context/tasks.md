## 1. The read tool

- [x] 1.1 Factor the per-atom entry `searchResult` builds inline into a named function
  taking an `experience.Match`, and have `searchResult` use it — no behaviour change, so
  the existing search tests must pass untouched
- [x] 1.2 Add the read-by-id tool to `assistantExperienceTools`, resolving ids by selecting
  from the owner's atoms and attaching employments the way `experienceSummary` does, rather
  than looping `GetAtom`
- [x] 1.3 Return the resolved achievements through the shared entry from 1.1, alongside the
  requested ids that resolved to nothing
- [x] 1.4 Cap how many one call reads, and report the ids left unread so an oversized
  request completes in a second call instead of losing achievements silently
- [x] 1.5 Write the tool description so the model reaches for it on an id and for
  `experience_search` on a requirement — the failure this change fixes began with a model
  putting a UUID into a search box

## 2. Tests for the read

- [x] 2.1 Unit test: several ids resolve to their full content, in one call
- [x] 2.2 Unit test: an id that names nothing comes back reported as unresolved while the
  others still resolve, and the call does not error
- [x] 2.3 Unit test: another account's atom id is reported exactly as a non-existent one is,
  disclosing nothing
- [x] 2.4 Unit test: over-cap requests read up to the cap and name the remainder
- [x] 2.5 Unit test: a read atom and a searched atom carry the same fields — the guard on
  the two shapes drifting apart

## 3. Reading before acting

- [x] 3.1 Add the instruction to the `profile` preset's prompt: read the achievements named
  to you before proposing a merge or writing a refinement, and state what they say
- [x] 3.2 Extend the prompt test the way the rehearsal preset's does — the tools a preset's
  prompt names must be the tools that preset registers
- [x] 3.3 Update `profileKickoff` so an opening message naming ids also says to read them
  first; keep the single builder both entries share

## 4. The panel leaves the content column

- [x] 4.1 Add the module store holding the dock's open state and width, following
  `auth-dialog.svelte.ts`
- [x] 4.2 Make the panel `fixed` at the viewport's left edge below the header, at the offset
  the selection action bar already uses, and remove the flex row that wrapped it and the
  bank in `ExperienceBankView`
- [x] 4.3 Offset the account shell by the dock width while open, in `my/+layout.svelte`, and
  collapse the nav to its icon rail as an override that never writes the stored preference
- [x] 4.4 Restore the candidate's own nav state when the panel closes
- [x] 4.5 Move the dock/overlay threshold from `xl` to the width the design's arithmetic
  gives, and record the number's derivation where it is set
- [x] 4.6 Confirm the overlay form below the threshold is unchanged: it covers the bank,
  locks scroll, and its close control returns

## 5. Verification

- [x] 5.1 `go build ./... && go vet ./...`, `go test ./...`, and `go vet -tags=integration
  ./...` before pushing
- [x] 5.2 `pnpm check` and the web unit tests
- [ ] 5.3 By hand, the defect that started this: select the four achievements, open the
  panel, and confirm the agent's first reply describes those four rather than searching for
  them and answering about others
- [ ] 5.4 By hand: ask for a merge and confirm the agent states what both achievements say
  before proposing it
- [ ] 5.5 By hand: at a wide viewport, confirm the bank is no narrower with the panel open
  than closed, and that the nav returns to the candidate's own setting on close
- [ ] 5.6 Confirm `/my/assistant?preset=profile&atoms=<ids>` opens the same conversation
  with the same opening message, and that the agent reads the ids there too
- [x] 5.7 Update `internal/assistant/AGENTS.md` and `web/AGENTS.md` — the read tool, and the
  panel's placement outside the account content column
- [x] 5.8 Offer a `/blog` changelog entry: the interviewer can now see the achievements it
  is asked about
