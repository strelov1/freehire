## Context

`internal/api/atsapply`'s `ResolveWithDrafting` (draft.go) runs the deterministic `Resolve`
pass, then offers every still-unmapped, `draftable` field to a `Drafter` — today always
`LLMDrafter`, which grounds a short 1-3 sentence answer in the candidate's experience-bank
atoms (`GroundingContext.Atoms`) and knows nothing about any cover letter that may already
exist for the job.

`internal/candidate/coverletter` already stores one drafted letter per `(userID, jobID)` pair
(`Repository.Get(ctx, userID, jobID) (Stored, error)`, backed by `db.GetCoverLetter`) — the
three-stage Select/Draft/Audit chain's output, gated so only atoms the candidate actually
asserted can be cited (see `cover-letter-draft` spec). The resolve call site
(`Client.resolve` in client.go) already has both `claimed.UserID` and `claimed.JobID` in
scope when it builds `GroundingContext` and calls `ResolveWithDrafting` — no new plumbing is
needed to know which job this attempt is for.

The field id vocabulary `internal/ingest/applyform/display.go` and `isResumeField`'s own
comment establish: `cover_letter` (file) vs `cover_letter_text` (free text) are the two
known cover-letter field shapes; `resolveOne`'s file branch already never resolves the file
variant, so this change is scoped to the free-text one.

## Goals / Non-Goals

**Goals:**
- Recognize `cover_letter_text`-shaped free-text fields distinctly from ordinary drafted questions.
- Reuse an existing `coverletter.Stored.Letter.Body` for that (user, job) pair verbatim when one exists.
- Leave every other `draftable` field's behavior (including the file-kind `cover_letter` field, and any cover-letter field when no letter is drafted yet) exactly as it is today.

**Non-Goals:**
- Drafting a cover letter on demand during auto-apply when none exists yet — that stays the candidate's own explicit action via `internal/candidate/coverletter`, unchanged. `LLMDrafter` remains the fallback for that case, not a trigger to run the three-stage chain.
- Changing `cover-letter-draft`'s own behavior, storage shape, or gating.
- Handling the file-upload `cover_letter` field variant — out of scope, unchanged (`resolveOne` still never resolves file fields other than the résumé).
- A configurable "prefer letter vs draft" toggle — reuse-when-present is the only behavior; there is no case where a fresher generic draft is preferable to the candidate's own tailored letter.

## Decisions

**Recognize the field by known id, the same way `isResumeField` recognizes the résumé field.**
Alternative considered: matching only by label keywords (like `matchLabelAnswerKey`). Rejected
as primary signal — `cover_letter_text` is a stable, observed platform id (Greenhouse), so an
id check is more precise and cheaper than a label heuristic; a label fallback can be added
later the same way `linkedin`/`visa_sponsorship_needed` grew label rules, without changing
this design's shape when a differently-shaped platform is measured live.

**Look up the existing letter inside `ResolveWithDrafting`'s per-field loop, only for a field already identified as cover-letter-semantic and only after the deterministic `Resolve` pass left it unmapped** — not as a new early pass, and not by threading a "do we already have one" check earlier in `Client.resolve`. This keeps the change local to draft.go's existing control flow: a cover-letter field that Resolve already handled some other way, or that isn't required, never triggers the lookup at all.

**Add a `LetterReader` port (mirroring `AtomReader`'s shape), satisfied directly by `*coverletter.Store` — not `coverletter.Repository`.** `AtomReader` sets the precedent to follow exactly: it is satisfied directly by `*experience.Store` (`cmd/auto-apply/main.go` passes `atoms` straight in, no adapter type), not by the lower-level `experience.Repository`. `coverletter.Store.Get(ctx, userID, jobID) (*Stored, error)` already exists and already does the "not-found becomes nil, not an error" translation this package needs — reaching one level lower, into `Repository`, would mean re-implementing that translation a second time against the same `pgx.ErrNoRows` check `store.go` already owns. `LetterReader` is therefore `{ Get(ctx, userID, jobID int64) (*coverletter.Stored, error) }`, and no adapter type is needed in `atsapply` at all — `*coverletter.Store` satisfies it as-is, the same structural fit `AtomReader`/`*experience.Store` already has. (This also means task 2.2, "implement the real adapter," resolves to "none needed" — the existing `*coverletter.Store` is the adapter.)

**Use the letter body verbatim, with no truncation.** Checked against the actual data model before writing this: neither `MergedField`, `applyform.Field`, nor `DOMField` carries any field-length constraint — this package resolves a free-text answer verbatim whenever a field has no enumerated options (`matchOption`'s own rule), and a cover-letter field is no exception. Inventing a bound with nothing in the schema to bound against would be speculative complexity this codebase's own "don't validate for scenarios that can't happen" convention rules out.

**"No letter exists" is read as `LetterReader.Get` returning a nil `*Stored`** — already `coverletter.Store.Get`'s own contract, not something this package re-derives. Any read error (a real DB failure) degrades the same way `buildGroundingContext`'s own read failure does today: fall back to `LLMDrafter` for that field rather than fail the whole attempt, logged. This mirrors the existing "a failure to read the grounding source degrades to drafting nothing" discipline in client.go rather than introducing a new failure-handling shape.

## Risks / Trade-offs

- **[Risk]** A stale letter (drafted before the candidate's profile or the vacancy changed) could be submitted verbatim. → **Mitigation**: this is the existing behavior of `internal/candidate/coverletter` generally — a letter's staleness is that capability's own concern (candidates re-request drafts), not something auto-apply should second-guess; auto-apply submitting the candidate's current stored letter is consistent with "submit what the candidate has", the same principle that already governs the tailored-CV attachment.
- **[Risk]** A platform's cover-letter field id differs from Greenhouse's (`cover_letter_text`), so the new path silently never fires on that platform. → **Mitigation**: matches the existing, accepted pattern for this whole package (`answerKeyFor`/`isResumeField` are Greenhouse-shaped today, widened only after a live measurement per platform, per resolve.go's own comments) — not a regression this change introduces, and not a reason to block landing the Greenhouse case now.
- **[Risk]** A platform DOES enforce a client-side or server-side max length this package's schema does not capture, and a full letter is rejected on submit. → **Mitigation**: no different from the risk any other unbounded free-text answer (a drafted `LLMDrafter` answer, or any deterministically-resolved value) already carries today — this package has never modeled field-length limits, so this change introduces no new exposure; a future finding here is a schema gap to close (add the field to `applyform.Field`/`MergedField`), not something to work around per-caller.
