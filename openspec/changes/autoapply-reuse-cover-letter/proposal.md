## Why

When an ATS application form has a free-text cover-letter field (e.g. `cover_letter_text`), auto-apply's form resolver (`internal/api/atsapply`) always falls through to its generic `LLMDrafter`, which writes a short (1-3 sentence) answer grounded only in raw experience atoms. It never checks whether the candidate already has a full, provenance-gated cover letter for that exact (candidate, job) pair from `internal/candidate/coverletter` — a three-stage chain (Select/Draft/Audit) that produces a materially better letter. The two paths never talk to each other, so a candidate who already invested in a tailored cover letter gets a visibly worse, generic one auto-submitted in its place.

## What Changes

- The ATS field resolver in `internal/api/atsapply` gains the ability to recognize a free-text field as cover-letter-semantic (by known field id/label patterns such as `cover_letter_text`, distinct from other free-text questions).
- When such a field is found and a `coverletter` draft already exists for the (user, job) pair, the resolver uses that letter (or a bounded excerpt of it, respecting any field length limit) as the answer instead of invoking `LLMDrafter`.
- When no existing cover letter is found for the pair, behavior is unchanged: `LLMDrafter` drafts the short grounded answer as it does today.
- No change to when or whether a cover letter is drafted — this only changes what auto-apply does when one already exists at resolution time.

## Capabilities

### New Capabilities
- `atsapply-cover-letter-reuse`: when resolving a free-text cover-letter field on an ATS application form, auto-apply prefers the candidate's existing drafted cover letter for that (candidate, job) pair over the generic grounded drafter, falling back to the generic drafter when no letter exists yet.

### Modified Capabilities
(none — `cover-letter-draft` itself is unchanged; this only changes what auto-apply's field resolver does with an already-drafted letter)

## Impact

- `internal/api/atsapply/resolve.go` (`ResolveWithDrafting`, `answerKeyFor`/`labelAnswerKeyFor`): needs to recognize cover-letter-semantic fields and check for an existing draft before falling to `LLMDrafter`.
- `internal/api/atsapply/llm_drafter.go`: unchanged — remains the fallback path.
- `internal/candidate/coverletter`: read-only dependency — auto-apply reads an existing draft for a (user, job) pair; no changes to drafting logic itself.
- No new layering block: `internal/api/atsapply` already sits above `internal/candidate/coverletter` in the layer graph (api → candidate), so this is a same-direction import addition, not a new edge.
- No migration, no new table — reuses the existing stored cover letter record.
